package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// BudgetStatus evaluates the plan for the given month (current month when
// year/month are zero) through the budget engine — see ComputeBudgetPlan
// for what each line's numbers mean.
func (s *insightService) BudgetStatus(year, month int) (*BudgetStatusReport, error) {
	if s.budgetRepo == nil {
		return nil, errors.New("budgets unavailable")
	}
	now := time.Now()
	if year == 0 || month == 0 {
		year, month = now.Year(), int(now.Month())
	}
	if month < 1 || month > 12 || year < 2000 || year > 2100 {
		return nil, fmt.Errorf("invalid month %04d-%02d", year, month)
	}
	allTxs, err := s.txSvc.ListAll()
	if err != nil {
		return nil, err
	}
	return s.budgetPlan(allTxs, fmt.Sprintf("%04d-%02d", year, month), now)
}

// budgetPlan loads the plan inputs and runs the engine.
func (s *insightService) budgetPlan(allTxs []domain.Transaction, month string, now time.Time) (*BudgetStatusReport, error) {
	budgets, err := s.budgetRepo.ListBudgets()
	if err != nil {
		return nil, err
	}
	amounts, err := s.budgetRepo.ListAmounts()
	if err != nil {
		return nil, err
	}
	var summary *domain.TransactionSummary
	if s.txSvc != nil {
		summary, _ = s.txSvc.GetSummary(domain.TransactionFilter{})
	}
	return s.budgetPlanWith(budgets, amounts, allTxs, month, now, summary), nil
}

func (s *insightService) budgetPlanWith(budgets []domain.Budget, amounts []domain.BudgetAmount, allTxs []domain.Transaction,
	month string, now time.Time, summary *domain.TransactionSummary) *BudgetStatusReport {
	var base float64
	var source string
	if summary != nil {
		settings, _ := s.budgetRepo.GetSettings()
		mid, _ := parseYM(month)
		base, source = s.incomeBase(settings, summary, mid.AddDate(0, 0, 14))
	}
	return ComputeBudgetPlan(budgets, amounts, allTxs, month, now, base, source)
}
