package cfo_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/goals"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// The owner on 7 Nov: bank money plus €6,000 of notes in the Cash account,
// the €1,000 alimony still to leave before payday (10 Dec), and October's
// €3,000 left over that the wish list would take. The emergency fund is what
// an emergency could use: the bank and the Cash account less what leaves
// before payday and the wish list's set-asides. The payday checklist offers
// the wish list only what is above three months of essentials — and with
// the Cash account left out of the fund, says to keep it for the fund.
func TestEmergencyFundComesBeforeTheWishList(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	if _, err := ledger.SaveAccount(d, ledger.Account{ID: "sav", Name: "Savings", Institution: "Swedbank", Kind: "savings", Liquid: true}); err != nil {
		t.Fatal(err)
	}
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", E(5000)
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	plan.SaveSettings(d, st)
	alimony := plan.Budget{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(1000)}
	plan.Save(d, &alimony, "")
	for m := 1; m <= 12; m++ {
		ym := now.AddDate(0, -m, 0).Format("2006-01")
		Tx(t, d, ledger.Tx{Date: ym + "-17", Amount: E(1000), Category: "kids.alimony", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: ym + "-08", Amount: E(800), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: ym + "-20", Kind: "income", Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: ym + "-10", Amount: E(30), Category: "subscriptions", AccountID: "swed", Merchant: "Telia"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Kind: "income", Amount: E(5000), Category: "salary", AccountID: "swed"})
	g := goals.Goal{Name: "Sauna", Target: E(10000), Status: "active"}
	if err := goals.Save(d, &g); err != nil {
		t.Fatal(err)
	}
	Bal(t, d, "swed", "2026-11-07", 1500)
	Bal(t, d, "sav", "2026-11-07", 4000)
	Bal(t, d, "cash", "2026-11-07", 6000)

	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	e := o.EmergencyPlan
	if e == nil || e.Bank != E(5500) || e.Cash != E(6000) || !e.CountsCash || e.Payday != "2026-12-10" || e.SetAside != 0 {
		t.Fatalf("plan %+v", e)
	}
	// Leaving before payday is exactly what the cash plan's balances drop by
	// — the €30 phone bill is inside the day-to-day allowance, not on top.
	var drop money.Cents
	within := false
	for _, a := range o.Cash.Accounts {
		drop += a.Balance - a.End
		for _, it := range a.Items {
			within = within || it.Within
		}
	}
	if !within || e.Committed != drop || e.Committed < E(1000) || e.Available != e.Bank+e.Cash-e.Committed || e.Target != o.Emergency.Target || e.Monthly <= 0 {
		t.Errorf("committed %v, balances drop %v (a bill within the allowance: %v), available %v, target %v (overview %v)", e.Committed, drop, within, e.Available, e.Target, o.Emergency.Target)
	}
	fiBefore := o.FIProgress
	if e.Gap != max(0, e.Target-e.Available) || e.Spare != max(0, e.Available-e.Target) {
		t.Errorf("gap %v spare %v", e.Gap, e.Spare)
	}

	step := func(rb *cfo.Runbook, kind string) *cfo.RunStep {
		for i := range rb.Steps {
			if rb.Steps[i].Kind == kind {
				return &rb.Steps[i]
			}
		}
		return nil
	}
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if w := step(rb, "wishlist"); e.Spare > 0 && (w == nil || w.Amount > e.Spare) {
		t.Errorf("wish list %+v with %v above the floor", w, e.Spare)
	}

	// Notes and coins out: the bank alone is short of three months, so the
	// leftovers stay for the fund and the wish list waits.
	st.EmergencyExcludeCash = true
	plan.SaveSettings(d, st)
	o, _ = cfo.BuildOverview(d, now, 0)
	e = o.EmergencyPlan
	if e.CountsCash || e.Available != e.Bank-e.Committed || e.Gap <= 0 || e.Spare != 0 || o.Emergency.Cash != E(5500) {
		t.Fatalf("without cash %+v (overview cash %v)", e, o.Emergency.Cash)
	}
	if o.FIProgress != fiBefore {
		t.Errorf("leaving notes out of the fund moved FI progress %v → %v", fiBefore, o.FIProgress)
	}
	rb, _ = cfo.BuildRunbook(d, now)
	keep, wish := step(rb, "emergency"), step(rb, "wishlist")
	if keep == nil || keep.Amount <= 0 || keep.Amount > e.Gap || keep.Status != "info" || !strings.Contains(keep.Detail, "3 months of essentials") {
		t.Errorf("keep step %+v (gap %v)", keep, e.Gap)
	}
	// It is advice, not a transfer: not in what is still to move.
	var total money.Cents
	open := 0
	for _, s := range rb.Steps {
		if s.Kind != "emergency" {
			total += s.Amount
			if s.Status != "done" {
				open++
			}
		}
	}
	if rb.Total != total || rb.Open != open {
		t.Errorf("totals %v/%d, steps without the keep step %v/%d", rb.Total, rb.Open, total, open)
	}
	if wish != nil {
		t.Errorf("wish list offered %+v while the fund is short", wish)
	}
}
