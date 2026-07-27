package service

// White-box tests for the label aggregation feeding the AI insight prompt.

import (
	"strings"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
)

func labTx(date time.Time, txType domain.TransactionType, amount float64, labels string) domain.Transaction {
	return domain.Transaction{Date: date, Type: txType, Amount: amount, Labels: labels}
}

func TestBuildLabelSections(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	lastMonth := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	twoYearsAgo := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	txs := []domain.Transaction{
		// Fixed obligations: labeled loan/evelina — must land in the fixed
		// section regardless of age, never in discretionary. The importer
		// tags loan transfers with BOTH fixed labels ("loan,evelina"); such a
		// payment must be attributed once (to loan), not double counted.
		labTx(twoYearsAgo, domain.TransactionTypeExpense, 1200, "loan"),
		labTx(lastMonth, domain.TransactionTypeExpense, 1200, "loan,evelina"),
		labTx(lastMonth, domain.TransactionTypeExpense, 950, "evelina"),
		// Discretionary spending within the last year.
		labTx(lastMonth, domain.TransactionTypeExpense, 80, "restaurant"),
		labTx(lastMonth, domain.TransactionTypeExpense, 45, "restaurant"),
		labTx(lastMonth, domain.TransactionTypeExpense, 260, "groceries,maxima"),
		// Old discretionary spend — outside the 12-month window.
		labTx(twoYearsAgo, domain.TransactionTypeExpense, 500, "electronics"),
		// Unlabeled expense — contributes to no label bucket.
		labTx(lastMonth, domain.TransactionTypeExpense, 33, ""),
		// Income with employer labels.
		labTx(lastMonth, domain.TransactionTypeIncome, 4200, "nexos"),
		labTx(twoYearsAgo, domain.TransactionTypeIncome, 3800, "pvcase"),
	}

	s := buildLabelSections(txs, 8000, now)

	// Fixed obligations include both labels with totals and income share.
	// The dual-labeled "loan,evelina" payment counts toward loan only, and
	// the TOTAL line reflects transaction-level money (no overlap).
	assert.Contains(t, s.fixedObligations, "loan: €2400 total (2 payments")
	assert.Contains(t, s.fixedObligations, "evelina: €950 total (1 payments")
	assert.Contains(t, s.fixedObligations, "TOTAL fixed obligations: €3350")
	// Per-label monthly rate spans the label's own lifetime, not the oldest
	// fixed transaction's: evelina started last month, not two years ago.
	assert.Contains(t, s.fixedObligations, "evelina: €950 total (1 payments, ≈€475/month since 2026-06)")
	assert.Contains(t, s.fixedObligations, "% of all-time income")

	// Discretionary top labels: last 12 months only, fixed labels excluded.
	assert.Contains(t, s.topSpendingLabels, "restaurant: €125 (2 transactions, avg €62)")
	assert.Contains(t, s.topSpendingLabels, "groceries: €260")
	assert.NotContains(t, s.topSpendingLabels, "electronics")
	assert.NotContains(t, s.topSpendingLabels, "loan")

	// Income sources list employers with share of all income.
	assert.Contains(t, s.incomeSources, "nexos: €4200 (1 payments)")
	assert.Contains(t, s.incomeSources, "pvcase: €3800 (1 payments)")

	// Per-month top labels exist for the recent month and rank by size.
	june := s.topMonthLabels["2026-06"]
	assert.True(t, strings.HasPrefix(june, "groceries €260"), "june top labels should start with groceries: %q", june)
	assert.Contains(t, june, "restaurant €125")
}

func TestBuildLabelSections_Empty(t *testing.T) {
	s := buildLabelSections(nil, 0, time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "  - none", s.fixedObligations)
	assert.Equal(t, "  - none", s.topSpendingLabels)
	assert.Equal(t, "  - none", s.incomeSources)
	assert.Empty(t, s.topMonthLabels)
}
