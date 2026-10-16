package cfo_test

import (
	"fmt"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/goals"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// The year ahead is the plan's income and obligations, usual everyday
// spending, a one-off allowance from the past year, and dated goals in
// their month; a scenario changes the inputs from its month on, and the
// comparison says what that does.
func TestCashForecast(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", 3000
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(500)}
	if err := plan.Save(d, &alimony, ""); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-15", Amount: E(500), Category: "kids.alimony", AccountID: "swed"})
		for k := 1; k <= 10; k++ {
			Tx(t, d, ledger.Tx{Date: fmt.Sprintf("%s-%02d", m, k+11), Amount: E(200), Category: "food.groceries", AccountID: "swed"})
		}
	}
	Tx(t, d, ledger.Tx{Date: "2026-05-20", Amount: E(1200), Category: "travel", AccountID: "swed"}) // one one-off in the year
	Tx(t, d, ledger.Tx{Date: "2026-10-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Bal(t, d, "swed", "2026-10-16", 5000)
	bike := goals.Goal{Name: "Bike", Target: E(900), TargetDate: "2027-01-15", Status: "active"}
	if err := goals.Save(d, &bike); err != nil {
		t.Fatal(err)
	}

	c, err := cfo.CompareForecast(d, now, 6, &cfo.Scenario{EverydayPct: -10, From: "2026-11"})
	if err != nil {
		t.Fatal(err)
	}
	f := c.Baseline
	a := f.Assumed
	if f.Cash != E(5000) || a.Income != E(3000) || a.Fixed != E(500) || a.Everyday != E(2000) || a.OneOffs != E(100) {
		t.Fatalf("cash %v, assumed %+v", f.Cash, a)
	}
	if len(f.Months) != 6 || f.Months[0].Month != "2026-10" {
		t.Fatalf("months %+v", f.Months)
	}
	// October: salary is in, alimony still to pay, 15 of 31 days of spending left.
	oct := f.Months[0]
	if oct.Income != 0 || oct.Fixed != E(500) || oct.Everyday != money.FromFloat(2000*15.0/31) {
		t.Errorf("october %+v", oct)
	}
	// A full month: 3000 − 500 − 2000 − 100.
	if f.Months[1].Net != E(400) {
		t.Errorf("november net %v", f.Months[1].Net)
	}
	jan := f.Months[3]
	if jan.Month != "2027-01" || len(jan.Dated) != 1 || jan.Dated[0].Label != "Bike" || jan.Net != E(-500) {
		t.Errorf("january %+v", jan)
	}
	var sum money.Cents
	for _, m := range f.Months {
		sum += m.Net
	}
	if f.Months[5].End != f.Cash+sum {
		t.Errorf("ends don't add up: %v vs %v", f.Months[5].End, f.Cash+sum)
	}
	// Scenario: everyday 10% lower from November → +200 a month for 5 months.
	s := c.Scenario
	if s.Months[0].Net != oct.Net || s.Months[1].Net != E(600) || s.Months[5].End-f.Months[5].End != E(1000) {
		t.Errorf("scenario %v / %v / %v", s.Months[0].Net, s.Months[1].Net, s.Months[5].End-f.Months[5].End)
	}
	if len(c.Summary) == 0 {
		t.Error("no summary")
	}
	// Leaving a goal out removes it.
	c2, _ := cfo.CompareForecast(d, now, 6, &cfo.Scenario{SkipGoals: []int64{bike.ID}})
	if len(c2.Scenario.Months[3].Dated) != 0 {
		t.Errorf("skipped goal still dated: %+v", c2.Scenario.Months[3].Dated)
	}
}
