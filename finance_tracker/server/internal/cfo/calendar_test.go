package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// A year of: salary (it lands on the salary rules' payday — by the 3rd by
// default — since income isn't on the recurring list), €1,400 to the mortgage on the 17th, Netflix
// on the 5th, and €500 moved between two cash accounts on the 20th. Over the
// next 60 days the salary lands, the mortgage leaves cash, Netflix shows on
// its day and the cash-to-cash move doesn't change the balance.
func TestCashFlowCalendarPutsEachMoneyMoveOnItsDay(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 12; m++ {
		mo := now.AddDate(0, -m, 0).Format("2006-01")
		Tx(t, d, ledger.Tx{Date: mo + "-10", Amount: E(4000), Category: "salary", AccountID: "swed", Merchant: "Employer"})
		Tx(t, d, ledger.Tx{Kind: "transfer", Date: mo + "-17", Amount: E(1400), Category: "transfer.debt", AccountID: "swed", ToAccountID: "mortgage"})
		Tx(t, d, ledger.Tx{Date: mo + "-05", Amount: E(12.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "Netflix"})
		Tx(t, d, ledger.Tx{Kind: "transfer", Date: mo + "-20", Amount: E(500), Category: "transfer.internal", AccountID: "swed", ToAccountID: "seb"})
		for k := 0; k < 10; k++ {
			Tx(t, d, ledger.Tx{Date: mo + "-12", Amount: E(30), Category: "food.groceries", AccountID: "swed", Merchant: "Lidl"})
		}
	}
	Bal(t, d, "swed", "2026-10-29", 6000)
	Bal(t, d, "house", "2022-01-01", 300000)
	Bal(t, d, "mortgage", "2026-10-01", -200000)
	c, err := cfo.CashFlowCalendar(d, now, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Days) != 60 || c.Days[0].Date != "2026-10-31" || c.Start != E(6000) || c.Daily <= 0 {
		t.Fatalf("calendar %+v", c)
	}
	on := map[string][]cfo.CalEvent{}
	for _, d := range c.Days {
		on[d.Date] = d.Events
	}
	has := func(day, kind string, sign int) bool {
		for _, e := range on[day] {
			if e.Kind == kind && ((sign > 0) == (e.Amount > 0)) {
				return true
			}
		}
		return false
	}
	if !has("2026-11-03", "income", 1) || !has("2026-12-03", "income", 1) || !has("2026-11-17", "transfer", -1) || !has("2026-11-05", "bill", -1) {
		t.Errorf("events: 3rd %v 5th %v 17th %v", on["2026-11-03"], on["2026-11-05"], on["2026-11-17"])
	}
	if len(on["2026-11-20"]) != 0 {
		t.Errorf("a cash-to-cash move changed the balance: %v", on["2026-11-20"])
	}
	// The balance falls by the daily allowance on a quiet day.
	for i := 1; i < len(c.Days); i++ {
		if len(c.Days[i].Events) == 0 && c.Days[i-1].Balance-c.Days[i].Balance != c.Daily {
			t.Fatalf("quiet day %s fell by %v, daily %v", c.Days[i].Date, c.Days[i-1].Balance-c.Days[i].Balance, c.Daily)
		}
	}
}

// Freedom comes in levels: covering the €1,500 of fixed obligations needs
// €450,000 at 4% and comes before full independence; €100 more a month
// brings FI sooner; with a birth year, coast has a date before FI.
func TestFIPlanLevelsCoastAndWhatIfs(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	plan.SaveSettings(d, plan.Settings{IncomeMode: "median", WithdrawalRate: 4, ExpectedReturn: 5, EmergencyMonths: 3, BirthYear: 1988, TargetAge: 55})
	b := plan.Budget{Name: "Loan", Kind: "fixed", Categories: []string{"housing.mortgage"}, Amount: E(1500)}
	plan.Save(d, &b, "")
	for m := 1; m <= 12; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-20"
		Tx(t, d, ledger.Tx{Date: day, Amount: E(6000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(1500), Category: "housing.mortgage", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(1500), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Kind: "transfer", Date: day, Amount: E(1500), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	}
	Bal(t, d, "swed", "2026-10-14", 20000)
	Bal(t, d, "ibkr", "2026-10-14", 100000)
	p, err := cfo.BuildFIPlan(d, now)
	if err != nil {
		t.Fatal(err)
	}
	lv := map[string]cfo.FreedomLevel{}
	for _, l := range p.Levels {
		lv[l.Key] = l
	}
	ob, life := lv["obligations"], lv["lifestyle"]
	if ob.Monthly != E(1500) || ob.Capital != E(450000) || ob.Date == "" || life.Date == "" || ob.Months >= life.Months {
		t.Fatalf("levels %+v", p.Levels)
	}
	if p.Coast.Date == "" || p.Coast.Months >= life.Months || p.Coast.Age != 55 {
		t.Errorf("coast %+v (FI in %d months)", p.Coast, life.Months)
	}
	if len(p.WhatIfs) != 3 || p.WhatIfs[0].Sooner <= 0 || p.WhatIfs[2].Sooner <= p.WhatIfs[0].Sooner {
		t.Errorf("what-ifs %+v", p.WhatIfs)
	}
	if len(p.Payouts) != 31 || p.Payouts[0].Age != 38 || p.Payouts[10].Monthly <= p.Payouts[0].Monthly {
		t.Errorf("payouts %+v", p.Payouts[:2])
	}
	if p.SavingsRate <= 0 || p.PassiveNow <= 0 {
		t.Errorf("rate %v passive %v", p.SavingsRate, p.PassiveNow)
	}
}

// An old job's rule (by the 3rd) and a new job's (by the 10th): one salary a
// month in the calendar, not two.
func TestCashFlowCalendarOneSalaryAMonth(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	s := plan.Settings{IncomeMode: "manual", ManualIncome: 5000, WithdrawalRate: 4, ExpectedReturn: 5, EmergencyMonths: 3,
		SalaryRules: []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-10-01", PaidByDay: 10}}}
	plan.SaveSettings(d, s)
	Bal(t, d, "swed", "2026-10-29", 3000)
	c, err := cfo.CashFlowCalendar(d, now, 60)
	if err != nil {
		t.Fatal(err)
	}
	per := map[string]int{}
	for _, day := range c.Days {
		for _, e := range day.Events {
			if e.Kind == "income" {
				per[day.Date[:7]]++
			}
		}
	}
	for m, n := range per {
		if n != 1 {
			t.Errorf("%s has %d salaries", m, n)
		}
	}
	if len(per) < 2 {
		t.Errorf("salaries %v", per)
	}
}

// On the 17th the €1,400 mortgage standing order hasn't shown at the bank
// yet: it still comes, so it lands on the first day — not a month later.
func TestCashFlowCalendarKeepsWhatIsDueOrLate(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	for m := 1; m <= 12; m++ {
		mo := now.AddDate(0, -m, 0).Format("2006-01")
		Tx(t, d, ledger.Tx{Kind: "transfer", Date: mo + "-17", Amount: E(1400), Category: "transfer.debt", AccountID: "swed", ToAccountID: "mortgage"})
	}
	Bal(t, d, "swed", "2026-10-16", 6000)
	Bal(t, d, "mortgage", "2026-10-01", -200000)
	c, err := cfo.CashFlowCalendar(d, now, 40)
	if err != nil {
		t.Fatal(err)
	}
	if c.Days[0].Date != "2026-10-18" || len(c.Days[0].Events) != 1 || c.Days[0].Events[0].Amount != -E(1400) {
		t.Fatalf("first day %s: %+v", c.Days[0].Date, c.Days[0].Events)
	}
	for _, day := range c.Days {
		if day.Date == "2026-11-17" && len(day.Events) == 0 {
			t.Errorf("November's mortgage is missing")
		}
	}
}
