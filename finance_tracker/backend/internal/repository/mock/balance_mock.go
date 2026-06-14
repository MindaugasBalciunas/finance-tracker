package mock

import (
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/mock"
)

type BalanceRepository struct {
	mock.Mock
}

func (m *BalanceRepository) Create(b *domain.Balance) error {
	args := m.Called(b)
	return args.Error(0)
}

func (m *BalanceRepository) GetByID(id uint) (*domain.Balance, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}

func (m *BalanceRepository) Update(b *domain.Balance) error {
	args := m.Called(b)
	return args.Error(0)
}

func (m *BalanceRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *BalanceRepository) DeleteAll() error {
	args := m.Called()
	return args.Error(0)
}

func (m *BalanceRepository) DeleteAllAuto() error {
	args := m.Called()
	return args.Error(0)
}

func (m *BalanceRepository) List(filter domain.BalanceFilter) ([]domain.Balance, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Balance), args.Error(1)
}

func (m *BalanceRepository) GetLatest() (*domain.Balance, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}

func (m *BalanceRepository) GetLatestManual() (*domain.Balance, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}

func (m *BalanceRepository) CreateAuto(b *domain.Balance) error {
	args := m.Called(b)
	return args.Error(0)
}

func (m *BalanceRepository) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.BalanceTrend), args.Error(1)
}
