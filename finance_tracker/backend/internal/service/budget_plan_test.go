package service

import (
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func planTx(y int, m time.Month, d int, typ domain.TransactionType, cat string, amount float64, labels string) domain.Transaction {
	return domain.Transaction{Date: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Type: typ, Category: domain.Category(cat), Amount: amount, Labels: labels}
}

func lineByName(t *testing.T, r *BudgetStatusReport, name string) BudgetLineStatus {
	t.Helper()
	for _, l := range r.Lines {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no line %q", name)
	return BudgetLineStatus{}
}

// A vacation fund accrues monthly from its start, a trip draws it down, the
// balance carries — and the trip doesn't sink that month's safe-to-spend.
func TestBudgetPlan_FundCarriesOver(t *testing.T) {
	budgets := []domain.Budget{
		{ID: 1, Name: "Vacation", Kind: "spending", Category: "Vacation", Amount: 400, Fund: true, StartMonth: "2026-01"},
		{ID: 2, Name: "Food", Kind: "spending", Category: "Food", Amount: 300},
	}
	txs := []domain.Transaction{
		planTx(2026, 3, 10, "expense", "Vacation", 1000, ""),
		planTx(2026, 4, 2, "expense", "Vacation", 900, ""),
		planTx(2026, 4, 5, "expense", "Food", 100, ""),
	}
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	r := ComputeBudgetPlan(budgets, nil, txs, "2026-04", now, 3000, "manual")

	v := lineByName(t, r, "Vacation")
	require.NotNil(t, v.FundState)
	// Jan–Mar: 3×400 − 1000 = 200 carried in; April adds 400, trip takes 900.
	assert.InDelta(t, 200, v.FundState.Opening, 0.01)
	assert.InDelta(t, 400, v.FundState.Contribution, 0.01)
	assert.InDelta(t, -300, v.FundState.Available, 0.01)
	assert.InDelta(t, 600, v.Budgeted, 0.01)
	assert.InDelta(t, 900, v.Spent, 0.01)
	assert.InDelta(t, 1600, v.FundState.Contributed, 0.01)
	// History marks the fund balance per month.
	require.Len(t, v.History, 12)
	assert.Equal(t, "2026-04", v.History[11].Month)
	require.NotNil(t, v.History[10].FundEnd)
	assert.InDelta(t, 200, *v.History[10].FundEnd, 0.01) // end of March
	assert.Nil(t, v.History[0].FundEnd, "before the fund started")

	// safe = 3000 − 400 set aside − (1000 discretionary − 900 from the fund).
	require.NotNil(t, r.SafeToSpend)
	assert.InDelta(t, 2500, *r.SafeToSpend, 0.01)
	assert.InDelta(t, 700, r.SpendingPlanned, 0.01)
}

// A yearly line is judged against the year: spend since January, pace and
// projection — not against one month.
func TestBudgetPlan_YearlyLine(t *testing.T) {
	budgets := []domain.Budget{{ID: 1, Name: "Clothing", Kind: "spending", Category: "Clothing", Amount: 1200, Period: domain.PeriodYearly}}
	txs := []domain.Transaction{
		planTx(2025, 12, 1, "expense", "Clothing", 500, ""), // last year: ignored
		planTx(2026, 2, 1, "expense", "Clothing", 300, ""),
		planTx(2026, 6, 1, "expense", "Clothing", 300, ""),
	}
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	r := ComputeBudgetPlan(budgets, nil, txs, "2026-06", now, 0, "")
	l := lineByName(t, r, "Clothing")
	require.NotNil(t, l.Year)
	assert.InDelta(t, 600, l.Year.Spent, 0.01)
	assert.InDelta(t, 600, l.Year.Pace, 0.01) // half the year elapsed by end of June
	assert.InDelta(t, 1200, l.Year.Projected, 0.01)
	assert.InDelta(t, 600, l.Remaining, 0.01)
	assert.InDelta(t, 100, l.MonthlyShare, 0.01)
	assert.InDelta(t, 100, r.SpendingPlanned, 0.01)
	assert.Nil(t, r.SafeToSpend, "no income base")
}

// An amount change applies from its month on; earlier months keep the
// amount they had.
func TestBudgetPlan_AmountHistory(t *testing.T) {
	budgets := []domain.Budget{{ID: 7, Name: "Kids", Kind: "spending", Category: "Kids", Amount: 600}}
	amounts := []domain.BudgetAmount{
		{BudgetID: 7, FromMonth: "2026-10", Amount: 600},
		{BudgetID: 7, FromMonth: "", Amount: 200},
	}
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	r := ComputeBudgetPlan(budgets, amounts, nil, "2026-10", now, 0, "")
	l := lineByName(t, r, "Kids")
	assert.InDelta(t, 600, l.Budgeted, 0.01)
	assert.InDelta(t, 200, l.History[10].Budget, 0.01) // September
	assert.InDelta(t, 600, l.History[11].Budget, 0.01)

	r = ComputeBudgetPlan(budgets, amounts, nil, "2026-08", now, 0, "")
	assert.InDelta(t, 200, lineByName(t, r, "Kids").Budgeted, 0.01)
}

// Suggestions size a steady line at its 75th percentile and flag a lumpy
// one for a fund at its monthly average.
func TestBudgetPlan_Suggestions(t *testing.T) {
	budgets := []domain.Budget{
		{ID: 1, Name: "Food", Kind: "spending", Category: "Food", Amount: 200},
		{ID: 2, Name: "Fun", Kind: "spending", Category: "Entertainment", Amount: 200},
	}
	var txs []domain.Transaction
	for m := time.Month(1); m <= 12; m++ {
		txs = append(txs, planTx(2025, m, 5, "expense", "Food", 400+float64(m)*10, ""))
	}
	txs = append(txs, planTx(2025, 4, 1, "expense", "Entertainment", 3000, ""),
		planTx(2025, 9, 1, "expense", "Entertainment", 60, ""),
		planTx(2025, 3, 1, "expense", "Vacation", 2400, "")) // unbudgeted
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	r := ComputeBudgetPlan(budgets, nil, txs, "2026-01", now, 0, "")

	food := lineByName(t, r, "Food")
	require.NotNil(t, food.Suggestion)
	assert.False(t, food.Suggestion.Lumpy)
	assert.Equal(t, 500.0, food.Suggestion.Amount) // p75 of 410..520 ≈ 492.5 → 500

	fun := lineByName(t, r, "Fun")
	require.NotNil(t, fun.Suggestion)
	assert.True(t, fun.Suggestion.Lumpy)
	assert.Equal(t, 260.0, fun.Suggestion.Amount) // mean 3060/12 = 255 → 260

	require.Len(t, r.Unbudgeted, 1)
	assert.Equal(t, "Vacation", r.Unbudgeted[0].Category)
	require.NotNil(t, r.Unbudgeted[0].Suggestion)
	assert.True(t, r.Unbudgeted[0].Suggestion.Lumpy)
	assert.Equal(t, 200.0, r.Unbudgeted[0].Suggestion.Amount)
}

// Fixed obligations claim their rows before spending limits do, and trip
// lines stay out of the monthly plan.
func TestBudgetPlan_FixedClaimsAndTripsExcluded(t *testing.T) {
	budgets := []domain.Budget{
		{ID: 1, Name: "Alimony", Kind: "fixed", Label: "alimony", Amount: 1000},
		{ID: 2, Name: "Kids", Kind: "spending", Category: "Kids", Amount: 300},
		{ID: 3, Name: "Zakopane", Kind: "trip", Label: "trip:zakopane", Amount: 1500},
	}
	txs := []domain.Transaction{
		planTx(2026, 5, 1, "expense", "Kids", 1000, "alimony"),
		planTx(2026, 5, 2, "expense", "Kids", 80, ""),
	}
	r := ComputeBudgetPlan(budgets, nil, txs, "2026-05", time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC), 0, "")
	assert.Len(t, r.Lines, 2)
	assert.InDelta(t, 80, lineByName(t, r, "Kids").Spent, 0.01)
	assert.InDelta(t, 1000, lineByName(t, r, "Alimony").Spent, 0.01)
	assert.InDelta(t, 80, r.DiscretionarySpent, 0.01)
}
