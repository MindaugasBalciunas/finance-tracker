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

type mockStockService struct{ mock.Mock }

func (m *mockStockService) Create(input service.CreateStockTradeInput) (*domain.StockTrade, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StockTrade), args.Error(1)
}
func (m *mockStockService) GetByID(id uint) (*domain.StockTrade, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StockTrade), args.Error(1)
}
func (m *mockStockService) Update(id uint, input service.UpdateStockTradeInput) (*domain.StockTrade, error) {
	args := m.Called(id, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StockTrade), args.Error(1)
}
func (m *mockStockService) Delete(id uint) error { return m.Called(id).Error(0) }
func (m *mockStockService) ListAll() ([]domain.StockTrade, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.StockTrade), args.Error(1)
}
func (m *mockStockService) ListSince(since time.Time) ([]domain.StockTrade, error) {
	args := m.Called(since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.StockTrade), args.Error(1)
}
func (m *mockStockService) GetPortfolio() (*domain.StockPortfolio, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StockPortfolio), args.Error(1)
}

func setupStockRouter(svc service.StockService) *gin.Engine {
	r := gin.New()
	h := handler.NewStockHandler(svc)
	h.RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestStockHandler_Create(t *testing.T) {
	t.Run("201 on valid buy trade", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		input := service.CreateStockTradeInput{
			Date:          "2026-01-10",
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        10,
			PricePerShare: 220.50,
		}
		returned := &domain.StockTrade{
			ID: 1, Ticker: "AAPL", Action: domain.StockActionBuy,
			Shares: 10, PricePerShare: 220.50, Currency: "USD",
		}
		svc.On("Create", input).Return(returned, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stocks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var trade domain.StockTrade
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &trade))
		assert.Equal(t, "AAPL", trade.Ticker)
		assert.Equal(t, 10.0, trade.Shares)
		svc.AssertExpectations(t)
	})

	t.Run("400 on missing required fields", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/stocks", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestStockHandler_GetByID(t *testing.T) {
	t.Run("200 found", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		trade := &domain.StockTrade{ID: 3, Ticker: "MSFT", Shares: 5}
		svc.On("GetByID", uint(3)).Return(trade, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/3", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("404 not found", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		svc.On("GetByID", uint(99)).Return(nil, assert.AnError)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/99", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("400 bad id", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestStockHandler_Delete(t *testing.T) {
	t.Run("204 deleted", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		svc.On("Delete", uint(2)).Return(nil)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/stocks/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("404 not found", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		svc.On("Delete", uint(99)).Return(assert.AnError)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/stocks/99", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestStockHandler_ListAll(t *testing.T) {
	svc := &mockStockService{}
	r := setupStockRouter(svc)

	trades := []domain.StockTrade{
		{ID: 1, Ticker: "AAPL", Shares: 10},
		{ID: 2, Ticker: "MSFT", Shares: 5},
	}
	svc.On("ListAll").Return(trades, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []domain.StockTrade
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 2)
	svc.AssertExpectations(t)
}

func TestStockHandler_GetPortfolio(t *testing.T) {
	t.Run("returns portfolio with holdings and totals", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		portfolio := &domain.StockPortfolio{
			Holdings: []domain.StockHolding{
				{
					Ticker:       "AAPL",
					Shares:       10,
					AvgCost:      domain.Money{Value: 200, Currency: domain.CurrencyUSD},
					TotalCost:    domain.Money{Value: 2000, Currency: domain.CurrencyUSD},
					RealizedGain: domain.Money{Value: 0, Currency: domain.CurrencyUSD},
				},
			},
			TotalCost:         domain.Money{Value: 2000, Currency: domain.CurrencyUSD},
			TotalRealizedGain: domain.Money{Value: 0, Currency: domain.CurrencyUSD},
		}
		svc.On("GetPortfolio").Return(portfolio, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/portfolio", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var result domain.StockPortfolio
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.Len(t, result.Holdings, 1)
		assert.Equal(t, "AAPL", result.Holdings[0].Ticker)
		assert.Equal(t, 2000.0, result.TotalCost.Value)
		svc.AssertExpectations(t)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		svc.On("GetPortfolio").Return(nil, assert.AnError)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/portfolio", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestStockHandler_Update(t *testing.T) {
	t.Run("200 on valid update", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		input := service.UpdateStockTradeInput{Shares: 15, PricePerShare: 230}
		returned := &domain.StockTrade{ID: 1, Ticker: "AAPL", Shares: 15, PricePerShare: 230}
		svc.On("Update", uint(1), input).Return(returned, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/stocks/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var trade domain.StockTrade
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &trade))
		assert.Equal(t, 15.0, trade.Shares)
		svc.AssertExpectations(t)
	})

	t.Run("404 when trade not found", func(t *testing.T) {
		svc := &mockStockService{}
		r := setupStockRouter(svc)

		input := service.UpdateStockTradeInput{Shares: 5}
		svc.On("Update", uint(99), input).Return(nil, assert.AnError)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/stocks/99", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}
