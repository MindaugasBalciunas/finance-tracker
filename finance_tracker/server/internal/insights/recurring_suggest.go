package insights

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// RecurringSuggestion is a regular payment the strict detector doesn't list:
// a merchant on an uneven rhythm, or a habit without a merchant at all
// (a haircut every ~5 weeks at whichever salon).
type RecurringSuggestion struct {
	Name      string      `json:"name"`
	Merchant  string      `json:"merchant,omitempty"` // set when grouped by merchant
	Category  string      `json:"category"`
	Amount    money.Cents `json:"amount"`     // typical payment
	EveryDays int         `json:"every_days"` // average gap
	Spread    int         `json:"spread_days"`
	Flexible  bool        `json:"flexible"` // the dates move around; suggest "about every N days"
	Count     int         `json:"count"`
	Last      string      `json:"last"`
	Next      string      `json:"next"`
	Notes     []string    `json:"notes"` // what the payments said (to recognise it)
}

var noteHead = regexp.MustCompile(`^[^.(\-–,:]+`)

// SuggestRecurring scans the last 12 months of expenses. `taken` holds
// lower-cased merchants/names already detected, saved or dismissed.
func SuggestRecurring(txs []ledger.Tx, now time.Time, taken map[string]bool) []RecurringSuggestion {
	since := now.AddDate(-1, 0, 0).Format("2006-01-02")
	byMerchant := map[string][]ledger.Tx{}
	noMerchant := map[string][]ledger.Tx{} // by category
	for _, t := range txs {
		if t.Kind != "expense" || t.Date < since || t.Amount <= 0 || t.Pending {
			continue
		}
		switch top := ledger.Top(t.Category); {
		case top == "transfer" || top == "finance" || t.Category == "food.groceries" || top == "transport":
			continue // everyday spending, not a subscription-like cost
		case t.Merchant != "":
			k := strings.ToLower(t.Merchant)
			byMerchant[k] = append(byMerchant[k], t)
		default:
			noMerchant[t.Category] = append(noMerchant[t.Category], t)
		}
	}
	var out []RecurringSuggestion
	consider := func(group []ledger.Tx, merchant string) {
		if s, ok := rhythm(group, now); ok {
			s.Merchant = merchant
			if merchant != "" {
				s.Name = merchant
			} else {
				s.Name = habitName(group)
			}
			if !taken[strings.ToLower(s.Name)] && (merchant == "" || !taken[strings.ToLower(merchant)]) {
				out = append(out, s)
			}
		}
	}
	for _, g := range byMerchant {
		consider(g, g[0].Merchant)
	}
	for _, g := range noMerchant {
		consider(amountCluster(g), "")
	}
	sort.Slice(out, func(i, j int) bool {
		mi, mj := out[i].Amount.Float()/float64(out[i].EveryDays), out[j].Amount.Float()/float64(out[j].EveryDays)
		return mi > mj
	})
	return out
}

// amountCluster keeps the largest group of payments within ±20% of each other.
func amountCluster(g []ledger.Tx) []ledger.Tx {
	var best []ledger.Tx
	for _, c := range g {
		var in []ledger.Tx
		for _, t := range g {
			if math.Abs(t.Amount.Float()-c.Amount.Float()) <= 0.2*c.Amount.Float() {
				in = append(in, t)
			}
		}
		if len(in) > len(best) {
			best = in
		}
	}
	return best
}

// rhythm decides whether payments recur: steady amount, a rhythm between two
// weeks and a quarter, and still going.
func rhythm(g []ledger.Tx, now time.Time) (RecurringSuggestion, bool) {
	if len(g) < 3 {
		return RecurringSuggestion{}, false
	}
	sort.Slice(g, func(i, j int) bool { return g[i].Date < g[j].Date })
	var dates []time.Time
	var amts []float64
	for _, t := range g {
		d, err := time.Parse("2006-01-02", t.Date)
		if err != nil {
			continue
		}
		if len(dates) > 0 && d.Sub(dates[len(dates)-1]) < 5*24*time.Hour {
			continue // same visit split across rows
		}
		dates = append(dates, d)
		amts = append(amts, t.Amount.Float())
	}
	if len(dates) < 3 {
		return RecurringSuggestion{}, false
	}
	var gaps []float64
	for i := 1; i < len(dates); i++ {
		gaps = append(gaps, dates[i].Sub(dates[i-1]).Hours()/24)
	}
	gm, gsd := meanSD(gaps)
	am, asd := meanSD(amts)
	if gm < 14 || gm > 100 || gsd/gm > 0.5 || am <= 0 || asd/am > 0.3 {
		return RecurringSuggestion{}, false
	}
	if len(dates) < 4 && gm < 45 { // three visits only make a pattern for a long rhythm
		return RecurringSuggestion{}, false
	}
	last := dates[len(dates)-1]
	if now.Sub(last).Hours()/24 > math.Max(1.6*gm, 30) {
		return RecurringSuggestion{}, false // stopped
	}
	sort.Float64s(amts)
	every := int(math.Round(gm))
	s := RecurringSuggestion{Category: g[len(g)-1].Category, Amount: money.FromFloat(amts[len(amts)/2]), EveryDays: every,
		Spread: int(math.Round(gsd)), Count: len(dates), Last: last.Format("2006-01-02"),
		Next: last.AddDate(0, 0, every).Format("2006-01-02")}
	s.Flexible = !(gsd <= 3 && every >= 27 && every <= 33) // a steady monthly date isn't flexible
	seen := map[string]bool{}
	for i := len(g) - 1; i >= 0 && len(s.Notes) < 4; i-- {
		n := strings.TrimSpace(g[i].Note)
		if n != "" && !seen[n] {
			seen[n] = true
			s.Notes = append(s.Notes, n)
		}
	}
	return s, true
}

// habitName: the most common start of the notes ("Barbara beauty"), else the category.
func habitName(g []ledger.Tx) string {
	count := map[string]int{}
	best, n := "", 0
	for _, t := range g {
		h := strings.TrimSpace(noteHead.FindString(t.Note))
		if len(h) < 3 {
			continue
		}
		count[h]++
		if count[h] > n || (count[h] == n && h < best) {
			best, n = h, count[h]
		}
	}
	if best == "" {
		return g[0].Category
	}
	return best
}

func meanSD(xs []float64) (float64, float64) {
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, math.Sqrt(v / float64(len(xs)))
}
