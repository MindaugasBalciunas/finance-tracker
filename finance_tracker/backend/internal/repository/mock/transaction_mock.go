package mock

import (
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/mock"
)

type TransactionRepository struct {
	mock.Mock
}

func (m *TransactionRepository) Create(tx *domain.Transaction) error {
	args := m.Called(tx)
	return args.Error(0)
}

func (m *TransactionRepository) GetByID(id uint) (*domain.Transaction, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transaction), args.Error(1)
}

func (m *TransactionRepository) Update(tx *domain.Transaction) error {
	args := m.Called(tx)
	return args.Error(0)
}

func (m *TransactionRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *TransactionRepository) List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaginatedTransactions), args.Error(1)
}

func (m *TransactionRepository) ListAll() ([]domain.Transaction, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Transaction), args.Error(1)
}

func (m *TransactionRepository) ListSince(since time.Time) ([]domain.Transaction, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Transaction), args.Error(1)
}

func (m *TransactionRepository) GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TransactionSummary), args.Error(1)
}

func (m *TransactionRepository) DeleteAll() error {
	args := m.Called()
	return args.Error(0)
}

func (m *TransactionRepository) DeleteBatch(ids []uint) error {
	args := m.Called(ids)
	return args.Error(0)
}

func (m *TransactionRepository) GetDistinctComments() ([]string, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}
