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

type mockBalanceService struct{ mock.Mock }

func (m *mockBalanceService) Create(input service.CreateBalanceInput) (*domain.Balance, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}
func (m *mockBalanceService) GetByID(id uint) (*domain.Balance, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}
func (m *mockBalanceService) Update(id uint, input service.UpdateBalanceInput) (*domain.Balance, error) {
	args := m.Called(id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}
func (m *mockBalanceService) Delete(id uint) error { return m.Called(id).Error(0) }
func (m *mockBalanceService) List(filter domain.BalanceFilter, liveBtcPrice float64) ([]domain.Balance, error) {
	args := m.Called(filter, liveBtcPrice)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Balance), args.Error(1)
}
func (m *mockBalanceService) GetLatest(liveBtcPrice float64) (*domain.Balance, error) {
	args := m.Called(liveBtcPrice)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}
func (m *mockBalanceService) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.BalanceTrend), args.Error(1)
}
func (m *mockBalanceService) GetAllocation() ([]domain.AccountAllocation, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.AccountAllocation), args.Error(1)
}
func (m *mockBalanceService) DeleteAll() error { return m.Called().Error(0) }
func (m *mockBalanceService) GetProjected(liveBtcPrice float64) (*domain.Balance, error) {
	args := m.Called(liveBtcPrice)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Balance), args.Error(1)
}
func (m *mockBalanceService) RebuildAutoSnapshots(liveBtcPrice float64) error {
	return m.Called(liveBtcPrice).Error(0)
}

func setupBalanceRouter(svc service.BalanceService) *gin.Engine {
	r := gin.New()
	h := handler.NewBalanceHandler(svc)
	h.RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestBalanceHandler_Create(t *testing.T) {
	t.Run("201 on valid input", func(t *testing.T) {
		svc := &mockBalanceService{}
		r := setupBalanceRouter(svc)

		input := service.CreateBalanceInput{Date: "2026-03-06", Seb: 208.86, Cash: 13890}
		returned := &domain.Balance{
			ID: 1, Date: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
			Seb: 208.86, Cash: 13890, Total: 14098.86,
		}
		svc.On("Create", input).Return(returned, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/balances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var b domain.Balance
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, 14098.86, b.Total)
		svc.AssertExpectations(t)
	})
}

func TestBalanceHandler_GetLatest(t *testing.T) {
	t.Run("200 with latest", func(t *testing.T) {
		svc := &mockBalanceService{}
		r := setupBalanceRouter(svc)

		latest := &domain.Balance{ID: 31, Total: 36562.48}
		svc.On("GetLatest", float64(0)).Return(latest, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/balances/latest", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var b domain.Balance
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, 36562.48, b.Total)
		svc.AssertExpectations(t)
	})

	t.Run("404 when empty", func(t *testing.T) {
		svc := &mockBalanceService{}
		r := setupBalanceRouter(svc)

		svc.On("GetLatest", float64(0)).Return(nil, assert.AnError)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/balances/latest", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestBalanceHandler_GetAllocation(t *testing.T) {
	svc := &mockBalanceService{}
	r := setupBalanceRouter(svc)

	allocations := []domain.AccountAllocation{
		{Account: "Seb", Amount: 208.86, Percentage: 0.57},
		{Account: "Cash", Amount: 13890, Percentage: 37.96},
	}
	svc.On("GetAllocation").Return(allocations, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/balances/allocation", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []domain.AccountAllocation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 2)
	svc.AssertExpectations(t)
}

func TestBalanceHandler_Delete(t *testing.T) {
	t.Run("204 deleted", func(t *testing.T) {
		svc := &mockBalanceService{}
		r := setupBalanceRouter(svc)

		svc.On("Delete", uint(2)).Return(nil)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/balances/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestBalanceHandler_GetTrend(t *testing.T) {
	svc := &mockBalanceService{}
	r := setupBalanceRouter(svc)

	trend := &domain.BalanceTrend{
		Dates:  []string{"2026-01-01", "2026-02-01"},
		Totals: []float64{34000, 36000},
	}
	svc.On("GetTrend", mock.AnythingOfType("domain.BalanceFilter")).Return(trend, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/balances/trend", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result domain.BalanceTrend
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 2, len(result.Dates))
	svc.AssertExpectations(t)
}
