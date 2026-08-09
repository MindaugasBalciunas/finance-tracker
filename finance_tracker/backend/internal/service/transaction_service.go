package service

import (
	"errors"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
)

// CreateTransactionInput is the input DTO for creating a transaction
type CreateTransactionInput struct {
	Date          string                 `json:"date" binding:"required"`
	Type          domain.TransactionType `json:"type" binding:"required,oneof=expense income investment"`
	Amount        float64                `json:"amount" binding:"required,gt=0"`
	Comment       string                 `json:"comment"`
	Category      domain.Category        `json:"category" binding:"required"`
	Labels        string                 `json:"labels"`
	DebitAccount  string                 `json:"debit_account"`
	CreditAccount string                 `json:"credit_account"`
	// SuppressedLabels lists rule labels the user explicitly removed in the
	// form — matching rules are skipped for this transaction only.
	SuppressedLabels string `json:"suppressed_labels"`
}

// UpdateTransactionInput is the input DTO for updating a transaction
type UpdateTransactionInput struct {
	Date          string                 `json:"date"`
	Type          domain.TransactionType `json:"type" binding:"omitempty,oneof=expense income investment"`
	Amount        float64                `json:"amount" binding:"omitempty,gt=0"`
	Comment       string                 `json:"comment"`
	Category      domain.Category        `json:"category"`
	Labels        *string                `json:"labels"`
	DebitAccount  string                 `json:"debit_account"`
	CreditAccount string                 `json:"credit_account"`
}

// LabelRuleSource provides auto-labeling rules applied on transaction create.
type LabelRuleSource interface {
	ListRules() ([]domain.LabelRule, error)
}

//go:generate mockery --name=TransactionService --output=../handler/mock --outpkg=mock
type TransactionService interface {
	Create(input CreateTransactionInput) (*domain.Transaction, error)
	GetByID(id uint) (*domain.Transaction, error)
	Update(id uint, input UpdateTransactionInput) (*domain.Transaction, error)
	Delete(id uint) error
	DeleteBatch(ids []uint) error
	DeleteAll() error
	List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error)
	ListAll() ([]domain.Transaction, error)
	ListSince(since time.Time) ([]domain.Transaction, error)
	GetDistinctComments() ([]string, error)
	GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error)
}

type transactionService struct {
	repo    repository.TransactionRepository
	balSvc  BalanceService
	ruleSrc LabelRuleSource
}

func NewTransactionService(repo repository.TransactionRepository, balSvc BalanceService) TransactionService {
	return &transactionService{repo: repo, balSvc: balSvc}
}

// NewTransactionServiceWithRules also auto-applies label rules on create.
func NewTransactionServiceWithRules(repo repository.TransactionRepository, balSvc BalanceService, ruleSrc LabelRuleSource) TransactionService {
	return &transactionService{repo: repo, balSvc: balSvc, ruleSrc: ruleSrc}
}

func populateTx(tx *domain.Transaction) {
	tx.AmountMoney = domain.Money{Value: tx.Amount, Currency: domain.CurrencyEUR}
}

func populateTxs(txs []domain.Transaction) {
	for i := range txs {
		populateTx(&txs[i])
	}
}

func (s *transactionService) Create(input CreateTransactionInput) (*domain.Transaction, error) {
	date, err := timeutil.ParseDate(input.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}

	tx := &domain.Transaction{
		Date:          date,
		Type:          input.Type,
		Amount:        input.Amount,
		Comment:       input.Comment,
		Category:      input.Category,
		Labels:        domain.NormalizeLabels(input.Labels),
		DebitAccount:  input.DebitAccount,
		CreditAccount: input.CreditAccount,
	}

	// Auto-apply label rules (e.g. Finance + "loan" → loan) — except those
	// the user explicitly dismissed in the form for this transaction.
	if s.ruleSrc != nil {
		suppressed := map[string]bool{}
		for _, l := range strings.Split(domain.NormalizeLabels(input.SuppressedLabels), ",") {
			if l != "" {
				suppressed[l] = true
			}
		}
		if rules, err := s.ruleSrc.ListRules(); err == nil {
			for _, rule := range rules {
				if rule.Matches(tx) && !suppressed[rule.Label] {
					tx.AddLabel(rule.Label)
				}
			}
		}
	}

	if err := s.repo.Create(tx); err != nil {
		return nil, err
	}
	populateTx(tx)
	if s.balSvc != nil {
		_ = s.balSvc.SnapshotFromTransaction(tx)
	}
	return tx, nil
}

func (s *transactionService) GetByID(id uint) (*domain.Transaction, error) {
	tx, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	populateTx(tx)
	return tx, nil
}

func (s *transactionService) Update(id uint, input UpdateTransactionInput) (*domain.Transaction, error) {
	tx, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Remember which rules matched the row BEFORE the edit: a rule that
	// matched then and still matches now must NOT re-add its label — the
	// user removing it is an explicit decision. Only rules that start
	// matching because of this edit (changed comment/category) may add.
	matchedBefore := map[string]bool{}
	if s.ruleSrc != nil {
		if rules, err := s.ruleSrc.ListRules(); err == nil {
			for _, rule := range rules {
				if rule.Matches(tx) {
					matchedBefore[rule.Label+"|"+rule.Category+"|"+rule.CommentMatch] = true
				}
			}
		}
	}

	if input.Date != "" {
		date, err := timeutil.ParseDate(input.Date)
		if err != nil {
			return nil, errors.New("invalid date format, use YYYY-MM-DD")
		}
		tx.Date = date
	}
	if input.Type != "" {
		tx.Type = input.Type
	}
	if input.Amount > 0 {
		tx.Amount = input.Amount
	}
	if input.Comment != "" {
		tx.Comment = input.Comment
	}
	if input.Category != "" {
		tx.Category = input.Category
	}
	if input.Labels != nil {
		tx.Labels = domain.NormalizeLabels(*input.Labels)
	}
	tx.DebitAccount = input.DebitAccount
	tx.CreditAccount = input.CreditAccount

	// Deterministic labeling on edits: only rules that NEWLY match (because
	// the comment/category changed) add their label. Rules that already
	// matched before the edit stay silent, so explicit label removals stick.
	if s.ruleSrc != nil {
		if rules, err := s.ruleSrc.ListRules(); err == nil {
			for _, rule := range rules {
				key := rule.Label + "|" + rule.Category + "|" + rule.CommentMatch
				if rule.Matches(tx) && !matchedBefore[key] {
					tx.AddLabel(rule.Label)
				}
			}
		}
	}

	if err := s.repo.Update(tx); err != nil {
		return nil, err
	}
	populateTx(tx)
	return tx, nil
}

func (s *transactionService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *transactionService) DeleteBatch(ids []uint) error {
	return s.repo.DeleteBatch(ids)
}

func (s *transactionService) DeleteAll() error {
	return s.repo.DeleteAll()
}

func (s *transactionService) List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error) {
	result, err := s.repo.List(filter)
	if err != nil {
		return nil, err
	}
	populateTxs(result.Data)
	return result, nil
}

func (s *transactionService) ListAll() ([]domain.Transaction, error) {
	txs, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	populateTxs(txs)
	return txs, nil
}

func (s *transactionService) ListSince(since time.Time) ([]domain.Transaction, error) {
	txs, err := s.repo.ListSince(since)
	if err != nil {
		return nil, err
	}
	populateTxs(txs)
	return txs, nil
}

func (s *transactionService) GetDistinctComments() ([]string, error) {
	return s.repo.GetDistinctComments()
}

func (s *transactionService) GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error) {
	return s.repo.GetSummary(filter)
}
