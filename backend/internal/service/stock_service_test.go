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

func TestStockService_Create(t *testing.T) {
	t.Run("buy trade defaults to USD and populates Money", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("Create", &domain.StockTrade{
			Date:          time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        10,
			PricePerShare: 220.50,
			Currency:      "USD",
		}).Return(nil)

		trade, err := svc.Create(service.CreateStockTradeInput{
			Date:          "2026-01-10",
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        10,
			PricePerShare: 220.50,
		})
		require.NoError(t, err)
		assert.Equal(t, "AAPL", trade.Ticker)
		assert.Equal(t, 10.0, trade.Shares)
		assert.Equal(t, "USD", trade.Currency)
		assert.Equal(t, 220.50, trade.PricePerShareMoney.Value)
		assert.Equal(t, domain.CurrencyUSD, trade.PricePerShareMoney.Currency)
		repo.AssertExpectations(t)
	})

	t.Run("sell trade with EUR currency", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("Create", &domain.StockTrade{
			Date:          time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Action:        domain.StockActionSell,
			Ticker:        "VWCE",
			Shares:        5,
			PricePerShare: 110.00,
			Currency:      "EUR",
		}).Return(nil)

		trade, err := svc.Create(service.CreateStockTradeInput{
			Date:          "2026-02-01",
			Action:        domain.StockActionSell,
			Ticker:        "VWCE",
			Shares:        5,
			PricePerShare: 110.00,
			Currency:      "EUR",
		})
		require.NoError(t, err)
		assert.Equal(t, domain.CurrencyEUR, trade.PricePerShareMoney.Currency)
		repo.AssertExpectations(t)
	})

	t.Run("invalid date", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		_, err := svc.Create(service.CreateStockTradeInput{
			Date:          "bad-date",
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        1,
			PricePerShare: 100,
		})
		assert.ErrorContains(t, err, "invalid date format")
	})

	t.Run("repository error", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("Create", &domain.StockTrade{
			Date:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        1,
			PricePerShare: 100,
			Currency:      "USD",
		}).Return(errors.New("db error"))

		_, err := svc.Create(service.CreateStockTradeInput{
			Date:          "2026-01-01",
			Action:        domain.StockActionBuy,
			Ticker:        "AAPL",
			Shares:        1,
			PricePerShare: 100,
		})
		assert.ErrorContains(t, err, "db error")
		repo.AssertExpectations(t)
	})
}

func TestStockService_GetByID(t *testing.T) {
	t.Run("found and Money populated", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		stored := &domain.StockTrade{ID: 1, Ticker: "AAPL", PricePerShare: 220, Currency: "USD"}
		repo.On("GetByID", uint(1)).Return(stored, nil)

		trade, err := svc.GetByID(1)
		require.NoError(t, err)
		assert.Equal(t, 220.0, trade.PricePerShareMoney.Value)
		assert.Equal(t, domain.CurrencyUSD, trade.PricePerShareMoney.Currency)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		_, err := svc.GetByID(99)
		assert.Error(t, err)
		repo.AssertExpectations(t)
	})
}

func TestStockService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("GetByID", uint(1)).Return(&domain.StockTrade{ID: 1}, nil)
		repo.On("Delete", uint(1)).Return(nil)

		require.NoError(t, svc.Delete(1))
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mock.StockRepository{}
		svc := service.NewStockService(repo)

		repo.On("GetByID", uint(99)).Return(nil, errors.New("record not found"))

		assert.Error(t, svc.Delete(99))
		repo.AssertExpectations(t)
	})
}

func TestStockService_ListAll(t *testing.T) {
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	stored := []domain.StockTrade{
		{ID: 1, Ticker: "AAPL", PricePerShare: 220, Currency: "USD"},
		{ID: 2, Ticker: "VWCE", PricePerShare: 110, Currency: "EUR"},
	}
	repo.On("ListAll").Return(stored, nil)

	result, err := svc.ListAll()
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, domain.CurrencyUSD, result[0].PricePerShareMoney.Currency)
	assert.Equal(t, domain.CurrencyEUR, result[1].PricePerShareMoney.Currency)
	repo.AssertExpectations(t)
}

func TestStockService_ListSince(t *testing.T) {
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	stored := []domain.StockTrade{
		{ID: 5, Ticker: "AAPL", PricePerShare: 230, Currency: "USD", Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	repo.On("ListSince", since).Return(stored, nil)

	result, err := svc.ListSince(since)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(5), result[0].ID)
	assert.Equal(t, 230.0, result[0].PricePerShareMoney.Value)
	repo.AssertExpectations(t)
}

// Portfolio calculation tests — the core business logic

func TestStockService_GetPortfolio_SingleBuy(t *testing.T) {
	// Buying 10 shares at $200 = $2000 total cost, avg = $200
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "AAPL", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 200, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	require.Len(t, portfolio.Holdings, 1)

	h := portfolio.Holdings[0]
	assert.Equal(t, "AAPL", h.Ticker)
	assert.Equal(t, 10.0, h.Shares)
	assert.Equal(t, 200.0, h.AvgCost.Value)
	assert.Equal(t, 2000.0, h.TotalCost.Value)
	assert.Equal(t, 0.0, h.RealizedGain.Value)
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_BuyThenSellAll(t *testing.T) {
	// Buy 10 @ $200, sell 10 @ $250 → realized gain = 10*(250-200) = $500
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "AAPL", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 200, Currency: "USD"},
		{Ticker: "AAPL", Action: domain.StockActionSell, Shares: 10, PricePerShare: 250, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	require.Len(t, portfolio.Holdings, 1)

	h := portfolio.Holdings[0]
	assert.Equal(t, 0.0, h.Shares)       // fully sold
	assert.Equal(t, 0.0, h.TotalCost.Value)
	assert.Equal(t, 500.0, h.RealizedGain.Value)
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_PartialSell(t *testing.T) {
	// Buy 10 @ $200 = $2000 total cost, avg = $200
	// Sell 4 @ $250 → realized = 4*(250-200) = $200; remaining cost = $200*6 = $1200
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "AAPL", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 200, Currency: "USD"},
		{Ticker: "AAPL", Action: domain.StockActionSell, Shares: 4, PricePerShare: 250, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	require.Len(t, portfolio.Holdings, 1)

	h := portfolio.Holdings[0]
	assert.Equal(t, 6.0, h.Shares)
	assert.Equal(t, 200.0, h.AvgCost.Value)
	assert.InDelta(t, 1200.0, h.TotalCost.Value, 0.01)
	assert.Equal(t, 200.0, h.RealizedGain.Value)
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_MultipleBuys_AverageCost(t *testing.T) {
	// Buy 10 @ $100 = $1000; then buy 10 @ $200 = $2000
	// Total: 20 shares, $3000 cost, avg = $150
	// Sell 5 @ $180 → realized = 5*(180-150) = $150; remaining cost = 15*$150 = $2250
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "MSFT", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 100, Currency: "USD"},
		{Ticker: "MSFT", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 200, Currency: "USD"},
		{Ticker: "MSFT", Action: domain.StockActionSell, Shares: 5, PricePerShare: 180, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	require.Len(t, portfolio.Holdings, 1)

	h := portfolio.Holdings[0]
	assert.Equal(t, 15.0, h.Shares)
	assert.Equal(t, 150.0, h.AvgCost.Value)
	assert.InDelta(t, 2250.0, h.TotalCost.Value, 0.01)
	assert.Equal(t, 150.0, h.RealizedGain.Value)
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_MultipleTickers(t *testing.T) {
	// AAPL: buy 5 @ $200; MSFT: buy 3 @ $300
	// Total portfolio cost = $1000 + $900 = $1900
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "AAPL", Action: domain.StockActionBuy, Shares: 5, PricePerShare: 200, Currency: "USD"},
		{Ticker: "MSFT", Action: domain.StockActionBuy, Shares: 3, PricePerShare: 300, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	assert.Len(t, portfolio.Holdings, 2)

	holdings := map[string]domain.StockHolding{}
	for _, h := range portfolio.Holdings {
		holdings[h.Ticker] = h
	}

	assert.Equal(t, 5.0, holdings["AAPL"].Shares)
	assert.Equal(t, 1000.0, holdings["AAPL"].TotalCost.Value)
	assert.Equal(t, 3.0, holdings["MSFT"].Shares)
	assert.Equal(t, 900.0, holdings["MSFT"].TotalCost.Value)

	assert.Equal(t, 1900.0, portfolio.TotalCost.Value)
	assert.Equal(t, 0.0, portfolio.TotalRealizedGain.Value)
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_SellAtLoss(t *testing.T) {
	// Buy 10 @ $200; sell 5 @ $150 → realized = 5*(150-200) = -$250
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{
		{Ticker: "AAPL", Action: domain.StockActionBuy, Shares: 10, PricePerShare: 200, Currency: "USD"},
		{Ticker: "AAPL", Action: domain.StockActionSell, Shares: 5, PricePerShare: 150, Currency: "USD"},
	}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	require.Len(t, portfolio.Holdings, 1)

	h := portfolio.Holdings[0]
	assert.Equal(t, 5.0, h.Shares)
	assert.Equal(t, -250.0, h.RealizedGain.Value)
	assert.Less(t, portfolio.TotalRealizedGain.Value, float64(0))
	repo.AssertExpectations(t)
}

func TestStockService_GetPortfolio_Empty(t *testing.T) {
	repo := &mock.StockRepository{}
	svc := service.NewStockService(repo)

	repo.On("ListAll").Return([]domain.StockTrade{}, nil)

	portfolio, err := svc.GetPortfolio()
	require.NoError(t, err)
	assert.Empty(t, portfolio.Holdings)
	assert.Equal(t, 0.0, portfolio.TotalCost.Value)
	repo.AssertExpectations(t)
}
