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
	st.IncomeMode, st.ManualIncome = "manual", E(3000)
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

// The new job pays October's salary on 5 Nov under a "by the 10th" rule. That
// payday is settled: the next is 10 Dec, so Home plans Swedbank through the
// €1,000 alimony on the 17th instead of stopping at the 10th and calling the
// money for it idle — and the calendar adds no salary on 10 Nov.
func TestPayAlreadyInMovesPaydayToNextMonth(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	if _, err := ledger.SaveAccount(d, ledger.Account{ID: "swed_sav", Name: "Swedbank savings", Institution: "Swedbank", Kind: "savings", Liquid: true}); err != nil {
		t.Fatal(err)
	}
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", E(6000)
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(1000)}
	plan.Save(d, &alimony, "")
	for _, m := range []string{"2026-08", "2026-09", "2026-10"} {
		Tx(t, d, ledger.Tx{Date: m + "-17", Amount: E(1000), Category: "kids.alimony", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-06", Amount: E(300), Category: "food.groceries", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(3100), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Amount: E(4125), Category: "salary", AccountID: "swed"})
	Bal(t, d, "swed", "2026-11-05", 978.84)
	Bal(t, d, "swed_sav", "2026-11-05", 8000)

	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := o.Cash
	if c == nil || c.Payday != "2026-12-10" {
		t.Fatalf("payday %+v", c)
	}
	for _, a := range c.Accounts {
		if a.ID != "swed" {
			continue
		}
		var alimonyIn bool
		for _, it := range a.Items {
			alimonyIn = alimonyIn || (it.Date == "2026-11-17" && it.Amount == -E(1000))
		}
		if !alimonyIn || a.Idle != 0 || a.Needed <= 0 {
			t.Errorf("swed until 10 Dec: alimony %v idle %v needed %v items %+v", alimonyIn, a.Idle, a.Needed, a.Items)
		}
	}
	for _, m := range c.ToSavings {
		if m.From == "swed" {
			t.Errorf("money for the alimony sent to savings: %+v", m)
		}
	}

	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "2026-11-05" || rb.Expected != "2026-12-10" {
		t.Errorf("runbook payday %q expected %q", rb.Payday, rb.Expected)
	}

	cal, err := cfo.CashFlowCalendar(d, now, 60)
	if err != nil {
		t.Fatal(err)
	}
	var salaries []string
	for _, day := range cal.Days {
		for _, e := range day.Events {
			if e.Kind == "income" {
				salaries = append(salaries, day.Date)
			}
		}
	}
	if len(salaries) != 1 || salaries[0] != "2026-12-10" {
		t.Errorf("calendar salaries %v, want only 2026-12-10", salaries)
	}
}

// A mid-month advance (the old job's 15th) is not next month's pay come
// early: on the 20th the final part is still due by the 3rd.
func TestAdvanceDoesNotSettleNextPayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-09-15", Amount: E(2000), Category: "salary", AccountID: "swed"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Expected != "2026-10-03" {
		t.Errorf("expected %q, want 2026-10-03", rb.Expected)
	}
}

// A €30 voucher booked as salary on 27 Nov is not December's pay come early:
// payday stays 10 Dec rather than jumping to January.
func TestSmallSalaryRowDoesNotSettlePayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	st := plan.LoadSettings(d)
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	plan.SaveSettings(d, st)
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Amount: E(4125), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-27", Amount: E(30), Category: "salary", AccountID: "swed", Note: "birthday voucher"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Expected != "2026-12-10" {
		t.Errorf("expected %q, want 2026-12-10", rb.Expected)
	}
	// The real pay on 6 Dec does settle it.
	Tx(t, d, ledger.Tx{Date: "2026-12-06", Amount: E(4200), Category: "salary", AccountID: "swed"})
	if rb, _ = cfo.BuildRunbook(d, time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)); rb.Expected != "2027-01-10" || rb.Payday != "2026-12-06" {
		t.Errorf("after December's pay: payday %q expected %q", rb.Payday, rb.Expected)
	}
}

// With payday a month out, an obligation due early next month (alimony on
// the 2nd) is in the plan — it is in neither month's list otherwise.
func TestCashPlanCountsObligationEarlyNextMonth(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", E(6000)
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	plan.SaveSettings(d, st)
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(1000)}
	plan.Save(d, &alimony, "")
	for _, m := range []string{"2026-09", "2026-10", "2026-11"} {
		Tx(t, d, ledger.Tx{Date: m + "-02", Amount: E(1000), Category: "kids.alimony", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(3100), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Amount: E(4125), Category: "salary", AccountID: "swed"})
	Bal(t, d, "swed", "2026-11-05", 1200)
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if o.Cash == nil || o.Cash.Payday != "2026-12-10" {
		t.Fatalf("cash %+v", o.Cash)
	}
	var found bool
	for _, a := range o.Cash.Accounts {
		for _, it := range a.Items {
			if it.Date == "2026-12-02" && it.Kind == "obligation" && it.Amount == -E(1000) && a.ID == "swed" {
				found = true
			}
		}
		if a.ID == "swed" && a.Idle > E(150) { // €1,200 − €1,000 alimony − the €50 buffer
			t.Errorf("money for December's alimony called idle: %v", a.Idle)
		}
	}
	if !found {
		t.Errorf("no 2 Dec alimony in %+v", o.Cash.Accounts)
	}
}

// Pay on 30 Sep for the "by the 3rd" payday is October's payday: Plan shows
// it in, not "Salary not in yet".
func TestPayOnTheThirtiethIsThisMonthsPayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	Tx(t, d, ledger.Tx{Date: "2026-08-31", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-09-30", Amount: E(3100), Category: "salary", AccountID: "swed"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "2026-09-30" || rb.Salary != E(3100) || rb.Expected != "2026-11-03" {
		t.Errorf("payday %q salary %v expected %q", rb.Payday, rb.Salary, rb.Expected)
	}
}
