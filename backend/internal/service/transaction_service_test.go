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

func TestTransactionService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

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
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

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
		svc := service.NewTransactionService(repo)

		input := service.CreateTransactionInput{
			Date:     "2026-01-15",
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Category: domain.CategoryFood,
		}

		repo.On("Create", &domain.Transaction{
			Date:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Type:     domain.TransactionTypeExpense,
			Amount:   50.00,
			Category: domain.CategoryFood,
		}).Return(errors.New("db error"))

		_, err := svc.Create(input)
		assert.ErrorContains(t, err, "db error")
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_GetByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		expected := &domain.Transaction{ID: 1, Amount: 100, Type: domain.TransactionTypeIncome}
		repo.On("GetByID", uint(1)).Return(expected, nil)

		tx, err := svc.GetByID(1)
		require.NoError(t, err)
		assert.Equal(t, expected, tx)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.GetByID(99)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		existing := &domain.Transaction{ID: 1, Amount: 50, Type: domain.TransactionTypeExpense, Category: domain.CategoryFood}
		repo.On("GetByID", uint(1)).Return(existing, nil)

		updated := *existing
		updated.Amount = 75
		repo.On("Update", &updated).Return(nil)

		result, err := svc.Update(1, service.UpdateTransactionInput{Amount: 75})
		require.NoError(t, err)
		assert.Equal(t, 75.0, result.Amount)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.Update(99, service.UpdateTransactionInput{Amount: 10})
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		existing := &domain.Transaction{ID: 1}
		repo.On("GetByID", uint(1)).Return(existing, nil)
		repo.On("Delete", uint(1)).Return(nil)

		err := svc.Delete(1)
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.TransactionRepository{}
		svc := service.NewTransactionService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		err := svc.Delete(99)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestTransactionService_List(t *testing.T) {
	repo := &mock.TransactionRepository{}
	svc := service.NewTransactionService(repo)

	filter := domain.TransactionFilter{Page: 1, PageSize: 10}
	expected := &domain.PaginatedTransactions{
		Data:  []domain.Transaction{{ID: 1, Amount: 100}},
		Total: 1,
	}
	repo.On("List", filter).Return(expected, nil)

	result, err := svc.List(filter)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
	repo.AssertExpectations(t)
}

func TestTransactionService_GetSummary(t *testing.T) {
	repo := &mock.TransactionRepository{}
	svc := service.NewTransactionService(repo)

	filter := domain.TransactionFilter{}
	expected := &domain.TransactionSummary{TotalExpenses: 500, TotalIncome: 5000}
	repo.On("GetSummary", filter).Return(expected, nil)

	result, err := svc.GetSummary(filter)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
	repo.AssertExpectations(t)
}
