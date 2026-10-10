package goals_test

import (
	"testing"
	"time"

	"ft/internal/goals"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// Six months of: salary 5,000; pay yourself first 1,000 to IBKR and 200 to
// the pension; obligations and spending 2,800 — 1,000 left for the wish list.
func TestWishListWaterfall(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	for m := 4; m <= 9; m++ {
		day := time.Date(2026, time.Month(m), 15, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		Tx(t, d, ledger.Tx{Date: day, Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(1000), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(200), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(2800), Category: "food.groceries", AccountID: "swed"})
	}
	printer := goals.Goal{Name: "Bambu X2D", Target: E(700)}
	sauna := goals.Goal{Name: "Sauna", Target: E(6000), TargetDate: "2027-06-01"}
	safe := goals.Goal{Name: "Gun safe", Target: E(400)}
	for _, g := range []*goals.Goal{&printer, &sauna, &safe} {
		if err := goals.Save(d, g); err != nil {
			t.Fatal(err)
		}
	}
	if printer.Priority != 1 || sauna.Priority != 2 || safe.Priority != 3 {
		t.Fatalf("appended in order: %d %d %d", printer.Priority, sauna.Priority, safe.Priority)
	}
	p, err := goals.Build(d, "", now)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Funding
	if f.Month != "2026-09" || !f.Complete || f.Income != E(5000) || f.Invested != E(1200) || f.Spending != E(2800) || f.LeftOver != E(1000) || f.Available != E(1000) {
		t.Fatalf("september after paying yourself first: %+v", f)
	}
	// Filled in priority order: the printer whole, the rest to the sauna.
	if len(p.Proposal) != 2 || p.Proposal[0].Amount != E(700) || p.Proposal[1].GoalID != sauna.ID || p.Proposal[1].Amount != E(300) {
		t.Fatalf("waterfall: %+v", p.Proposal)
	}
	if p.AvgLeftOver != E(1000) || p.Goals[0].ETA != "2026-11" || p.Goals[1].ETA != "2027-05" || p.Goals[2].ETA != "2027-06" {
		t.Fatalf("ETAs at €1,000/month: %v %s %s %s", p.AvgLeftOver, p.Goals[0].ETA, p.Goals[1].ETA, p.Goals[2].ETA)
	}
	if p.Goals[1].PerMonth != E(750) { // €6,000 over the 8 months to June
		t.Errorf("per month for the target date: %v", p.Goals[1].PerMonth)
	}

	// Safe first now: the split follows the new order.
	if err := goals.Reorder(d, []int64{safe.ID, printer.ID}); err != nil {
		t.Fatal(err)
	}
	p, _ = goals.Build(d, "", now)
	if p.Proposal[0].GoalID != safe.ID || p.Proposal[0].Amount != E(400) || p.Proposal[1].Amount != E(600) {
		t.Fatalf("after reorder: %+v", p.Proposal)
	}
	// Funding: never more than left over, nor than a goal needs; not an open month.
	if _, err := goals.Fund(d, "2026-09", []goals.Allocation{{GoalID: safe.ID, Amount: E(500)}}, now); err == nil {
		t.Error("over-funded a goal")
	}
	if _, err := goals.Fund(d, "2026-09", []goals.Allocation{{GoalID: safe.ID, Amount: E(400)}, {GoalID: sauna.ID, Amount: E(700)}}, now); err == nil {
		t.Error("funded more than the month left over")
	}
	if _, err := goals.Fund(d, "2026-10", []goals.Allocation{{GoalID: safe.ID, Amount: E(10)}}, now); err == nil {
		t.Error("funded a month still running")
	}
	p, err = goals.Fund(d, "2026-09", p.Proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Funding.Funded != E(1000) || p.Funding.Available != 0 || len(p.Proposal) != 0 {
		t.Fatalf("funded: %+v %+v", p.Funding, p.Proposal)
	}
	byID := map[int64]goals.Goal{}
	for _, g := range p.Goals {
		byID[g.ID] = g
	}
	if byID[safe.ID].Saved != E(400) || byID[safe.ID].Remaining != 0 || byID[printer.ID].Saved != E(600) || p.TotalSaved != E(1000) {
		t.Fatalf("saved: %+v", byID)
	}
	// Taking money out: allowed down to €0, never below.
	if err := goals.AddMove(d, &goals.Move{GoalID: printer.ID, Month: "2026-10", Amount: -E(700)}); err == nil {
		t.Error("took out more than saved")
	}
	if err := goals.AddMove(d, &goals.Move{GoalID: printer.ID, Month: "2026-10", Amount: -E(100), Note: "dentist"}); err != nil {
		t.Fatal(err)
	}
	// Bought: leaves the active list and the totals, keeps its history.
	safeG, _ := goals.Get(d, safe.ID)
	safeG.Status = "bought"
	if err := goals.Save(d, &safeG); err != nil || safeG.DoneOn == "" {
		t.Fatal(err, safeG)
	}
	p, _ = goals.Build(d, "", now)
	if p.TotalSaved != E(500) || p.Goals[len(p.Goals)-1].ID != safe.ID {
		t.Fatalf("after buying the safe: saved %v, last %+v", p.TotalSaved, p.Goals[len(p.Goals)-1])
	}
	for _, bad := range []goals.Goal{{Name: "", Target: E(1)}, {Name: "x", Target: -E(1)}, {Name: "x", Target: E(1), TargetDate: "06/2027"}, {Name: "x", Target: E(1), URL: "javascript:alert(1)"}} {
		if err := goals.Save(d, &bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// A planned trip is a goal with a trip tag: funded like any goal, with the
// spending tagged to it (flights booked early) counted as spent.
func TestTripGoal(t *testing.T) {
	d := DB(t)
	rome := goals.Goal{Name: "Rome with the boys", Target: E(2000), Tag: "Trip:Rome 2027"}
	if err := goals.Save(d, &rome); err != nil {
		t.Fatal(err)
	}
	if rome.Tag != "trip:rome-2027" {
		t.Fatalf("tag normalised: %q", rome.Tag)
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-05", Amount: E(480), Category: "travel.flights", AccountID: "swed", Tags: []string{"trip:rome-2027", "kids"}})
	Tx(t, d, ledger.Tx{Date: "2026-10-06", Amount: E(30), Category: "refunds", AccountID: "swed", Tags: []string{"trip:rome-2027"}})
	Tx(t, d, ledger.Tx{Date: "2026-10-06", Amount: E(99), Category: "food.groceries", AccountID: "swed"})
	goals.AddMove(d, &goals.Move{GoalID: rome.ID, Month: "2026-10", Amount: E(600)})
	g, _ := goals.Get(d, rome.ID)
	if g.Spent != E(450) || g.Saved != E(600) || g.Remaining != E(1400) {
		t.Fatalf("trip goal: spent %v saved %v remaining %v", g.Spent, g.Saved, g.Remaining)
	}
	bad := goals.Goal{Name: "x", Target: E(1), Tag: "trip:"}
	if err := goals.Save(d, &bad); err == nil {
		t.Error("empty trip name accepted")
	}
}

// A goal can go on the list before its price is known: listed in order,
// never funded or forecast until it has one.
func TestGoalWithoutPrice(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	Tx(t, d, ledger.Tx{Date: "2026-09-15", Amount: E(1000), Category: "salary", AccountID: "swed"})
	sauna := goals.Goal{Name: "Sauna"}
	safe := goals.Goal{Name: "Gun safe", Target: E(450)}
	goals.Save(d, &sauna)
	goals.Save(d, &safe)
	p, err := goals.Build(d, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Proposal) != 1 || p.Proposal[0].GoalID != safe.ID || p.Goals[0].ETA != "" || p.TotalNeeded != E(450) {
		t.Fatalf("unpriced goal skipped: %+v %+v", p.Proposal, p.Goals[0])
	}
	if _, err := goals.Fund(d, "2026-09", []goals.Allocation{{GoalID: sauna.ID, Amount: E(10)}}, now); err == nil {
		t.Error("funded a goal without a price")
	}
}
