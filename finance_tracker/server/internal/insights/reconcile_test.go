package insights_test

import (
	"testing"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

// Swedbank over a week: a card payment dated a day before the bank booked
// it (timing, undone next day), a €1,000 transfer to SEB the ledger lacks
// (unexplained), and a stretch that matches. SEB shows the other side.
func TestReconcile(t *testing.T) {
	d := DB(t)
	Bal(t, d, "swed", "2026-09-30", 2000) // before the window: opens it
	Bal(t, d, "seb", "2026-09-30", 300)
	Tx(t, d, ledger.Tx{Date: "2026-10-02", Kind: "expense", Amount: E(50), Category: "food.groceries", AccountID: "swed"})
	Bal(t, d, "swed", "2026-10-02", 2000) // the bank booked it on the 3rd
	Bal(t, d, "swed", "2026-10-03", 1950)
	Bal(t, d, "swed", "2026-10-05", 950) // €1,000 to SEB, not in the ledger
	Bal(t, d, "seb", "2026-10-05", 1300)
	Tx(t, d, ledger.Tx{Date: "2026-10-06", Kind: "income", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-06", Kind: "transfer", Amount: E(500), Category: "transfer.internal", AccountID: "swed", ToAccountID: "seb"})
	Bal(t, d, "swed", "2026-10-06", 3450)
	Bal(t, d, "seb", "2026-10-06", 1800)
	Bal(t, d, "ibkr", "2026-10-01", 100) // brokers move with prices: not checked
	Bal(t, d, "ibkr", "2026-10-06", 900)
	book, _ := wealth.LoadBook(d)
	txs, _ := ledger.All(d, ledger.Filter{})
	got := insights.Reconcile(book, txs, "2026-10-01")
	by := map[string]insights.ReconAccount{}
	for _, r := range got {
		by[r.AccountID] = r
	}
	if _, ok := by["ibkr"]; ok {
		t.Error("a broker was reconciled")
	}
	s := by["swed"]
	if s.From != "2026-09-30" || s.Opening != E(2000) || s.Closing != E(3450) || s.Ledger != E(2450) || s.Unexplained != -E(1000) || s.Intervals != 4 || s.Matched != 1 {
		t.Fatalf("swed: %+v", s)
	}
	if len(s.Gaps) != 3 || !s.Gaps[0].Timing || !s.Gaps[1].Timing || s.Gaps[2].Timing || s.Gaps[2].Unexplained != -E(1000) || s.Gaps[2].From != "2026-10-03" {
		t.Fatalf("swed gaps: %+v", s.Gaps)
	}
	if b := by["seb"]; b.Unexplained != E(1000) || len(b.Gaps) != 1 {
		t.Fatalf("seb: %+v", b)
	}
	if s.Reconciles || by["seb"].Reconciles || s.LateBooking != nil {
		t.Errorf("a missing €1,000 is not a late booking: %+v", s)
	}
}

// A card payment dated two days before the window opens and booked by the
// bank inside it explains the whole difference — not a missing payment.
func TestReconcileLateBooking(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-10-02", Kind: "expense", Amount: E(65.20), Category: "food.groceries", Merchant: "Maxima", AccountID: "swed"})
	Bal(t, d, "swed", "2026-10-04", 609.46) // not booked yet
	Bal(t, d, "swed", "2026-10-05", 583.24)
	Bal(t, d, "swed", "2026-10-07", 544.26) // booked in two parts of the day's moves
	book, _ := wealth.LoadBook(d)
	txs, _ := ledger.All(d, ledger.Filter{})
	r := insights.Reconcile(book, txs, "2026-10-04")[0]
	if r.Unexplained != -E(65.20) || r.LateBooking == nil || r.LateBooking.Merchant != "Maxima" || r.LateBooking.Edge != "opening" || !r.Reconciles {
		t.Fatalf("late booking: %+v %+v", r, r.LateBooking)
	}
	for _, g := range r.Gaps {
		if !g.Timing {
			t.Errorf("gap not marked timing: %+v", g)
		}
	}
	// A difference no single payment near the edges explains stays open.
	Bal(t, d, "swed", "2026-10-09", 444.26)
	book, _ = wealth.LoadBook(d)
	if r := insights.Reconcile(book, txs, "2026-10-04")[0]; r.Reconciles || r.LateBooking != nil {
		t.Fatalf("€100 more went missing: %+v", r)
	}
}
