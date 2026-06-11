package service

import (
	"errors"
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
	DebitAccount  string                 `json:"debit_account"`
	CreditAccount string                 `json:"credit_account"`
}

// UpdateTransactionInput is the input DTO for updating a transaction
type UpdateTransactionInput struct {
	Date          string                 `json:"date"`
	Type          domain.TransactionType `json:"type" binding:"omitempty,oneof=expense income investment"`
	Amount        float64                `json:"amount" binding:"omitempty,gt=0"`
	Comment       string                 `json:"comment"`
	Category      domain.Category        `json:"category"`
	DebitAccount  string                 `json:"debit_account"`
	CreditAccount string                 `json:"credit_account"`
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
	repo   repository.TransactionRepository
	balSvc BalanceService
}

func NewTransactionService(repo repository.TransactionRepository, balSvc BalanceService) TransactionService {
	return &transactionService{repo: repo, balSvc: balSvc}
}

// refreshProjected recomputes and persists the auto balance snapshot after any mutation.
// Failures are non-fatal — the transaction itself is the source of truth.
func (s *transactionService) refreshProjected() {
	_ = s.balSvc.UpsertProjected(0)
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
		DebitAccount:  input.DebitAccount,
		CreditAccount: input.CreditAccount,
	}

	if err := s.repo.Create(tx); err != nil {
		return nil, err
	}
	populateTx(tx)
	s.refreshProjected()
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
	tx.DebitAccount = input.DebitAccount
	tx.CreditAccount = input.CreditAccount

	if err := s.repo.Update(tx); err != nil {
		return nil, err
	}
	populateTx(tx)
	s.refreshProjected()
	return tx, nil
}

func (s *transactionService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.refreshProjected()
	return nil
}

func (s *transactionService) DeleteBatch(ids []uint) error {
	if err := s.repo.DeleteBatch(ids); err != nil {
		return err
	}
	s.refreshProjected()
	return nil
}

func (s *transactionService) DeleteAll() error {
	if err := s.repo.DeleteAll(); err != nil {
		return err
	}
	s.refreshProjected()
	return nil
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
