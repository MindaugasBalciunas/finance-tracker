package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
)

// A cost in the same month every year is found, with the median of the last
// three years and next year's month; a shop paid most months, two purchases
// years apart, and what the plan already holds as an obligation are not.
func TestDetectYearly(t *testing.T) {
	e := money.FromFloat
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tx := func(date, m, cat string, eur float64) ledger.Tx {
		return ledger.Tx{Date: date, Kind: "expense", Amount: e(eur), Merchant: m, Category: cat}
	}
	txs := []ledger.Tx{
		tx("2023-10-05", "Tokvila", "transport.car", 339), tx("2024-10-12", "Tokvila", "transport.car", 440),
		tx("2025-10-03", "Tokvila", "transport.car", 300), tx("2025-10-20", "Tokvila", "transport.car", 225), // one service, two invoices
		tx("2026-10-14", "Tokvila", "transport.car", 769),
		tx("2024-12-10", "Toy shop", "gifts", 300), tx("2025-11-28", "Toy shop", "gifts", 120), // Christmas, a month early once
		tx("2022-12-08", "Mk Trade", "shopping.electronics", 370), tx("2025-12-08", "Mk Trade", "shopping.electronics", 2825),
		tx("2025-10-15", "Bank", "kids.alimony", 1000), tx("2026-10-15", "Bank", "kids.alimony", 1000),
	}
	for _, m := range []string{"2025-11", "2026-01", "2026-03", "2026-05", "2026-07", "2026-09"} {
		txs = append(txs, tx(m+"-10", "Lidl", "food.groceries", 120))
	}
	got := insights.DetectYearly(txs, now, func(t *ledger.Tx) bool { return t.Category == "kids.alimony" })
	if len(got) != 2 {
		t.Fatalf("found %+v", got)
	}
	car := got[0]
	if car.Merchant != "Tokvila" || car.Month != 10 || car.Typical != e(525) || car.Last != e(769) || car.Next != "2027-10" || car.Monthly != e(525)/12 {
		t.Errorf("car service %+v", car)
	}
	if g := got[1]; g.Merchant != "Toy shop" || g.Typical != e(210) || g.Month != 11 || g.Next != "2026-11" { // the latest payment sets the month
		t.Errorf("gifts %+v", g)
	}
}
