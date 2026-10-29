package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// On payday the runbook lists each saving line with where its money usually
// goes, and ticks each off as the bank shows the transfer: €500 to IBKR with
// €200 sent is half done, the €100 pension already paid is done.
func TestRunbookTicksOffTransfersAfterPayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	invest := plan.Budget{Name: "Invest", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(500)}
	pension := plan.Budget{Name: "III pillar", Kind: "saving", Categories: []string{"transfer.pension"}, Amount: E(100)}
	plan.Save(d, &invest, "")
	plan.Save(d, &pension, "")
	for _, m := range []string{"2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-02", Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Kind: "transfer", Date: m + "-04", Amount: E(500), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	}
	// A transfer before this payday belongs to last month's list.
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-09-30", Amount: E(100), Category: "transfer.pension", AccountID: "seb", ToAccountID: "artea"})
	Tx(t, d, ledger.Tx{Date: "2026-10-02", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-03", Amount: E(200), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-05", Amount: E(100), Category: "transfer.pension", AccountID: "seb", ToAccountID: "artea"})

	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "2026-10-02" || rb.Salary != E(3000) || rb.Month != "2026-10" {
		t.Fatalf("payday %+v", rb)
	}
	got := map[string]cfo.RunStep{}
	for _, s := range rb.Steps {
		got[s.Title] = s
	}
	inv, pen := got["Invest"], got["III pillar"]
	if inv.Status != "partial" || inv.Done != E(200) || inv.Amount != E(500) || inv.From != "swed" || inv.To != "ibkr" || inv.ToName != "IBKR" {
		t.Errorf("invest %+v", inv)
	}
	if pen.Status != "done" || pen.Done != E(100) || pen.From != "seb" || pen.To != "artea" {
		t.Errorf("pension %+v", pen)
	}
	if rb.Open != 1 || rb.Steps[0].Title != "Invest" {
		t.Errorf("open steps first, one open: %+v", rb.Steps)
	}
}

// Before the salary arrives nothing can be done yet: every step waits.
func TestRunbookBeforeSalaryWaits(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	b := plan.Budget{Name: "Invest", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(300)}
	plan.Save(d, &b, "")
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-10", Amount: E(300), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "" || len(rb.Steps) != 1 || rb.Steps[0].Status != "todo" || rb.Expected != "2026-11-03" {
		t.Errorf("%+v", rb)
	}
}

// A purchase's impact speaks in the household's own numbers.
func TestDecisionImpactNamesTheWishListAndMonth(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 12; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-20"
		Tx(t, d, ledger.Tx{Date: day, Amount: E(4000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(2000), Category: "food.groceries", AccountID: "swed"})
	}
	Bal(t, d, "swed", "2026-10-14", 9000)
	im, err := cfo.DecisionImpact(d, E(1200), now)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, l := range im.Lines {
		keys[l.Key] = l.Text
	}
	for _, k := range []string{"month", "days", "buffer"} {
		if keys[k] == "" {
			t.Errorf("no %s line in %+v", k, im.Lines)
		}
	}
	if _, err := cfo.DecisionImpact(d, 0, now); err == nil {
		t.Error("a free purchase has no impact to show")
	}
}
