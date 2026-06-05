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
func newSvc(balRepo *mock.BalanceRepository) service.BalanceService {
	return service.NewBalanceService(balRepo, &mock.TransactionRepository{})
}

func newSvcWithTx(balRepo *mock.BalanceRepository, txRepo *mock.TransactionRepository) service.BalanceService {
	return service.NewBalanceService(balRepo, txRepo)
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
		expectedTotal := 1000 + 2000 + 500 + 300 + 400 + 200 + 800 + 150 + 50 + 600 // = 6000
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

		// Cash=5000, RevStocks=1200, IBKRStocks=3000 → total = 9200
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

		// Cash=5000, RBTC=0.01 at 80000 = 800, IBKRStocks=2000 → total = 7800
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
		svc := newSvc(repo)

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
		updated.Total = 3000 // 2000 + 1000
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

		// Cash=3000, RevStocks=1000, IBKRStocks=2500 → total = 6500
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

	t.Run("IBKRStocks included in live-price total recalculation", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		svc := newSvc(repo)

		// Cash=10000, RBTC=0.01 at snapshot 70000, IBKRStocks=5000 → stored total=15700
		// Live price 80000 → RBTC = 0.01*80000 = 800 → new total = 10000 + 800 + 5000 = 15800
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

		// Total=10000: Cash=6000 (60%), IBKRStocks=4000 (40%)
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
// GetProjected tests
// ---------------------------------------------------------------------------

// snapDate and txDate are helpers to create UTC-midnight time.Time values.
func snapDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestBalanceService_GetProjected(t *testing.T) {
	t.Run("no snapshots returns empty balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		balRepo.On("GetLatestManual").Return(nil, errors.New("record not found"))

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 0.0, b.Total)
		assert.Equal(t, uint(0), b.ID)
		balRepo.AssertExpectations(t)
	})

	t.Run("no tagged transactions returns snapshot values unchanged", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Swed: 3000, Cash: 1000, Total: 4000,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 3000.0, b.Swed)
		assert.Equal(t, 1000.0, b.Cash)
		assert.Equal(t, 4000.0, b.Total)
		assert.Equal(t, uint(0), b.ID, "projected result must not carry snapshot ID")
		balRepo.AssertExpectations(t)
		txRepo.AssertExpectations(t)
	})

	t.Run("income transaction adds to tagged account", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Swed: 3000, Total: 3000,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{
				Date:          snapDate(2026, 5, 10),
				Type:          domain.TransactionTypeIncome,
				Amount:        1500,
				SourceAccount: "swed",
			},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 4500.0, b.Swed, "income must increase swed")
		assert.Equal(t, 4500.0, b.Total)
		balRepo.AssertExpectations(t)
		txRepo.AssertExpectations(t)
	})

	t.Run("expense transaction deducts from tagged account", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Cash: 2000, Total: 2000,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{
				Date:          snapDate(2026, 5, 5),
				Type:          domain.TransactionTypeExpense,
				Amount:        200,
				SourceAccount: "cash",
			},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 1800.0, b.Cash, "expense must decrease cash")
		assert.Equal(t, 1800.0, b.Total)
	})

	t.Run("investment transaction adds to tagged account", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			IBKRStocks: 5000, Total: 5000,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{
				Date:          snapDate(2026, 5, 15),
				Type:          domain.TransactionTypeInvestment,
				Amount:        1000,
				SourceAccount: "ibkr_stocks",
			},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 6000.0, b.IBKRStocks, "investment must increase ibkr_stocks")
		assert.Equal(t, 6000.0, b.Total)
	})

	t.Run("multiple transactions on the same day all apply", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Swed: 3000, Cash: 1000, Total: 4000,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		sameDay := snapDate(2026, 5, 20)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: sameDay, Type: domain.TransactionTypeIncome, Amount: 2000, SourceAccount: "swed"},
			{Date: sameDay, Type: domain.TransactionTypeExpense, Amount: 150, SourceAccount: "cash"},
			{Date: sameDay, Type: domain.TransactionTypeExpense, Amount: 50, SourceAccount: "cash"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		// swed: 3000 + 2000 = 5000
		// cash: 1000 - 150 - 50 = 800
		assert.Equal(t, 5000.0, b.Swed)
		assert.Equal(t, 800.0, b.Cash)
		assert.Equal(t, 5800.0, b.Total)
	})

	t.Run("transactions on snapshot date are included in projection", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snapD := snapDate(2026, 5, 1)
		snap := &domain.Balance{ID: 1, Date: snapD, Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snapD).Return([]domain.Transaction{
			// same day as snapshot — must be applied (snapshot may predate the transaction)
			{Date: snapD, Type: domain.TransactionTypeIncome, Amount: 500, SourceAccount: "swed"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 3500.0, b.Swed, "same-day tx must be applied to projected value")
		assert.Equal(t, 3500.0, b.Total)
	})

	t.Run("transactions strictly before snapshot date are skipped", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snapD := snapDate(2026, 5, 10)
		snap := &domain.Balance{ID: 1, Date: snapD, Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snapD).Return([]domain.Transaction{
			// day before snapshot — must be ignored (already captured in snapshot)
			{Date: snapDate(2026, 5, 9), Type: domain.TransactionTypeIncome, Amount: 500, SourceAccount: "swed"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 3000.0, b.Swed, "pre-snapshot tx must not change projected value")
		assert.Equal(t, 3000.0, b.Total)
	})

	t.Run("transactions without source_account are skipped", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Swed: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeIncome, Amount: 500, SourceAccount: ""},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 3000.0, b.Swed)
	})

	t.Run("all five account types route correctly", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Seb: 1000, Swed: 2000, RevM: 500, IBKRStocks: 3000, Cash: 800,
			Total: 7300,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		d := snapDate(2026, 5, 10)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 100, SourceAccount: "seb"},
			{Date: d, Type: domain.TransactionTypeIncome, Amount: 200, SourceAccount: "swed"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 50, SourceAccount: "rev_m"},
			{Date: d, Type: domain.TransactionTypeInvestment, Amount: 400, SourceAccount: "ibkr_stocks"},
			{Date: d, Type: domain.TransactionTypeExpense, Amount: 30, SourceAccount: "cash"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 900.0, b.Seb)        // 1000 - 100
		assert.Equal(t, 2200.0, b.Swed)      // 2000 + 200
		assert.Equal(t, 450.0, b.RevM)       // 500 - 50
		assert.Equal(t, 3400.0, b.IBKRStocks) // 3000 + 400
		assert.Equal(t, 770.0, b.Cash)       // 800 - 30
		// total = 900+2200+450+3400+770 = 7720
		assert.Equal(t, 7720.0, b.Total)
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
		d := snapDate(2026, 5, 10)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
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

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 900.0, b.Seb)         // 1000 - 100
		assert.Equal(t, 2200.0, b.Swed)       // 2000 + 200
		assert.Equal(t, 550.0, b.SwedETF)     // 500 + 50
		assert.Equal(t, 330.0, b.SebPen)      // 300 + 30
		assert.Equal(t, 380.0, b.Luminor)     // 400 - 20
		assert.Equal(t, 190.0, b.Art)         // 200 - 10
		assert.Equal(t, 450.0, b.RevM)        // 500 - 50
		assert.Equal(t, 325.0, b.RevR)        // 250 + 75
		assert.Equal(t, 3400.0, b.IBKRStocks) // 3000 + 400
		assert.Equal(t, 770.0, b.Cash)        // 800 - 30
		// total = 900+2200+550+330+380+190+450+325+3400+770 = 9495
		assert.Equal(t, 9495.0, b.Total)
	})

	t.Run("live BTC price recalculates total after applying deltas", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		// Cash=10000, RBTC=0.01 at snapshot price 70000 → stored total=10700
		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Cash: 10000, RBTC: 0.01, BtcPrice: 70000, Total: 10700,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{
				Date:          snapDate(2026, 5, 10),
				Type:          domain.TransactionTypeIncome,
				Amount:        500,
				SourceAccount: "cash",
			},
		}, nil)

		// Live BTC = 80000; cash → 10500; BTC = 0.01*80000 = 800 → total = 10500+800 = 11300
		b, err := svc.GetProjected(80000)
		require.NoError(t, err)
		assert.Equal(t, 10500.0, b.Cash)
		assert.Equal(t, 800.0, b.RBtcEur)
		assert.Equal(t, 11300.0, b.Total)
	})

	t.Run("projected ID is always zero (not a stored snapshot)", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 42, Date: snapDate(2026, 5, 1), Cash: 1000, Total: 1000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, uint(0), b.ID)
	})

	t.Run("transaction repo error is propagated", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Total: 1000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return(nil, errors.New("db error"))

		_, err := svc.GetProjected(0)
		assert.Error(t, err)
	})

	// --- delete / update reflection ----------------------------------------
	// GetProjected reads whatever ListSince returns, so deleting a transaction
	// (absent from the list) or updating one (different values in the list)
	// is immediately reflected in the next projected balance calculation.

	t.Run("deleted expense is no longer deducted from projected balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Cash: 2000, Total: 2000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Expense was deleted — list is empty, so Cash stays at snapshot value.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 2000.0, b.Cash, "deleted expense must not reduce cash")
		assert.Equal(t, 2000.0, b.Total)
	})

	t.Run("deleted income is no longer added to projected balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Seb: 5000, Total: 5000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Income transaction was deleted — projected equals snapshot.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 5000.0, b.Seb, "deleted income must not increase seb")
		assert.Equal(t, 5000.0, b.Total)
	})

	t.Run("updated expense amount is reflected in projected balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Cash: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Expense was updated from 500 → 200; repo returns the updated record.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeExpense, Amount: 200, SourceAccount: "cash"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 2800.0, b.Cash, "updated (smaller) expense must deduct only new amount")
		assert.Equal(t, 2800.0, b.Total)
	})

	t.Run("updated income amount is reflected in projected balance", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Swed: 1000, Total: 1000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Income was updated from 300 → 700; repo returns the updated record.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 12), Type: domain.TransactionTypeIncome, Amount: 700, SourceAccount: "swed"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 1700.0, b.Swed, "updated (larger) income must add new amount")
		assert.Equal(t, 1700.0, b.Total)
	})

	t.Run("transaction type changed from expense to income reverses sign", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), RevM: 2000, Total: 2000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Same 400 amount, but type flipped to income → adds instead of deducts.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 8), Type: domain.TransactionTypeIncome, Amount: 400, SourceAccount: "rev_m"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 2400.0, b.RevM, "income must add; type change from expense must flip sign")
		assert.Equal(t, 2400.0, b.Total)
	})

	t.Run("transaction type changed from income to expense reverses sign", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Seb: 3000, Total: 3000}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Same 500 amount, but type flipped to expense → deducts instead of adds.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 9), Type: domain.TransactionTypeExpense, Amount: 500, SourceAccount: "seb"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 2500.0, b.Seb, "expense must deduct; type change from income must flip sign")
		assert.Equal(t, 2500.0, b.Total)
	})

	t.Run("transaction source_account change routes delta to new account only", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{
			ID: 1, Date: snapDate(2026, 5, 1),
			Seb: 1000, Cash: 500, Total: 1500,
		}
		balRepo.On("GetLatestManual").Return(snap, nil)

		// Expense was moved from seb → cash; repo returns the updated record.
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 7), Type: domain.TransactionTypeExpense, Amount: 300, SourceAccount: "cash"},
		}, nil)

		b, err := svc.GetProjected(0)
		require.NoError(t, err)
		assert.Equal(t, 1000.0, b.Seb, "seb must be unchanged after source_account update")
		assert.Equal(t, 200.0, b.Cash, "cash must absorb expense after source_account update")
		assert.Equal(t, 1200.0, b.Total)
	})
}

// ---------------------------------------------------------------------------
// UpsertProjected tests
// ---------------------------------------------------------------------------

func TestBalanceService_UpsertProjected(t *testing.T) {
	t.Run("persists projected balance as auto snapshot for today", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Cash: 2000, Total: 2000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{
			{Date: snapDate(2026, 5, 10), Type: domain.TransactionTypeIncome, Amount: 500, SourceAccount: "cash"},
		}, nil)
		balRepo.On("UpsertAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto && b.Cash == 2500 && b.Total == 2500
		})).Return(nil)

		require.NoError(t, svc.UpsertProjected(0))
		balRepo.AssertExpectations(t)
	})

	t.Run("skips upsert when no manual snapshot exists", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		balRepo.On("GetLatestManual").Return(nil, errors.New("record not found"))

		require.NoError(t, svc.UpsertProjected(0))
		balRepo.AssertNotCalled(t, "UpsertAuto")
	})

	t.Run("auto snapshot is marked is_auto=true", func(t *testing.T) {
		balRepo := &mock.BalanceRepository{}
		txRepo := &mock.TransactionRepository{}
		svc := newSvcWithTx(balRepo, txRepo)

		snap := &domain.Balance{ID: 1, Date: snapDate(2026, 5, 1), Seb: 1000, Total: 1000}
		balRepo.On("GetLatestManual").Return(snap, nil)
		txRepo.On("ListSince", snap.Date).Return([]domain.Transaction{}, nil)
		balRepo.On("UpsertAuto", testifymock.MatchedBy(func(b *domain.Balance) bool {
			return b.IsAuto == true
		})).Return(nil)

		require.NoError(t, svc.UpsertProjected(0))
		balRepo.AssertExpectations(t)
	})
}
