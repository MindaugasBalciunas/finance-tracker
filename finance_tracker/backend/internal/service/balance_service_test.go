package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSvc is a convenience helper so tests don't need a txRepo when they don't care about it.
// The .Maybe() on GetLatestManual handles the RebuildAutoSnapshots side-effect from Create/Update/Delete.
func newSvc(balRepo *mock.BalanceRepository) service.BalanceService {
	balRepo.On("GetLatestManual").Maybe().Return(nil, errors.New("no records"))
	return service.NewBalanceService(balRepo, &mock.TransactionRepository{})
}

func newSvcWithTx(balRepo *mock.BalanceRepository, txRepo *mock.TransactionRepository) service.BalanceService {
	return service.NewBalanceService(balRepo, txRepo)
}

// snapDate creates a UTC-midnight time.Time value.
func snapDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
			Date:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
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

	t.Run("IBKRStocks included in auto total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date:       "2026-03-01",
			Cash:       5000,
			RevStocks:  1200,
			IBKRStocks: 3000,
		}
		repo.On("Create", &domain.Balance{
			Date:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Cash:       5000,
			RevStocks:  1200,
			IBKRStocks: 3000,
			Total:      9200,
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, 9200.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("IBKRStocks and BTC both contribute to total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date:       "2026-03-01",
			Cash:       5000,
			RBTC:       0.01,
			BtcPrice:   80000,
			IBKRStocks: 2000,
		}
		repo.On("Create", &domain.Balance{
			Date:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Cash:       5000,
			RBTC:       0.01,
			BtcPrice:   80000,
			IBKRStocks: 2000,
			Total:      7800,
		}).Return(nil)

		b, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, 7800.0, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("BTC contribution: btcPrice * (rbtc + mbtc) added to total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date:     "2026-03-01",
			Cash:     5000,
			RBTC:     0.01,
			MBTC:     0.005,
			BtcPrice: 80000,
		}
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
		svc := newSvc(repo)

		input := service.CreateBalanceInput{
			Date:  "2026-03-01",
			Cash:  1000,
			Total: 9999,
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
		svc := newSvc(repo)

		_, err := svc.Create(service.CreateBalanceInput{Date: "bad-date"})
		assert.ErrorContains(t, err, "invalid date format")
	})
}

func TestBalanceService_Update(t *testing.T) {
	t.Run("recalculates total from updated accounts", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		existing := &domain.Balance{ID: 1, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Seb: 1000, Cash: 500, Total: 1500}
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

	t.Run("recalculates total with BTC when accounts change", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		existing := &domain.Balance{ID: 2, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		repo.On("GetByID", uint(2)).Return(existing, nil)

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
		svc := newSvc(repo)

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

	t.Run("IBKRStocks included in updated total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		existing := &domain.Balance{ID: 4, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		repo.On("GetByID", uint(4)).Return(existing, nil)

		updated := *existing
		updated.Cash = 3000
		updated.RevStocks = 1000
		updated.IBKRStocks = 2500
		updated.Total = 6500
		repo.On("Update", &updated).Return(nil)

		b, err := svc.Update(4, service.UpdateBalanceInput{Cash: 3000, RevStocks: 1000, IBKRStocks: 2500})
		require.NoError(t, err)
		assert.Equal(t, 6500.0, b.Total)
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

		expected := &domain.Balance{ID: 5, Total: 36562.48, Date: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)}
		repo.On("GetLatest").Return(expected, nil)

		b, err := svc.GetLatest(0)
		require.NoError(t, err)
		assert.Equal(t, 36562.48, b.Total)
		repo.AssertExpectations(t)
	})

	t.Run("live BTC price recalculates total", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		stored := &domain.Balance{
			Cash:     10000,
			RBTC:     0.01,
			BtcPrice: 70000,
			Total:    10700,
		}
		repo.On("GetLatest").Return(stored, nil)

		b, err := svc.GetLatest(80000)
		require.NoError(t, err)
		assert.Equal(t, 10800.0, b.Total)
		assert.Equal(t, 800.0, b.RBtcEur)
		repo.AssertExpectations(t)
	})

	t.Run("IBKRStocks included in live-price total recalculation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		stored := &domain.Balance{
			Cash:       10000,
			RBTC:       0.01,
			BtcPrice:   70000,
			IBKRStocks: 5000,
			Total:      15700,
		}
		repo.On("GetLatest").Return(stored, nil)

		b, err := svc.GetLatest(80000)
		require.NoError(t, err)
		assert.Equal(t, 15800.0, b.Total)
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

	t.Run("zero live price leaves totals unchanged", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

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
		svc := newSvc(repo)

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
		svc := newSvc(repo)

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
		assert.InDelta(t, 13.79, btcAlloc.Percentage, 0.01)
		repo.AssertExpectations(t)
	})

	t.Run("zero accounts are excluded from allocation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(&domain.Balance{Total: 1000, Cash: 1000}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 1)
		assert.Equal(t, "Cash", allocations[0].Account)
		repo.AssertExpectations(t)
	})

	t.Run("IBKRStocks appears in allocation with correct percentage", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(&domain.Balance{
			Total:      10000,
			Cash:       6000,
			IBKRStocks: 4000,
		}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 2)

		alloc := map[string]domain.AccountAllocation{}
		for _, a := range allocations {
			alloc[a.Account] = a
		}
		require.Contains(t, alloc, "IBKR stocks")
		assert.Equal(t, 4000.0, alloc["IBKR stocks"].Amount)
		assert.InDelta(t, 40.0, alloc["IBKR stocks"].Percentage, 0.01)
		assert.InDelta(t, 60.0, alloc["Cash"].Percentage, 0.01)
		repo.AssertExpectations(t)
	})

	t.Run("IBKRStocks zero is excluded from allocation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		repo.On("GetLatest").Return(&domain.Balance{Total: 1000, Cash: 1000, IBKRStocks: 0}, nil)

		allocations, err := svc.GetAllocation()
		require.NoError(t, err)
		assert.Len(t, allocations, 1)
		assert.Equal(t, "Cash", allocations[0].Account)
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

// ---------------------------------------------------------------------------
// GetProjected tests — now a thin wrapper around GetLatest + BTC adjustment
// ---------------------------------------------------------------------------

func TestBalanceService_GetProjected(t *testing.T) {
	t.Run("no snapshots returns empty balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		svc := newSvcWithTx(balRepo, &mock.TransactionRepository{})

		balRepo.On("GetLatest").Return(nil, errors.New("record not found"))

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 0.0, b.Total)
		assert.Equal(t, uint(0), b.ID)
		balRepo.AssertExpectations(t)
	})

	t.Run("returns latest balance with ID zeroed out", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		svc := newSvcWithTx(balRepo, &mock.TransactionRepository{})

		snap := &domain.Balance{ID: 42, Date: snapDate(2026, 6, 13), Swed: 3507.14, Cash: 500, Total: 4007.14}
		balRepo.On("GetLatest").Return(snap, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, uint(0), b.ID, "projected must not carry snapshot ID")
		assert.Equal(t, 3507.14, b.Swed)
		assert.Equal(t, 4007.14, b.Total)
		balRepo.AssertExpectations(t)
	})

	t.Run("live BTC price is applied", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		svc := newSvcWithTx(balRepo, &mock.TransactionRepository{})

		snap := &domain.Balance{
			ID: 5, Date: snapDate(2026, 6, 13),
			Cash: 10000, RBTC: 0.01, BtcPrice: 70000, Total: 10700,
		}
		balRepo.On("GetLatest").Return(snap, nil)

		// Live 80000 → 0.01*80000 = 800 → total = 10000+800 = 10800
		b, err := svc.GetProjected(80000)
		require.NoError(t, err)
		assert.Equal(t, 10800.0, b.Total)
		assert.Equal(t, 800.0, b.RBtcEur)
		assert.Equal(t, uint(0), b.ID)
		balRepo.AssertExpectations(t)
	})

	t.Run("returns auto snapshot when it is the most recent", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		svc := newSvcWithTx(balRepo, &mock.TransactionRepository{})

		// Auto snapshot created by RebuildAutoSnapshots after an expense
		autoSnap := &domain.Balance{
			ID: 10, IsAuto: true,
			Date: snapDate(2026, 6, 14),
			Swed: 3479.70, Cash: 500, Total: 3979.70,
		}
		balRepo.On("GetLatest").Return(autoSnap, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 3479.70, b.Swed)
		assert.Equal(t, uint(0), b.ID)
		balRepo.AssertExpectations(t)
	})
}

// ---------------------------------------------------------------------------
// RebuildAutoSnapshots tests
// ---------------------------------------------------------------------------

func TestBalanceService_RebuildAutoSnapshots(t *testing.T) {
	t.Run("no manual snapshot — returns nil without touching repo", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		balRepo.On("GetLatestManual").Return(nil, errors.New("record not found"))

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertNotCalled(t, "DeleteAllAuto")
		balRepo.AssertNotCalled(t, "CreateAuto")
	})

	t.Run("no transactions after snapshot — deletes old autos, creates nothing", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 6, 13), Swed: 3507.14, Total: 3507.14}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{}, nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertNotCalled(t, "CreateAuto")
		balRepo.AssertExpectations(t)
	})

	t.Run("expense creates one auto snapshot with deducted amount", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 6, 13), Swed: 3507.14, Total: 3507.14}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{
				Date:          snapDate(2026, 6, 14),
				Type:          domain.TransactionTypeExpense,
				Amount:        27.44,
				SourceAccount: "swed",
			},
		}, nil)
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Swed == 3479.70 && b.Total == 3479.70
		})).Return(nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertExpectations(t)
	})

	t.Run("income creates auto snapshot with added amount", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeIncome, Amount: 1500, SourceAccount: "swed"},
		}, nil)
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Swed == 4500 && b.Total == 4500
		})).Return(nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertExpectations(t)
	})

	t.Run("two transactions create two auto snapshots (one per tx)", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Cash: 1000, Swed: 2000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeExpense, Amount: 200, SourceAccount: "cash"},
			{Date: snapDate(2026, 5, 15), Type: domain.TransactionTypeIncome, Amount: 500, SourceAccount: "swed"},
		}, nil)
		// After tx1: cash=800, swed=2000, total=2800
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Cash == 800 && b.Swed == 2000 && b.Total == 2800
		})).Return(nil).Once()
		// After tx2: cash=800, swed=2500, total=3300
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Cash == 800 && b.Swed == 2500 && b.Total == 3300
		})).Return(nil).Once()

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertExpectations(t)
	})

	t.Run("transactions on snapshot date are excluded (strictly after)", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snapD := snapDate(2026, 5, 1)
		snap := &domain.Balance{ID: 1, Date: snapD, Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		// ListStrictlyAfter with date > snapD means same-day txs are NOT returned
		txRepo.On("ListStrictlyAfter", snapD).Return([]domain.Transaction{}, nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertNotCalled(t, "CreateAuto")
		balRepo.AssertExpectations(t)
	})

	t.Run("transactions without account info are skipped", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeExpense, Amount: 500, SourceAccount: ""},
		}, nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertNotCalled(t, "CreateAuto")
		balRepo.AssertExpectations(t)
	})

	t.Run("all ten account types route correctly", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Seb: 1000, Swed: 2000, SwedETF: 500, SebPen: 300, Luminor: 400,
			Art: 200, RevM: 500, RevR: 250, IBKRStocks: 3000, Cash: 800,
			Total: 8950,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		d := snapDate(2026, 5, 10)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 100, SourceAccount: "seb"},
			{Date: d, Type: domain.TransactionTypeIncome, Amount: 200, SourceAccount: "swed"},
			{Date: d, Type: domain.TransactionTypeInvestment, Amount: 50, SourceAccount: "swed_etf"},
			{Date: d, Type: domain.TransactionTypeInvestment, Amount: 30, SourceAccount: "seb_pen"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 20, SourceAccount: "luminor"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 10, SourceAccount: "art"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 50, SourceAccount: "rev_m"},
			{Date: d, Type: domain.TransactionTypeIncome, Amount: 75, SourceAccount: "rev_r"},
			{Date: d, Type: domain.TransactionTypeInvestment, Amount: 400, SourceAccount: "ibkr_stocks"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 30, SourceAccount: "cash"},
		}, nil)
		// Each tx creates one auto snapshot; only verify the final state
		balRepo.On("CreateAuto", testifymock.Anything).Return(nil).Times(10)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertExpectations(t)

		// Verify final snapshot values via the last CreateAuto call
		calls := balRepo.Calls
		var lastCreate *testifymock.Call
		for i := range calls {
			if calls[i].Method == "CreateAuto" {
				lastCreate = &calls[i]
			}
		}
		require.NotNil(t, lastCreate)
		b := lastCreate.Arguments.Get(0).(*domain.Balance)
		assert.Equal(t, 900.0, b.Seb)
		assert.Equal(t, 2200.0, b.Swed)
		assert.Equal(t, 550.0, b.SwedETF)
		assert.Equal(t, 330.0, b.SebPen)
		assert.Equal(t, 380.0, b.Luminor)
		assert.Equal(t, 190.0, b.Art)
		assert.Equal(t, 450.0, b.RevM)
		assert.Equal(t, 325.0, b.RevR)
		assert.Equal(t, 3400.0, b.IBKRStocks)
		assert.Equal(t, 770.0, b.Cash)
	})

	t.Run("auto snapshot date matches transaction date", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Cash: 1000, Total: 1000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txDate := snapDate(2026, 6, 14)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: txDate, Type: domain.TransactionTypeExpense, Amount: 50, SourceAccount: "cash"},
		}, nil)
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.Date.Equal(txDate)
		})).Return(nil)

		require.NoError(t, svc.RebuildAutoSnapshots(0))
		balRepo.AssertExpectations(t)
	})

	t.Run("live BTC price is applied to each auto snapshot", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Cash: 10000, RBTC: 0.01, BtcPrice: 70000, Total: 10700,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		balRepo.On("DeleteAllAuto").Return(nil)
		txRepo.On("ListStrictlyAfter", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeExpense, Amount: 200, SourceAccount: "cash"},
		}, nil)
		// cash=9800, RBTC=0.01 at live 80000 → btcEur=800 → total=10600
		balRepo.On("CreateAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Cash == 9800 && b.Total == 10600
		})).Return(nil)

		require.NoError(t, svc.RebuildAutoSnapshots(80000))
		balRepo.AssertExpectations(t)
	})
}

// ---------------------------------------------------------------------------
// E2E: transaction before snapshot is ignored
// ---------------------------------------------------------------------------

func TestE2E_TransactionBeforeSnapshotIsIgnored(t *testing.T) {
	balRepo := &mock.BalanceRepository{}
	txRepo := &mock.TransactionRepository{}
	svc := newSvcWithTx(balRepo, txRepo)

	manualSnap := &domain.Balance{
		ID: 1, Date: snapDate(2026, 2, 1),
		Swed: 5000, Total: 5000,
	}
	balRepo.On("GetLatestManual").Return(manualSnap, nil)
	balRepo.On("DeleteAllAuto").Return(nil)
	// Jan 15 tx is NOT strictly after Feb 1 — repo returns empty list (correct behaviour)
	txRepo.On("ListStrictlyAfter", manualSnap.Date).Return([]domain.Transaction{}, nil)

	require.NoError(t, svc.RebuildAutoSnapshots(0))
	balRepo.AssertNotCalled(t, "CreateAuto")
	balRepo.AssertExpectations(t)
}
