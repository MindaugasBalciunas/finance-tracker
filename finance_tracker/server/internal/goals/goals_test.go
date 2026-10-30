package goals_test

import (
	"testing"
	"time"

	"ft/internal/decisions"
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

// Each dated goal says plainly whether it makes its date, counting the goals
// ahead of it: on track, late (and what a month would fix it), or never
// when nothing is left over.
func TestGoalOutlook(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	for m := 4; m <= 9; m++ {
		day := time.Date(2026, time.Month(m), 15, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		Tx(t, d, ledger.Tx{Date: day, Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(4000), Category: "food.groceries", AccountID: "swed"})
	}
	printer := goals.Goal{Name: "Printer", Target: E(700)}
	sauna := goals.Goal{Name: "Sauna", Target: E(6000), TargetDate: "2027-06-01"}
	trip := goals.Goal{Name: "Trip", Target: E(3000), TargetDate: "2026-12-01"}
	for _, g := range []*goals.Goal{&printer, &sauna, &trip} {
		goals.Save(d, g)
	}
	p, _ := goals.Build(d, "", now)
	byName := map[string]goals.Goal{}
	for _, g := range p.Goals {
		byName[g.Name] = g
	}
	// €1,000 a month left over. Sauna: 6,700 with the printer over 8 months = 838 → on track.
	if o := byName["Sauna"].Outlook; o == nil || o.Tone != "good" {
		t.Errorf("sauna %+v", o)
	}
	// Trip: 9,700 with both ahead over 2 months = 4,850 → €3,850 a month short.
	if o := byName["Trip"].Outlook; o == nil || o.Tone != "warn" || o.Gap != E(3850) {
		t.Errorf("trip %+v", o)
	}
	if byName["Printer"].Outlook != nil {
		t.Error("no date, money left over: the ETA says it")
	}

	// Nothing left over: every unfunded goal says so.
	d2 := DB(t)
	for m := 4; m <= 9; m++ {
		day := time.Date(2026, time.Month(m), 15, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		Tx(t, d2, ledger.Tx{Date: day, Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d2, ledger.Tx{Date: day, Amount: E(3200), Category: "food.groceries", AccountID: "swed"})
	}
	gun := goals.Goal{Name: "Licence", Target: E(1500), TargetDate: "2027-03-01"}
	goals.Save(d2, &gun)
	p, _ = goals.Build(d2, "", now)
	if o := p.Goals[0].Outlook; o == nil || o.Tone != "bad" {
		t.Errorf("licence %+v", o)
	}
}

// A goal without a date can just hold money set aside in a savings account:
// never more than the account has free, the account shows what is spoken
// for, and a bought goal stops holding it.
func TestMoneySetAsideInSavings(t *testing.T) {
	d := DB(t)
	if _, err := ledger.SaveAccount(d, ledger.Account{ID: "swed_sav", Name: "Swedbank savings", Kind: "savings", Liquid: true}); err != nil {
		t.Fatal(err)
	}
	Bal(t, d, "swed_sav", "2026-10-01", 3500)
	sauna := goals.Goal{Name: "Sauna", Target: E(6000)} // no date
	bike := goals.Goal{Name: "Bike", Target: E(900)}
	for _, g := range []*goals.Goal{&sauna, &bike} {
		if err := goals.Save(d, g); err != nil {
			t.Fatal(err)
		}
	}
	put := func(g int64, eur float64, acct string) error {
		return goals.AddMove(d, &goals.Move{GoalID: g, Month: "2026-10", Amount: E(eur), AccountID: acct})
	}
	if err := put(sauna.ID, 3000, "swed_sav"); err != nil {
		t.Fatal(err)
	}
	if err := put(bike.ID, 600, "swed_sav"); err == nil {
		t.Error("only €500 of the account is free")
	}
	if err := put(bike.ID, 500, "swed_sav"); err != nil {
		t.Fatal(err)
	}
	if err := put(bike.ID, 100, "ibkr"); err == nil {
		t.Error("a broker isn't where wish-list money is set aside")
	}
	if err := put(sauna.ID, -3500, "swed_sav"); err == nil {
		t.Error("can't take out more than the goal holds there")
	}
	// A current account with nothing set aside yet is still limited to its balance.
	Bal(t, d, "seb", "2026-10-01", 100)
	if err := put(bike.ID, 500, "seb"); err == nil {
		t.Error("SEB holds €100")
	}
	// Taking out without naming an account can't drain money set aside in one.
	if err := put(sauna.ID, -100, ""); err == nil {
		t.Error("all of the sauna's money is in Swedbank savings — release it there")
	}
	p, err := goals.Build(d, "", time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pots) != 1 || p.Pots[0].SetAside != E(3500) || p.Pots[0].Free != 0 {
		t.Errorf("pots %+v", p.Pots)
	}
	g, _ := goals.Get(d, sauna.ID)
	if g.Saved != E(3000) || len(g.Held) != 1 || g.Held[0].Name != "Swedbank savings" || g.Held[0].Amount != E(3000) {
		t.Errorf("sauna %+v", g)
	}
	// Bought: its money is spent, the account is free again.
	bike.Status = "bought"
	if err := goals.Save(d, &bike); err != nil {
		t.Fatal(err)
	}
	p, _ = goals.Build(d, "", time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	if p.Pots[0].SetAside != E(3000) || p.Pots[0].Free != E(500) {
		t.Errorf("after buying %+v", p.Pots)
	}
}

// A €2,400 sofa can't be paid for now: "Save for it" puts it on the wish
// list, and the journal follows the goal — bought there is bought here
// (asking "worth it?" 60 days on), dropped is let go, deleted is back to
// thinking about it.
func TestDecisionFollowsItsWishListGoal(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	x := decisions.Decision{Title: "Sofa", Amount: E(2400), Note: "the old one sags"}
	if err := decisions.Create(d, &x, now); err != nil {
		t.Fatal(err)
	}
	g, err := goals.FromDecision(d, x.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Sofa" || g.Target != E(2400) || g.Note != "the old one sags" || g.Status != "active" {
		t.Fatalf("goal %+v", g)
	}
	got, _ := decisions.Get(d, x.ID)
	if got.Status != "saving" || got.GoalID != g.ID {
		t.Fatalf("decision %+v", got)
	}
	if _, err := goals.FromDecision(d, x.ID, now); err == nil {
		t.Error("saved for twice")
	}
	got.Status = "bought" // decided in the journal while the goal is open: refused
	if err := decisions.Update(d, &got, now); err == nil {
		t.Error("bought in the journal while the wish list still saves for it")
	}

	g.Status, g.DoneOn = "bought", "2026-12-01"
	if err := goals.Save(d, &g); err != nil {
		t.Fatal(err)
	}
	got, _ = decisions.Get(d, x.ID)
	if got.Status != "bought" || got.DecidedOn != "2026-12-01" || got.ReviewOn != "2027-01-30" {
		t.Errorf("after buying %+v", got)
	}
	got.Worth = "yes"
	if err := decisions.Update(d, &got, now.AddDate(0, 3, 0)); err != nil {
		t.Errorf("rating a wish-list purchase: %v", err)
	}

	y := decisions.Decision{Title: "Kayak", Amount: E(900)}
	decisions.Create(d, &y, now)
	k, _ := goals.FromDecision(d, y.ID, now)
	k.Status, k.DoneOn = "dropped", "2026-11-15"
	goals.Save(d, &k)
	if got, _ := decisions.Get(d, y.ID); got.Status != "skipped" {
		t.Errorf("dropped goal → let go, got %+v", got)
	}

	z := decisions.Decision{Title: "Drone", Amount: E(700)}
	decisions.Create(d, &z, now)
	dr, _ := goals.FromDecision(d, z.ID, now)
	if err := goals.Delete(d, dr.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := decisions.Get(d, z.ID); got.Status != "thinking" || got.GoalID != 0 {
		t.Errorf("deleted goal → thinking again, got %+v", got)
	}
}
