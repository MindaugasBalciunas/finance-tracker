package mock

import (
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/mock"
)

type StockRepository struct {
	mock.Mock
}

func (m *StockRepository) Create(t *domain.StockTrade) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *StockRepository) GetByID(id uint) (*domain.StockTrade, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StockTrade), args.Error(1)
}

func (m *StockRepository) Update(t *domain.StockTrade) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *StockRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *StockRepository) DeleteAll() error {
	args := m.Called()
	return args.Error(0)
}

func (m *StockRepository) ListAll() ([]domain.StockTrade, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.StockTrade), args.Error(1)
}

func (m *StockRepository) ListSince(since time.Time) ([]domain.StockTrade, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.StockTrade), args.Error(1)
}
