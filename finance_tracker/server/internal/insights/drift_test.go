package insights_test

import (
	"fmt"
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// Pay up 10% in a year while spending is up 30%: creep, and the gym that
// started this year is named as a new commitment.
func TestLifestyleCreepNamesWhatGrew(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 24; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-15"
		recent := m <= 12
		pay, food := 3000.0, 1000.0
		if recent {
			pay, food = 3300, 1300
		}
		Tx(t, d, ledger.Tx{Date: day, Amount: E(pay), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(food), Category: "food.restaurants", AccountID: "swed", Merchant: "Various"})
		if m <= 6 {
			Tx(t, d, ledger.Tx{Date: day, Amount: E(40), Category: "health.fitness", AccountID: "swed", Merchant: "Gym"})
		}
	}
	c, err := insights.LifestyleCreep(d, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Years != 1 || c.IncomeGrowth != 10 || c.Tone != "bad" || c.Gap <= 5 {
		t.Fatalf("%+v", c)
	}
	if len(c.Drivers) == 0 || c.Drivers[0].Category != "food" {
		t.Errorf("drivers %+v", c.Drivers)
	}
	if len(c.New) != 1 || c.New[0].Merchant != "Gym" {
		t.Errorf("new %+v", c.New)
	}
	// Asked for three years with two of history, it compares what there is.
	if c3, _ := insights.LifestyleCreep(d, now, 3); c3.Years != 1 {
		t.Errorf("3 years asked, %d compared", c3.Years)
	}
}

// Insurance renewing in a month cost €300 → €360 last time: flagged early,
// with the rise; the steady phone bill pulls the personal rate down to the
// weighted average of the two.
func TestPriceCreepFlagsRenewalAndWeighsInflation(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	Tx(t, d, ledger.Tx{Date: "2024-11-10", Amount: E(300), Category: "finance.insurance", AccountID: "swed", Merchant: "Insurer"})
	Tx(t, d, ledger.Tx{Date: "2025-11-10", Amount: E(360), Category: "finance.insurance", AccountID: "swed", Merchant: "Insurer"})
	for m := 1; m <= 14; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-05"
		Tx(t, d, ledger.Tx{Date: day, Amount: E(25), Category: "utilities.telecom", AccountID: "swed", Merchant: "Telia"})
	}
	p, err := insights.PriceCreep(d, now)
	if err != nil {
		t.Fatal(err)
	}
	var ins insights.PriceWatch
	for _, it := range p.Items {
		if it.Merchant == "Insurer" {
			ins = it
		}
	}
	if ins.Change != 20 || ins.Extra != E(60) || !ins.RenewsSoon || ins.Advice == "" {
		t.Fatalf("insurer %+v (all %+v)", ins, p.Items)
	}
	if p.Items[0].Merchant != "Insurer" {
		t.Errorf("renewal first: %+v", p.Items)
	}
	// (360 + 300) / (300 + 300) − 1 = 10%.
	if p.Personal == nil || *p.Personal != 10 || p.Basket != 2 {
		t.Errorf("personal %v basket %d", fmt.Sprint(p.Personal), p.Basket)
	}
}

// Telia €41.70 → €14.95 was a cheaper contract, not deflation: it stays on
// the list but out of the personal rate, which follows YouTube's real rise.
func TestPersonalInflationLeavesPlanChangesOut(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 14; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-05"
		yt, tel := 9.99, 41.70
		if m <= 6 {
			yt, tel = 10.99, 14.95
		}
		Tx(t, d, ledger.Tx{Date: day, Amount: E(yt), Category: "subscriptions.media", AccountID: "swed", Merchant: "YouTube"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(tel), Category: "utilities.telecom", AccountID: "swed", Merchant: "Telia"})
	}
	p, err := insights.PriceCreep(d, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range p.Items {
		if it.PlanChange != (it.Merchant == "Telia") {
			t.Errorf("plan change flag on %s: %v", it.Merchant, it.PlanChange)
		}
	}
	if p.Personal == nil || *p.Personal != 10 || p.Basket != 1 || p.Excluded != 1 || len(p.Items) != 2 {
		t.Fatalf("personal %v basket %d excluded %d items %d", fmt.Sprint(*p.Personal), p.Basket, p.Excluded, len(p.Items))
	}
}
