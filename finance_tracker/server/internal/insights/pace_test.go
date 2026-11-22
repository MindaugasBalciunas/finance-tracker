package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// kinds sets the account kinds a transfer moves between, as ledger.All loads them.
func kinds(t ledger.Tx, from, to string) ledger.Tx { t.FromKind, t.ToKind = from, to; return t }

func TestPaceOutAddsNetInvestingToSpending(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	txs := []ledger.Tx{
		tx("2026-10-02", "expense", "food", 100, "swed"),
		kinds(tx("2026-10-03", "transfer", "transfer.invest", 500, "swed", "ibkr"), "checking", "brokerage"),
		kinds(tx("2026-10-04", "transfer", "transfer.debt", 400, "swed", "mortgage"), "checking", "loan"),
		// Money back out of the broker counts against invested.
		kinds(tx("2026-10-05", "transfer", "transfer.invest", 150, "ibkr", "swed"), "brokerage", "checking"),
		// A move between two wealth accounts is neutral.
		kinds(tx("2026-10-06", "transfer", "transfer.invest", 999, "ibkr", "crypto"), "brokerage", "crypto"),
		// Shuffling cash isn't investing.
		kinds(tx("2026-10-07", "transfer", "transfer.internal", 777, "swed", "seb"), "checking", "checking"),
		// Pension paid by payroll never left a bank account.
		kinds(tx("2026-10-08", "transfer", "transfer.pension", 88, "", "pension"), "", "pension"),
		kinds(tx("2026-09-15", "transfer", "transfer.pension", 200, "swed", "pension"), "checking", "pension"),
		tx("2026-09-01", "expense", "food", 300, "swed"),
		// Six months back: a net payout month pulls the typical line down.
		kinds(tx("2026-05-02", "transfer", "transfer.pension", 600, "swed", "pension"), "checking", "pension"),
		kinds(tx("2026-06-02", "transfer", "transfer.invest", 1200, "ibkr", "swed"), "brokerage", "checking"),
		tx("2026-06-02", "expense", "food", 60, "swed"),
	}
	p := insights.PaceOut(txs, now)
	if len(p) != 31 || p[10].Current != nil || p[10].CurrentInvested != nil {
		t.Fatalf("future days must be empty: %+v", p[10])
	}
	if *p[9].CurrentInvested != E(750) || *p[9].Current != E(850) {
		t.Fatalf("this month: invested %v total %v, want 750 / 850", *p[9].CurrentInvested, *p[9].Current)
	}
	if p[13].LastMonth != E(300) || p[14].LastMonth != E(500) || p[14].LastMonthInvested != E(200) {
		t.Fatalf("last month: %+v %+v", p[13], p[14])
	}
	// Typical = (Sep 200 − Jun 1200 + May 600) / 6 invested; (Sep 300 + Jun 60) / 6 spent.
	if p[1].TypicalInvested != E(-1200+600)/6 || p[30].TypicalInvested != E(200-1200+600)/6 || p[30].Typical != E(360)/6+E(-400)/6 {
		t.Fatalf("typical: %+v %+v", p[1], p[30])
	}
	// The spending lines stay what the plain pace chart shows.
	if s := insights.Pace(txs, now); *s[9].Current != E(100) {
		t.Fatal(*s[9].Current)
	}
}
