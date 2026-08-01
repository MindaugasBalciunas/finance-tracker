package handler

import (
	"encoding/csv"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
)

// INVL (Artea) 3rd-pillar pension statement importer. The fund is
// unit-based: every row states the operation's EUR amount, the unit price
// that day and the units bought or sold. Column layout:
//
//	0 date, 1 payment in/out, 2 fee, 3 amount, 4 unit price, 5 units,
//	6 operation ("Kliento įmoka" / "Darbdavio įmoka" / "Dalies lėšų išmoka"),
//	7 payer/recipient
//
// Client and employer rows are both real contributions (the 2022-07-04
// withdrawal drains the ledger to exactly 1.0000 unit only when both are
// counted). The account's value on a date is cumulative units × that day's
// unit price, which is what restores the Artea balance history.

// invlPoint is the account's value on a statement date.
type invlPoint struct {
	Date  time.Time
	Value float64
}

// invlNumber parses "1 234.56 €" (currency sign, spaces, NBSP) to a float.
func invlNumber(s string) (float64, error) {
	clean := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return -1
	}, s)
	return strconv.ParseFloat(clean, 64)
}

// parseINVLCSV replays the statement's unit ledger and returns one valuation
// point per statement date (after all of that date's rows, at that date's
// unit price), oldest first.
func parseINVLCSV(r io.Reader) ([]invlPoint, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	type row struct {
		date       time.Time
		units      float64
		price      float64
		withdrawal bool
	}
	var rows []row
	first := true
	for {
		rec, rerr := reader.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
		if first { // header
			first = false
			continue
		}
		if len(rec) < 7 {
			continue
		}
		date, derr := time.Parse("2006-01-02", strings.TrimSpace(rec[0]))
		price, perr := invlNumber(rec[4])
		units, uerr := invlNumber(rec[5])
		if derr != nil || perr != nil || uerr != nil || units <= 0 || price <= 0 {
			continue
		}
		// "Dalies lėšų išmoka" folds to "dalies lesu ismoka"; contributions
		// ("kliento/darbdavio įmoka") fold to "... imoka". foldLT also
		// tolerates a double-encoded export losing its diacritics.
		op := foldLT(strings.TrimSpace(rec[6]))
		rows = append(rows, row{date: date, units: units, price: price, withdrawal: strings.Contains(op, "ismoka")})
	}

	// Statements list newest first — replay oldest first.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })

	var points []invlPoint
	units := 0.0
	for i, r := range rows {
		if r.withdrawal {
			units -= r.units
		} else {
			units += r.units
		}
		// Emit one point per date, after the date's last row.
		if i+1 < len(rows) && rows[i+1].date.Equal(r.date) {
			continue
		}
		points = append(points, invlPoint{Date: r.date, Value: math.Round(units*r.price*100) / 100})
	}
	return points, nil
}

type invlImportResult struct {
	Points   int    `json:"points"`
	Enriched int    `json:"enriched"` // existing snapshots that gained an Artea value
	Created  int    `json:"created"`  // new art-only snapshots (pre-tracked era)
	Skipped  int    `json:"skipped"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
}

// ImportINVLCSV godoc
// @Summary      Import an INVL (Artea) pension statement CSV
// @Description  Replays the fund's unit ledger and restores the Artea balance history: months whose snapshot has no Artea value get it filled in, months without any snapshot (before the fully-tracked era) get an art-only snapshot.
// @Tags         import
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "INVL statement export (CSV)"
// @Success      200  {object}  invlImportResult
// @Failure      400  {object}  ErrorResponse
// @Router       /import/invl [post]
func (h *ImportHandler) ImportINVLCSV(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "missing 'file' field in form"})
		return
	}
	defer file.Close()

	points, err := parseINVLCSV(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to parse statement: " + err.Error()})
		return
	}
	if len(points) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no valuation points in statement"})
		return
	}

	existing, _ := h.balRepo.List(domain.BalanceFilter{})
	// The fully-tracked era starts at the first snapshot holding accounts
	// beyond the statement-restorable ones (swed + art) — past that point a
	// new single-account snapshot would show as a dip in total net worth.
	earliestFull := time.Time{}
	byMonth := make(map[string][]*domain.Balance)
	for i := range existing {
		b := &existing[i]
		byMonth[b.Date.Format("2006-01")] = append(byMonth[b.Date.Format("2006-01")], b)
		if b.Total-b.Swed-b.Art > 0.005 {
			if earliestFull.IsZero() || b.Date.Before(earliestFull) {
				earliestFull = b.Date
			}
		}
	}

	result := invlImportResult{
		Points:   len(points),
		DateFrom: points[0].Date.Format("2006-01-02"),
		DateTo:   points[len(points)-1].Date.Format("2006-01-02"),
	}
	for _, p := range points {
		month := p.Date.Format("2006-01")
		snaps := byMonth[month]

		// A month that already tracks Artea anywhere is left alone.
		tracked := false
		for _, b := range snaps {
			if b.Art > 0.005 {
				tracked = true
				break
			}
		}
		if tracked {
			result.Skipped++
			continue
		}

		if len(snaps) > 0 {
			// Enrich the snapshot closest to the valuation date. Total is
			// bumped by the delta, not recomputed from columns — recomputing
			// would drop any BTC valuation embedded in it.
			closest := snaps[0]
			for _, b := range snaps[1:] {
				if absDuration(b.Date.Sub(p.Date)) < absDuration(closest.Date.Sub(p.Date)) {
					closest = b
				}
			}
			closest.Art = p.Value
			closest.Total = math.Round((closest.Total+p.Value)*100) / 100
			if err := h.balRepo.Update(closest); err == nil {
				result.Enriched++
			} else {
				result.Skipped++
			}
			continue
		}

		if !earliestFull.IsZero() && !p.Date.Before(earliestFull) {
			result.Skipped++
			continue
		}
		nb := &domain.Balance{Date: p.Date, Art: p.Value, Total: p.Value}
		if err := h.balRepo.Create(nb); err == nil {
			byMonth[month] = append(byMonth[month], nb)
			result.Created++
		} else {
			result.Skipped++
		}
	}

	c.JSON(http.StatusOK, result)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
