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
	t.Run("auto total from three accounts", func(t *testing.T) {
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

	t.Run("auto total sums all accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		input := service.CreateBalanceInput{
			Date:      "2026-03-01",
			Seb:       1000,
			Swed:      2000,
			SwedETF:   500,
			SebPen:   300,
			Luminor:   400,
			Art:       200,
			Cash:      800,
			RevM:      150,
			RevR:      50,
			RevStocks: 600,
		}
		expectedTotal := 1000 + 2000 + 500 + 300 + 400 + 200 + 800 + 150 + 50 + 600 // = 6000
		repo.On("Create", &domain.Balance{
			Date:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Seb:       1000,
			Swed:      2000,
			SwedETF:   500,
			SebPen:   300,
			Luminor:   400,
			Art:       200,
			Cash:      800,
			RevM:      150,
			RevR:      50,
			RevStocks: 600,
			Total:     float64(expectedTotal),
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, float64(expectedTotal), b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("BTC contribution: btcPrice * (rbtc + mbtc) added to total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		// 0.01 BTC + 0.005 BTC at 80000 EUR/BTC = 1200 EUR BTC contribution
		input := service.CreateBalanceInput{
			Date:     "2026-03-01",
			Cash:     5000,
			RBTC:     0.01,
			MBTC:     0.005,
			BtcPrice: 80000,
		}
		// Total = 5000 + 80000*(0.01+0.005) = 5000 + 1200 = 6200
		repo.On("Create", &domain.Balance{
			Date:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Cash:     5000,
			RBTC:     0.01,
			MBTC:     0.005,
			BtcPrice: 80000,
			Total:    6200,
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, 6200.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("explicit total is not overridden", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		input := service.CreateBalanceInput{
			Date:  "2026-03-01",
			Cash:  1000,
			Total: 9999, // explicitly provided
		}
		repo.On("Create", &domain.Balance{
			Date:  time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Cash:  1000,
			Total: 9999,
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, 9999.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		_, err := svc.Create(service.CreateBalanceInput{Date: "bad-date"})
		assert.ErrorContains(t, err, "invalid date format")
	})
}

func TestBalanceService_Update(t *testing.T) {
	t.Run("recalculates total from updated accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		existing := &domain.Balance{ID: 1, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Seb: 1000, Cash: 500, Total: 1500}
		repo.On("GetByID", uint(1)).Return(existing, nil)

		updated := *existing
		updated.Seb = 2000
		updated.Cash = 1000
		updated.Total = 3000 // 2000 + 1000
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(1, service.UpdateBalanceInput{Seb: 2000, Cash: 1000})
		require.NoError(t, err)
		assert.Equal(t, 3000.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("recalculates total with BTC when accounts change", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		existing := &domain.Balance{ID: 2, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		repo.On("GetByID", uint(2)).Return(existing, nil)

		// Cash=5000, RBTC=0.01 at BtcPrice=80000 → total = 5000 + 800 = 5800
		updated := *existing
		updated.Cash = 5000
		updated.RBTC = 0.01
		updated.BtcPrice = 80000
		updated.Total = 5800
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(2, service.UpdateBalanceInput{Cash: 5000, RBTC: 0.01, BtcPrice: 80000})
		require.NoError(t, err)
		assert.Equal(t, 5800.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("explicit total overrides recalculation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		existing := &domain.Balance{ID: 3, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		repo.On("GetByID", uint(3)).Return(existing, nil)

		updated := *existing
		updated.Cash = 1000
		updated.Total = 9999
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(3, service.UpdateBalanceInput{Cash: 1000, Total: 9999})
		require.NoError(t, err)
		assert.Equal(t, 9999.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.Update(99, service.UpdateBalanceInput{Cash: 100})
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetLatest(t *testing.T) {
	t.Run("returns balance without live price adjustment", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		expected := &domain.Balance{ID: 5, Total: 36562.48, Date: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)}
		repo.On("GetLatest").Return(expected, nil)

		b, err := svc.GetLatest(0)
		require.NoError(t, err)
		assert.Equal(t, 36562.48, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("live BTC price recalculates total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		// Snapshot: Cash=10000, RBTC=0.01, snapshot BtcPrice=70000 → stored Total=10700
		stored := &domain.Balance{
			Cash:     10000,
			RBTC:     0.01,
			BtcPrice: 70000,
			Total:    10700,
		}
		repo.On("GetLatest").Return(stored, nil)

		// Live price is 80000 → RBTC contribution = 0.01*80000 = 800 → new total = 10800
		b, err := svc.GetLatest(80000)
		require.NoError(t, err)
		assert.Equal(t, 10800.0, b.Total)
		assert.Equal(t, 800.0, b.RBtcEur)
		repo.AssertExpectations(t)
	})

	t.Run("no records", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		_, err := svc.GetLatest(0)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_List_LiveBtcPrice(t *testing.T) {
	t.Run("live price recalculates each balance in list", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		balances := []domain.Balance{
			{Cash: 5000, RBTC: 0.01, BtcPrice: 70000, Total: 5700},
			{Cash: 6000, MBTC: 0.02, BtcPrice: 70000, Total: 7400},
		}
		filter := domain.BalanceFilter{}
		repo.On("List", filter).Return(balances, nil)

		// Live price 80000: recalculate
		// Row 1: 5000 + 0.01*80000 = 5000 + 800 = 5800
		// Row 2: 6000 + 0.02*80000 = 6000 + 1600 = 7600
		result, err := svc.List(filter, 80000)
		require.NoError(t, err)
		assert.InDelta(t, 5800.0, result[0].Total, 0.01)
		assert.InDelta(t, 7600.0, result[1].Total, 0.01)
		repo.AssertExpectations(t)
	})

	t.Run("zero live price leaves totals unchanged", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		balances := []domain.Balance{{Cash: 5000, Total: 5000}}
		filter := domain.BalanceFilter{}
		repo.On("List", filter).Return(balances, nil)

		result, err := svc.List(filter, 0)
		require.NoError(t, err)
		assert.Equal(t, 5000.0, result[0].Total)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetAllocation(t *testing.T) {
	t.Run("calculates percentages for non-zero accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetLatest").Return(&domain.Balance{Total: 1000, Seb: 600, Cash: 400}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 2)

		for _, a := range allocations {
			switch a.Account {
			case "Seb":
				assert.Equal(t, 600.0, a.Amount)
				assert.Equal(t, 60.0, a.Percentage)
			case "Cash":
				assert.Equal(t, 400.0, a.Amount)
				assert.Equal(t, 40.0, a.Percentage)
			}
		}
		repo.AssertExpectations(t)
	})

	t.Run("BTC holdings appear as EUR in allocation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		// 0.01 BTC at 80000 = 800 EUR BTC contribution
		repo.On("GetLatest").Return(&domain.Balance{
			Total:    5800,
			Cash:     5000,
			RBTC:     0.01,
			BtcPrice: 80000,
		}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)

		var btcAlloc *domain.AccountAllocation
		for i := range allocations {
			if allocations[i].Account == "Revolut R account BTC" {
				btcAlloc = &allocations[i]
			}
		}
		require.NotNil(t, btcAlloc, "BTC allocation should be present")
		assert.Equal(t, 800.0, btcAlloc.Amount)
		assert.InDelta(t, 13.79, btcAlloc.Percentage, 0.01) // 800/5800*100
		repo.AssertExpectations(t)
	})

	t.Run("zero accounts are excluded from allocation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetLatest").Return(&domain.Balance{Total: 1000, Cash: 1000}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 1)
		assert.Equal(t, "Cash", allocations[0].Account)
		repo.AssertExpectations(t)
	})

	t.Run("returns empty when no balance exists", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Empty(t, allocations)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetByID", uint(1)).Return(&domain.Balance{ID: 1}, nil)
		repo.On("Delete", uint(1)).Return(nil)

		require.NoError(t, svc.Delete(1))
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := service.NewBalanceService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		assert.Error(t, svc.Delete(99))
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
