package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/decisions"
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

// A step paid before the salary is in still counts: money moved this month
// is money moved, and the list says the salary is still to come.
func TestRunbookBeforeSalaryCountsWhatIsPaid(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	b := plan.Budget{Name: "Invest", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(300)}
	plan.Save(d, &b, "")
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-10", Amount: E(300), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "" || len(rb.Steps) != 1 || rb.Steps[0].Status != "done" || rb.Expected != "2026-11-03" {
		t.Errorf("%+v", rb)
	}
}

// The owner's October: salary in two parts (1 Oct, 9 Oct) and the €1,000 ETF
// top-up on 2 Oct, between them. Counting only from the latest part missed
// it; payday is the first part and the whole month counts.
func TestRunbookSalaryInPartsKeepsEarlyTransfers(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	etf := plan.Budget{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(1000)}
	plan.Save(d, &etf, "")
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(3107.18), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-02", Amount: E(1000), Category: "transfer.invest", AccountID: "cash", ToAccountID: "ibkr"})
	Tx(t, d, ledger.Tx{Date: "2026-10-09", Amount: E(3505.38), Category: "salary", AccountID: "swed"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Payday != "2026-10-01" || rb.Salary != E(3107.18+3505.38) {
		t.Errorf("payday %s salary %v", rb.Payday, rb.Salary)
	}
	if len(rb.Steps) != 1 || rb.Steps[0].Status != "done" || rb.Steps[0].Done != E(1000) || rb.Open != 0 {
		t.Errorf("ETF step %+v", rb.Steps)
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

// Selling €500 of ETF back to Swedbank in the month is money out, not a
// payday step done; a broker → pension rebalance moves nothing either.
func TestRunbookSaleBackOutIsNotSaving(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	etf := plan.Budget{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(500)}
	plan.Save(d, &etf, "")
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-09-02", Amount: E(500), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-10-05", Amount: E(500), Category: "transfer.invest", AccountID: "ibkr", ToAccountID: "swed"})
	rb, err := cfo.BuildRunbook(d, now)
	if err != nil {
		t.Fatal(err)
	}
	s := rb.Steps[0]
	if s.Status != "todo" || s.Done != 0 || s.From != "swed" || s.To != "ibkr" {
		t.Errorf("a sale counted as saving: %+v", s)
	}
}

// Home's bell: a decision whose cooling-off is over asks for a call, a bought
// one past its review date asks whether it was worth it, and payday money
// still unmoved a week after the salary is a warning.
func TestHomeRemindsOfDecisionsAndPayday(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	b := plan.Budget{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(1000)}
	plan.Save(d, &b, "")
	Tx(t, d, ledger.Tx{Date: "2026-10-02", Amount: E(3000), Category: "salary", AccountID: "swed"})
	cool := decisions.Decision{Title: "Kayak", Amount: E(900)}
	decisions.Create(d, &cool, now.AddDate(0, 0, -10)) // a week's cooling-off, now over
	bought := decisions.Decision{Title: "Bike", Amount: E(400)}
	decisions.Create(d, &bought, now.AddDate(0, -3, 0))
	bought.Status = "bought"
	decisions.Update(d, &bought, now.AddDate(0, -3, 0)) // review 60 days on: past

	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]cfo.Action{}
	for _, a := range o.Actions {
		kinds[a.Kind] = a
	}
	if a := kinds["decide"]; a.Link != "/plan/wishlist" || a.Text == "" {
		t.Errorf("no cooling-off reminder: %+v", o.Actions)
	}
	if a := kinds["worth"]; a.Link != "/plan/wishlist" {
		t.Errorf("no worth-it question: %+v", o.Actions)
	}
	if a := kinds["payday"]; a.Level != "warn" || a.Link != "/plan" {
		t.Errorf("payday a week on: %+v", a)
	}
}
