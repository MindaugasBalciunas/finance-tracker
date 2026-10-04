package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
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
	if len(o.Spark) < 24 || len(o.Recent) == 0 || o.Emergency.Months <= 0 {
		t.Errorf("spark %d recent %d emergency %+v", len(o.Spark), len(o.Recent), o.Emergency)
	}
	if len(o.Stale) != 1 { // the mortgage value is 400 days old
		t.Errorf("stale %+v", o.Stale)
	}
}
