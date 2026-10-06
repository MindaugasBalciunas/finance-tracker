package plan_test

import (
	"math"
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
	r := plan.Compute(budgets, txs, "2026-10", now, plan.Settings{IncomeMode: "manual", ManualIncome: 5000})
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
	r := plan.Compute([]plan.Budget{vac}, txs, "2026-12", now, plan.Settings{IncomeMode: "manual", ManualIncome: 1000})
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
	base, src := plan.IncomeBase(plan.Settings{IncomeMode: "gross", GrossSalary: 8400, MonthlyDeductions: 30}, inc, "2026-10", "2026-10")
	if math.Abs(base.Float()-plan.LTNetSalary(8400, 30)) > 0.01 || src == "" {
		t.Error("gross")
	}
	// Lithuanian net: 8400 gross → Sodra 19.5% + GPM 20% (no NPD at this level) − 30.
	if got := plan.LTNetSalary(8400, 30); math.Abs(got-(8400-1638-1680-30)) > 0.01 {
		t.Errorf("LT net %v", got)
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
	s := plan.Settings{IncomeMode: "gross", GrossSalary: 10000, BirthYear: 1990, TargetAge: 50}
	plan.SaveSettings(d, s)
	if got := plan.LoadSettings(d); got.GrossSalary != 10000 || got.WithdrawalRate != 4 || got.EmergencyMonths != 3 {
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
