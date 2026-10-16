package insights

import (
	"sort"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Yearly is a cost that comes around once a year — the car service every
// October, the tax payment in spring. Paid from everyday money it tips the
// month it lands in; known in advance it can be set aside for.
type Yearly struct {
	Merchant string       `json:"merchant"`
	Category string       `json:"category"`
	Month    int          `json:"month"`   // calendar month it usually comes (1–12)
	Typical  money.Cents  `json:"typical"` // median of the last three years' totals
	Last     money.Cents  `json:"last"`    // the most recent year's total
	Years    []YearlyPaid `json:"years"`
	Next     string       `json:"next"`    // YYYY-MM it is next expected
	Monthly  money.Cents  `json:"monthly"` // Typical / 12: what to set aside
}

type YearlyPaid struct {
	Year   int         `json:"year"`
	Amount money.Cents `json:"amount"`
}

// DetectYearly finds merchants paid in the same month (±1) in at least two
// different years, for €100 or more, and not paid most months (those are
// regular bills or shops). skip leaves out what is planned already (fixed
// obligations).
func DetectYearly(txs []ledger.Tx, now time.Time, skip func(*ledger.Tx) bool) []Yearly {
	since := now.AddDate(-4, 0, 0).Format("2006-01-02")
	type pay struct {
		t      time.Time
		amount money.Cents
		cat    string
		name   string
	}
	by := map[string][]pay{}
	for i := range txs {
		t := &txs[i]
		if t.Kind != "expense" || t.Date < since || t.Merchant == "" || (skip != nil && skip(t)) {
			continue
		}
		d, err := time.Parse("2006-01-02", t.Date)
		if err != nil {
			continue
		}
		k := merchant.Fold(t.Merchant)
		by[k] = append(by[k], pay{d, t.Amount, t.Category, t.Merchant})
	}
	var out []Yearly
	for _, ps := range by {
		sort.Slice(ps, func(i, j int) bool { return ps[i].t.Before(ps[j].t) })
		// Paid in four or more different calendar months over two years:
		// a regular (a shop, a therapist), not a once-a-year cost.
		months := map[time.Month]bool{}
		for _, p := range ps {
			if p.t.After(now.AddDate(-2, 0, 0)) {
				months[p.t.Month()] = true
			}
		}
		if len(months) >= 4 {
			continue
		}
		// Cluster by the month of the latest payment, ±1 month, per year.
		anchor := ps[len(ps)-1].t
		perYear := map[int]money.Cents{}
		for _, p := range ps {
			diff := (int(p.t.Month()) - int(anchor.Month()) + 12) % 12
			if diff <= 1 || diff >= 11 {
				y := p.t.Year()
				if anchor.Month() == time.January && p.t.Month() == time.December {
					y++ // a December payment belongs with the next January's
				}
				perYear[y] += p.amount
			}
		}
		// At least two of the last three years: a habit, not two purchases
		// that happen to share a month years apart.
		recentYears := 0
		for y := range perYear {
			if y >= now.Year()-2 {
				recentYears++
			}
		}
		if recentYears < 2 || anchor.Before(now.AddDate(0, -14, 0)) {
			continue
		}
		var years []YearlyPaid
		for y, a := range perYear {
			years = append(years, YearlyPaid{y, a})
		}
		sort.Slice(years, func(i, j int) bool { return years[i].Year < years[j].Year })
		recent := years
		if len(recent) > 3 {
			recent = recent[len(recent)-3:]
		}
		var vals []money.Cents
		for _, y := range recent {
			vals = append(vals, y.Amount)
		}
		sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
		typical := vals[len(vals)/2]
		if len(vals)%2 == 0 {
			typical = (vals[len(vals)/2-1] + vals[len(vals)/2]) / 2
		}
		if typical < money.FromFloat(100) {
			continue
		}
		next := time.Date(anchor.Year()+1, anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
		for next.Before(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)) {
			next = next.AddDate(1, 0, 0)
		}
		last := ps[len(ps)-1]
		out = append(out, Yearly{Merchant: last.name, Category: last.cat, Month: int(anchor.Month()), Typical: typical,
			Last: years[len(years)-1].Amount, Years: years, Next: next.Format("2006-01"), Monthly: typical / 12})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Typical > out[j].Typical })
	return out
}
