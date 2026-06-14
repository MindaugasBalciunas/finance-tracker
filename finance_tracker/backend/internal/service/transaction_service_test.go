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

// stubBalanceSvc is a no-op BalanceService used in transaction service tests
// that only care about transaction behaviour, not balance projection.
type stubBalanceSvc struct{ testifymock.Mock }

func (s *stubBalanceSvc) RebuildAutoSnapshots(liveBtcPrice float64) error {
	return s.Called(liveBtcPrice).Error(0)
}
func (s *stubBalanceSvc) Create(_ service.CreateBalanceInput) (*domain.Balance, error) { return nil, nil }
func (s *stubBalanceSvc) GetByID(_ uint) (*domain.Balance, error)                      { return nil, nil }
func (s *stubBalanceSvc) Update(_ uint, _ service.UpdateBalanceInput) (*domain.Balance, error) {
	return nil, nil
}
func (s *stubBalanceSvc) Delete(_ uint) error                                              { return nil }
func (s *stubBalanceSvc) DeleteAll() error                                                 { return nil }
func (s *stubBalanceSvc) List(_ domain.BalanceFilter, _ float64) ([]domain.Balance, error) { return nil, nil }
func (s *stubBalanceSvc) GetLatest(_ float64) (*domain.Balance, error)                    { return nil, nil }
func (s *stubBalanceSvc) GetProjected(_ float64) (*domain.Balance, error)                 { return nil, nil }
func (s *stubBalanceSvc) GetTrend(_ domain.BalanceFilter) (*domain.BalanceTrend, error)   { return nil, nil }
func (s *stubBalanceSvc) GetAllocation() ([]domain.AccountAllocation, error)              { return nil, nil }

// newTxSvc builds a transactionService backed by a stub BalanceService.
// RebuildAutoSnapshots is pre-registered so it never panics on mutation paths;
// call balSvc.AssertExpectations(t) in a test to verify it was actually invoked.
func newTxSvc(repo *mock.TransactionRepository) (service.TransactionService, *stubBalanceSvc) {
	balSvc := &stubBalanceSvc{}
	balSvc.On("RebuildAutoSnapshots", float64(0)).Return(nil)
	return service.NewTransactionService(repo, balSvc), balSvc
}

func TestTransactionService_Create(t *testing.T) {
	t.Run("expense", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		input := service.CreateTransactionInput{
			Date:     "2026-01-15",
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Comment:  "Maxima food",
			Category: domain.CategoryFood,
		}
		repo.On("Create", &domain.Transaction{
			Date:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Comment:  "Maxima food",
			Category: domain.CategoryFood,
		}).Return(nil)

		tx, err := svc.Create(input)
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionTypeExpense, tx.Type)
		assert.Equal(t, 50.00, tx.Amount)
		assert.Equal(t, domain.CategoryFood, tx.Category)
		// Money type should be populated
		assert.Equal(t, 50.00, tx.AmountMoney.Value)
		assert.Equal(t, domain.CurrencyEUR, tx.AmountMoney.Currency)
		repo.AssertExpectations(t)
	})

	t.Run("income", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("Create", &domain.Transaction{
			Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Type:     domain.TransactionTypeIncome,
			Amount:   3500.00,
			Category: domain.CategorySalary,
		}).Return(nil)

		tx, err := svc.Create(service.CreateTransactionInput{
			Date:     "2026-02-01",
			Type:     domain.TransactionTypeIncome,
			Amount:   3500.00,
			Category: domain.CategorySalary,
		})
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionTypeIncome, tx.Type)
		assert.Equal(t, 3500.00, tx.Amount)
		repo.AssertExpectations(t)
	})

	t.Run("investment", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("Create", &domain.Transaction{
			Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Type:     domain.TransactionTypeInvestment,
			Amount:   500.00,
			Category: domain.CategoryStocksETF,
		}).Return(nil)

		tx, err := svc.Create(service.CreateTransactionInput{
			Date:     "2026-02-01",
			Type:     domain.TransactionTypeInvestment,
			Amount:   500.00,
			Category: domain.CategoryStocksETF,
		})
		require.NoError(t, err)
		assert.Equal(t, domain.TransactionTypeInvestment, tx.Type)
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		_, err := svc.Create(service.CreateTransactionInput{
			Date:     "not-a-date",
			Type:     domain.TransactionTypeExpense,
			Amount:   10,
			Category: domain.CategoryFood,
		})
		assert.ErrorContains(t, err, "invalid date format")
	})

	t.Run("repository error", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("Create", &domain.Transaction{
			Date:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Category: domain.CategoryFood,
		}).Return(errors.New("db error"))

		_, err := svc.Create(service.CreateTransactionInput{
			Date:     "2026-01-15",
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Category: domain.CategoryFood,
		})
		assert.ErrorContains(t, err, "db error")
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_GetByID(t *testing.T) {
	t.Run("found and Money populated", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		stored := &domain.Transaction{ID: 1, Amount: 100, Type: domain.TransactionTypeIncome}
		repo.On("GetByID", uint(1)).Return(stored, nil)

		tx, err := svc.GetByID(1)
		require.NoError(t, err)
		assert.Equal(t, uint(1), tx.ID)
		assert.Equal(t, 100.0, tx.AmountMoney.Value)
		assert.Equal(t, domain.CurrencyEUR, tx.AmountMoney.Currency)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.GetByID(99)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		existing := &domain.Transaction{ID: 1, Amount: 50, Type: domain.TransactionTypeExpense, Category: domain.CategoryFood}
		repo.On("GetByID", uint(1)).Return(existing, nil)

		updated := *existing
		updated.Amount = 75
		repo.On("Update", &updated).Return(nil)

		result, err := svc.Update(1, service.UpdateTransactionInput{Amount: 75})
		require.NoError(t, err)
		assert.Equal(t, 75.0, result.Amount)
		assert.Equal(t, 75.0, result.AmountMoney.Value)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.Update(99, service.UpdateTransactionInput{Amount: 10})
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("GetByID", uint(1)).Return(&domain.Transaction{ID: 1}, nil)
		repo.On("Delete", uint(1)).Return(nil)

		require.NoError(t, svc.Delete(1))
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		assert.Error(t, svc.Delete(99))
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_List(t *testing.T) {
	t.Run("populates AmountMoney for each transaction", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		filter := domain.TransactionFilter{Page: 1, PageSize: 10}
		repoResult := &domain.PaginatedTransactions{
			Data: []domain.Transaction{
				{ID: 1, Amount: 100, Type: domain.TransactionTypeExpense},
				{ID: 2, Amount: 3500, Type: domain.TransactionTypeIncome},
			},
			Total: 2,
		}
		repo.On("List", filter).Return(repoResult, nil)

		result, err := svc.List(filter)
		require.NoError(t, err)
		assert.Equal(t, 100.0, result.Data[0].AmountMoney.Value)
		assert.Equal(t, domain.CurrencyEUR, result.Data[0].AmountMoney.Currency)
		assert.Equal(t, 3500.0, result.Data[1].AmountMoney.Value)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_ListAll(t *testing.T) {
	t.Run("returns all transactions with Money populated", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		stored := []domain.Transaction{
			{ID: 1, Amount: 50, Type: domain.TransactionTypeExpense},
			{ID: 2, Amount: 200, Type: domain.TransactionTypeIncome},
		}
		repo.On("ListAll").Return(stored, nil)

		result, err := svc.ListAll()
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 50.0, result[0].AmountMoney.Value)
		assert.Equal(t, 200.0, result[1].AmountMoney.Value)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_ListSince(t *testing.T) {
	t.Run("returns transactions on or after the given date", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		stored := []domain.Transaction{
			{ID: 5, Amount: 80, Date: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)},
		}
		repo.On("ListSince", since).Return(stored, nil)

		result, err := svc.ListSince(since)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, uint(5), result[0].ID)
		assert.Equal(t, 80.0, result[0].AmountMoney.Value)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_GetSummary(t *testing.T) {
	t.Run("income increases net balance, expenses and investments decrease it", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		// Net = 5000 - 1200 - 800 = 3000
		filter := domain.TransactionFilter{}
		summary := &domain.TransactionSummary{
			TotalIncome:      5000,
			TotalExpenses:    1200,
			TotalInvestments: 800,
			NetBalance:       3000,
		}
		repo.On("GetSummary", filter).Return(summary, nil)

		result, err := svc.GetSummary(filter)
		require.NoError(t, err)
		assert.Equal(t, 5000.0, result.TotalIncome)
		assert.Equal(t, 1200.0, result.TotalExpenses)
		assert.Equal(t, 800.0, result.TotalInvestments)
		// Net balance: income minus expenses and investments
		assert.Equal(t, 3000.0, result.NetBalance)
		assert.Greater(t, result.TotalIncome, result.TotalExpenses+result.TotalInvestments)
		repo.AssertExpectations(t)
	})

	t.Run("negative net balance when expenses exceed income", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc, _ := newTxSvc(repo)

		filter := domain.TransactionFilter{}
		// Net = 1000 - 1500 - 0 = -500
		summary := &domain.TransactionSummary{
			TotalIncome:   1000,
			TotalExpenses: 1500,
			NetBalance:    -500,
		}
		repo.On("GetSummary", filter).Return(summary, nil)

		result, err := svc.GetSummary(filter)
		require.NoError(t, err)
		assert.Equal(t, -500.0, result.NetBalance)
		assert.Less(t, result.NetBalance, float64(0))
		repo.AssertExpectations(t)
	})
}
