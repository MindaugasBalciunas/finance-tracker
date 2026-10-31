package insights_test

import (
	"testing"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

func paid(date, merchant, cat string, eur float64) ledger.Tx {
	t := tx(date, "expense", cat, eur, "swed")
	t.Merchant = merchant
	return t
}

// Spellings merge the way the Ledger's merchant filter matches, a split
// purchase is one visit, a refund lowers the merchant's total, and rows
// after the window are not counted.
func TestMerchantsRankWhoWasPaid(t *testing.T) {
	receipt := paid("2026-10-12", "Maxima", "food.groceries", 20)
	receipt.ID = 99
	split := paid("2026-10-12", "Maxima", "home", 10)
	split.SplitOf = 99
	moved := paid("2026-10-12", "Eurovaistinė", "health", 8) // a part of the same receipt, moved to the pharmacy
	moved.SplitOf = 99
	refund := tx("2026-10-20", "income", "refunds", 5, "swed")
	refund.Merchant = "IKEA"
	txs := []ledger.Tx{
		paid("2026-10-02", "Circle K", "transport.fuel", 60),
		paid("2026-10-15", "CIRCLE K", "transport.fuel", 40),
		paid("2026-10-20", "Circle K", "food", 5),
		paid("2026-09-10", "Circle K", "transport.fuel", 50), // previous window
		paid("2026-10-05", "Maxima", "food.groceries", 30),
		receipt, split, moved, refund,
		paid("2026-10-18", "IKEA", "home", 45),
		paid("2026-11-02", "Maxima", "food.groceries", 999), // after the window
		tx("2026-10-08", "expense", "food", 7, "swed"),      // no merchant
		paid("2026-08-01", "Oldie", "food", 3),              // before both windows
	}
	r := insights.Merchants(txs, "2026-10-01", "2026-10-31", 10)
	if r.PrevFrom != "2026-08-31" || r.PrevTo != "2026-09-30" {
		t.Fatalf("previous window %s–%s", r.PrevFrom, r.PrevTo)
	}
	if len(r.Items) != 4 || r.Count != 4 {
		t.Fatalf("got %+v", r.Items)
	}
	ck, mx, ik := r.Items[0], r.Items[1], r.Items[2]
	if ck.Name != "Circle K" || ck.Total != E(105) || ck.Count != 3 || ck.Average != E(35) || ck.Category != "transport.fuel" {
		t.Fatalf("Circle K: %+v", ck)
	}
	if ck.Previous != E(50) || ck.Change != 1.1 || ck.First != "2026-09-10" || ck.New {
		t.Fatalf("Circle K trend: %+v", ck)
	}
	if mx.Name != "Maxima" || mx.Total != E(60) || mx.Count != 2 || mx.Last != "2026-10-12" {
		t.Fatalf("Maxima: a split part is not a visit, November is outside: %+v", mx)
	}
	if ik.Total != E(40) || !ik.New || ik.Previous != 0 {
		t.Fatalf("IKEA net of its refund, new this month: %+v", ik)
	}
	if ev := r.Items[3]; ev.Name != "Eurovaistinė" || ev.Count != 1 || ev.Total != E(8) {
		t.Fatalf("a split part moved to another merchant is a visit there: %+v", ev)
	}
	if r.Total != E(213) || r.Unnamed != E(7) || r.NewCount != 3 || r.NewTotal != E(108) {
		t.Fatalf("totals %+v", r)
	}
	if len(ck.Months) != 12 || ck.Months[11].Month != "2026-10" || ck.Months[11].Amount != E(105) || ck.Months[10].Amount != E(50) || ck.Months[0].Month != "2025-11" {
		t.Fatalf("months %+v", ck.Months)
	}
	if ck.Share != 0.493 {
		t.Fatalf("share %v", ck.Share)
	}
	if got := insights.Merchants(txs, "2026-10-01", "2026-10-31", 1); len(got.Items) != 1 || got.Count != 4 {
		t.Fatal("limit cuts the list, not the counts")
	}
	// All time starts before the first row: nothing is "new" then.
	if all := insights.Merchants(txs, "2000-01-01", "2026-10-31", 10); all.NewCount != 0 || all.Items[0].New {
		t.Fatalf("all time: %d new", all.NewCount)
	}
}

// A regular (paid in 3+ of the 6 months before the window) not seen for
// two months by the window's end is lapsed; a recent or occasional one is not.
func TestMerchantsFlagRegularsThatStopped(t *testing.T) {
	txs := []ledger.Tx{
		paid("2026-05-03", "Gym+", "health", 30), paid("2026-06-03", "Gym+", "health", 30), paid("2026-07-03", "Gym+", "health", 30),
		paid("2026-06-10", "Wolt", "food", 20), paid("2026-08-10", "Wolt", "food", 20), paid("2026-09-20", "Wolt", "food", 20),
		paid("2026-08-01", "Rare", "food", 9), paid("2026-09-01", "Rare", "food", 9),
		paid("2026-10-04", "Lidl", "food", 15), paid("2026-07-04", "Lidl", "food", 15), paid("2026-08-04", "Lidl", "food", 15), paid("2026-09-04", "Lidl", "food", 15),
		paid("2026-10-05", "Bought", "home", 50),
	}
	ret := tx("2026-10-06", "income", "refunds", 50, "swed")
	ret.Merchant = "Bought"
	txs = append(txs, ret)
	r := insights.Merchants(txs, "2026-10-01", "2026-10-31", 10)
	if len(r.Lapsed) != 1 || r.Lapsed[0].Name != "Gym+" || r.Lapsed[0].Last != "2026-07-03" || r.Lapsed[0].Monthly != E(15) {
		t.Fatalf("only the gym stopped (Wolt was last paid 41 days ago): %+v", r.Lapsed)
	}
	if len(r.Items) != 1 || r.Items[0].Name != "Lidl" {
		t.Fatalf("bought-and-returned drops out: %+v", r.Items)
	}
}

func TestMerchantsOnAnEmptyLedger(t *testing.T) {
	r := insights.Merchants(nil, "2026-10-01", "2026-10-31", 10)
	if r.Items == nil || r.Lapsed == nil || r.Total != 0 {
		t.Fatalf("empty lists, not null: %+v", r)
	}
}
