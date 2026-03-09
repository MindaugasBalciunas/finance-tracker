package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceService_Create(t *testing.T) {
	t.Run("success with auto total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		input := service.CreateBalanceInput{
			Date: "2026-01-09",
			Seb:  4854,
			Swed: 4760,
			Cash: 12650,
		}

		repo.On("Create", &domain.Balance{
			Date:  time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC),
			Seb:   4854,
			Swed:  4760,
			Cash:  12650,
			Total: 22264,
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, 22264.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		_, err := svc.Create(service.CreateBalanceInput{Date: "bad-date"})
		assert.ErrorContains(t, err, "invalid date format")
	})
}

func TestBalanceService_GetLatest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		expected := &domain.Balance{ID: 5, Total: 36562.48, Date: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)}
		repo.On("GetLatest").Return(expected, nil)

		b, err := svc.GetLatest()
		require.NoError(t, err)
		assert.Equal(t, 36562.48, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("no records", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		_, err := svc.GetLatest()
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetAllocation(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := service.NewBalanceService(repo)

	latest := &domain.Balance{
		Total: 1000,
		Seb:   600,
		Cash:  400,
	}
	repo.On("GetLatest").Return(latest, nil)

	allocations, err := svc.GetAllocation()
	require.NoError(t, err)
	assert.Len(t, allocations, 2) // only non-zero accounts

	for _, a := range allocations {
		if a.Account == "Seb" {
			assert.Equal(t, 600.0, a.Amount)
			assert.Equal(t, 60.0, a.Percentage)
		}
		if a.Account == "Cash" {
			assert.Equal(t, 400.0, a.Amount)
			assert.Equal(t, 40.0, a.Percentage)
		}
	}
	repo.AssertExpectations(t)
}

func TestBalanceService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		existing := &domain.Balance{ID: 1}
		repo.On("GetByID", uint(1)).Return(existing, nil)
		repo.On("Delete", uint(1)).Return(nil)

		err := svc.Delete(1)
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		err := svc.Delete(99)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetTrend(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := service.NewBalanceService(repo)

	filter := domain.BalanceFilter{}
	expected := &domain.BalanceTrend{
		Dates:  []string{"2026-01-01", "2026-02-01"},
		Totals: []float64{30000, 35000},
	}
	repo.On("GetTrend", filter).Return(expected, nil)

	trend, err := svc.GetTrend(filter)
	require.NoError(t, err)
	assert.Equal(t, expected, trend)
	repo.AssertExpectations(t)
}
