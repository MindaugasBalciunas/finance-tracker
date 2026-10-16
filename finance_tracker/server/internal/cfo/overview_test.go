package cfo_test

import (
	"fmt"
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
		for k := 0; k < 5; k++ { // everyday shopping, not one €1,000 one-off
			Tx(t, d, ledger.Tx{Date: ds, Amount: E(200), Category: "food.groceries", AccountID: "swed"})
		}
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

// Home tests the plan against how recent months really ended: six complete
// months of day-to-day spending above what the plan leaves all end over. A
// fund that paid more than it held is named, with what it held.
func TestTrackRecordAndFundsOver(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for _, m := range []string{"2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-12", Amount: E(3500), Category: "food.groceries", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-05", Amount: E(300), Category: "transport.car", AccountID: "swed"})
	transport := plan.Budget{Name: "Transport", Kind: "spending", Categories: []string{"transport"}, Amount: E(50), Fund: true, StartMonth: "2026-10"}
	if err := plan.Save(d, &transport, ""); err != nil {
		t.Fatal(err)
	}
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	p := o.Plan
	if len(p.Track) != 6 || p.Track[0].Month != "2026-04" || p.Track[5].Month != "2026-09" {
		t.Fatalf("track %+v", p.Track)
	}
	for _, m := range p.Track {
		if m.Left != E(-500) || m.Spent != E(3500) {
			t.Errorf("%s: left %v spent %v", m.Month, m.Left, m.Spent)
		}
	}
	// A usual month: each of these spent €500 more than came in.
	if o.UsualSaved != E(-500) || o.UsualRate > -0.16 || o.UsualRate < -0.17 {
		t.Errorf("usual month %v / %.3f", o.UsualSaved, o.UsualRate)
	}
	if p.FreePlanned != E(2950) {
		t.Errorf("free planned %v", p.FreePlanned)
	}
	if len(p.FundsOver) != 1 || p.FundsOver[0].Name != "Transport" || p.FundsOver[0].Budgeted != E(50) || p.FundsOver[0].Remaining != E(-250) {
		t.Errorf("funds over %+v", p.FundsOver)
	}
	if p.Days[len(p.Days)-1].Proj == nil || p.ExpectedRest <= 0 || p.ProjectedLeft != p.SafeToSpend-p.ExpectedRest {
		t.Errorf("forecast rest %v left %v", p.ExpectedRest, p.ProjectedLeft)
	}
}

// Spending still waiting in the bank inbox counts on the day it happened —
// the line and left to spend move when you spend, not when you review. Rows
// flagged as duplicates or own-account moves, and obligations the plan
// already counts in full, are left out.
func TestUnreviewedSpendingCountsToday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for _, m := range []string{"2026-07", "2026-08", "2026-09", "2026-10"} {
		Tx(t, d, ledger.Tx{Date: m + "-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-05", Amount: E(100), Category: "food.groceries", AccountID: "swed"})
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(500)}
	if err := plan.Save(d, &alimony, ""); err != nil {
		t.Fatal(err)
	}
	before, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range []struct {
		date, kind, cat, verdict, state string
		amount                          float64
	}{
		{"2026-10-15", "expense", "food.restaurants", "new", "open", 40},     // counts
		{"2026-10-16", "expense", "", "needs_review", "open", 12.5},          // counts, no category yet
		{"2026-10-16", "expense", "food.groceries", "duplicate", "open", 99}, // already in the ledger
		{"2026-10-16", "expense", "kids.alimony", "new", "open", 500},        // the obligation is planned in full
		{"2026-10-14", "expense", "food.groceries", "new", "imported", 77},   // already imported
		{"2026-09-30", "expense", "food.groceries", "new", "open", 66},       // last month
		{"2026-10-16", "transfer", "transfer.internal", "internal", "open", 1000},
	} {
		if _, err := d.Exec(`INSERT INTO bank_inbox(external_id,date,kind,amount,category,verdict,state,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			fmt.Sprint("x", i), r.date, r.kind, E(r.amount), r.cat, r.verdict, r.state, "2026-10-16T00:00:00Z", "2026-10-16T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	p := o.Plan
	if p.Unreviewed != E(52.5) || p.UnreviewedCount != 2 {
		t.Fatalf("unreviewed %v ×%d", p.Unreviewed, p.UnreviewedCount)
	}
	if p.SafeToSpend != before.Plan.SafeToSpend-E(52.5) || p.FreeSpent != before.Plan.FreeSpent+E(52.5) {
		t.Errorf("safe %v (was %v), free %v (was %v)", p.SafeToSpend, before.Plan.SafeToSpend, p.FreeSpent, before.Plan.FreeSpent)
	}
	if p.Days[14].Spent != E(40) || p.Days[15].Spent != E(12.5) || *p.Days[15].Cum != E(152.5) {
		t.Errorf("days 15–16: %v %v cum %v", p.Days[14].Spent, p.Days[15].Spent, *p.Days[15].Cum)
	}	// Plan shows the same left-to-spend as Home.
	if r, err := cfo.BudgetReport(d, "2026-10", now); err != nil || r.SafeToSpend != p.SafeToSpend || r.Unreviewed != E(52.5) {
		t.Errorf("plan %v / unreviewed %v vs home %v", r.SafeToSpend, r.Unreviewed, p.SafeToSpend)
	}
}
