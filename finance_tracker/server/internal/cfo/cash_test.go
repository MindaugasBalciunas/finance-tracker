package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// Cash until payday: obligations, standing orders and day-to-day spending
// per account, topped up from savings in stages as late as possible — and
// working it out changes nothing in the ledger.
func TestCashUntilPayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	if _, err := ledger.SaveAccount(d, ledger.Account{ID: "swed_sav", Name: "Swedbank savings", Institution: "Swedbank", Kind: "savings", Liquid: true}); err != nil {
		t.Fatal(err)
	}
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", 3000
	st.SalaryRules = []plan.SalaryRule{{PaidByDay: 20}}
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(1000)}
	plan.Save(d, &alimony, "")
	for _, m := range []string{"2026-07", "2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-16", Amount: E(1000), Category: "kids.alimony", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-10", Kind: "transfer", Amount: E(200), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea", Merchant: "Artea"})
		Tx(t, d, ledger.Tx{Date: m + "-05", Amount: E(300), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-25", Amount: E(3000), Category: "salary", AccountID: "swed"})
	}
	Bal(t, d, "swed", "2026-10-06", 500)
	Bal(t, d, "swed_sav", "2026-10-06", 3000)

	fingerprint := func() string {
		var s string
		d.QueryRow(`SELECT (SELECT COUNT(*)||':'||COALESCE(SUM(amount),0) FROM transactions)||'|'||
			(SELECT COUNT(*)||':'||COALESCE(SUM(value),0) FROM balances)||'|'||(SELECT COUNT(*) FROM recurring_items)||'|'||
			(SELECT COUNT(*) FROM budgets)||'|'||(SELECT COALESCE(GROUP_CONCAT(key||value),'') FROM settings)`).Scan(&s)
		return s
	}
	before := fingerprint()
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(); after != before {
		t.Fatalf("working out cash changed the data:\n%s\n%s", before, after)
	}
	c := o.Cash
	if c == nil || c.Payday != "2026-10-20" || c.SalaryAccount != "swed" || c.DailyAccount != "swed" || c.Daily <= 0 {
		t.Fatalf("plan %+v", c)
	}
	var swed *cfo.CashAccount
	for i := range c.Accounts {
		if c.Accounts[i].ID == "swed" {
			swed = &c.Accounts[i]
		}
	}
	if swed == nil {
		t.Fatalf("no Swedbank in %+v", c.Accounts)
	}
	got := map[string]money.Cents{}
	for _, it := range swed.Items {
		got[it.Date+" "+it.Kind] += it.Amount
	}
	if got["2026-10-16 obligation"] != -E(1000) || got["2026-10-10 transfer"] != -E(200) {
		t.Errorf("items %+v", swed.Items)
	}
	// Staged: the standing order and alimony each get their own top-up, the
	// later one dated the day before alimony leaves.
	var staged, moved money.Cents
	for _, s := range swed.Stages {
		staged += s.Amount
	}
	// Top-ups keep the account at or above its buffer (worked out from its own
	// everyday spending), not merely above zero.
	if !swed.BufferAuto || swed.Buffer < E(50) {
		t.Errorf("buffer %v auto %v", swed.Buffer, swed.BufferAuto)
	}
	if len(swed.Stages) < 2 || swed.Stages[len(swed.Stages)-1].By != "2026-10-15" || staged < swed.Buffer-swed.Low {
		t.Errorf("stages %+v (low %v)", swed.Stages, swed.Low)
	}
	for _, m := range c.Moves {
		if m.From != "swed_sav" || m.To != "swed" || m.Amount <= 0 {
			t.Errorf("move %+v", m)
		}
		moved += m.Amount
	}
	if moved != staged || c.Short != 0 {
		t.Errorf("moved %v of %v, short %v", moved, staged, c.Short)
	}
	// "Needs you" shows only the next top-up; the schedule is in the cash card.
	found := 0
	for _, a := range o.Actions {
		if a.Kind == "move" {
			found++
			if a.Due != c.Moves[0].By {
				t.Errorf("next move action %+v", a)
			}
		}
	}
	if found != 1 {
		t.Errorf("%d move actions", found)
	}
}
