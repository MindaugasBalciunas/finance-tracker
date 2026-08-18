package handler

import (
	"encoding/csv"
	"fmt"
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

type invlKind int

const (
	invlClient invlKind = iota
	invlEmployer
	invlWithdrawal
)

// invlEntry is one statement row, used to generate the transactions the
// bank statements never saw (payroll-deducted contributions, payouts).
type invlEntry struct {
	Date   time.Time
	Amount float64
	Payer  string
	Kind   invlKind
	// Payroll marks client contributions deducted from gross salary: they
	// are paired with an employer row of the same amount a few days apart
	// and never crossed the personal bank account.
	Payroll bool
}

// invlEmployerLabel maps the statement's payer to the app's employer label.
func invlEmployerLabel(payer string) string {
	p := foldLT(payer)
	switch {
	case strings.Contains(p, "vipps"):
		return "vipps mobilepay"
	case strings.Contains(p, "danske"):
		return "danske"
	}
	return ""
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

// parseINVLCSV replays the statement's unit ledger. It returns one valuation
// point per statement date (after all of that date's rows, at that date's
// unit price) plus the raw entries, both oldest first.
func parseINVLCSV(r io.Reader) ([]invlPoint, []invlEntry, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	type row struct {
		date   time.Time
		amount float64
		units  float64
		price  float64
		payer  string
		kind   invlKind
	}
	var rows []row
	first := true
	for {
		rec, rerr := reader.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, nil, rerr
		}
		if first { // header
			first = false
			continue
		}
		if len(rec) < 8 {
			continue
		}
		date, derr := time.Parse("2006-01-02", strings.TrimSpace(rec[0]))
		amount, aerr := invlNumber(rec[3])
		price, perr := invlNumber(rec[4])
		units, uerr := invlNumber(rec[5])
		if derr != nil || aerr != nil || perr != nil || uerr != nil || units <= 0 || price <= 0 {
			continue
		}
		// "Dalies lėšų išmoka" folds to "dalies lesu ismoka"; contributions
		// ("kliento/darbdavio įmoka") fold to "... imoka". foldLT also
		// tolerates a double-encoded export losing its diacritics.
		op := foldLT(strings.TrimSpace(rec[6]))
		kind := invlClient
		switch {
		case strings.Contains(op, "ismoka"):
			kind = invlWithdrawal
		case strings.Contains(op, "darbdavio"):
			kind = invlEmployer
		}
		rows = append(rows, row{date: date, amount: amount, units: units, price: price, payer: strings.TrimSpace(rec[7]), kind: kind})
	}

	// Statements list newest first — replay oldest first.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })

	var points []invlPoint
	var entries []invlEntry
	units := 0.0
	for i, r := range rows {
		if r.kind == invlWithdrawal {
			units -= r.units
		} else {
			units += r.units
		}
		entries = append(entries, invlEntry{Date: r.date, Amount: r.amount, Payer: r.payer, Kind: r.kind})
		// Emit one point per date, after the date's last row.
		if i+1 < len(rows) && rows[i+1].date.Equal(r.date) {
			continue
		}
		points = append(points, invlPoint{Date: r.date, Value: math.Round(units*r.price*100) / 100})
	}

	// A client contribution matching an employer one within a few days was
	// deducted from gross salary (both parts arrive via payroll) — it never
	// crossed the personal bank account. Match multiset-style so months with
	// two contribution pairs (a late posting) pair up correctly.
	employerFree := make([]bool, len(entries))
	for i := range entries {
		employerFree[i] = entries[i].Kind == invlEmployer
	}
	for i := range entries {
		if entries[i].Kind != invlClient {
			continue
		}
		for j := range entries {
			if !employerFree[j] || math.Abs(entries[j].Amount-entries[i].Amount) > 0.005 ||
				absDuration(entries[j].Date.Sub(entries[i].Date)) > 5*24*time.Hour {
				continue
			}
			entries[i].Payroll = true
			employerFree[j] = false
			break
		}
	}
	return points, entries, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

type invlImportResult struct {
	Points   int    `json:"points"`
	Enriched int    `json:"enriched"` // existing snapshots that gained an Artea value
	Created  int    `json:"created"`  // new art-only snapshots (pre-tracked era)
	Skipped  int    `json:"skipped"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	// Transaction side: payroll contributions arrive as income + Pension
	// pairs, payouts as Transfers; rows the bank already delivered dedup.
	TxCreated int `json:"tx_created"`
	TxSkipped int `json:"tx_skipped"`
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

	points, entries, err := parseINVLCSV(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to parse statement: " + err.Error()})
		return
	}
	if len(points) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no valuation points in statement"})
		return
	}

	// Atomic: any hard failure below rolls the whole statement back.
	var result invlImportResult
	if err := h.txScope(func(s *ImportHandler) error {
		var ierr error
		result, ierr = s.runINVLImport(points, entries)
		return ierr
	}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "import failed and was rolled back: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *ImportHandler) runINVLImport(points []invlPoint, entries []invlEntry) (invlImportResult, error) {
	existing, err := h.balRepo.List(domain.BalanceFilter{})
	if err != nil {
		return invlImportResult{}, fmt.Errorf("reading existing balances: %w", err)
	}
	// The fully-tracked era starts at the first snapshot holding accounts
	// beyond the statement-restorable/backfillable ones (swed, art, cash,
	// seb_pen) — past that point a new single-account snapshot would show
	// as a dip in total net worth.
	earliestFull := time.Time{}
	byMonth := make(map[string][]*domain.Balance)
	for i := range existing {
		b := &existing[i]
		byMonth[b.Date.Format("2006-01")] = append(byMonth[b.Date.Format("2006-01")], b)
		if b.Total-b.Swed-b.Art-b.Cash-b.SebPen > 0.005 {
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

	// The ledger is a step function: units only change on statement dates,
	// so between points the account's value is the last point's value. Fill
	// it into EVERY art-less snapshot inside the statement's range — months
	// with no fund activity (no contribution row) would otherwise stay at 0
	// and cut a false dip into the net-worth trend. Total is bumped by the
	// delta, not recomputed from columns — recomputing would drop any BTC
	// valuation embedded in it.
	stepValue := func(t time.Time) float64 {
		v := 0.0
		for _, p := range points {
			if p.Date.After(t) {
				break
			}
			v = p.Value
		}
		return v
	}
	first, last := points[0].Date, points[len(points)-1].Date
	monthEnriched := make(map[string]bool)
	for i := range existing {
		b := &existing[i]
		if b.Art > 0.005 || b.Date.Before(first) || b.Date.After(last) {
			continue
		}
		v := stepValue(b.Date)
		if v <= 0.005 {
			continue
		}
		b.Art = v
		b.Total = math.Round((b.Total+v)*100) / 100
		if err := h.balRepo.Update(b); err != nil {
			return result, fmt.Errorf("enriching balance snapshot %s: %w", b.Date.Format("2006-01-02"), err)
		}
		monthEnriched[b.Date.Format("2006-01")] = true
		result.Enriched++
	}

	// Months with no snapshot at all get an art-only one — but only before
	// the fully-tracked era, where a single-account snapshot would show as
	// a dip in total net worth.
	for _, p := range points {
		month := p.Date.Format("2006-01")
		if len(byMonth[month]) > 0 {
			if !monthEnriched[month] {
				result.Skipped++
			}
			continue
		}
		if !earliestFull.IsZero() && !p.Date.Before(earliestFull) {
			result.Skipped++
			continue
		}
		nb := &domain.Balance{Date: p.Date, Art: p.Value, Total: p.Value}
		if err := h.balRepo.Create(nb); err != nil {
			return result, fmt.Errorf("creating art-only snapshot %s: %w", p.Date.Format("2006-01-02"), err)
		}
		byMonth[month] = append(byMonth[month], nb)
		result.Created++
	}

	if err := h.createINVLTransactions(entries, &result); err != nil {
		return result, err
	}

	return result, nil
}

// createINVLTransactions turns statement entries into the transactions the
// bank statements never delivered. Payroll contributions (employer rows and
// their paired client rows) become an income + Pension pair — compensation
// that went straight into the fund, so income and invested stay consistent.
// Direct client contributions and payouts usually exist already from the
// bank side and dedup fuzzily (dates differ between bank and fund by days).
func (h *ImportHandler) createINVLTransactions(entries []invlEntry, result *invlImportResult) error {
	existing, err := h.txRepo.ListAll()
	if err != nil {
		return fmt.Errorf("reading existing transactions for dedup: %w", err)
	}
	fp := make(map[string]bool, len(existing))
	type ref struct {
		date   time.Time
		amount float64
	}
	var pensionRows, artOutflows []ref
	for _, t := range existing {
		fp[fmt.Sprintf("%s|%s|%.2f|%s", t.Date.Format("2006-01-02"), t.Type, t.Amount, t.Comment)] = true
		if t.Type == domain.TransactionTypeInvestment && t.Category == domain.CategoryPension {
			pensionRows = append(pensionRows, ref{t.Date, t.Amount})
		}
		if t.Type == domain.TransactionTypeInvestment && t.Category == domain.CategoryTransfers && t.DebitAccount == "art" {
			artOutflows = append(artOutflows, ref{t.Date, t.Amount})
		}
	}
	near := func(rows []ref, date time.Time, amount float64, days int, anyAmount bool) bool {
		for _, r := range rows {
			if (anyAmount || math.Abs(r.amount-amount) < 0.005) &&
				absDuration(r.date.Sub(date)) <= time.Duration(days)*24*time.Hour {
				return true
			}
		}
		return false
	}
	create := func(tx domain.Transaction) error {
		key := fmt.Sprintf("%s|%s|%.2f|%s", tx.Date.Format("2006-01-02"), tx.Type, tx.Amount, tx.Comment)
		if fp[key] {
			result.TxSkipped++
			return nil
		}
		if err := h.txRepo.Create(&tx); err != nil {
			return fmt.Errorf("creating pension transaction (%s, %.2f): %w", tx.Date.Format("2006-01-02"), tx.Amount, err)
		}
		fp[key] = true
		result.TxCreated++
		return nil
	}

	for _, e := range entries {
		switch {
		case e.Kind == invlWithdrawal:
			// The bank-side arrival (net of tax) may already be recorded as
			// an art→bank transfer — don't double the outflow.
			if near(artOutflows, e.Date, 0, 14, true) {
				result.TxSkipped++
				continue
			}
			if err := create(domain.Transaction{
				Date: e.Date, Type: domain.TransactionTypeInvestment, Amount: e.Amount,
				Category: domain.CategoryTransfers, Labels: "artea",
				Comment:      "Artea (INVL) partial withdrawal (gross, before tax)",
				DebitAccount: "art", CreditAccount: "swed",
			}); err != nil {
				return err
			}
		case e.Kind == invlEmployer || e.Payroll:
			who, side := "employer", " ("+strings.TrimSpace(e.Payer)+")"
			if e.Kind == invlClient {
				who, side = "own share", ", deducted from gross salary"
			}
			labels := domain.NormalizeLabels("artea,payroll," + invlEmployerLabel(e.Payer))
			if err := create(domain.Transaction{
				Date: e.Date, Type: domain.TransactionTypeIncome, Amount: e.Amount,
				Category: domain.CategorySalary, Labels: labels,
				Comment: "Artea (INVL) pension contribution via payroll — " + who + side,
			}); err != nil {
				return err
			}
			if err := create(domain.Transaction{
				Date: e.Date, Type: domain.TransactionTypeInvestment, Amount: e.Amount,
				Category: domain.CategoryPension, Labels: labels,
				Comment:       "Artea (INVL) 3rd pillar pension (payroll — " + who + ")",
				CreditAccount: "art",
			}); err != nil {
				return err
			}
		default:
			// Direct client contribution — paid from the bank, so it is
			// normally already here from the statement import or by hand.
			if near(pensionRows, e.Date, e.Amount, 7, false) {
				result.TxSkipped++
				continue
			}
			if err := create(domain.Transaction{
				Date: e.Date, Type: domain.TransactionTypeInvestment, Amount: e.Amount,
				Category: domain.CategoryPension, Labels: "artea",
				Comment:      "Artea (INVL) 3rd pillar pension",
				DebitAccount: "swed", CreditAccount: "art",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
