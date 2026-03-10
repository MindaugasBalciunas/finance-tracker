package service

import (
	"errors"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
)

type CreateStockTradeInput struct {
	Date          string             `json:"date" binding:"required"`
	Action        domain.StockAction `json:"action" binding:"required,oneof=buy sell"`
	Ticker        string             `json:"ticker" binding:"required"`
	Shares        float64            `json:"shares" binding:"required,gt=0"`
	PricePerShare float64            `json:"price_per_share" binding:"required,gt=0"`
	Currency      string             `json:"currency"`
	Notes         string             `json:"notes"`
}

type UpdateStockTradeInput struct {
	Date          string             `json:"date"`
	Action        domain.StockAction `json:"action" binding:"omitempty,oneof=buy sell"`
	Ticker        string             `json:"ticker"`
	Shares        float64            `json:"shares" binding:"omitempty,gt=0"`
	PricePerShare float64            `json:"price_per_share" binding:"omitempty,gt=0"`
	Currency      string             `json:"currency"`
	Notes         string             `json:"notes"`
}

type StockService interface {
	Create(input CreateStockTradeInput) (*domain.StockTrade, error)
	GetByID(id uint) (*domain.StockTrade, error)
	Update(id uint, input UpdateStockTradeInput) (*domain.StockTrade, error)
	Delete(id uint) error
	ListAll() ([]domain.StockTrade, error)
	GetPortfolio() (*domain.StockPortfolio, error)
}

type stockService struct {
	repo repository.StockRepository
}

func NewStockService(repo repository.StockRepository) StockService {
	return &stockService{repo: repo}
}

// populateStockTradeMoney adds Money type fields to a stock trade
func populateStockTradeMoney(t *domain.StockTrade) {
	if t != nil {
		currency := domain.Currency(t.Currency)
		if currency != domain.CurrencyEUR && currency != domain.CurrencyUSD {
			currency = domain.CurrencyUSD
		}
		t.PricePerShareMoney = domain.Money{
			Value:    t.PricePerShare,
			Currency: currency,
		}
		t.TotalCostMoney = domain.Money{
			Value:    t.Shares * t.PricePerShare,
			Currency: currency,
		}
	}
}

// populateStockTradesMoney adds Money type fields to multiple trades
func populateStockTradesMoney(trades []domain.StockTrade) {
	for i := range trades {
		populateStockTradeMoney(&trades[i])
	}
}

// populateStockHoldingMoney adds Money type fields to a stock holding
func populateStockHoldingMoney(h *domain.StockHolding) {
	if h != nil {
		currency := domain.Currency(h.Currency)
		if currency != domain.CurrencyEUR && currency != domain.CurrencyUSD {
			currency = domain.CurrencyUSD
		}
		h.AvgCostMoney = domain.Money{
			Value:    h.AvgCostUSD,
			Currency: currency,
		}
		h.TotalCostMoney = domain.Money{
			Value:    h.TotalCostUSD,
			Currency: currency,
		}
		h.RealizedGainMoney = domain.Money{
			Value:    h.RealizedGain,
			Currency: currency,
		}
	}
}

// populateStockHoldingsMoney adds Money type fields to multiple holdings
func populateStockHoldingsMoney(holdings []domain.StockHolding) {
	for i := range holdings {
		populateStockHoldingMoney(&holdings[i])
	}
}

func (s *stockService) Create(input CreateStockTradeInput) (*domain.StockTrade, error) {
	date, err := timeutil.ParseDate(input.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}
	currency := input.Currency
	if currency == "" {
		currency = "USD"
	}
	t := &domain.StockTrade{
		Date:          date,
		Action:        input.Action,
		Ticker:        input.Ticker,
		Shares:        input.Shares,
		PricePerShare: input.PricePerShare,
		Currency:      currency,
		Notes:         input.Notes,
	}
	if err := s.repo.Create(t); err != nil {
		return nil, err
	}
	populateStockTradeMoney(t)
	return t, nil
}

func (s *stockService) GetByID(id uint) (*domain.StockTrade, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	populateStockTradeMoney(t)
	return t, nil
}

func (s *stockService) Update(id uint, input UpdateStockTradeInput) (*domain.StockTrade, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if input.Date != "" {
		date, err := timeutil.ParseDate(input.Date)
		if err != nil {
			return nil, errors.New("invalid date format, use YYYY-MM-DD")
		}
		t.Date = date
	}
	if input.Action != "" {
		t.Action = input.Action
	}
	if input.Ticker != "" {
		t.Ticker = input.Ticker
	}
	if input.Shares > 0 {
		t.Shares = input.Shares
	}
	if input.PricePerShare > 0 {
		t.PricePerShare = input.PricePerShare
	}
	if input.Currency != "" {
		t.Currency = input.Currency
	}
	t.Notes = input.Notes
	if err := s.repo.Update(t); err != nil {
		return nil, err
	}
	populateStockTradeMoney(t)
	return t, nil
}

func (s *stockService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *stockService) ListAll() ([]domain.StockTrade, error) {
	trades, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	populateStockTradesMoney(trades)
	return trades, nil
}

// GetPortfolio computes current holdings using average cost method.
func (s *stockService) GetPortfolio() (*domain.StockPortfolio, error) {
	trades, err := s.repo.ListAll() // ordered by date ASC
	if err != nil {
		return nil, err
	}

	type position struct {
		currency     string
		shares       float64
		totalCost    float64
		realizedGain float64
	}
	positions := map[string]*position{}

	for _, t := range trades {
		ticker := t.Ticker
		if positions[ticker] == nil {
			currency := t.Currency
			if currency == "" {
				currency = "USD"
			}
			positions[ticker] = &position{currency: currency}
		}
		pos := positions[ticker]

		switch t.Action {
		case domain.StockActionBuy:
			pos.totalCost += t.Shares * t.PricePerShare
			pos.shares += t.Shares

		case domain.StockActionSell:
			if pos.shares > 0 {
				avgCost := pos.totalCost / pos.shares
				pos.realizedGain += t.Shares * (t.PricePerShare - avgCost)
				pos.totalCost -= t.Shares * avgCost
			}
			pos.shares -= t.Shares
		}
	}

	portfolio := &domain.StockPortfolio{}
	for ticker, pos := range positions {
		avgCost := 0.0
		if pos.shares > 0 {
			avgCost = pos.totalCost / pos.shares
		}
		holding := domain.StockHolding{
			Ticker:       ticker,
			Currency:     pos.currency,
			Shares:       pos.shares,
			AvgCostUSD:   avgCost,
			TotalCostUSD: pos.totalCost,
			RealizedGain: pos.realizedGain,
		}
		populateStockHoldingMoney(&holding)
		portfolio.Holdings = append(portfolio.Holdings, holding)
		portfolio.TotalCostUSD += pos.totalCost
		portfolio.TotalRealizedGain += pos.realizedGain
	}

	// Populate Money fields for portfolio totals
	portfolio.TotalCostMoney = domain.Money{
		Value:    portfolio.TotalCostUSD,
		Currency: domain.CurrencyUSD, // Portfolio totals are typically in the primary currency
	}
	portfolio.TotalRealizedGainMoney = domain.Money{
		Value:    portfolio.TotalRealizedGain,
		Currency: domain.CurrencyUSD,
	}

	return portfolio, nil
}
