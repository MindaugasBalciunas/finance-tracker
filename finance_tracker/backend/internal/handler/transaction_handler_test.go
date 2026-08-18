package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockTransactionService implements service.TransactionService for handler tests.
type mockTransactionService struct{ mock.Mock }

func (m *mockTransactionService) Create(input service.CreateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transaction), args.Error(1)
}
func (m *mockTransactionService) GetByID(id uint) (*domain.Transaction, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transaction), args.Error(1)
}
func (m *mockTransactionService) Update(id uint, input service.UpdateTransactionInput) (*domain.Transaction, error) {
	args := m.Called(id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transaction), args.Error(1)
}
func (m *mockTransactionService) Delete(id uint) error {
	return m.Called(id).Error(0)
}
func (m *mockTransactionService) List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PaginatedTransactions), args.Error(1)
}
func (m *mockTransactionService) GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TransactionSummary), args.Error(1)
}
func (m *mockTransactionService) ListAll() ([]domain.Transaction, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Transaction), args.Error(1)
}
func (m *mockTransactionService) ListSince(since time.Time) ([]domain.Transaction, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Transaction), args.Error(1)
}
func (m *mockTransactionService) DeleteAll() error             { return m.Called().Error(0) }
func (m *mockTransactionService) DeleteBatch(ids []uint) error { return m.Called(ids).Error(0) }
func (m *mockTransactionService) GetDistinctComments() ([]string, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func setupTransactionRouter(svc service.TransactionService) *gin.Engine {
	r := gin.New()
	h := handler.NewTransactionHandler(svc)
	h.RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestTransactionHandler_Create(t *testing.T) {
	t.Run("201 on valid input", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		input := service.CreateTransactionInput{
			Date:     "2026-01-15",
			Type:     domain.TransactionTypeExpense,
			Amount:   88.91,
			Comment:  "Maxima",
			Category: domain.CategoryFood,
		}
		returned := &domain.Transaction{
			ID: 1, Date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Type: domain.TransactionTypeExpense, Amount: 88.91, Category: domain.CategoryFood,
			AmountMoney: domain.Money{Value: 88.91, Currency: domain.CurrencyEUR},
		}
		svc.On("Create", input).Return(returned, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var tx domain.Transaction
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tx))
		assert.Equal(t, uint(1), tx.ID)
		assert.Equal(t, 88.91, tx.AmountMoney.Value)
		svc.AssertExpectations(t)
	})

	t.Run("400 on missing fields", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTransactionHandler_GetByID(t *testing.T) {
	t.Run("200 found", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		tx := &domain.Transaction{ID: 5, Amount: 50, Type: domain.TransactionTypeExpense}
		svc.On("GetByID", uint(5)).Return(tx, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/5", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("404 not found", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		svc.On("GetByID", uint(99)).Return(nil, assert.AnError)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/99", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("400 bad id", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTransactionHandler_Delete(t *testing.T) {
	t.Run("204 deleted", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		svc.On("Delete", uint(3)).Return(nil)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/transactions/3", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("404 not found", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(svc)

		svc.On("Delete", uint(99)).Return(assert.AnError)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/transactions/99", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestTransactionHandler_List(t *testing.T) {
	svc := &mockTransactionService{}
	r := setupTransactionRouter(svc)

	expected := &domain.PaginatedTransactions{
		Data:       []domain.Transaction{{ID: 1, Amount: 100}},
		Total:      1,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}
	svc.On("List", mock.AnythingOfType("domain.TransactionFilter")).Return(expected, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
}

func TestTransactionHandler_GetSummary(t *testing.T) {
	svc := &mockTransactionService{}
	r := setupTransactionRouter(svc)

	expected := &domain.TransactionSummary{TotalExpenses: 500, TotalIncome: 5000}
	svc.On("GetSummary", mock.AnythingOfType("domain.TransactionFilter")).Return(expected, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var summary domain.TransactionSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.Equal(t, 500.0, summary.TotalExpenses)
	svc.AssertExpectations(t)
}
