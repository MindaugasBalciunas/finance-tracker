package insights

import (
	"sort"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// MerchantStat is one merchant over a window: what was spent there, how
// often, and how that compares with the equal window before.
type MerchantStat struct {
	Name     string      `json:"name"`
	Category string      `json:"category"` // the one used most often with it
	Total    money.Cents `json:"total"`    // net of refunds from it
	Share    float64     `json:"share"`
	Count    int         `json:"count"`   // visits: a split purchase is one
	Average  money.Cents `json:"average"` // total ÷ visits
	Previous money.Cents `json:"previous"`
	Change   float64     `json:"change"` // vs previous, ratio (0 when there was none)
	First    string      `json:"first"`
	Last     string      `json:"last"`
	New      bool        `json:"new"` // first ever paid inside the window
	// Spend per month for the 12 months ending with the window's last month.
	Months []MonthAmount `json:"months"`
}

type MonthAmount struct {
	Month  string      `json:"month"`
	Amount money.Cents `json:"amount"`
}

// LapsedMerchant is a regular that hasn't been paid in the window.
type LapsedMerchant struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	Last     string      `json:"last"`
	Monthly  money.Cents `json:"monthly"` // average month in the half-year before the window
}

type MerchantReport struct {
	From     string           `json:"from"`
	To       string           `json:"to"`
	PrevFrom string           `json:"prev_from"`
	PrevTo   string           `json:"prev_to"`
	Total    money.Cents      `json:"total"`   // spent at named merchants
	Unnamed  money.Cents      `json:"unnamed"` // spending with no merchant on it
	Count    int              `json:"count"`   // merchants paid in the window
	NewCount int              `json:"new_count"`
	NewTotal money.Cents      `json:"new_total"`
	Items    []MerchantStat   `json:"merchants"` // by total, at most n
	Lapsed   []LapsedMerchant `json:"lapsed"`
}

// Merchants ranks who was paid in [from,to] — expenses net of refunds from
// the same merchant — with visits, average ticket, the trend against the
// equal window before, first/last seen, and the regulars that stopped.
// Spellings are merged the way the Ledger's merchant filter matches them
// (merchant.Key), so "Circle K" and "CIRCLE K" are one row and its link
// finds every transaction behind it. Rows after `to` are ignored: a past
// window reads as it stood then.
func Merchants(txs []ledger.Tx, from, to string, n int) MerchantReport {
	pf, pt := prevWindow(from, to)
	rep := MerchantReport{From: from, To: to, PrevFrom: pf, PrevTo: pt, Items: []MerchantStat{}, Lapsed: []LapsedMerchant{}}
	toMonth := to[:7]
	tm, _ := time.Parse("2006-01", toMonth)
	months := make([]string, 12)
	for i := range months {
		months[i] = tm.AddDate(0, i-11, 0).Format("2006-01")
	}
	// The half-year before the window decides who was a regular.
	fm, _ := time.Parse("2006-01", from[:7])
	regFrom, regTo := fm.AddDate(0, -6, 0).Format("2006-01"), fm.AddDate(0, -1, 0).Format("2006-01")
	tt, _ := time.Parse("2006-01-02", to)
	lapseBy := tt.AddDate(0, 0, -60).Format("2006-01-02")

	type agg struct {
		names       map[string]int
		cats        map[string]int
		catLast     map[string]string
		total, prev money.Cents
		count       int            // rows of its own in the window
		ownIDs      map[int64]bool // …by id, so their split parts aren't counted again
		parts       map[int64]bool // parents of split parts paid here in the window
		first, last string
		byMonth     map[string]money.Cents
		regMonths   map[string]bool
		regSpend    money.Cents
	}
	by := map[string]*agg{}
	// "New" needs history before the window: over all time (from 2000) no
	// merchant is new, or every one would be.
	earliest := ""
	for i := range txs {
		x := &txs[i]
		if earliest == "" || x.Date < earliest {
			earliest = x.Date
		}
		if x.Date > to {
			continue
		}
		if x.Merchant == "" {
			if x.Kind == "expense" && x.Date >= from {
				rep.Unnamed += x.Amount
			}
			continue
		}
		refund := isRefund(x)
		if x.Kind != "expense" && !refund {
			continue
		}
		k := merchant.Key(x.Merchant)
		a := by[k]
		if a == nil {
			a = &agg{names: map[string]int{}, cats: map[string]int{}, catLast: map[string]string{}, byMonth: map[string]money.Cents{}, regMonths: map[string]bool{},
				ownIDs: map[int64]bool{}, parts: map[int64]bool{}}
			by[k] = a
		}
		amt := x.Amount
		if refund {
			amt = -amt
		} else {
			a.names[x.Merchant]++
			a.cats[x.Category]++
			if x.Date > a.catLast[x.Category] {
				a.catLast[x.Category] = x.Date
			}
			if a.first == "" || x.Date < a.first {
				a.first = x.Date
			}
			if x.Date > a.last {
				a.last = x.Date
			}
		}
		m := x.Date[:7]
		a.byMonth[m] += amt
		switch {
		case x.Date >= from:
			a.total += amt
			switch {
			case refund:
			case x.SplitOf == 0:
				a.count++
				if x.ID != 0 {
					a.ownIDs[x.ID] = true
				}
			default:
				a.parts[x.SplitOf] = true
			}
		case x.Date >= pf && x.Date <= pt:
			a.prev += amt
		}
		if !refund && m >= regFrom && m <= regTo {
			a.regMonths[m] = true
			a.regSpend += amt
		}
	}

	top := func(m map[string]int, tie map[string]string) string {
		best, n := "", 0
		for k, c := range m {
			if c > n || c == n && (tie[k] > tie[best] || tie[k] == tie[best] && k < best) {
				best, n = k, c
			}
		}
		return best
	}
	for _, a := range by {
		// A split purchase is one visit; a part moved to another merchant
		// (the pharmacy share of a Maxima receipt) is a visit there.
		for p := range a.parts {
			if !a.ownIDs[p] {
				a.count++
			}
		}
		name := top(a.names, nil)
		cat := top(a.cats, a.catLast)
		if a.count == 0 {
			// A regular in the half-year before, quiet for two months by
			// the window's end: worth a look (moved shop? cancelled?).
			if a.total == 0 && len(a.regMonths) >= 3 && a.last != "" && a.last < lapseBy {
				rep.Lapsed = append(rep.Lapsed, LapsedMerchant{Name: name, Category: cat, Last: a.last, Monthly: a.regSpend / 6})
			}
			continue
		}
		if a.total <= 0 { // bought and returned
			continue
		}
		s := MerchantStat{Name: name, Category: cat, Total: a.total, Count: a.count, Average: a.total / money.Cents(a.count),
			Previous: a.prev, First: a.first, Last: a.last, New: a.first >= from && earliest < from, Months: make([]MonthAmount, len(months))}
		if a.prev > 0 {
			s.Change = round3(a.total.Float()/a.prev.Float() - 1)
		}
		for i, m := range months {
			s.Months[i] = MonthAmount{m, a.byMonth[m]}
		}
		rep.Total += s.Total
		rep.Count++
		if s.New {
			rep.NewCount++
			rep.NewTotal += s.Total
		}
		rep.Items = append(rep.Items, s)
	}
	sort.Slice(rep.Items, func(i, j int) bool {
		if rep.Items[i].Total != rep.Items[j].Total {
			return rep.Items[i].Total > rep.Items[j].Total
		}
		return rep.Items[i].Name < rep.Items[j].Name
	})
	for i := range rep.Items {
		rep.Items[i].Share = round3(rep.Items[i].Total.Float() / rep.Total.Float())
	}
	if len(rep.Items) > n {
		rep.Items = rep.Items[:n]
	}
	sort.Slice(rep.Lapsed, func(i, j int) bool { return rep.Lapsed[i].Monthly > rep.Lapsed[j].Monthly })
	return rep
}

// prevWindow is the equal-length window just before [from,to].
func prevWindow(from, to string) (string, string) {
	f, _ := time.Parse("2006-01-02", from)
	t, _ := time.Parse("2006-01-02", to)
	span := t.Sub(f) + 24*time.Hour
	return f.Add(-span).Format("2006-01-02"), f.Add(-24 * time.Hour).Format("2006-01-02")
}
