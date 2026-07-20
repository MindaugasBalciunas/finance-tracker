package mock

import (
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/mock"
)

type AssetRepository struct {
	mock.Mock
}

func (m *AssetRepository) Create(a *domain.Asset) error {
	args := m.Called(a)
	return args.Error(0)
}

func (m *AssetRepository) GetByID(id uint) (*domain.Asset, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Asset), args.Error(1)
}

func (m *AssetRepository) Update(a *domain.Asset) error {
	args := m.Called(a)
	return args.Error(0)
}

func (m *AssetRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *AssetRepository) DeleteAll() error {
	args := m.Called()
	return args.Error(0)
}

func (m *AssetRepository) ListAll() ([]domain.Asset, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Asset), args.Error(1)
}

func (m *AssetRepository) ListSince(since time.Time) ([]domain.Asset, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Asset), args.Error(1)
}
