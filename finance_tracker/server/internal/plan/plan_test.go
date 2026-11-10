package plan_test

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

func tx(date, cat string, eur float64, tags ...string) ledger.Tx {
	kind := "expense"
	switch ledger.Top(cat) {
	case "salary", "refunds", "benefits", "side_income":
		kind = "income"
	case "transfer":
		kind = "transfer"
	}
	return ledger.Tx{Date: date, Kind: kind, Category: cat, Amount: money.FromFloat(eur), Tags: tags}
}

func line(id int64, name, kind string, cats []string, eur float64) plan.Budget {
	return plan.Budget{ID: id, Name: name, Kind: kind, Categories: cats, Period: "monthly", Amount: money.FromFloat(eur)}
}

func find(r *plan.Report, name string) plan.Line {
	for _, l := range r.Lines {
		if l.Name == name {
			return l
		}
	}
	return plan.Line{}
}

func TestAmountForSteps(t *testing.T) {
	b := plan.Budget{Amount: E(500), Steps: []plan.Step{{"", E(400)}, {"2026-10", E(500)}}}
	if b.AmountFor("2026-09") != E(400) || b.AmountFor("2026-10") != E(500) || b.AmountFor("2027-01") != E(500) {
		t.Error("steps")
	}
	b2 := plan.Budget{Steps: []plan.Step{{"2026-05", E(100)}}}
	if b2.AmountFor("2026-01") != E(100) {
		t.Error("before the first dated step: earliest known amount")
	}
	y := plan.Budget{Period: "yearly"}
	if y.MonthlyShare(E(1200)) != E(100) {
		t.Error("yearly share")
	}
}

func TestMatchesByKind(t *testing.T) {
	sp := line(1, "Food", "spending", []string{"food"}, 1)
	if !sp.Matches(&ledger.Tx{Kind: "expense", Category: "food.groceries"}) || sp.Matches(&ledger.Tx{Kind: "transfer", Category: "food"}) {
		t.Error("spending")
	}
	sv := line(2, "ETF", "saving", []string{"transfer.invest"}, 1)
	if !sv.Matches(&ledger.Tx{Kind: "transfer", Category: "transfer.invest"}) {
		t.Error("saving")
	}
	fx := line(3, "Loan", "fixed", []string{"housing.mortgage_interest", "transfer.debt"}, 1)
	if !fx.Matches(&ledger.Tx{Kind: "transfer", Category: "transfer.debt"}) || !fx.Matches(&ledger.Tx{Kind: "expense", Category: "housing.mortgage_interest"}) {
		t.Error("fixed spans expense and transfer")
	}
	tg := plan.Budget{Kind: "spending", Tag: "trip:rome"}
	if !tg.Matches(&ledger.Tx{Kind: "expense", Category: "travel", Tags: []string{"trip:rome"}}) || tg.Matches(&ledger.Tx{Kind: "expense", Category: "travel"}) {
		t.Error("tag lines")
	}
}

func TestComputeFixedNeverCountsAgainstSpending(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	budgets := []plan.Budget{
		line(1, "Alimony", "fixed", []string{"kids.alimony"}, 1000),
		line(2, "Kids", "spending", []string{"kids"}, 200),
		line(3, "ETF", "saving", []string{"transfer.invest"}, 1000),
	}
	txs := []ledger.Tx{
		tx("2026-10-17", "kids.alimony", 1000),
		tx("2026-10-05", "kids.general", 50),
		tx("2026-10-01", "transfer.invest", 1000),
		tx("2026-10-15", "salary", 5000),
		tx("2026-10-10", "shopping.online", 80),
	}
	r := plan.Compute(budgets, txs, "2026-10", now, plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)})
	if k := find(r, "Kids"); k.Spent != E(50) {
		t.Fatalf("alimony leaked into the Kids envelope: %v", k.Spent)
	}
	if a := find(r, "Alimony"); a.Spent != E(1000) || a.Remaining != 0 {
		t.Fatal("fixed line", a.Spent)
	}
	if e := find(r, "ETF"); e.Spent != E(1000) {
		t.Fatal("saving line", e.Spent)
	}
	if r.DiscretionarySpent != E(130) || r.FixedSpent != E(1000) || r.SavedActual != E(1000) || r.IncomeActual != E(5000) {
		t.Fatalf("totals %v %v %v %v", r.DiscretionarySpent, r.FixedSpent, r.SavedActual, r.IncomeActual)
	}
	if len(r.Unbudgeted) != 1 || r.Unbudgeted[0].Category != "shopping" {
		t.Fatalf("unbudgeted %+v", r.Unbudgeted)
	}
	// safe = income − fixed − saving − funds − discretionary outside funds
	if r.SafeToSpend != E(5000-1000-1000-0-130) {
		t.Fatalf("safe %v", r.SafeToSpend)
	}
}

func TestComputeFundCarriesOver(t *testing.T) {
	now := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	vac := line(1, "Vacation", "spending", []string{"travel"}, 300)
	vac.Fund, vac.StartMonth = true, "2026-10"
	txs := []ledger.Tx{tx("2026-11-20", "travel.flights", 200), tx("2026-12-02", "travel.lodging", 500)}
	r := plan.Compute([]plan.Budget{vac}, txs, "2026-12", now, plan.Settings{IncomeMode: "manual", ManualIncome: E(1000)})
	l := find(r, "Vacation")
	// Oct +300, Nov +300 −200, Dec opens at 400, +300, −500 → 200.
	if l.FundState.Opening != E(400) || l.FundState.Available != E(200) || l.Budgeted != E(700) || l.Spent != E(500) {
		t.Fatalf("%+v", l.FundState)
	}
	if r.FundContributions != E(300) || r.FundSpent != E(500) {
		t.Fatal("fund totals")
	}
	// A trip paid from its fund doesn't sink the month.
	if r.SafeToSpend != E(1000-300-(500-500)) {
		t.Fatalf("safe %v", r.SafeToSpend)
	}
}

// A fund that starts next month holds nothing yet: this month it is a plain
// monthly line, so a €901 car bill in October counts against October's left
// to spend instead of vanishing into a November fund (prod, October 2026).
func TestComputeFundBeforeStart(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	set := plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)}
	car := line(1, "Transport", "spending", []string{"transport"}, 80)
	car.Fund, car.StartMonth = true, "2026-11"
	txs := []ledger.Tx{tx("2026-09-12", "transport.car", 600), tx("2026-10-14", "transport.car", 901.37)}
	r := plan.Compute([]plan.Budget{car}, txs, "2026-10", now, set)
	l := find(r, "Transport")
	if l.FundState != nil || l.Budgeted != E(80) || l.Spent != E(901.37) || l.Remaining != E(80-901.37) {
		t.Fatalf("line %+v", l)
	}
	if r.FundContributions != 0 || r.FundSpent != 0 || r.SafeToSpend != E(5000-901.37) {
		t.Fatalf("funds %v/%v, safe %v", r.FundContributions, r.FundSpent, r.SafeToSpend)
	}
	// Earlier months' free spending (the month track) keeps it too.
	if r.FreeSpentByMonth["2026-09"] != E(600) || r.FreeSpentByDay["2026-10"][14] != E(901.37) {
		t.Fatalf("free by month %v", r.FreeSpentByMonth)
	}
	// From November it is a fund: €80 set aside, its spending covered.
	r = plan.Compute([]plan.Budget{car}, append(txs, tx("2026-11-05", "transport.fuel", 50)), "2026-11", now, set)
	if l := find(r, "Transport"); l.FundState == nil || r.FundSpent != E(50) || r.SafeToSpend != E(5000-80) {
		t.Fatalf("november: fund %+v, safe %v", l.FundState, r.SafeToSpend)
	}
}

// A pension payout is not saving: the line and saved go down, not up.
func TestComputeSavingWithdrawal(t *testing.T) {
	now := time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)
	p := line(1, "Pension", "saving", []string{"transfer.pension"}, 200)
	in := tx("2025-12-01", "transfer.pension", 200)
	in.FromKind, in.ToKind = "checking", "pension"
	out := tx("2025-12-02", "transfer.pension", 17197.92)
	out.FromKind, out.ToKind = "pension", "checking"
	r := plan.Compute([]plan.Budget{p}, []ledger.Tx{in, out}, "2025-12", now, plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)})
	if l := find(r, "Pension"); l.Spent != E(200-17197.92) || r.SavedActual != E(200-17197.92) {
		t.Fatalf("line %v, saved %v", l.Spent, r.SavedActual)
	}
}

// A fund pays only what it holds: the rest of a big bill (a €769 car
// service from a transport fund opened this month with €80) comes out of
// this month's free money. An overdraft carried in from earlier months
// already hit those months and isn't counted again.
func TestComputeFundOverdrawn(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	set := plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)}
	car := line(1, "Transport", "spending", []string{"transport"}, 80)
	car.Fund, car.StartMonth = true, "2026-10"
	txs := []ledger.Tx{tx("2026-10-03", "transport.fuel", 58.33), tx("2026-10-14", "transport.car", 769.25)}
	r := plan.Compute([]plan.Budget{car}, txs, "2026-10", now, set)
	if r.FundOverdrawn != E(747.58) || r.SafeToSpend != E(5000-80-747.58) {
		t.Fatalf("overdrawn %v, safe %v", r.FundOverdrawn, r.SafeToSpend)
	}
	// The day chart shows it the day the service bill went past the fund
	// (the €58.33 of fuel on the 3rd fitted in the €80).
	if d := r.FreeSpentByDay["2026-10"]; d[14] != E(747.58) || d[3] != 0 {
		t.Fatalf("overdraft by day: %v", d)
	}
	// Enough saved: nothing extra.
	car.StartMonth = "2025-10" // a year of €80 (€960) held before October
	r = plan.Compute([]plan.Budget{car}, txs, "2026-10", now, set)
	if r.FundOverdrawn != 0 || r.SafeToSpend != E(5000-80) {
		t.Fatalf("covered: overdrawn %v, safe %v", r.FundOverdrawn, r.SafeToSpend)
	}
	// In the red since September: October's €80 share pays €80 of
	// October's €100, the other €20 is October's own overdraft.
	car.StartMonth = "2026-09"
	r = plan.Compute([]plan.Budget{car}, []ledger.Tx{tx("2026-09-10", "transport.car", 500), tx("2026-10-05", "transport.fuel", 100)}, "2026-10", now, set)
	if r.FundOverdrawn != E(20) {
		t.Fatalf("carried overdraft counted again: %v", r.FundOverdrawn)
	}
}

// Money back (an insurance payout for a repair) reduces the month's
// spending, as in cash flow: safe to spend grows by it, income doesn't.
func TestComputeRefundsReduceSpending(t *testing.T) {
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	set := plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)}
	food := line(1, "Food", "spending", []string{"food"}, 500)
	txs := []ledger.Tx{tx("2026-10-02", "housing.maintenance", 300), tx("2026-10-05", "food.groceries", 100), tx("2026-10-14", "refunds", 184.28), tx("2026-10-09", "salary", 3500)}
	r := plan.Compute([]plan.Budget{food}, txs, "2026-10", now, set)
	if r.Refunds != E(184.28) || r.DiscretionarySpent != E(400-184.28) || r.IncomeActual != E(3500) {
		t.Fatalf("refunds %v, discretionary %v, income %v", r.Refunds, r.DiscretionarySpent, r.IncomeActual)
	}
	if r.SafeToSpend != E(5000-400+184.28) {
		t.Fatalf("safe %v", r.SafeToSpend)
	}
	if r.FreeSpentByDay["2026-10"][14] != -E(184.28) {
		t.Errorf("refund day on the timeline: %v", r.FreeSpentByDay["2026-10"])
	}
	// The median income base counts salaries, not money back.
	med := plan.Compute(nil, []ledger.Tx{tx("2026-09-10", "salary", 3000), tx("2026-09-20", "refunds", 900)}, "2026-10", now, plan.Settings{})
	if med.IncomeBase != E(3000) {
		t.Errorf("median base %v", med.IncomeBase)
	}
}

func TestComputeYearlyAndSuggestion(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	gifts := line(1, "Gifts", "spending", []string{"gifts"}, 1200)
	gifts.Period = "yearly"
	food := line(2, "Food", "spending", []string{"food"}, 100)
	var txs []ledger.Tx
	for m := 1; m <= 12; m++ {
		mo := time.Date(2025, time.Month(m), 10, 0, 0, 0, 0, time.UTC).AddDate(0, 6, 0)
		if mo.After(now) {
			break
		}
		txs = append(txs, tx(mo.Format("2006-01-02"), "food.groceries", 400))
	}
	txs = append(txs, tx("2026-03-01", "gifts", 300), tx("2026-06-01", "gifts", 300))
	r := plan.Compute([]plan.Budget{gifts, food}, txs, "2026-07", now, plan.Settings{})
	g := find(r, "Gifts")
	if g.Year == nil || g.Year.Spent != E(600) || g.Remaining != E(600) {
		t.Fatalf("yearly %+v", g.Year)
	}
	f := find(r, "Food")
	if f.Suggestion == nil || f.Suggestion.Amount != E(400) {
		t.Fatalf("suggestion for a line 4× over: %+v", f.Suggestion)
	}
	if r.IncomeBaseSource == "" {
		t.Error("income source")
	}
}

func TestIncomeBaseModes(t *testing.T) {
	inc := map[string]money.Cents{"2026-07": E(5000), "2026-08": E(5100), "2026-09": E(3000)}
	base, _ := plan.IncomeBase(plan.Settings{IncomeMode: "median"}, inc, "2026-10", "2026-10")
	if base != E(5000) {
		t.Error("median of complete months", base)
	}
	base, src := plan.IncomeBase(plan.Settings{IncomeMode: "gross", GrossSalary: E(8400), MonthlyDeductions: E(30)}, inc, "2026-10", "2026-10")
	if base != plan.LTNetSalary(E(8400), E(30)) || src == "" {
		t.Error("gross")
	}
}

// Lithuanian net pay, to the cent, in each NPD band.
func TestLTNetSalary(t *testing.T) {
	for _, c := range []struct{ gross, ded, want money.Cents }{
		{E(8400), E(30), E(8400 - 1638 - 1680 - 30)}, // no NPD left: Sodra 19.5% + GPM 20%, less deductions
		{E(1000), 0, E(754.40)},                      // below MMA: full €747 NPD
		{E(1500), 0, E(1011.62)},                     // NPD tapering: 747 − 0.49×462
		{E(1234.57), 0, E(877.05)},                   // odd cents round once, at the end
		{E(500), 0, E(402.50)},                       // NPD above gross: no GPM
		{0, 0, 0},
	} {
		if got := plan.LTNetSalary(c.gross, c.ded); got != c.want {
			t.Errorf("LTNetSalary(%v, %v) = %v, want %v", c.gross, c.ded, got, c.want)
		}
	}
}

// The cents version gives what the float one it replaced gave, rounded to
// the cent, for every salary — so no figure on Plan or Forecast moves.
func TestLTNetSalaryMatchesFloatVersion(t *testing.T) {
	old := func(gross, deductions float64) float64 {
		const sodraRate, gpmRate, mma, npdMax, npdSlope = 0.195, 0.20, 1038.0, 747.0, 0.49
		npd := npdMax
		if gross > mma {
			npd = max(0, npdMax-npdSlope*(gross-mma))
		}
		return gross - gross*sodraRate - max(0, (gross-npd)*gpmRate) - deductions
	}
	for g := money.Cents(0); g <= E(20000); g += 137 {
		f := old(g.Float(), 25.5) * 100
		if math.Abs(f-math.Floor(f)-0.5) < 1e-6 {
			continue // an exact half cent: float noise picks the side
		}
		if got, want := plan.LTNetSalary(g, E(25.5)), money.FromFloat(f/100); got != want {
			t.Fatalf("gross %v: %v, float version %v", g, got, want)
		}
	}
}

func TestBudgetCRUD(t *testing.T) {
	d := DB(t)
	b := plan.Budget{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(400)}
	if err := plan.Save(d, &b, ""); err != nil {
		t.Fatal(err)
	}
	b.Amount = E(500)
	if err := plan.Save(d, &b, "2026-10"); err != nil {
		t.Fatal(err)
	}
	list, _ := plan.List(d, false)
	if len(list) != 1 || len(list[0].Steps) != 2 || list[0].Amount != E(500) || list[0].AmountFor("2026-09") != E(400) {
		t.Fatalf("%+v", list)
	}
	if err := plan.Save(d, &plan.Budget{Name: "x", Kind: "spending"}, ""); err == nil {
		t.Error("budget without match accepted")
	}
	if err := plan.Save(d, &plan.Budget{Name: "x", Kind: "nope", Tag: "a"}, ""); err == nil {
		t.Error("bad kind accepted")
	}
	s := plan.Settings{IncomeMode: "gross", GrossSalary: E(10000), BirthYear: 1990, TargetAge: 50}
	plan.SaveSettings(d, s)
	if got := plan.LoadSettings(d); got.GrossSalary != E(10000) || got.WithdrawalRate != 4 || got.EmergencyMonths != 3 {
		t.Error(got)
	}
	plan.Delete(d, b.ID)
	if list, _ := plan.List(d, true); len(list) != 0 {
		t.Error("delete")
	}
}

func TestTrips(t *testing.T) {
	recent := time.Now().AddDate(0, -1, 0)
	day := func(n int) string { return recent.AddDate(0, 0, n).Format("2006-01-02") }
	txs := []ledger.Tx{
		tx(day(0), "travel.flights", 300, "trip:rome"),
		tx(day(3), "travel.lodging", 400, "trip:rome"),
		tx(day(4), "food.restaurants", 100, "trip:rome"),
		{Date: day(5), Kind: "income", Category: "refunds", Amount: E(50), Tags: []string{"trip:rome"}},
		tx(day(20), "travel.trip", 40),
		tx(day(21), "travel.trip", 60),
		tx(day(40), "travel.trip", 10), // alone: no suggestion
	}
	budgets := []plan.Budget{{Name: "Rome", Kind: "spending", Tag: "trip:rome", Amount: E(1000)}}
	trips, sugg := plan.Trips(txs, budgets)
	if len(trips) != 1 || trips[0].Total != E(750) || trips[0].Days != 6 || trips[0].Budget == nil {
		t.Fatalf("%+v", trips)
	}
	// Flights booked three months ahead don't stretch the trip.
	pre := append([]ledger.Tx{tx(recent.AddDate(0, -3, 0).Format("2006-01-02"), "travel.flights", 200, "trip:rome")}, txs...)
	trips2, _ := plan.Trips(pre, nil)
	if trips2[0].Days != 6 || trips2[0].BookedFrom == "" || trips2[0].Total != E(950) {
		t.Fatalf("prepaid booking: %+v", trips2[0])
	}
	if len(sugg) != 1 || sugg[0].Count != 2 || sugg[0].Total != E(100) {
		t.Fatalf("suggestions %+v", sugg)
	}
}

// A job change: month-end payroll paid by the 3rd until 11 Oct 2026, then a
// new employer paying once a month by the 10th.
func TestSalaryRulesAcrossAJobChange(t *testing.T) {
	rules := []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-10-12", PaidByDay: 10}}
	sal := func(d string) *ledger.Tx { return &ledger.Tx{Date: d, Kind: "income", Category: "salary"} }
	cases := map[string]string{
		"2026-10-01": "2026-09-30", // old employer, September's final payment
		"2026-10-07": "2026-10-07", // before the new rule: day 7 > 3, stays in October
		"2026-10-15": "2026-10-15", // old employer's advance for October
		"2026-11-01": "2026-10-31", // old employer's final pay for 1–11 October
		"2026-11-07": "2026-10-31", // new employer, first salary (12–31 October)
		"2026-12-09": "2026-11-30",
		"2026-12-12": "2026-12-12", // later than the 10th: counted when it arrived
	}
	for in, want := range cases {
		if got := plan.FlowDate(sal(in), rules); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	if got := plan.FlowDate(&ledger.Tx{Date: "2026-11-05", Kind: "income", Category: "side_income"}, rules); got != "2026-11-05" {
		t.Error("only salary moves")
	}
	if plan.MaxSalaryDays(rules) != 10 {
		t.Error("load window")
	}
	if plan.ValidSalaryRules([]plan.SalaryRule{{From: "x", PaidByDay: 3}}) == nil || plan.ValidSalaryRules([]plan.SalaryRule{{PaidByDay: 31}}) == nil {
		t.Error("validation")
	}
	if len((plan.Settings{}).Salary()) != 1 {
		t.Error("default rule when none saved")
	}
}

// Patch changes only what it is given; an amount change starts this month
// (or from_month) so past months keep what they were planned with.
func TestPatchBudget(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	amt := func(v float64) *float64 { return &v }
	str := func(v string) *string { return &v }
	cats := []string{"food"}
	b, err := plan.Patch(d, plan.BudgetPatch{Name: str("Food"), Kind: str("spending"), Categories: &cats, Amount: amt(500)}, now)
	if err != nil || b.ID == 0 || b.Amount != money.FromFloat(500) {
		t.Fatal(err, b)
	}
	// A rename keeps the amount and its history.
	b, err = plan.Patch(d, plan.BudgetPatch{ID: b.ID, Name: str("Groceries")}, now)
	if err != nil || b.Name != "Groceries" || b.Amount != money.FromFloat(500) || len(b.Steps) != 1 {
		t.Fatal(err, b)
	}
	b, _ = plan.Patch(d, plan.BudgetPatch{ID: b.ID, Amount: amt(400)}, now)
	if b.AmountFor("2026-09") != money.FromFloat(500) || b.AmountFor("2026-10") != money.FromFloat(400) {
		t.Fatalf("this month on: %+v", b.Steps)
	}
	b, _ = plan.Patch(d, plan.BudgetPatch{ID: b.ID, Amount: amt(450), FromMonth: "2026-12"}, now)
	if b.AmountFor("2026-11") != money.FromFloat(400) || b.AmountFor("2026-12") != money.FromFloat(450) {
		t.Fatalf("from month: %+v", b.Steps)
	}
	b, _ = plan.Patch(d, plan.BudgetPatch{ID: b.ID, Amount: amt(420), AllMonths: true}, now)
	if len(b.Steps) != 1 || b.AmountFor("2026-01") != money.FromFloat(420) {
		t.Fatalf("all months: %+v", b.Steps)
	}
	yes := true
	b, _ = plan.Patch(d, plan.BudgetPatch{ID: b.ID, Archived: &yes}, now)
	if !b.Archived || b.Amount != money.FromFloat(420) {
		t.Fatal("archived, amount kept", b)
	}
	bad := []string{"nope"}
	for _, p := range []plan.BudgetPatch{{ID: 999, Name: str("x")}, {Name: str("x"), Kind: str("spending"), Categories: &cats}, {ID: b.ID, Categories: &bad}, {ID: b.ID, FromMonth: "10/2026", Amount: amt(1)}} {
		if _, err := plan.Patch(d, p, now); err == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}

func TestPatchSettings(t *testing.T) {
	d := DB(t)
	if _, err := plan.PatchSettings(d, []byte(`{"gross_salary":10000,"buffers":{"swed":300,"seb":100}}`)); err != nil {
		t.Fatal(err)
	}
	st, err := plan.PatchSettings(d, []byte(`{"salary_rules":[{"from":"","paid_by_day":3},{"from":"2026-11-01","paid_by_day":10}],"buffers":{"seb":0}}`))
	if err != nil || st.GrossSalary != E(10000) || len(st.SalaryRules) != 2 || st.Buffers["swed"] != E(300) || len(st.Buffers) != 1 {
		t.Fatalf("%v %+v", err, st)
	}
	for _, bad := range []string{`{"income_mode":"x"}`, `{"buffers":{"nope":1}}`, `{"salary_account":"nope"}`, `{"emergency_months":"x"}`, `{"manual_income":-1}`, `{"gross_salary":1e17}`, `{"monthly_deductions":2e6}`} {
		if _, err := plan.PatchSettings(d, []byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if plan.LoadSettings(d).GrossSalary != E(10000) {
		t.Error("a rejected patch changed nothing")
	}
}

// Settings saved while the money fields were float euros load into cents
// unchanged, and save back as the same numbers, so neither the stored row
// nor what the web reads moves. (Every value saved so far has whole cents;
// a stray third decimal would round to the cent.)
func TestOldSettingsJSONRoundTrip(t *testing.T) {
	d := DB(t)
	old := `{"income_mode":"gross","manual_income":0,"gross_salary":8400,"gross_changes":[{"from":"2026-11","gross":10000.5}],"monthly_deductions":37.5,"target_age":45,"birth_year":1990,"fi_monthly_spend":2150.55,"withdrawal_rate":8,"expected_return":9,"emergency_months":3,"salary_rules":[{"from":"","paid_by_day":3},{"from":"2026-11-01","paid_by_day":10}],"salary_account":"swed","buffers":{"swed":300.5,"seb":0.1}}`
	if _, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('plan',?)`, old); err != nil {
		t.Fatal(err)
	}
	st := plan.LoadSettings(d)
	if st.GrossSalary != E(8400) || st.GrossChanges[0].Gross != E(10000.5) || st.MonthlyDeductions != E(37.5) || st.FIMonthlySpend != E(2150.55) ||
		st.ManualIncome != 0 || st.Buffers["swed"] != E(300.5) || st.Buffers["seb"] != E(0.1) || st.WithdrawalRate != 8 || st.ExpectedReturn != 9 || st.EmergencyMonths != 3 {
		t.Fatalf("loaded %+v", st)
	}
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	var saved string
	d.QueryRow(`SELECT value FROM settings WHERE key='plan'`).Scan(&saved)
	// Same keys, same numbers: only the formatting of whole euros differs (8400 → 8400.00).
	var a, b any
	json.Unmarshal([]byte(old), &a)
	json.Unmarshal([]byte(saved), &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("stored settings changed:\nold %s\nnew %s", old, saved)
	}
	if plan.LoadSettings(d).GrossSalary != E(8400) {
		t.Error("reload")
	}
}

// A planned raise counts from its month: the months before keep the old
// salary (the past and this month aren't rewritten).
func TestGrossChangesByMonth(t *testing.T) {
	s := plan.Settings{IncomeMode: "gross", GrossSalary: E(8400), GrossChanges: []plan.GrossChange{{From: "2026-12", Gross: E(10000)}, {From: "2027-06", Gross: E(11000)}}}
	for m, want := range map[string]money.Cents{"2026-04": E(8400), "2026-10": E(8400), "2026-12": E(10000), "2027-03": E(10000), "2027-06": E(11000)} {
		if g := s.GrossFor(m); g != want {
			t.Errorf("%s: %v want %v", m, g, want)
		}
	}
	oct, _ := plan.IncomeBase(s, nil, "2026-10", "2026-10")
	dec, _ := plan.IncomeBase(s, nil, "2026-12", "2026-10")
	if oct != plan.LTNetSalary(E(8400), E(0)) || dec != plan.LTNetSalary(E(10000), E(0)) {
		t.Errorf("income base oct %v dec %v", oct, dec)
	}
}

// A finished month is measured against the salary that came in, so a new
// gross salary setting doesn't rewrite it — but only once its pay window has
// closed: a salary paid by the 10th of the next month (the rule from
// November 2026) still belongs to the month before.
func TestFinishedMonthUsesActualSalary(t *testing.T) {
	s := plan.Settings{IncomeMode: "gross", GrossSalary: E(10000), SalaryRules: []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}}
	setting := plan.LTNetSalary(E(10000), E(0))
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }
	txs := []ledger.Tx{
		tx("2026-10-02", "salary", 5000),          // September's, paid on the 2nd
		tx("2026-09-20", "side_income.rent", 300), // not salary: gross mode leaves it out
		tx("2026-10-30", "salary.final_pay", 800), // October: a final payment from the old job…
	}
	sep := plan.Compute(nil, txs, "2026-09", at(2026, 11, 5), s)
	if sep.IncomeBase != E(5000) || sep.IncomeBaseSource != "salary received for that month" {
		t.Errorf("September: actual salary, not the new setting: %v (%s)", sep.IncomeBase, sep.IncomeBaseSource)
	}
	// …so on 5 November October has some salary, but its main pay can still
	// come until the 10th: the setting stands in.
	oct := plan.Compute(nil, txs, "2026-10", at(2026, 11, 5), s)
	if oct.IncomeBase != setting {
		t.Errorf("October inside its pay window: %v want %v", oct.IncomeBase, setting)
	}
	// Paid on 7 November: October's. After the 10th October uses what came in.
	txs = append(txs, tx("2026-11-07", "salary", 5100))
	oct = plan.Compute(nil, txs, "2026-10", at(2026, 11, 11), s)
	if oct.IncomeBase != E(5900) {
		t.Errorf("October after its window: %v (%s)", oct.IncomeBase, oct.IncomeBaseSource)
	}
	// November is in progress: the setting, and the 7 Nov pay isn't November's.
	nov := plan.Compute(nil, txs, "2026-11", at(2026, 11, 12), s)
	if nov.IncomeBase != setting || nov.IncomeActual != 0 {
		t.Errorf("November: %v, actual %v", nov.IncomeBase, nov.IncomeActual)
	}
	// Paid on the 15th: past the rule's day, so it is November's own.
	late := append(txs[:2:2], tx("2026-11-15", "salary", 5100))
	oct = plan.Compute(nil, late, "2026-10", at(2026, 11, 20), s)
	if oct.IncomeBase != setting {
		t.Errorf("a salary on the 15th is not October's: %v", oct.IncomeBase)
	}
	// A fixed amount stands for all income; history mode keeps its median.
	man := plan.Compute(nil, txs, "2026-09", at(2026, 11, 20), plan.Settings{IncomeMode: "manual", ManualIncome: E(9000), SalaryRules: s.SalaryRules})
	if man.IncomeBase != E(5300) {
		t.Errorf("manual: September's income %v", man.IncomeBase)
	}
	med := plan.Compute(nil, txs, "2026-09", at(2026, 11, 20), plan.Settings{IncomeMode: "median", SalaryRules: s.SalaryRules})
	if med.IncomeBaseSource != "median of the last 12 complete months" {
		t.Errorf("median mode unchanged: %s", med.IncomeBaseSource)
	}
}

// A month's salary may arrive until the paid-by day of the next month under
// the rule in force then; with no early pay counted back it is due by the
// month's own last day. Settled is the day after.
func TestSalaryDueAndSettled(t *testing.T) {
	rules := []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	for month, want := range map[string]string{"2026-09": "2026-10-03", "2026-10": "2026-11-10", "2027-02": "2027-03-10"} {
		if got := plan.SalaryDue(month, rules); got != want {
			t.Errorf("%s due %s want %s", month, got, want)
		}
	}
	if got := plan.SalaryDue("2026-02", []plan.SalaryRule{{PaidByDay: 0}}); got != "2026-02-28" {
		t.Errorf("no early pay: due %s", got)
	}
	at := func(d string) time.Time { x, _ := time.Parse("2006-01-02", d); return x }
	if plan.SalarySettled("2026-10", rules, at("2026-11-10")) || !plan.SalarySettled("2026-10", rules, at("2026-11-11")) {
		t.Error("October settles on 11 November")
	}
	if !plan.SalarySettled("2026-02", []plan.SalaryRule{{PaidByDay: 0}}, at("2026-03-01")) {
		t.Error("no early pay: settled on the 1st")
	}
}

// October: a loan line matches the interest (an expense) and the principal
// (transfer.debt, which is saving too). Fixed spent keeps both, as the line
// shows; FixedExpense leaves the principal to saving, so the month's income
// splits into obligations + saved + spending + left over without counting
// the €438.95 twice.
func TestFixedExpenseLeavesPrincipalToSaving(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	budgets := []plan.Budget{
		line(1, "Loan payments", "fixed", []string{"housing.mortgage_interest", "transfer.debt"}, 1361.66),
		line(2, "Alimony", "fixed", []string{"kids.alimony"}, 1000),
	}
	principal := tx("2026-10-17", "transfer", 438.95)
	principal.Category, principal.AccountID, principal.ToAccountID = "transfer.debt", "seb", "mortgage"
	txs := []ledger.Tx{
		tx("2026-10-19", "housing.mortgage_interest", 922.71), principal,
		tx("2026-10-17", "kids.alimony", 1000),
		tx("2026-10-09", "salary", 7884.92),
		tx("2026-10-14", "transport.car", 2077.87),
	}
	r := plan.Compute(budgets, txs, "2026-10", now, plan.Settings{IncomeMode: "manual", ManualIncome: E(5000)})
	if r.FixedSpent != E(2361.66) || r.FixedExpense != E(1922.71) || r.SavedActual != E(438.95) || r.DiscretionarySpent != E(2077.87) {
		t.Fatalf("fixed %v expense %v saved %v spending %v", r.FixedSpent, r.FixedExpense, r.SavedActual, r.DiscretionarySpent)
	}
	if left := r.IncomeActual - r.FixedExpense - r.SavedActual - r.DiscretionarySpent; left != E(3445.39) {
		t.Errorf("left over %v", left)
	}
}
