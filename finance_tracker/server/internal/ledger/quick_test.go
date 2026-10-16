package ledger_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// A new expense starts from habit: the account usually paid from by hand,
// and the merchants usually paid on this kind of day with their category —
// weekend shops don't show on a weekday, one-offs and one-time merchants
// never do.
func TestQuickPicks(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)                                  // a Wednesday
	for _, day := range []string{"2026-10-07", "2026-10-08", "2026-10-13", "2026-09-30"} { // weekdays
		Tx(t, d, ledger.Tx{Date: day, Amount: E(8.5), Category: "food.work_lunch", AccountID: "cash", Merchant: "Caffeine"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-06", Amount: E(55), Category: "transport.fuel", AccountID: "swed", Merchant: "Neste"})
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(60), Category: "transport.fuel", AccountID: "swed", Merchant: "Neste"})
	for _, day := range []string{"2026-10-10", "2026-10-11", "2026-10-04"} { // weekends
		Tx(t, d, ledger.Tx{Date: day, Amount: E(70), Category: "food.groceries", AccountID: "swed", Merchant: "Maxima"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-05", Amount: E(14), Category: "utilities.telecom", AccountID: "swed", Merchant: "Telia"}) // a bill: never a pick
	Tx(t, d, ledger.Tx{Date: "2026-09-07", Amount: E(14), Category: "utilities.telecom", AccountID: "swed", Merchant: "Telia"})
	Tx(t, d, ledger.Tx{Date: "2026-10-09", Amount: E(800), Category: "shopping.electronics", AccountID: "swed", Merchant: "Senukai"})
	Tx(t, d, ledger.Tx{Date: "2026-10-12", Amount: E(900), Category: "shopping.electronics", AccountID: "swed", Merchant: "Senukai"})
	q, err := ledger.Quick(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if q.AccountID != "swed" || len(q.Merchants) != 2 {
		t.Fatalf("%+v", q)
	}
	c := q.Merchants[0]
	if c.Merchant != "Caffeine" || c.Category != "food.work_lunch" || c.AccountID != "cash" || c.Typical != E(8.5) || q.Merchants[1].Merchant != "Neste" {
		t.Errorf("weekday picks %+v", q.Merchants)
	}
	sat, _ := ledger.Quick(d, time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	if len(sat.Merchants) != 1 || sat.Merchants[0].Merchant != "Maxima" {
		t.Errorf("saturday picks %+v", sat.Merchants)
	}
}
