package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/merchant"
	. "ft/internal/testutil"
)

// Habits count purchases by receipt, leave obligations and the people paid
// one out, skip pending rows and the month in progress, and split the month
// and the week where the money went.
func TestHabits(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	ex := func(id int64, date, cat string, eur float64, merch string) ledger.Tx {
		return ledger.Tx{ID: id, Date: date, Kind: "expense", Category: cat, Amount: E(eur), Merchant: merch}
	}
	txs := []ledger.Tx{
		ex(1, "2025-08-10", "food.groceries", 100, "Lidl"),
		ex(2, "2026-10-03", "food.groceries", 50, "Lidl"),        // Saturday, days 1–10
		ex(3, "2026-10-12", "food.coffee", 4, "Caffeine"),        // Monday, a small buy
		ex(4, "2026-10-15", "food.restaurants", 30, "Fancy"),     // one receipt…
		ex(0, "2026-10-15", "kids.food", 5, "Fancy"),             // …split (below)
		ex(9, "2026-10-26", "food.restaurants", 1113.5, ""),      // an occasion: spent, not eating out
		ex(5, "2026-10-17", "kids.alimony", 1000, "Evelina"),     // an obligation
		ex(6, "2026-10-20", "kids.general", 50, "Evelina"),       // paid to the same person
		ex(7, "2026-10-25", "shopping.online", 20, "AliExpress"), // pending (below)
		ex(8, "2026-11-02", "food.groceries", 70, "Lidl"),        // the month in progress
	}
	txs[4].SplitOf = 4
	txs[8].Pending = true
	h := insights.Habits(txs, nil, now, 3)
	if len(h.Months) != 3 || h.Months[2].Month != "2026-10" {
		t.Fatalf("months %+v", h.Months)
	}
	o := h.Months[2]
	if o.Spent != E(1202.5) || o.Purchases != 4 || o.Median != E(42.5) || o.Small != 1 || o.SmallSpent != E(4) || o.Days != 4 {
		t.Fatalf("october %+v", o)
	}
	if o.Early != E(50) || o.Mid != E(39) || o.Late != E(1113.5) || o.Weekend != E(50) || o.EatingOut != E(34) || o.Groceries != E(50) {
		t.Fatalf("october split %+v", o)
	}
	if h.Weekdays[0].Average <= 0 || h.Weekdays[5].Average <= 0 || h.Weekdays[1].Average != 0 {
		t.Fatalf("weekdays %+v", h.Weekdays)
	}
	// October over the years seen: 2026 (€1,202.50) and 2025 (nothing).
	if s := h.Season[9]; s.Years != 2 || s.Average != E(601.25) {
		t.Fatalf("october season %+v", s)
	}
	if h.Recent != [2]string{"2026-08", "2026-10"} || h.YearAgo != [2]string{"2025-08", "2025-10"} {
		t.Fatalf("windows %v %v", h.Recent, h.YearAgo)
	}
	got := map[string][2]float64{}
	for _, c := range h.Changes {
		got[c.Key] = [2]float64{c.Now, c.Then}
	}
	if got["spent"] != [2]float64{400.83, 33.33} || got["purchases"] != [2]float64{4.0 / 3, 1.0 / 3} || got["eating_out"][0] != 11.33 {
		t.Fatalf("changes %+v", got)
	}
}

// No history: no months, no comparison. History that starts inside the
// year-ago window: months to show, nothing to compare with. An even count
// of purchases: the usual one is halfway between the middle two.
func TestHabitsEdges(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	if h := insights.Habits(nil, nil, now, 12); len(h.Months) != 0 || len(h.Changes) != 0 {
		t.Fatalf("empty: %+v", h)
	}
	txs := []ledger.Tx{
		{ID: 1, Date: "2026-01-05", Kind: "expense", Category: "food.groceries", Amount: E(10)},
		{ID: 2, Date: "2026-01-06", Kind: "expense", Category: "food.groceries", Amount: E(30)},
	}
	h := insights.Habits(txs, nil, now, 24)
	if len(h.Months) != 10 || h.Months[0].Month != "2026-01" || len(h.Changes) != 0 {
		t.Fatalf("young history: %d months from %s, %d changes", len(h.Months), h.Months[0].Month, len(h.Changes))
	}
	if h.Months[0].Median != E(20) {
		t.Fatalf("median of two: %v", h.Months[0].Median)
	}
	// An ex-partner paid alimony years ago is still not a shop.
	gift := []ledger.Tx{{ID: 3, Date: "2026-10-20", Kind: "expense", Category: "kids.general", Amount: E(50), Merchant: "Evelina"}}
	if h := insights.Habits(gift, map[string]bool{merchant.Key("Evelina"): true}, now, 1); h.Months[0].Spent != 0 {
		t.Fatalf("payee left out: %+v", h.Months[0])
	}
}
