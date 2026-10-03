package service

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/marketdata"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCurrentMonthSection(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	txs := []domain.Transaction{
		{Date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Type: domain.TransactionTypeExpense, Amount: 100},
		{Date: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), Type: domain.TransactionTypeExpense, Amount: 60},
		{Date: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Type: domain.TransactionTypeIncome, Amount: 3000},
		{Date: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Type: domain.TransactionTypeInvestment, Amount: 200},
		{Date: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), Type: domain.TransactionTypeExpense, Amount: 999}, // last month, ignored
	}
	svc := &insightService{}
	sec := svc.currentMonthSection(txs, now)
	assert.Contains(t, sec, "day 16 of 31")
	assert.Contains(t, sec, "Income received:  €3000")
	assert.Contains(t, sec, "Spent:            €160")
	assert.Contains(t, sec, "Invested:         €200")
	assert.NotContains(t, sec, "999")
	// 160 spent over 16 days = €10/day → projected €310 over 31 days.
	assert.Contains(t, sec, "€10.0/day")
	assert.Contains(t, sec, "€310")
}

func TestBudgetStatusSection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Budget{}, &domain.BudgetAmount{}, &domain.LabelRule{}, &domain.BudgetSettings{}))
	budgetRepo := repository.NewBudgetRepository(db)
	require.NoError(t, db.Create(&domain.Budget{Name: "Loan payments", Kind: "fixed", Label: "loan", Amount: 1285}).Error)
	require.NoError(t, db.Create(&domain.Budget{Name: "VWCE / ETF", Kind: "investment", Category: "Stocks & ETF", Amount: 1000}).Error)
	require.NoError(t, db.Create(&domain.Budget{Name: "Food", Kind: "spending", Category: "Food", Amount: 200}).Error)
	// Manual income base so safe-to-spend is deterministic.
	s, _ := budgetRepo.GetSettings()
	s.IncomeMode = "manual"
	s.ManualIncome = 3000
	require.NoError(t, budgetRepo.SaveSettings(s))

	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }
	txs := []domain.Transaction{
		{Date: day(3), Type: domain.TransactionTypeExpense, Category: "Finance", Amount: 1285, Labels: "loan"},
		{Date: day(4), Type: domain.TransactionTypeInvestment, Category: "Stocks & ETF", Amount: 600},
		{Date: day(6), Type: domain.TransactionTypeExpense, Category: "Food", Amount: 150},
	}
	summary := &domain.TransactionSummary{}

	svc := &insightService{budgetRepo: budgetRepo}
	sec := svc.budgetStatusSection(txs, summary, now)

	assert.Contains(t, sec, "Income base: €3000 (manual)")
	assert.Contains(t, sec, "Loan payments: €1285 / €1285 (paid)")
	assert.Contains(t, sec, "VWCE / ETF: €600 / €1000 (in progress)")
	assert.Contains(t, sec, "Food: €150 / €200 (75%)")
	// safe = 3000 − 1285 fixed − 1000 invest target − 150 discretionary = 565.
	assert.Contains(t, sec, "Safe to spend the rest of the month: €565")

	// Nil budget repo → no section, no panic.
	assert.Equal(t, "", (&insightService{}).budgetStatusSection(txs, summary, now))
}

func TestStockPositionsSectionWithFakeQuotes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.StockTrade{}))
	require.NoError(t, db.Create(&domain.StockTrade{
		Date: time.Now().AddDate(0, -2, 0), Action: "buy", Ticker: "VWCE",
		Shares: 10, PricePerShare: 100, Currency: "EUR", Source: "IBKR"}).Error)
	stockSvc := NewStockService(repository.NewStockRepository(db))

	svc := &insightService{
		stockSvc: stockSvc,
		quote: func(ticker string) (*marketdata.Quote, error) {
			return &marketdata.Quote{Ticker: ticker, Price: 120, Currency: "EUR"}, nil
		},
	}
	latest := &domain.Balance{SwedETF: 5000, RevStocks: 1000, IBKRStocks: 2000}
	sec := svc.stockPositionsSection(latest)

	assert.Contains(t, sec, "live prices via Yahoo Finance")
	// 10 sh, avg 100 EUR, cost 1000; now 120 → mkt 1200 = +20%.
	assert.Contains(t, sec, "VWCE: 10 sh @ 100.00 EUR avg = 1000 EUR cost")
	assert.Contains(t, sec, "now 120.00 EUR → 1200 EUR (+20.0%)")
	assert.Contains(t, sec, "Total cost basis: 1000 EUR")
	assert.Contains(t, sec, "Swed ETF €5000")

	// Currency mismatch (EUR cost, USD quote): show the live price honestly,
	// invent NO cross-currency gain %.
	svc.quote = func(ticker string) (*marketdata.Quote, error) {
		return &marketdata.Quote{Ticker: ticker, Price: 120, Currency: "USD"}, nil
	}
	sec = svc.stockPositionsSection(latest)
	assert.Contains(t, sec, "now 120.00 USD (live; cost recorded in EUR, no FX conversion applied)")
	assert.NotContains(t, sec, "%)", "no fabricated gain percentage on a currency mismatch")

	// London pence (GBp) is normalized to GBP before comparison — a GBp quote
	// must never be multiplied against a bare-pound cost basis (the +11900% bug).
	svc.quote = func(ticker string) (*marketdata.Quote, error) {
		return &marketdata.Quote{Ticker: ticker, Price: 12000, Currency: "GBp"}, nil
	}
	sec = svc.stockPositionsSection(latest)
	assert.NotContains(t, sec, "12000.00", "pence must be converted to pounds")
	assert.Contains(t, sec, "120.00 GBP")

	// Quote failure → cost basis only, no crash.
	svc.quote = func(string) (*marketdata.Quote, error) { return nil, assert.AnError }
	sec = svc.stockPositionsSection(latest)
	assert.Contains(t, sec, "live prices unavailable")
	assert.Contains(t, sec, "VWCE: 10 sh")

	// Nil stock service → no section.
	assert.Equal(t, "", (&insightService{}).stockPositionsSection(latest))
}

func TestLtNetSalary(t *testing.T) {
	// Gross 3000, no deductions: sodra 585, npd = 747-0.49*(3000-1038)=? <0 →0,
	// gpm = 3000*0.20 = 600, net = 3000-585-600 = 1815.
	assert.InDelta(t, 1815, ltNetSalary(3000, 0), 0.5)
	// Deductions subtract directly.
	assert.InDelta(t, 1715, ltNetSalary(3000, 100), 0.5)
}

func TestMedianMonthlyIncomeExcludesCurrent(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	byMonth := []domain.MonthlySummary{
		{Year: 2026, Month: 5, Income: 3000},
		{Year: 2026, Month: 6, Income: 3200},
		{Year: 2026, Month: 7, Income: 2800},
		{Year: 2026, Month: 8, Income: 500}, // current, incomplete → excluded
	}
	assert.InDelta(t, 3000, medianMonthlyIncome(byMonth, now), 0.001)
}

func TestBudgetMatchesTx(t *testing.T) {
	tx := domain.Transaction{Category: "Food", Labels: "restaurant,work lunch"}
	assert.True(t, budgetMatchesTx(domain.Budget{Label: "restaurant"}, tx))
	assert.True(t, budgetMatchesTx(domain.Budget{Category: "Food"}, tx))
	assert.True(t, budgetMatchesTx(domain.Budget{Label: "restaurant", Category: "Food"}, tx), "both match → matches")
	assert.False(t, budgetMatchesTx(domain.Budget{Label: "restaurant", Category: "Transport"}, tx), "AND semantics")
	assert.False(t, budgetMatchesTx(domain.Budget{Label: "groceries"}, tx))
	assert.False(t, budgetMatchesTx(domain.Budget{}, tx), "no matcher → never matches")
}
