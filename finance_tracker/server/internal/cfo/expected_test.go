package cfo_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

func expectedFlows(t *testing.T, d *sql.DB, now time.Time) map[string]insights.Flow {
	t.Helper()
	txs, err := ledger.All(d, ledger.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	cats, _ := ledger.CategoryMap(d)
	fs := insights.CashFlow(txs, cats, "month", plan.LoadSettings(d).Salary())
	if err := cfo.ExpectSalary(d, fs, now); err != nil {
		t.Fatal(err)
	}
	out := map[string]insights.Flow{}
	for _, f := range fs {
		out[f.Period] = f
	}
	return out
}

func rateOf(f insights.Flow) float64 {
	if f.ExpectedSavingsRate == nil {
		return -99
	}
	return *f.ExpectedSavingsRate
}

// The owner's plan (gross salary, a new job paying by the 10th): on 5 Nov
// November had €100 of gifts in and €257 spent — −157% — with its salary
// due by 10 Dec. It is rated with the net salary the plan expects; the
// gifts come on top. October, paid on 5 Nov, keeps its real numbers, and a
// month whose pay window closed without pay keeps its gap.
func TestSalaryStillDueIsCountedInTheSavingsRate(t *testing.T) {
	d := DB(t)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.GrossSalary = "gross", 8400
	st.GrossChanges = []plan.GrossChange{{From: "2026-11", Gross: 10000}}
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	netNov := money.FromFloat(plan.LTNetSalary(10000, 0))
	netOct := money.FromFloat(plan.LTNetSalary(8400, 0))
	Tx(t, d, ledger.Tx{Date: "2026-09-12", Amount: E(300), Category: "food.groceries", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-12", Amount: E(400), Category: "food.groceries", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-02", Kind: "income", Amount: E(100), Category: "other_income", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-03", Amount: E(257), Category: "food.groceries", AccountID: "swed"})

	// 3 Nov: October's pay may still come (by the 10th), November's too.
	got := expectedFlows(t, d, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	if oct := got["2026-10"]; oct.ExpectedSalary != netOct {
		t.Errorf("October before its pay: expected %v, want %v", oct.ExpectedSalary, netOct)
	}
	nov := got["2026-11"]
	in := (E(100) + netNov).Float()
	if want := float64(int((in-257)/in*1000+0.5)) / 1000; nov.ExpectedSalary != netNov || rateOf(nov) != want || nov.Income != E(100) || nov.SavingsRate != -1.57 {
		t.Errorf("November: expected %v rate %v (want %v %v), actual income %v rate %v", nov.ExpectedSalary, rateOf(nov), netNov, want, nov.Income, nov.SavingsRate)
	}
	if sep := got["2026-09"]; sep.ExpectedSalary != 0 || sep.ExpectedSavingsRate != nil {
		t.Errorf("September, window long closed: %+v", sep)
	}

	// 5 Nov: October's pay is in — its real numbers stand.
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Kind: "income", Amount: E(5200), Category: "salary", AccountID: "swed"})
	got = expectedFlows(t, d, time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC))
	if oct := got["2026-10"]; oct.ExpectedSalary != 0 || oct.ExpectedSavingsRate != nil || oct.Income != E(5200) {
		t.Errorf("October once paid: %+v", oct)
	}

	// An advance on 15 Nov leaves the rest due, so the bar stays an estimate.
	Tx(t, d, ledger.Tx{Date: "2026-11-15", Kind: "income", Amount: E(2000), Category: "salary", AccountID: "swed"})
	if nov := expectedFlows(t, d, time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC))["2026-11"]; nov.ExpectedSalary != netNov-E(2000) {
		t.Errorf("after an advance: still due %v, want %v", nov.ExpectedSalary, netNov-E(2000))
	}
}

// A fixed plan income models all income: €2,000 of other income before
// payday counts against it, not on top of it.
func TestFixedIncomePlanDoesNotCountOtherIncomeTwice(t *testing.T) {
	d := DB(t)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.ManualIncome = "manual", 6000
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 10}}
	plan.SaveSettings(d, st)
	Tx(t, d, ledger.Tx{Date: "2026-11-02", Kind: "income", Amount: E(2000), Category: "other_income", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-03", Amount: E(600), Category: "food.groceries", AccountID: "swed"})
	nov := expectedFlows(t, d, time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC))["2026-11"]
	if nov.ExpectedSalary != E(4000) || rateOf(nov) != 0.9 { // (6,000 − 600) / 6,000
		t.Errorf("still due %v rate %v, want €4,000 and 0.9", nov.ExpectedSalary, rateOf(nov))
	}
}

// Month review of November on the 5th: not "You spent €157 more than you
// earned" — the salary due by 10 Dec is counted, and the review says so.
func TestMonthReviewCountsPayStillToCome(t *testing.T) {
	d := DB(t)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.GrossSalary = "gross", 10000
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}, {From: "2026-11-01", PaidByDay: 10}}
	plan.SaveSettings(d, st)
	for _, m := range []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"} {
		Tx(t, d, ledger.Tx{Date: m + "-20", Amount: E(4000), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-25", Kind: "income", Amount: E(5000), Category: "salary", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-11-02", Kind: "income", Amount: E(100), Category: "other_income", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-03", Amount: E(257), Category: "food.groceries", AccountID: "swed"})
	r, err := cfo.BuildMonthReview(d, "2026-11", time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	net := money.FromFloat(plan.LTNetSalary(10000, 0))
	if r.Flow.ExpectedSalary != net || r.Flow.ExpectedSavingsRate == nil || r.Flow.Income != E(100) {
		t.Fatalf("flow %+v", r.Flow)
	}
	if r.Tone != "neutral" || !strings.HasPrefix(r.Verdict, "So far: ") || strings.Contains(r.Verdict, "more than you earned") {
		t.Errorf("verdict %q tone %s", r.Verdict, r.Tone)
	}
	if len(r.Highlights) == 0 || !strings.Contains(r.Highlights[0].Text, "still to come by 10 Dec") {
		t.Errorf("highlights %+v", r.Highlights)
	}
}

// Pay a little under the plan's figure is the month's pay: October paid
// €30 short on 2 Nov is final in spirit — no "still to come", no "So far".
// A month with nothing in it yet still says so.
func TestPayUnderPlanSettlesTheMonth(t *testing.T) {
	d := DB(t)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.GrossSalary = "gross", 10000
	st.SalaryRules = []plan.SalaryRule{{From: "", PaidByDay: 3}}
	plan.SaveSettings(d, st)
	net := money.FromFloat(plan.LTNetSalary(10000, 0))
	for _, m := range []string{"2026-07", "2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-20", Amount: E(4000), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-30", Kind: "income", Amount: net, Category: "salary", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-20", Amount: E(4000), Category: "food.groceries", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-11-02", Kind: "income", Amount: net - E(30), Category: "salary", AccountID: "swed"})
	now := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	if oct := expectedFlows(t, d, now)["2026-10"]; oct.ExpectedSalary != 0 || oct.ExpectedSavingsRate != nil {
		t.Errorf("October paid €30 under plan: still to come %v", oct.ExpectedSalary)
	}
	r, err := cfo.BuildMonthReview(d, "2026-10", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range r.Highlights {
		if strings.Contains(h.Text, "still to come") || strings.Contains(h.Text, "Not final") {
			t.Errorf("paid month: %q", h.Text)
		}
	}
	if strings.HasPrefix(r.Verdict, "So far") {
		t.Errorf("verdict %q", r.Verdict)
	}
	empty, err := cfo.BuildMonthReview(d, "2026-12", time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Verdict != "Nothing recorded for this month yet." || empty.Tone != "neutral" {
		t.Errorf("empty December: %q %s", empty.Verdict, empty.Tone)
	}
}
