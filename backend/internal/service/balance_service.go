package service

import (
	"errors"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
)

// CreateBalanceInput is the input DTO for creating a balance snapshot
type CreateBalanceInput struct {
	Date      string  `json:"date" binding:"required"`
	Total     float64 `json:"total"`
	Seb       float64 `json:"seb"`
	Swed      float64 `json:"swed"`
	SwedETF   float64 `json:"swed_etf"`
	SwedPen   float64 `json:"swed_pen"`
	Luminor   float64 `json:"luminor"`
	Art       float64 `json:"art"`
	Cash      float64 `json:"cash"`
	RevM      float64 `json:"rev_m"`
	RevR      float64 `json:"rev_r"`
	RBTC      float64 `json:"r_btc"`     // BTC units
	MBTC      float64 `json:"m_btc"`     // BTC units
	BtcPrice  float64 `json:"btc_price"` // EUR/BTC at snapshot time
	RevStocks float64 `json:"rev_stocks"`
}

// UpdateBalanceInput is the input DTO for updating a balance snapshot
type UpdateBalanceInput struct {
	Date      string  `json:"date"`
	Total     float64 `json:"total"`
	Seb       float64 `json:"seb"`
	Swed      float64 `json:"swed"`
	SwedETF   float64 `json:"swed_etf"`
	SwedPen   float64 `json:"swed_pen"`
	Luminor   float64 `json:"luminor"`
	Art       float64 `json:"art"`
	Cash      float64 `json:"cash"`
	RevM      float64 `json:"rev_m"`
	RevR      float64 `json:"rev_r"`
	RBTC      float64 `json:"r_btc"`     // BTC units
	MBTC      float64 `json:"m_btc"`     // BTC units
	BtcPrice  float64 `json:"btc_price"` // EUR/BTC at snapshot time
	RevStocks float64 `json:"rev_stocks"`
}

const minValidBtcPrice = 100.0

// btcToEur converts a BTC amount to EUR using snapshot and live prices.
// Mirrors the same logic used on the frontend (btcToEur in utils/btc.ts).
func btcToEur(btcAmount, snapshotPrice, livePrice float64) float64 {
	if snapshotPrice >= minValidBtcPrice {
		price := snapshotPrice
		if livePrice >= minValidBtcPrice {
			price = livePrice
		}
		return btcAmount * price
	}
	// Legacy: btcAmount stored as BTC units, convert with live price
	if btcAmount > 0 && btcAmount < 1 && livePrice >= minValidBtcPrice {
		return btcAmount * livePrice
	}
	// Already in EUR or zero
	return btcAmount
}

// applyBtcEur populates the computed RBtcEur/MBtcEur fields and recalculates Total.
func applyBtcEur(b *domain.Balance, livePrice float64) {
	if livePrice <= 0 {
		return
	}
	b.RBtcEur = btcToEur(b.RBTC, b.BtcPrice, livePrice)
	b.MBtcEur = btcToEur(b.MBTC, b.BtcPrice, livePrice)
	b.Total = b.Seb + b.Swed + b.SwedETF + b.SwedPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + b.RBtcEur + b.MBtcEur + b.RevStocks
}

//go:generate mockery --name=BalanceService --output=../handler/mock --outpkg=mock
type BalanceService interface {
	Create(input CreateBalanceInput) (*domain.Balance, error)
	GetByID(id uint) (*domain.Balance, error)
	Update(id uint, input UpdateBalanceInput) (*domain.Balance, error)
	Delete(id uint) error
	List(filter domain.BalanceFilter, liveBtcPrice float64) ([]domain.Balance, error)
	GetLatest(liveBtcPrice float64) (*domain.Balance, error)
	GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error)
	GetAllocation() ([]domain.AccountAllocation, error)
}

type balanceService struct {
	repo repository.BalanceRepository
}

func NewBalanceService(repo repository.BalanceRepository) BalanceService {
	return &balanceService{repo: repo}
}

func (s *balanceService) Create(input CreateBalanceInput) (*domain.Balance, error) {
	date, err := timeutil.ParseDate(input.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}

	b := &domain.Balance{
		Date:      date,
		Total:     input.Total,
		Seb:       input.Seb,
		Swed:      input.Swed,
		SwedETF:   input.SwedETF,
		SwedPen:   input.SwedPen,
		Luminor:   input.Luminor,
		Art:       input.Art,
		Cash:      input.Cash,
		RevM:      input.RevM,
		RevR:      input.RevR,
		RBTC:      input.RBTC,
		MBTC:      input.MBTC,
		BtcPrice:  input.BtcPrice,
		RevStocks: input.RevStocks,
	}

	// Auto-calculate total if not provided
	// BTC fields are stored in BTC units; multiply by btc_price to get EUR contribution
	if b.Total == 0 {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = b.Seb + b.Swed + b.SwedETF + b.SwedPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks
	}

	if err := s.repo.Create(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *balanceService) GetByID(id uint) (*domain.Balance, error) {
	return s.repo.GetByID(id)
}

func (s *balanceService) Update(id uint, input UpdateBalanceInput) (*domain.Balance, error) {
	b, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if input.Date != "" {
		date, err := timeutil.ParseDate(input.Date)
		if err != nil {
			return nil, errors.New("invalid date format, use YYYY-MM-DD")
		}
		b.Date = date
	}

	// Update individual accounts
	b.Seb = input.Seb
	b.Swed = input.Swed
	b.SwedETF = input.SwedETF
	b.SwedPen = input.SwedPen
	b.Luminor = input.Luminor
	b.Art = input.Art
	b.Cash = input.Cash
	b.RevM = input.RevM
	b.RevR = input.RevR
	b.RBTC = input.RBTC
	b.MBTC = input.MBTC
	b.BtcPrice = input.BtcPrice
	b.RevStocks = input.RevStocks

	// Recalculate total; BTC stored as BTC units × btc_price = EUR contribution
	if input.Total != 0 {
		b.Total = input.Total
	} else {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = b.Seb + b.Swed + b.SwedETF + b.SwedPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks
	}

	if err := s.repo.Update(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *balanceService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *balanceService) List(filter domain.BalanceFilter, liveBtcPrice float64) ([]domain.Balance, error) {
	balances, err := s.repo.List(filter)
	if err != nil {
		return nil, err
	}
	for i := range balances {
		applyBtcEur(&balances[i], liveBtcPrice)
	}
	return balances, nil
}

func (s *balanceService) GetLatest(liveBtcPrice float64) (*domain.Balance, error) {
	b, err := s.repo.GetLatest()
	if err != nil {
		return nil, err
	}
	applyBtcEur(b, liveBtcPrice)
	return b, nil
}

func (s *balanceService) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	return s.repo.GetTrend(filter)
}

func (s *balanceService) GetAllocation() ([]domain.AccountAllocation, error) {
	latest, err := s.repo.GetLatest()
	if err != nil {
		return []domain.AccountAllocation{}, nil
	}

	accounts := map[string]float64{
		"Seb":                         latest.Seb,
		"Swedbank":                    latest.Swed,
		"Swedbank ETF":                latest.SwedETF,
		"SwedBank 2nd pillar pension": latest.SwedPen,
		"Luminor":                     latest.Luminor,
		"Artea 3rd pillar pension":    latest.Art,
		"Cash":                        latest.Cash,
		"Revolut M account":           latest.RevM,
		"Revolut R account":           latest.RevR,
		"Revolut R account BTC":       latest.RBTC,
		"Revolut M account BTC":       latest.MBTC,
		"Revolut M account stocks":    latest.RevStocks,
	}

	var allocations []domain.AccountAllocation
	for name, amount := range accounts {
		if amount == 0 {
			continue
		}
		pct := 0.0
		if latest.Total > 0 {
			pct = (amount / latest.Total) * 100
		}
		allocations = append(allocations, domain.AccountAllocation{
			Account:    name,
			Amount:     amount,
			Percentage: pct,
		})
	}
	return allocations, nil
}
