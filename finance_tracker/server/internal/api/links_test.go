package api_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// A figure's link opens the transactions behind it: following it in the
// Ledger gives the same total — one place across spellings, obligations
// left out where the figure leaves them out, and a salary paid early next
// month counted for the month it pays for.
func TestLinksReproduceTheirFigures(t *testing.T) {
	s, c := newServer(t)
	now := time.Now()
	m := func(back, day int) string {
		return time.Date(now.Year(), now.Month()-time.Month(back), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	add := func(x ledger.Tx) { x.AccountID = "swed"; Tx(t, s.DB, x) }
	for back := 1; back <= 3; back++ {
		add(ledger.Tx{Date: m(back, 5), Kind: "expense", Amount: E(40), Category: "food.groceries", Merchant: "Šilas"})
		add(ledger.Tx{Date: m(back, 6), Kind: "expense", Amount: E(25), Category: "food.groceries", Merchant: "SILAS"})
		add(ledger.Tx{Date: m(back, 7), Kind: "expense", Amount: E(900), Category: "housing.mortgage_interest", Merchant: "Bank"}) // an obligation
		add(ledger.Tx{Date: m(back-1, 2), Kind: "income", Amount: E(5000), Category: "salary", Merchant: "Employer"})              // paid on the 2nd for the month before
	}
	type item struct {
		Label string  `json:"label"`
		N     float64 `json:"n"`
		Link  string  `json:"link"`
	}
	var facts []struct {
		Key   string `json:"key"`
		Items []item `json:"items"`
	}
	c.ok("GET", "/insights/fun", nil, &facts)
	ledgerTotal := func(link string) (exp, inc float64) {
		var r struct{ Expense, Income float64 }
		c.ok("GET", strings.Replace(link, "/ledger?", "/transactions?", 1)+"&limit=1000", nil, &r)
		return r.Expense, r.Income
	}
	checked := 0
	for _, f := range facts {
		for _, it := range f.Items {
			if (f.Key == "spent_most" || f.Key == "months") && it.Link != "" {
				if exp, _ := ledgerTotal(it.Link); math.Abs(exp-it.N) > 0.01 {
					t.Errorf("%s %s: figure %.2f, its transactions %.2f (%s)", f.Key, it.Label, it.N, exp, it.Link)
				}
				checked++
			}
		}
	}
	if checked < 2 {
		t.Fatalf("expected shop and month figures to check, got %d: %+v", checked, facts)
	}
	// Money in for a month, salary counted for the month it pays for.
	month := m(2, 1)[:7]
	if _, inc := ledgerTotal(ledger.MonthLink(month, "kind", "income", "salary", "month")); inc != 5000 {
		t.Errorf("salary for %s: %v", month, inc)
	}
}
