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

func TestBuildOverview(t *testing.T) {
	d := DB(t)
	now := time.Now()
	for i := 0; i < 13; i++ {
		day := now.AddDate(0, -i, 0)
		ds := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		Tx(t, d, ledger.Tx{Date: ds, Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: ds, Amount: E(1000), Category: "food.groceries", AccountID: "swed"})
	}
	Bal(t, d, "swed", now.AddDate(-1, 0, 0).Format("2006-01-02"), 1000)
	Bal(t, d, "swed", now.Format("2006-01-02"), 5000)
	Bal(t, d, "house", "2022-01-01", 300000)
	Bal(t, d, "mortgage", now.AddDate(0, 0, -400).Format("2006-01-02"), -200000)
	b := plan.Budget{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(500)}
	plan.Save(d, &b, "")
	quiet := plan.Budget{Name: "Gifts", Kind: "spending", Categories: []string{"gifts"}, Amount: E(100)}
	plan.Save(d, &quiet, "")
	o, err := cfo.BuildOverview(d, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	if o.NetWorth != E(5000+300000-200000) || o.Debt != E(200000) || o.InboxOpen != 3 {
		t.Fatalf("%+v", o)
	}
	if o.NetWorth12m != E(4000) {
		t.Errorf("12-month change %v", o.NetWorth12m)
	}
	// Liquid view: only cash moved, so its change matches; the house is out.
	if o.Liquid != E(5000) || o.Liquid12m != E(4000) || o.Spark[len(o.Spark)-1].Liquid != E(5000) {
		t.Errorf("liquid %v / 12m %v / spark %+v", o.Liquid, o.Liquid12m, o.Spark[len(o.Spark)-1])
	}
	if o.Avg12.Income != E(5000) || o.Avg12.Spending != E(1000) || o.Avg12.SavingsRate != 0.8 {
		t.Errorf("trailing average %+v", o.Avg12)
	}
	if o.Month.Spending != E(1000) || len(o.Plan.Over) != 1 || o.Plan.Over[0].Name != "Food" {
		t.Errorf("month + plan pulse %+v %+v", o.Month, o.Plan)
	}
	// Busiest budgets first: Food (200% used) before Gifts (0%).
	if len(o.Plan.Lines) != 2 || o.Plan.Lines[0].Name != "Food" || o.Plan.Lines[1].Name != "Gifts" {
		t.Errorf("plan lines %+v", o.Plan.Lines)
	}
	// The breakdown adds up to left-to-spend, and the unpaid obligation is due
	// on the day it was paid last month (unless that day has passed).
	// An obligation paid on the 28th last month, not yet this month.
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(300)}
	plan.Save(d, &alimony, "")
	prev := time.Date(now.Year(), now.Month()-1, 28, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	Tx(t, d, ledger.Tx{Date: prev, Amount: E(300), Category: "kids.alimony", AccountID: "swed"})
	o, _ = cfo.BuildOverview(d, now, 3)
	p := o.Plan
	if p.IncomeBase-p.FixedPlanned-p.SavingPlanned-p.FundSetAside-p.FreeSpent != p.SafeToSpend || p.FixedPlanned != E(300) {
		t.Errorf("breakdown %+v", p)
	}
	wantDue := ""
	if now.Day() <= 28 {
		wantDue = time.Date(now.Year(), now.Month(), 28, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if len(p.Fixed) != 1 || p.Fixed[0].Name != "Alimony" || p.Fixed[0].Spent != 0 || p.Fixed[0].Due != wantDue {
		t.Errorf("fixed %+v, want due %q", p.Fixed, wantDue)
	}
	// The month timeline: every day of the month, actuals only up to today,
	// adding up to "spent so far"; the due obligation is an event.
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var daySum money.Cents
	for _, d := range p.Days {
		daySum += d.Spent
		if (d.Cum != nil) != (d.Day <= now.Day()) {
			t.Errorf("day %d cum %v", d.Day, d.Cum)
		}
	}
	if len(p.Days) != dim || daySum != p.FreeSpent || p.Days[dim-1].Typical <= 0 {
		t.Errorf("timeline %d days, sum %v vs free %v, typical %v", len(p.Days), daySum, p.FreeSpent, p.Days[dim-1].Typical)
	}
	found := wantDue == ""
	for _, e := range p.Events {
		if e.Kind == "fixed" && e.Label == "Alimony" && !e.Done && e.Day == 28 {
			found = true
		}
	}
	if !found {
		t.Errorf("events %+v", p.Events)
	}
	if len(o.Spark) < 24 || len(o.Recent) == 0 || o.Emergency.Months <= 0 {
		t.Errorf("spark %d recent %d emergency %+v", len(o.Spark), len(o.Recent), o.Emergency)
	}
	if len(o.Stale) != 1 { // the mortgage value is 400 days old
		t.Errorf("stale %+v", o.Stale)
	}
}
