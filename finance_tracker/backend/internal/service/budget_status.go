package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Structured budget status — the same month-to-date math the AI report's
// text section uses, as JSON for machine clients (chat tools, MCP). One
// source of truth for "how am I doing against the plan this month".

// BudgetLineStatus is one budget line's month-to-date progress.
type BudgetLineStatus struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"` // fixed | investment | spending
	Label     string  `json:"label,omitempty"`
	Category  string  `json:"category,omitempty"`
	Budgeted  float64 `json:"budgeted"`
	Spent     float64 `json:"spent"`
	Remaining float64 `json:"remaining"`
}

type BudgetStatusReport struct {
	Month              string             `json:"month"`
	IncomeBase         float64            `json:"income_base,omitempty"`
	IncomeBaseSource   string             `json:"income_base_source,omitempty"`
	Lines              []BudgetLineStatus `json:"lines"`
	FixedPlanned       float64            `json:"fixed_planned"`
	InvestmentPlanned  float64            `json:"investment_planned"`
	SpendingPlanned    float64            `json:"spending_planned"`
	DiscretionarySpent float64            `json:"discretionary_spent"`
	// SafeToSpend = income base − fixed planned − investment planned −
	// discretionary already spent. Present only when an income base exists.
	SafeToSpend *float64 `json:"safe_to_spend,omitempty"`
}

// BudgetStatus computes per-budget month-to-date progress for the given
// month (current month when year/month are zero). Semantics match the AI
// report's budget section: fixed obligations claim expenses AND investments,
// investment targets count investments, spending limits count only
// discretionary expenses (those not claimed by a fixed obligation).
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

	budgets, err := s.budgetRepo.ListBudgets()
	if err != nil {
		return nil, err
	}
	allTxs, err := s.txSvc.ListAll()
	if err != nil {
		return nil, err
	}
	var monthTxs []domain.Transaction
	for _, tx := range allTxs {
		if tx.Date.Year() == year && int(tx.Date.Month()) == month {
			monthTxs = append(monthTxs, tx)
		}
	}

	var fixed []domain.Budget
	for _, b := range budgets {
		if b.Kind == "fixed" {
			fixed = append(fixed, b)
		}
	}
	isFixedTx := func(tx domain.Transaction) bool {
		for _, b := range fixed {
			if budgetMatchesTx(b, tx) {
				return true
			}
		}
		return false
	}
	var discretionary []domain.Transaction
	var discretionarySpent float64
	for _, tx := range monthTxs {
		if tx.Type == domain.TransactionTypeExpense && !isFixedTx(tx) {
			discretionary = append(discretionary, tx)
			discretionarySpent += tx.Amount
		}
	}

	spent := func(b domain.Budget) float64 {
		var wantTypes []domain.TransactionType
		switch b.Kind {
		case "investment":
			wantTypes = []domain.TransactionType{domain.TransactionTypeInvestment}
		case "fixed":
			// Pension/leasing land as investments but are still fixed costs.
			wantTypes = []domain.TransactionType{domain.TransactionTypeExpense, domain.TransactionTypeInvestment}
		default:
			wantTypes = []domain.TransactionType{domain.TransactionTypeExpense}
		}
		pool := monthTxs
		if b.Kind == "spending" {
			pool = discretionary
		}
		var total float64
		for _, tx := range pool {
			if containsType(wantTypes, tx.Type) && budgetMatchesTx(b, tx) {
				total += tx.Amount
			}
		}
		return total
	}

	report := &BudgetStatusReport{
		Month: fmt.Sprintf("%04d-%02d", year, month),
		Lines: []BudgetLineStatus{},
	}
	report.DiscretionarySpent = discretionarySpent
	for _, b := range budgets {
		used := spent(b)
		report.Lines = append(report.Lines, BudgetLineStatus{
			Name: b.Name, Kind: b.Kind, Label: b.Label, Category: b.Category,
			Budgeted: b.Amount, Spent: used, Remaining: b.Amount - used,
		})
		switch b.Kind {
		case "fixed":
			report.FixedPlanned += b.Amount
		case "investment":
			report.InvestmentPlanned += b.Amount
		case "spending":
			report.SpendingPlanned += b.Amount
		}
	}

	settings, _ := s.budgetRepo.GetSettings()
	summary, err := s.txSvc.GetSummary(domain.TransactionFilter{})
	if err == nil {
		base, source := s.incomeBase(settings, summary, time.Date(year, time.Month(month), 15, 0, 0, 0, 0, time.UTC))
		if base > 0 {
			report.IncomeBase = base
			report.IncomeBaseSource = source
			safe := base - report.FixedPlanned - report.InvestmentPlanned - discretionarySpent
			report.SafeToSpend = &safe
		}
	}
	return report, nil
}
