package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

func tx(date, kind, cat string, eur float64, acct ...string) ledger.Tx {
	t := ledger.Tx{Date: date, Kind: kind, Category: cat, Amount: money.FromFloat(eur)}
	if len(acct) > 0 {
		t.AccountID = acct[0]
	}
	if len(acct) > 1 {
		t.ToAccountID = acct[1]
	}
	return t
}

func cats(t *testing.T) map[string]ledger.Category {
	d := DB(t)
	m, _ := ledger.CategoryMap(d)
	return m
}

func TestCashFlowDefinitions(t *testing.T) {
	txs := []ledger.Tx{
		tx("2026-09-15", "income", "salary", 5000, "swed"),
		tx("2026-09-20", "income", "refunds", 100, "swed"),
		tx("2026-09-05", "expense", "food.groceries", 600, "swed"),
		tx("2026-09-06", "expense", "leisure", 400, "swed"),
		tx("2026-09-17", "expense", "housing.mortgage_interest", 900, "seb"),
		tx("2026-09-17", "transfer", "transfer.debt", 460, "seb", "mortgage"),
		tx("2026-09-01", "transfer", "transfer.invest", 1000, "swed", "ibkr"),
		tx("2026-09-02", "transfer", "transfer.internal", 1400, "swed", "seb"),
		tx("2026-09-25", "transfer", "transfer.pension", 120, "", "artea"), // via payroll
	}
	f := insights.CashFlow(txs, cats(t), "month")
	if len(f) != 1 {
		t.Fatal(f)
	}
	m := f[0]
	if m.Income != E(5000) {
		t.Error("refunds are not income", m.Income)
	}
	if m.Spending != E(600+400+900-100) {
		t.Error("spending nets refunds", m.Spending)
	}
	if m.Essential != E(1500) || m.Discretionary != E(400) {
		t.Error("essential split", m.Essential, m.Discretionary)
	}
	if m.Saved != E(5000-1800) || m.SavingsRate != 0.64 {
		t.Error("saved", m.Saved, m.SavingsRate)
	}
	if m.Invested != E(1460) || m.Principal != E(460) {
		t.Error("principal is investing, internal moves are not", m.Invested, m.Principal)
	}
	if m.PayrollPension != E(120) {
		t.Error("payroll pension", m.PayrollPension)
	}
	y := insights.CashFlow(txs, cats(t), "year")
	if y[0].Period != "2026" {
		t.Error(y[0].Period)
	}
}

func TestBreakdownComparesWithPreviousWindow(t *testing.T) {
	txs := []ledger.Tx{
		tx("2026-08-10", "expense", "food.groceries", 100),
		tx("2026-09-10", "expense", "food.groceries", 150),
		tx("2026-09-12", "expense", "food.restaurants", 50),
		tx("2026-09-13", "expense", "leisure", 200),
		tx("2026-09-14", "income", "refunds", 20),
	}
	txs[1].Merchant = "Lidl"
	b := insights.Breakdown(txs, "2026-09-01", "2026-09-30")
	if b[0].Category != "food" && b[0].Category != "leisure" {
		t.Fatal(b)
	}
	var food insights.CategoryStat
	for _, c := range b {
		if c.Category == "food" {
			food = c
		}
	}
	if food.Total != E(200) || food.Previous != E(100) || food.Change != 1 || food.TopMerchant != "Lidl" || len(food.Children) != 2 {
		t.Fatalf("%+v", food)
	}
}

func TestDetectRecurring(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	var txs []ledger.Tx
	for i := 0; i < 8; i++ {
		d := now.AddDate(0, -i, -2).Format("2006-01-02")
		y := tx(d, "expense", "subscriptions.media", 9.99)
		y.Merchant = "YouTube Premium"
		g := tx(d, "expense", "food.groceries", float64(20+i*37%90))
		g.Merchant = "Lidl"
		txs = append(txs, y, g, g) // Lidl: many, irregular amounts
	}
	ins := tx("2025-11-01", "expense", "finance.insurance", 300)
	ins.Merchant = "Gjensidige"
	ins2 := tx("2024-11-03", "expense", "finance.insurance", 290)
	ins2.Merchant = "Gjensidige"
	txs = append(txs, ins, ins2)
	rec := insights.DetectRecurring(txs, now)
	byM := map[string]insights.Recurring{}
	for _, r := range rec {
		byM[r.Merchant] = r
	}
	if r, ok := byM["YouTube Premium"]; !ok || r.Cadence != "monthly" || r.Amount != E(9.99) {
		t.Errorf("monthly subscription: %+v", r)
	}
	if _, ok := byM["Lidl"]; ok {
		t.Error("irregular groceries are not a subscription")
	}
	if r, ok := byM["Gjensidige"]; ok {
		t.Errorf("a charge older than 13 months is not current: %+v", r)
	}
}

func TestAnomalies(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	var txs []ledger.Tx
	for i := 1; i <= 12; i++ {
		txs = append(txs, tx(now.AddDate(0, -i, 0).Format("2006-01-02"), "expense", "food.groceries", 400))
	}
	txs = append(txs, tx("2026-10-05", "expense", "food.groceries", 500), tx("2026-10-06", "expense", "leisure", 20))
	a := insights.Anomalies(txs, "2026-10", now)
	if len(a) != 1 || a[0].Category != "food" || a[0].Ratio < 2 {
		t.Fatalf("%+v", a)
	}
}

func TestComputeFI(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	Bal(t, d, "swed", "2026-10-01", 12000)
	Bal(t, d, "ibkr", "2026-10-01", 100000)
	Bal(t, d, "house", "2026-10-01", 400000)
	var txs []ledger.Tx
	for i := 0; i < 12; i++ {
		m := now.AddDate(0, -i, -1).Format("2006-01-02")
		txs = append(txs, tx(m, "income", "salary", 5000), tx(m, "expense", "food.groceries", 1000), tx(m, "expense", "leisure", 1000))
	}
	book, _ := wealth.LoadBook(d)
	f := insights.ComputeFI(txs, cats(t), book, plan.Settings{WithdrawalRate: 4, ExpectedReturn: 5, EmergencyMonths: 3, BirthYear: 1990, TargetAge: 50}, now)
	if f.AnnualSpend != E(24000) || f.Target != E(600000) {
		t.Fatal(f.AnnualSpend, f.Target)
	}
	// Investable: brokers + cash above a 3-month essential buffer (3 × 1000); the house never counts.
	if f.Investable != E(100000+12000-3000) || f.Emergency.Months != 12 {
		t.Fatalf("investable %v months %v", f.Investable, f.Emergency.Months)
	}
	if f.YearsToFI <= 0 || f.YearsToFI > 20 || f.FIAge == 0 || f.RequiredMonthly <= 0 {
		t.Fatalf("%+v", f)
	}
	if len(f.Projection) != 26 {
		t.Error(len(f.Projection))
	}
}

func TestWindowPresets(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	for preset, want := range map[string][2]string{
		"month": {"2026-10-01", "2026-10-04"}, "last_month": {"2026-09-01", "2026-09-30"}, "ytd": {"2026-01-01", "2026-10-04"},
		"last_year": {"2025-01-01", "2025-12-31"}, "12m": {"2025-10-05", "2026-10-04"},
	} {
		f, to := insights.Window(preset, now)
		if f != want[0] || to != want[1] {
			t.Errorf("%s: %s..%s", preset, f, to)
		}
	}
}

func TestTopExpensesAndMerchants(t *testing.T) {
	a := tx("2026-09-01", "expense", "travel", 900)
	a.Merchant = "Airbnb"
	b := tx("2026-09-02", "expense", "food", 10)
	b.Merchant = "Lidl"
	c := tx("2026-09-03", "expense", "food", 15)
	c.Merchant = "Lidl"
	txs := []ledger.Tx{a, b, c}
	if top := insights.TopExpenses(txs, "2026-09-01", "2026-09-30", 1); top[0].Merchant != "Airbnb" {
		t.Error(top)
	}
	m := insights.MerchantTotals(txs, "2026-09-01", "2026-09-30", 5)
	if m[0].Name != "Airbnb" || m[1].Count != 2 || m[1].Amount != E(25) {
		t.Error(m)
	}
}

func TestPace(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	txs := []ledger.Tx{
		tx("2026-10-02", "expense", "food", 100), tx("2026-10-09", "expense", "food", 50),
		tx("2026-09-01", "expense", "food", 300), tx("2026-09-20", "expense", "food", 300),
		tx("2026-10-05", "income", "refunds", 20),
	}
	for i := 2; i <= 6; i++ {
		txs = append(txs, tx(now.AddDate(0, -i, 0).Format("2006-01")+"-01", "expense", "food", 600))
	}
	p := insights.Pace(txs, now)
	if len(p) != 31 || *p[9].Current != E(130) || p[10].Current != nil {
		t.Fatalf("current: %+v %+v", p[9], p[10])
	}
	if p[0].LastMonth != E(300) || p[30].LastMonth != E(600) {
		t.Fatal("last month cumulative")
	}
	// Six months average: Sep 300 by day 1 (600 by day 30), five months of 600 on day 1.
	if p[0].Typical != E((300+5*600)/6.0) {
		t.Fatal(p[0].Typical)
	}
}

func TestTagTotals(t *testing.T) {
	a := tx("2026-09-01", "expense", "food", 50)
	a.Tags = []string{"kids", "trip:rome"}
	b := tx("2026-09-02", "expense", "leisure", 30)
	b.Tags = []string{"kids"}
	c := tx("2026-09-03", "income", "refunds", 10)
	c.Tags = []string{"kids"}
	got := insights.TagTotals([]ledger.Tx{a, b, c}, "2026-09-01", "2026-09-30")
	if len(got) != 2 || got[0].Name != "kids" || got[0].Amount != E(70) || got[0].Count != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestRecurringEditsHideAndManual(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	var txs []ledger.Tx
	for i := 0; i < 8; i++ {
		date := now.AddDate(0, -i, -2).Format("2006-01-02")
		y := tx(date, "expense", "subscriptions.media", 9.99)
		y.Merchant = "YouTube Premium"
		n := tx(date, "expense", "subscriptions.media", 15)
		n.Merchant = "Netflix"
		txs = append(txs, y, n)
	}
	// Edit YouTube to a family plan billed quarterly; hide Netflix; add rent by hand.
	yt := insights.RecurringItem{Merchant: "youtube premium", Cadence: "quarterly", Amount: E(30), Note: "family"}
	if err := insights.SaveRecurringItem(d, &yt); err != nil {
		t.Fatal(err)
	}
	if err := insights.SaveRecurringItem(d, &insights.RecurringItem{Merchant: "Netflix", Hidden: true}); err != nil {
		t.Fatal(err)
	}
	rent := insights.RecurringItem{Merchant: "Landlord", Category: "housing.rent", Amount: E(600), NextDate: "2026-09-10"}
	if err := insights.SaveRecurringItem(d, &rent); err != nil {
		t.Fatal(err)
	}
	// Saving the same merchant again updates, never duplicates.
	again := insights.RecurringItem{Merchant: "LANDLORD", Category: "housing.rent", Amount: E(650), NextDate: "2026-09-10"}
	if err := insights.SaveRecurringItem(d, &again); err != nil || again.ID != rent.ID {
		t.Fatalf("upsert: %v id %d vs %d", err, again.ID, rent.ID)
	}
	if err := insights.SaveRecurringItem(d, &insights.RecurringItem{Merchant: "X", Cadence: "weekly"}); err == nil {
		t.Error("bad cadence accepted")
	}
	list, hidden, err := insights.RecurringCosts(d, txs, now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]insights.Recurring{}
	for _, r := range list {
		by[r.Merchant] = r
	}
	if r := by["YouTube Premium"]; r.Source != "edited" || r.Cadence != "quarterly" || r.Amount != E(30) || r.Monthly != E(10) || r.Note != "family" {
		t.Errorf("edit: %+v", r)
	}
	if _, ok := by["Netflix"]; ok || len(hidden) != 1 || hidden[0].Merchant != "Netflix" {
		t.Errorf("hidden: %+v / %+v", by["Netflix"], hidden)
	}
	// A past next date rolls forward by its cadence.
	if r := by["Landlord"]; r.Source != "manual" || r.Amount != E(650) || r.Next != "2026-10-10" || r.Category != "housing.rent" {
		t.Errorf("manual: %+v", r)
	}
	insights.DeleteRecurringItem(d, yt.ID)
	list, _, _ = insights.RecurringCosts(d, txs, now)
	for _, r := range list {
		if r.Merchant == "YouTube Premium" && (r.Source != "detected" || r.Amount != E(9.99)) {
			t.Errorf("delete restores detection: %+v", r)
		}
	}
}
