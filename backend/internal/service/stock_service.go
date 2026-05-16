package service

import (
	"errors"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
)

type CreateStockTradeInput struct {
	Date          string              `json:"date" binding:"required"`
	Action        domain.StockAction  `json:"action" binding:"required,oneof=buy sell"`
	Ticker        string              `json:"ticker" binding:"required"`
	Shares        float64             `json:"shares" binding:"required,gt=0"`
	PricePerShare float64             `json:"price_per_share" binding:"required,gt=0"`
	Currency      string              `json:"currency"`
	Source        domain.StockSource  `json:"source" binding:"required,oneof=Revolut IBKR"`
	Notes         string              `json:"notes"`
}

type UpdateStockTradeInput struct {
	Date          string              `json:"date"`
	Action        domain.StockAction  `json:"action" binding:"omitempty,oneof=buy sell"`
	Ticker        string              `json:"ticker"`
	Shares        float64             `json:"shares" binding:"omitempty,gt=0"`
	PricePerShare float64             `json:"price_per_share" binding:"omitempty,gt=0"`
	Currency      string              `json:"currency"`
	Source        domain.StockSource  `json:"source" binding:"omitempty,oneof=Revolut IBKR"`
	Notes         string              `json:"notes"`
}

type StockService interface {
	Create(input CreateStockTradeInput) (*domain.StockTrade, error)
	GetByID(id uint) (*domain.StockTrade, error)
	Update(id uint, input UpdateStockTradeInput) (*domain.StockTrade, error)
	Delete(id uint) error
	ListAll() ([]domain.StockTrade, error)
	ListSince(since time.Time) ([]domain.StockTrade, error)
	GetPortfolio() (*domain.StockPortfolio, error)
}

type stockService struct {
	repo repository.StockRepository
}

func NewStockService(repo repository.StockRepository) StockService {
	return &stockService{repo: repo}
}

func tradeCurrency(t *domain.StockTrade) domain.Currency {
	if t.Currency == string(domain.CurrencyEUR) {
		return domain.CurrencyEUR
	}
	return domain.CurrencyUSD
}

func populateTrade(t *domain.StockTrade) {
	ccy := tradeCurrency(t)
	t.PricePerShareMoney = domain.Money{Value: t.PricePerShare, Currency: ccy}
}

func populateTrades(trades []domain.StockTrade) {
	for i := range trades {
		populateTrade(&trades[i])
	}
}

func holdingMoney(value float64, currency string) domain.Money {
	ccy := domain.CurrencyUSD
	if currency == string(domain.CurrencyEUR) {
		ccy = domain.CurrencyEUR
	}
	return domain.Money{Value: value, Currency: ccy}
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
	source := input.Source
	if source == "" {
		source = domain.StockSourceRevolut
	}
	t := &domain.StockTrade{
		Date:          date,
		Action:        input.Action,
		Ticker:        input.Ticker,
		Shares:        input.Shares,
		PricePerShare: input.PricePerShare,
		Currency:      currency,
		Source:        source,
		Notes:         input.Notes,
	}
	if err := s.repo.Create(t); err != nil {
		return nil, err
	}
	populateTrade(t)
	return t, nil
}

func (s *stockService) GetByID(id uint) (*domain.StockTrade, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	populateTrade(t)
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
	if input.Source != "" {
		t.Source = input.Source
	}
	t.Notes = input.Notes
	if err := s.repo.Update(t); err != nil {
		return nil, err
	}
	populateTrade(t)
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
	populateTrades(trades)
	return trades, nil
}

func (s *stockService) ListSince(since time.Time) ([]domain.StockTrade, error) {
	trades, err := s.repo.ListSince(since)
	if err != nil {
		return nil, err
	}
	populateTrades(trades)
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
	totalCost := 0.0
	totalGain := 0.0

	for ticker, pos := range positions {
		avgCost := 0.0
		if pos.shares > 0 {
			avgCost = pos.totalCost / pos.shares
		}
		portfolio.Holdings = append(portfolio.Holdings, domain.StockHolding{
			Ticker:       ticker,
			Currency:     pos.currency,
			Shares:       pos.shares,
			AvgCost:      holdingMoney(avgCost, pos.currency),
			TotalCost:    holdingMoney(pos.totalCost, pos.currency),
			RealizedGain: holdingMoney(pos.realizedGain, pos.currency),
		})
		totalCost += pos.totalCost
		totalGain += pos.realizedGain
	}

	portfolio.TotalCost = domain.Money{Value: totalCost, Currency: domain.CurrencyUSD}
	portfolio.TotalRealizedGain = domain.Money{Value: totalGain, Currency: domain.CurrencyUSD}

	return portfolio, nil
}
