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

func TestMonthIncomeUseSplitsInvestedByAccount(t *testing.T) {
	txs := []ledger.Tx{
		tx("2026-11-03", "income", "salary", 3000, "swed"),
		tx("2026-11-04", "expense", "food", 400, "swed"),
		kinds(tx("2026-11-05", "transfer", "transfer.invest", 1000, "swed", "ibkr"), "checking", "brokerage"),
		kinds(tx("2026-11-06", "transfer", "transfer.invest", 500, "swed", "ibkr"), "checking", "brokerage"),
		kinds(tx("2026-11-07", "transfer", "transfer.invest", 300, "revolut", "swed"), "brokerage", "checking"),
		kinds(tx("2026-11-08", "transfer", "transfer.debt", 440, "seb", "mortgage"), "checking", "loan"),
		kinds(tx("2026-11-09", "transfer", "transfer.invest", 999, "ibkr", "revolut"), "brokerage", "brokerage"),
		kinds(tx("2026-10-20", "transfer", "transfer.invest", 777, "swed", "ibkr"), "checking", "brokerage"),
	}
	f := insights.CashFlow(txs, cats(t), "month", nil)
	var nov insights.Flow
	for _, x := range f {
		if x.Period == "2026-11" {
			nov = x
		}
	}
	nov.ExpectedSalary = E(200)
	names := map[string]ledger.Account{"ibkr": {Name: "IBKR"}, "revolut": {Name: "Revolut ETF"}, "mortgage": {Name: "Mortgage"}}
	u := insights.MonthIncomeUse(nov, txs, names)
	if u.Invested != E(1640) || u.Spent != E(400) || u.Left != E(3000+200-400-1640) {
		t.Fatalf("totals: %+v", u)
	}
	want := []struct {
		name string
		amt  float64
	}{{"IBKR", 1500}, {"Mortgage", 440}, {"Revolut ETF", -300}}
	if len(u.Into) != len(want) {
		t.Fatalf("into: %+v", u.Into)
	}
	var sum = E(0)
	for i, w := range want {
		if u.Into[i].Name != w.name || u.Into[i].Amount != E(w.amt) {
			t.Fatalf("into[%d] = %+v, want %s %v", i, u.Into[i], w.name, w.amt)
		}
		sum += u.Into[i].Amount
	}
	if sum != u.Invested {
		t.Fatal("the accounts must add up to invested")
	}
}

func TestMonthIncomeUseKeepsUnlinkedMoneyInAndOutApart(t *testing.T) {
	txs := []ledger.Tx{
		// A bank row for money invested that names no destination account.
		kinds(tx("2026-11-05", "transfer", "transfer.invest", 500, "swed"), "checking", ""),
		// A pension payout arriving from outside, no source account.
		kinds(tx("2026-11-06", "transfer", "transfer.pension", 200, "", "swed"), "", "checking"),
		kinds(tx("2026-11-07", "transfer", "transfer.invest", 100, "swed", "gone"), "checking", "brokerage"),
	}
	f := insights.CashFlow(txs, cats(t), "month", nil)
	u := insights.MonthIncomeUse(f[0], txs, map[string]ledger.Account{})
	got := map[string]float64{}
	for _, x := range u.Into {
		got[x.Name] = x.Amount.Float()
	}
	if len(u.Into) != 3 || got["Not linked to an account"] != 500 || got["Paid back out"] != -200 || got["Removed account"] != 100 {
		t.Fatalf("%+v", u.Into)
	}
	if u.Invested != E(400) {
		t.Fatal(u.Invested)
	}
}
