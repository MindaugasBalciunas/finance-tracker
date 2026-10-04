package handler

import (
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func rtx(id uint, date string, typ domain.TransactionType, amount float64, cat, labels string) domain.Transaction {
	return domain.Transaction{ID: id, Date: d(date), Type: typ, Amount: amount, Category: domain.Category(cat), Labels: labels}
}

func TestMonthReviewTotalsAndComparisons(t *testing.T) {
	txs := []domain.Transaction{
		// September (reviewed)
		rtx(1, "2026-09-01", "income", 3000, "Salary", ""),
		rtx(2, "2026-09-03", "expense", 400, "Food", "groceries"),
		rtx(3, "2026-09-10", "expense", 900, "Vacation", "trip"),
		rtx(4, "2026-09-11", "expense", 50, "Transfers", "owed,owed-tomas"), // fronted, not spending
		rtx(5, "2026-09-12", "investment", 500, "Stocks & ETF", ""),
		rtx(6, "2026-09-15", "income", 20, "Transfers", "owed,owed-tomas"), // repayment, not income
		// August
		rtx(7, "2026-08-01", "income", 3000, "Salary", ""),
		rtx(8, "2026-08-05", "expense", 600, "Food", "x"),
		// July
		rtx(9, "2026-07-01", "income", 2000, "Salary", ""),
		rtx(10, "2026-07-05", "expense", 200, "Food", "x"),
	}
	bals := []domain.Balance{
		{Date: d("2026-09-30"), Total: 11500},
		{Date: d("2026-08-31"), Total: 10000},
		{Date: d("2026-08-15"), Total: 9000},
	}
	budget := &service.BudgetStatusReport{Lines: []service.BudgetLineStatus{
		{Name: "Food", Kind: "spending", Period: domain.PeriodMonthly, Amount: 300, MonthSpent: 400},
		{Name: "Fun", Kind: "spending", Period: domain.PeriodMonthly, Amount: 100, MonthSpent: 20},
		{Name: "Trip fund", Kind: "spending", Period: domain.PeriodMonthly, Fund: true, Amount: 100, MonthSpent: 900},
		{Name: "Car", Kind: "spending", Period: domain.PeriodYearly, Amount: 1200, MonthSpent: 500},
	}}

	r := buildMonthReview(reviewInput{month: d("2026-09-01"), now: d("2026-10-04"), txs: txs, balances: bals, budget: budget})

	assert.True(t, r.Complete)
	assert.Equal(t, 3000.0, r.Income)
	assert.Equal(t, 1300.0, r.Spending)
	assert.Equal(t, 500.0, r.Invested)
	assert.Equal(t, 1700.0, r.NetSaved)
	require.NotNil(t, r.SavingsRate)
	assert.InDelta(t, 56.7, *r.SavingsRate, 0.001)

	assert.Equal(t, 600.0, r.Previous.Spending)
	assert.Equal(t, 2500.0, r.SixMonthAvg.Income, "averages only months that have data")
	assert.Equal(t, 400.0, r.SixMonthAvg.Spending)

	require.NotNil(t, r.NetWorth)
	assert.Equal(t, "2026-08-31", r.NetWorth.StartDate)
	assert.Equal(t, 1500.0, r.NetWorth.Change)

	require.NotEmpty(t, r.Categories)
	assert.Equal(t, "Vacation", r.Categories[0].Category)
	assert.Equal(t, 900.0, r.Categories[0].Delta)

	require.Len(t, r.TopExpenses, 2)
	assert.Equal(t, uint(3), r.TopExpenses[0].ID)

	require.NotNil(t, r.Budget)
	require.Len(t, r.Budget.Over, 1, "funds and yearly lines are not judged by one month")
	assert.Equal(t, "Food", r.Budget.Over[0].Name)
	assert.Equal(t, 1, r.Budget.WithinCount)

	assert.Equal(t, 30.0, r.OwedToYou)

	// Visual series.
	require.Len(t, r.Trend, 12)
	assert.Equal(t, "2025-10", r.Trend[0].Month)
	assert.Equal(t, "2026-09", r.Trend[11].Month)
	assert.Equal(t, 1300.0, r.Trend[11].Spending)
	assert.Equal(t, 600.0, r.Trend[10].Spending)
	require.Len(t, r.Daily, 30)
	assert.Equal(t, 400.0, r.Daily[2].Value, "Sept 3")
	assert.Zero(t, r.Daily[10].Value, "the owed share on Sept 11 is not spending")
	require.Len(t, r.ByCategory, 2)
	assert.Equal(t, "Vacation", r.ByCategory[0].Category)
	assert.Equal(t, 400.0, r.ByCategory[1].Average, "Food over Jul+Aug")
	require.NotNil(t, r.NetWorth)
	assert.Equal(t, []reviewPoint{{"2026-08-31", 10000}, {"2026-09-30", 11500}}, r.NetWorth.Points)
	require.Len(t, r.ByCategory[0].Top, 1)
	assert.Equal(t, uint(3), r.ByCategory[0].Top[0].ID)
	assert.Equal(t, 1, r.ByCategory[0].Count)
	require.Len(t, r.TopIncome, 1, "the repayment is not income")
	assert.Equal(t, 3000.0, r.TopIncome[0].Amount)
	require.Len(t, r.Budget.Lines, 2)
	assert.Equal(t, "Food", r.Budget.Lines[0].Name, "fullest first")
}

func TestMonthReviewChecks(t *testing.T) {
	now := d("2026-10-04")
	old := d("2025-12-01")
	reset := d("2026-10-20")
	r := buildMonthReview(reviewInput{
		month: d("2026-09-01"), now: now,
		txs:      []domain.Transaction{rtx(1, "2026-09-03", "expense", 10, "Food", "")},
		balances: []domain.Balance{{Date: d("2026-08-01"), Total: 1}},
		assets:   []domain.Asset{{Name: "Flat", ValuationDate: &old, LoanRateResetDate: &reset}},
		conns: []domain.BankConnection{
			{ASPSPName: "Swedbank", Status: domain.BankConnAuthorized, ValidUntil: now.AddDate(0, 0, 5)},
			{ASPSPName: "Old", Status: domain.BankConnRevoked, ValidUntil: now.AddDate(0, 0, -5)},
		},
		bankWaiting: 3,
	})
	texts := ""
	for _, c := range r.Checks {
		texts += c.Text + "\n"
	}
	assert.Contains(t, texts, "64 days old")
	assert.Contains(t, texts, "No balance snapshot inside this month")
	assert.Contains(t, texts, "Flat was last valued 2025-12-01")
	assert.Contains(t, texts, "Flat loan rate resets on 2026-10-20")
	assert.Contains(t, texts, "Swedbank bank access expires in 5 days")
	assert.NotContains(t, texts, "Old bank", "revoked connections are not nagged about")
	assert.Contains(t, texts, "3 bank rows waiting")
	assert.Contains(t, texts, "1 expense this month without labels")
	assert.Nil(t, r.NetWorth)
}

func TestMonthReviewRunningMonthIsIncomplete(t *testing.T) {
	r := buildMonthReview(reviewInput{month: d("2026-10-01"), now: d("2026-10-04")})
	assert.False(t, r.Complete)
	assert.Nil(t, r.SavingsRate)
}

func TestMonthReviewFixedObligations(t *testing.T) {
	r := buildMonthReview(reviewInput{month: d("2026-09-01"), now: d("2026-10-04"), txs: []domain.Transaction{
		rtx(1, "2026-09-17", "expense", 1000, "Finance", "alimony"),
		rtx(2, "2026-09-17", "expense", 900, "Finance", "loan"),
		rtx(3, "2026-09-20", "expense", 100, "Food", "groceries"),
		rtx(4, "2026-08-17", "expense", 1000, "Finance", "alimony"),
	}})
	assert.Equal(t, 1900.0, r.Fixed)
	assert.Equal(t, 1000.0, r.FixedAverage)
	assert.Contains(t, r.FixedLabels, "loan")
}

func TestMonthReviewMarksRecurringPayments(t *testing.T) {
	alimony := rtx(1, "2026-09-17", "expense", 1000, "Finance", "alimony")
	alimony.Comment = "Aliments 2026.09 Evelina"
	trip := rtx(2, "2026-09-21", "expense", 920, "Vacation", "")
	trip.Comment = "Final payment for Navaturas Egypt trip"
	prev := rtx(3, "2026-08-17", "expense", 1000, "Kids", "alimony")
	prev.Comment = "Aliments 2026.08"
	r := buildMonthReview(reviewInput{month: d("2026-09-01"), now: d("2026-10-04"), txs: []domain.Transaction{alimony, trip, prev}})
	require.Len(t, r.TopExpenses, 2)
	assert.True(t, r.TopExpenses[0].Recurring, "same first word and amount, comment reworded")
	assert.Equal(t, "Kids", r.TopExpenses[0].MovedFrom, "filed under Kids last time")
	assert.False(t, r.TopExpenses[1].Recurring)
	assert.Equal(t, "aliments evelina", recurKey("Aliments 2026.09 Evelina"))
}
