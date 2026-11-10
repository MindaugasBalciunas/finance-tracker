package cfo_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

// stressHousehold: €3,000 pay, €500 alimony, €2,000 of groceries a month,
// €5,000 cash, €10,000 invested, €2,000 in crypto, a pension and a house.
func stressHousehold(t *testing.T) (*sql.DB, time.Time) {
	t.Helper()
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", E(3000)
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
	Tx(t, d, ledger.Tx{Date: "2026-10-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Bal(t, d, "swed", "2026-10-16", 5000)
	Bal(t, d, "ibkr", "2026-10-16", 10000)
	Bal(t, d, "btc_m", "2026-10-16", 2000)
	Bal(t, d, "artea", "2026-10-16", 20000)
	Bal(t, d, "house", "2026-10-16", 100000)
	return d, now
}

func scenario(t *testing.T, r *cfo.StressReport, id string) cfo.StressResult {
	t.Helper()
	for _, s := range r.Scenarios {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no %s scenario", id)
	return cfo.StressResult{}
}

// A lost job is played out on the forecast: cash goes first, then the
// investments and crypto are sold — in that order, only what is needed —
// and pay coming back stops the selling. The II pillar and the house are
// never touched.
func TestJobLossSellsInvestmentsOnlyOnceCashIsGone(t *testing.T) {
	h, now := stressHousehold(t)
	r, err := cfo.StressTest(h, now, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Cash != E(5000) || r.Locked != E(20000) || r.RealAsset != E(100000) || len(r.Reserves) != 2 {
		t.Fatalf("report %+v", r)
	}
	// A month without pay: €500 alimony + €2,000 everyday.
	if r.Runway.Burn != E(2500) || r.Runway.CashMonths != 2 || r.Runway.ReserveMonths != 6.8 {
		t.Fatalf("runway %+v", r.Runway)
	}
	j := scenario(t, r, "job_loss")
	// October: the pay is in; alimony and 15 of 31 days of spending to come.
	// November on cash; December cash runs out and the selling starts.
	oct := E(5000) - E(500) - money.FromFloat(2000*15.0/31)
	if j.Months[0].End != oct || j.Months[1].End != oct-E(2500) {
		t.Fatalf("first months %+v", j.Months[:2])
	}
	if j.Level != "sell" || j.CashOut != "2026-12" || j.Broke != "" || j.Short != 0 {
		t.Fatalf("job loss %s cash out %s broke %s short %v", j.Level, j.CashOut, j.Broke, j.Short)
	}
	need := E(2500)*6 - (oct - 0) // six months of burn, less what October left
	if j.SoldTotal != need || len(j.Sold) != 2 || j.Sold[0].Key != "investments" || j.Sold[0].Amount != E(10000) || j.Sold[1].Key != "crypto" {
		t.Fatalf("sold %v %+v (need %v)", j.SoldTotal, j.Sold, need)
	}
	// Pay back in May: €500 a month again, nothing more sold.
	if m := j.Months[7]; m.Month != "2027-05" || m.Sold != 0 || m.End != E(500) {
		t.Fatalf("May %+v", m)
	}
	// The cost is the lost pay: six months of €3,000.
	if j.Cost != E(18000) {
		t.Fatalf("cost %v", j.Cost)
	}
}

// A recession that outlasts the reserves says when they run out and how
// much is still missing; the market drop counts against net worth.
func TestDeepRecessionRunsShortAndCountsTheDrop(t *testing.T) {
	h, now := stressHousehold(t)
	r, err := cfo.StressTest(h, now, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	x := scenario(t, r, "recession")
	// −35% stocks, −60% crypto, −20% pension, −15% house.
	if x.MarketHit != E(3500+1200+4000+15000) {
		t.Fatalf("market hit %v", x.MarketHit)
	}
	if x.Level != "short" || x.Broke == "" || x.Short <= 0 || x.SoldTotal != E(6500+800) {
		t.Fatalf("recession %+v", x)
	}
	// Everything sold was sold after the drop: €6,500 of stocks worth
	// €10,000 before and €800 of crypto worth €2,000 make €4,700 of it real;
	// the pension and house drops stay on paper.
	if x.LockedIn != E(3500+1200) || x.Paper != E(4000+15000) {
		t.Fatalf("locked in %v, paper %v", x.LockedIn, x.Paper)
	}
	crash := scenario(t, r, "market_crash")
	// Pay goes on: cash follows the plan (which dips under the target in
	// October on its own), and the cost is the drop alone.
	if crash.Level != "tight" || crash.CashOut != "" || crash.Months[5].End != crash.Months[5].Base || !strings.HasSuffix(crash.Summary[0], "the plan dips there too.") || crash.MarketHit != E(4000+1300+5000) || crash.Cost != 0 || crash.Paper != crash.MarketHit {
		t.Fatalf("crash %+v", crash)
	}
}

// A rate rise is paid from the loan's next reset, not before, and only on a
// variable loan.
func TestRateRiseStartsAtTheReset(t *testing.T) {
	h, now := stressHousehold(t)
	a, err := ledger.GetAccount(h, "mortgage")
	if err != nil {
		t.Fatal(err)
	}
	var det ledger.LoanDetails
	json.Unmarshal(a.Details, &det)
	det.RateResetDate = "2027-03-01"
	a.Details, _ = json.Marshal(det)
	if _, err := ledger.SaveAccount(h, a); err != nil {
		t.Fatal(err)
	}
	Bal(t, h, "mortgage", "2026-10-16", -200000)
	// A second variable loan with no reset date: its rise starts next month,
	// whatever the mortgage's reset says.
	if _, err := ledger.SaveAccount(h, ledger.Account{ID: "car_loan", Name: "Car loan", Kind: "loan", Details: json.RawMessage(`{"base_rate":3,"margin":1,"monthly_payment":300}`)}); err != nil {
		t.Fatal(err)
	}
	Bal(t, h, "car_loan", "2026-10-16", -10000)
	r, err := cfo.StressTest(h, now, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	x := scenario(t, r, "rates")
	car := x.Months[0].End - x.Months[1].End - (x.Months[0].Base - x.Months[1].Base) // the car loan's rise, from November
	if car <= 0 || car > E(30) {
		t.Fatalf("car loan rise %v", car)
	}
	mortgage := x.LoanExtra - car
	if mortgage < E(250) || mortgage > E(450) {
		t.Fatalf("€200k at 3.95%% → 6.95%%: extra %v", mortgage)
	}
	for i, m := range x.Months {
		want := m.Base
		if i >= 1 {
			want -= car * money.Cents(i)
		}
		if m.Month >= "2027-03" {
			want -= mortgage * money.Cents(i-4)
		}
		if m.End != want {
			t.Fatalf("%s: %v, want %v", m.Month, m.End, want)
		}
	}
}

// The owner's own crisis runs beside the ready-made ones; one out of range
// is refused in words.
func TestCustomShock(t *testing.T) {
	h, now := stressHousehold(t)
	r, err := cfo.StressTest(h, now, 24, &cfo.Shock{OneOff: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if r.Custom == nil || r.Custom.Months[1].End != r.Custom.Months[1].Base-E(1000) || r.Custom.Months[0].End != r.Custom.Months[0].Base {
		t.Fatalf("custom %+v", r.Custom)
	}
	for _, bad := range []cfo.Shock{{IncomePct: -150}, {Months: 40}, {StocksPct: -101}, {OneOff: -5}} {
		if _, err := cfo.StressTest(h, now, 24, &bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

// Goals put off in a crisis are bought when it ends, so they still count
// against the money; the III pillar is sold last and loses its tax.
func TestPutOffGoalsAreBoughtWhenTheCrisisEnds(t *testing.T) {
	h, now := stressHousehold(t)
	if _, err := ledger.SaveAccount(h, ledger.Account{ID: "p3", Name: "Third pillar", Kind: "pension"}); err != nil {
		t.Fatal(err)
	}
	Bal(t, h, "p3", "2026-10-16", 10000)
	sofa := goals.Goal{Name: "Sofa", Target: E(4000), TargetDate: "2027-01-15", Status: "active"}
	if err := goals.Save(h, &sofa); err != nil {
		t.Fatal(err)
	}
	r, err := cfo.StressTest(h, now, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Reserves) != 3 || r.Reserves[2].Key != "pillar3" || r.Reserves[2].Net != E(8500) {
		t.Fatalf("reserves %+v", r.Reserves)
	}
	j := scenario(t, r, "job_loss")
	// January without the sofa; May, the first month with pay, buys it.
	if j.Months[3].Month != "2027-01" || j.Months[3].Out != E(2500) || j.Months[7].Out != E(6500) {
		t.Fatalf("Jan %+v, May %+v", j.Months[3], j.Months[7])
	}
	if j.Level != "sell" || len(j.Sold) != 3 || j.Sold[2].Key != "pillar3" {
		t.Fatalf("%s sold %+v", j.Level, j.Sold)
	}
	p3 := j.Sold[2]
	if p3.Net != money.FromFloat(p3.Amount.Float()*0.85) && p3.Net != money.FromFloat(p3.Amount.Float()*0.85)+1 {
		t.Fatalf("III pillar sold %v for %v", p3.Amount, p3.Net)
	}
	// The cost: six months' pay and the tax lost on the III pillar — the
	// sofa is bought either way.
	if j.Cost != E(18000)+p3.Amount-p3.Net {
		t.Fatalf("cost %v", j.Cost)
	}
}

// Pay coming back refills cash after months of owing money: the shortfall
// is the deepest it went, not where it ends. A benefit on its own counts.
func TestShortfallIsTheDeepestPoint(t *testing.T) {
	h, now := stressHousehold(t)
	r, err := cfo.StressTest(h, now, 24, &cfo.Shock{Months: 9, IncomePct: -100, StocksPct: -100, CryptoPct: -100})
	if err != nil {
		t.Fatal(err)
	}
	c := r.Custom
	last := c.Months[len(c.Months)-1].End
	if c.Level != "short" || c.Short != -c.Lowest || last <= c.Lowest || !strings.HasSuffix(c.Summary[0], "short at worst.") {
		t.Fatalf("short %v lowest %v end %v %q", c.Short, c.Lowest, last, c.Summary)
	}
	if len(c.Levers) != 1 || c.Levers[0].Amount != E(400) || !c.Levers[0].Monthly {
		t.Fatalf("levers %+v", c.Levers)
	}
	r, err = cfo.StressTest(h, now, 24, &cfo.Shock{Months: 2, Benefit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if m := r.Custom.Months; m[1].Income != E(3500) || m[3].Income != E(3000) {
		t.Fatalf("benefit %+v", m[:4])
	}
}

// With nothing to go by there is no runway to show, rather than "0 months".
func TestNoRunwayWithoutAUsualMonth(t *testing.T) {
	r, err := cfo.StressTest(DB(t), time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Runway != nil {
		t.Fatalf("runway %+v", r.Runway)
	}
}

// Every ready-made crisis is in a group the page knows, with a unique id
// and a shock the custom builder would accept; the new ones behave as
// their names say.
func TestPresetsAreGroupedAndSane(t *testing.T) {
	groups := map[string]bool{}
	for _, g := range cfo.StressGroups {
		groups[g.ID] = true
	}
	seen := map[string]bool{}
	for _, p := range cfo.StressPresets {
		if !groups[p.Group] || seen[p.ID] || p.Name == "" || p.Blurb == "" {
			t.Errorf("preset %+v", p)
		}
		seen[p.ID] = true
		if err := p.Shock.Validate(24); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
	}
	h, now := stressHousehold(t)
	r, err := cfo.StressTest(h, now, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Groups) != 4 || len(r.Scenarios) != len(cfo.StressPresets) {
		t.Fatalf("%d groups, %d scenarios", len(r.Groups), len(r.Scenarios))
	}
	// A slump takes a quarter of the house; a crypto winter 80% of the coins
	// and 15% of the stocks; cash follows the plan in both.
	if x := scenario(t, r, "housing_slump"); x.MarketHit != E(25000) || x.Group != "markets" {
		t.Fatalf("housing slump %+v", x)
	}
	if x := scenario(t, r, "crypto_winter"); x.MarketHit != E(1600+1500) || x.Months[5].End != x.Months[5].Base {
		t.Fatalf("crypto winter %+v", x)
	}
	// A pay cut costs exactly the pay not received: 12 months of €600.
	if x := scenario(t, r, "pay_cut"); x.Cost != E(7200) || x.CashOut != "" {
		t.Fatalf("pay cut cost %v, cash out %s", x.Cost, x.CashOut)
	}
	// A longer search is worse than a shorter one, and everything at once
	// worse than either.
	job, long, storm := scenario(t, r, "job_loss"), scenario(t, r, "long_search"), scenario(t, r, "perfect_storm")
	if !(job.Cost < long.Cost && long.Cost < storm.Cost) || storm.Level != "short" {
		t.Fatalf("costs %v < %v < %v, storm %s", job.Cost, long.Cost, storm.Cost, storm.Level)
	}
}

// Short of cash, the plan stops investing before it sells anything: a bill
// that investing less can cover sells nothing, and a bigger one sells only
// what pausing doesn't cover. Investing less costs no net worth.
func TestInvestingStopsBeforeAnythingIsSold(t *testing.T) {
	h, now := stressHousehold(t)
	etf := plan.Budget{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(400)}
	if err := plan.Save(h, &etf, ""); err != nil {
		t.Fatal(err)
	}
	r, err := cfo.StressTest(h, now, 24, &cfo.Shock{OneOff: 3500})
	if err != nil {
		t.Fatal(err)
	}
	if r.Runway == nil || r.Base[1]-r.Base[0] != E(100) {
		t.Fatalf("plan: €3,000 in, €2,500 out, €400 invested → +€100 a month; base %v", r.Base[:3])
	}
	c := r.Custom
	// November: €100 left after investing, the bill takes €3,500 → €3,132
	// of October's cash covers most; investing less covers the rest.
	if c.Level != "pause" || c.PausedFrom != "2026-11" || c.SoldTotal != 0 || c.CashOut != "" || c.Months[1].End != 0 || c.Months[1].Out != c.Months[1].Income+r.Base[0] {
		t.Fatalf("custom %+v", c)
	}
	if c.SavingCut != E(3500)-r.Base[1] || c.Cost != E(3500) || !strings.Contains(c.Summary[0], "nothing has to be sold") {
		t.Fatalf("cut %v cost %v %q", c.SavingCut, c.Cost, c.Summary)
	}
	big := scenario(t, r, "big_bill")
	if big.Level != "sell" || big.SavingCut != E(400) || big.SoldTotal != E(10000)-r.Base[1]-E(400) || big.Cost != E(10000) {
		t.Fatalf("big bill %+v", big)
	}
	// Investing goes on in the plan: pausing it from the start is offered.
	if len(big.Levers) == 0 || big.Levers[0].Label != "Pause investing from the start" || big.Levers[0].Amount != E(400) {
		t.Fatalf("levers %+v", big.Levers)
	}
	// "Worst month" is the month as its row shows it, after the cut.
	if m := c.Months[1]; c.MonthlyGap != m.Income-m.Out {
		t.Fatalf("worst month %v, row %v − %v", c.MonthlyGap, m.Income, m.Out)
	}
	// The €3,750 target is 3 months of essentials but 1.5 of a €2,500 month.
	if r.TargetMonths != 3 || r.FloorMonths != 1.5 {
		t.Fatalf("floor %v months of %v", r.FloorMonths, r.TargetMonths)
	}
	// A gain on what is still held doesn't hide a loss made real by selling:
	// stocks up, crypto down and sold, a year without pay.
	r, err = cfo.StressTest(h, now, 24, &cfo.Shock{Months: 12, IncomePct: -100, StocksPct: 50, CryptoPct: -80})
	if err != nil {
		t.Fatal(err)
	}
	if c := r.Custom; c.LockedIn <= 0 || c.Paper != 0 || c.Cost < E(36000)+c.LockedIn {
		t.Fatalf("locked %v paper %v cost %v", c.LockedIn, c.Paper, c.Cost)
	}
}
