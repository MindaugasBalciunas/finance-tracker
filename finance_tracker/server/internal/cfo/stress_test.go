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
	crash := scenario(t, r, "market_crash")
	// Pay goes on: cash follows the plan (which dips under the target in
	// October on its own), and the cost is the drop alone.
	if crash.Level != "tight" || crash.CashOut != "" || crash.Months[5].End != crash.Months[5].Base || !strings.HasSuffix(crash.Summary[0], "the plan dips there too.") || crash.MarketHit != E(4000+1300+5000) || crash.Cost != crash.MarketHit {
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
