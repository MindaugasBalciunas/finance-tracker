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

func newSvc(balRepo *mock.BalanceRepository) service.BalanceService {
	return service.NewBalanceService(balRepo, nil)
}

func snapDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func snapDatetime(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, time.UTC)
}

func TestBalanceService_Create(t *testing.T) {
	t.Run("auto total from three accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date: "2026-01-09",
			Seb:  4854,
			Swed: 4760,
			Cash: 12650,
		}
		repo.On("Create", &domain.Balance{
			Date:  snapDate(2026, 1, 9),
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

	t.Run("datetime with hour and minute is stored", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("Create", &domain.Balance{
			Date:  snapDatetime(2026, 6, 13, 17, 9),
			Swed:  3507.14,
			Total: 3507.14,
		}).Return(nil)

		b, err := svc.Create(service.CreateBalanceInput{
			Date: "2026-06-13T17:09",
			Swed: 3507.14,
		})
		require.NoError(t, err)
		assert.Equal(t, 17, b.Date.Hour())
		assert.Equal(t, 9, b.Date.Minute())
		repo.AssertExpectations(t)
	})

	t.Run("two entries same day different times both stored", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("Create", &domain.Balance{
			Date:  snapDatetime(2026, 6, 13, 17, 9),
			Swed:  3507.14,
			Total: 3507.14,
		}).Return(nil)
		repo.On("Create", &domain.Balance{
			Date:  snapDatetime(2026, 6, 13, 18, 0),
			Swed:  3480.00,
			Total: 3480.00,
		}).Return(nil)

		b1, err := svc.Create(service.CreateBalanceInput{Date: "2026-06-13T17:09", Swed: 3507.14})
		require.NoError(t, err)
		b2, err := svc.Create(service.CreateBalanceInput{Date: "2026-06-13T18:00", Swed: 3480.00})
		require.NoError(t, err)
		assert.NotEqual(t, b1.Date, b2.Date)
		repo.AssertExpectations(t)
	})

	t.Run("auto total sums all accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date:      "2026-03-01",
			Seb:       1000,
			Swed:      2000,
			SwedETF:   500,
			SebPen:    300,
			Luminor:   400,
			Art:       200,
			Cash:      800,
			RevM:      150,
			RevR:      50,
			RevStocks: 600,
		}
		expectedTotal := 1000 + 2000 + 500 + 300 + 400 + 200 + 800 + 150 + 50 + 600
		repo.On("Create", &domain.Balance{
			Date:      snapDate(2026, 3, 1),
			Seb:       1000,
			Swed:      2000,
			SwedETF:   500,
			SebPen:    300,
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

	t.Run("IBKRStocks and BTC both contribute to total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("Create", &domain.Balance{
			Date:       snapDate(2026, 3, 1),
			Cash:       5000,
			RBTC:       0.01,
			BtcPrice:   80000,
			IBKRStocks: 2000,
			Total:      7800,
		}).Return(nil)

		b, err := svc.Create(service.CreateBalanceInput{
			Date: "2026-03-01", Cash: 5000, RBTC: 0.01, BtcPrice: 80000, IBKRStocks: 2000,
		})
		require.NoError(t, err)
		assert.Equal(t, 7800.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("explicit total is not overridden", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("Create", &domain.Balance{
			Date:  snapDate(2026, 3, 1),
			Cash:  1000,
			Total: 9999,
		}).Return(nil)

		b, err := svc.Create(service.CreateBalanceInput{Date: "2026-03-01", Cash: 1000, Total: 9999})
		require.NoError(t, err)
		assert.Equal(t, 9999.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		_, err := svc.Create(service.CreateBalanceInput{Date: "bad-date"})
		assert.ErrorContains(t, err, "invalid date format")
	})
}

func TestBalanceService_Update(t *testing.T) {
	t.Run("recalculates total from updated accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		existing := &domain.Balance{ID: 1, Date: snapDate(2026, 1, 1), Seb: 1000, Cash: 500, Total: 1500}
		repo.On("GetByID", uint(1)).Return(existing, nil)

		updated := *existing
		updated.Seb = 2000
		updated.Cash = 1000
		updated.Total = 3000
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(1, service.UpdateBalanceInput{Seb: 2000, Cash: 1000})
		require.NoError(t, err)
		assert.Equal(t, 3000.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("datetime string updates date with time", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		existing := &domain.Balance{ID: 2, Date: snapDate(2026, 1, 1)}
		repo.On("GetByID", uint(2)).Return(existing, nil)

		updated := *existing
		updated.Date = snapDatetime(2026, 6, 14, 9, 30)
		updated.Swed = 3479.70
		updated.Total = 3479.70
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(2, service.UpdateBalanceInput{Date: "2026-06-14T09:30", Swed: 3479.70})
		require.NoError(t, err)
		assert.Equal(t, 9, b.Date.Hour())
		assert.Equal(t, 30, b.Date.Minute())
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.Update(99, service.UpdateBalanceInput{Cash: 100})
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetLatest(t *testing.T) {
	t.Run("returns balance without live price adjustment", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		expected := &domain.Balance{ID: 5, Total: 36562.48, Date: snapDate(2026, 3, 6)}
		repo.On("GetLatest").Return(expected, nil)

		b, err := svc.GetLatest(0)
		require.NoError(t, err)
		assert.Equal(t, 36562.48, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("live BTC price recalculates total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		stored := &domain.Balance{Cash: 10000, RBTC: 0.01, BtcPrice: 70000, Total: 10700}
		repo.On("GetLatest").Return(stored, nil)

		b, err := svc.GetLatest(80000)
		require.NoError(t, err)
		assert.Equal(t, 10800.0, b.Total)
		assert.Equal(t, 800.0, b.RBtcEur)
		repo.AssertExpectations(t)
	})

	t.Run("no records", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		_, err := svc.GetLatest(0)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetProjected(t *testing.T) {
	t.Run("returns latest snapshot with ID zeroed", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		snap := &domain.Balance{ID: 42, Date: snapDatetime(2026, 6, 13, 17, 9), Swed: 3507.14, Total: 3507.14}
		repo.On("GetLatest").Return(snap, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, uint(0), b.ID)
		assert.Equal(t, 3507.14, b.Swed)
		assert.Equal(t, 3507.14, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("no records returns empty balance", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 0.0, b.Total)
		assert.Equal(t, uint(0), b.ID)
		repo.AssertExpectations(t)
	})

	t.Run("live BTC price is applied", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		snap := &domain.Balance{ID: 5, Cash: 10000, RBTC: 0.01, BtcPrice: 70000, Total: 10700}
		repo.On("GetLatest").Return(snap, nil)

		b, err := svc.GetProjected(80000)
		require.NoError(t, err)
		assert.Equal(t, 10800.0, b.Total)
		assert.Equal(t, 800.0, b.RBtcEur)
		assert.Equal(t, uint(0), b.ID)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_List_LiveBtcPrice(t *testing.T) {
	t.Run("live price recalculates each balance in list", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		balances := []domain.Balance{
			{Cash: 5000, RBTC: 0.01, BtcPrice: 70000, Total: 5700},
			{Cash: 6000, MBTC: 0.02, BtcPrice: 70000, Total: 7400},
		}
		filter := domain.BalanceFilter{}
		repo.On("List", filter).Return(balances, nil)

		result, err := svc.List(filter, 80000)
		require.NoError(t, err)
		assert.InDelta(t, 5800.0, result[0].Total, 0.01)
		assert.InDelta(t, 7600.0, result[1].Total, 0.01)
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetByID", uint(1)).Return(&domain.Balance{ID: 1}, nil)
		repo.On("Delete", uint(1)).Return(nil)

		require.NoError(t, svc.Delete(1))
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		assert.Error(t, svc.Delete(99))
		repo.AssertExpectations(t)
	})
}

func TestBalanceService_GetTrend(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := newSvc(repo)

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

func TestBalanceService_GetAllocation(t *testing.T) {
	t.Run("calculates percentages for non-zero accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(&domain.Balance{Total: 1000, Seb: 600, Cash: 400}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 2)
		repo.AssertExpectations(t)
	})

	t.Run("returns empty when no balance exists", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(nil, errors.New("record not found"))

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Empty(t, allocations)
		repo.AssertExpectations(t)
	})
}
