package service

import (
	"errors"
	"math"
	"time"

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
	SebPen   float64 `json:"seb_pen"`
	Luminor   float64 `json:"luminor"`
	Art       float64 `json:"art"`
	Cash      float64 `json:"cash"`
	RevM      float64 `json:"rev_m"`
	RevR      float64 `json:"rev_r"`
	RBTC      float64 `json:"r_btc"`     // BTC units
	MBTC      float64 `json:"m_btc"`     // BTC units
	BtcPrice  float64 `json:"btc_price"` // EUR/BTC at snapshot time
	RevStocks  float64 `json:"rev_stocks"`
	IBKRStocks float64 `json:"ibkr_stocks"`
}

// UpdateBalanceInput is the input DTO for updating a balance snapshot
type UpdateBalanceInput struct {
	Date       string  `json:"date"`
	Total      float64 `json:"total"`
	Seb        float64 `json:"seb"`
	Swed       float64 `json:"swed"`
	SwedETF    float64 `json:"swed_etf"`
	SebPen     float64 `json:"seb_pen"`
	Luminor    float64 `json:"luminor"`
	Art        float64 `json:"art"`
	Cash       float64 `json:"cash"`
	RevM       float64 `json:"rev_m"`
	RevR       float64 `json:"rev_r"`
	RBTC       float64 `json:"r_btc"`     // BTC units
	MBTC       float64 `json:"m_btc"`     // BTC units
	BtcPrice   float64 `json:"btc_price"` // EUR/BTC at snapshot time
	RevStocks  float64 `json:"rev_stocks"`
	IBKRStocks float64 `json:"ibkr_stocks"`
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

// applyBtcEur recalculates Total using live BTC price and populates BTC computed fields
// only for rows that actually have BTC holdings.
func applyBtcEur(b *domain.Balance, livePrice float64) {
	if livePrice <= 0 {
		return
	}

	rBtcEur := btcToEur(b.RBTC, b.BtcPrice, livePrice)
	mBtcEur := btcToEur(b.MBTC, b.BtcPrice, livePrice)
	b.Total = b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + rBtcEur + mBtcEur + b.RevStocks + b.IBKRStocks
	if b.RBTC > 0 {
		b.RBtcEur = rBtcEur
	}
	if b.MBTC > 0 {
		b.MBtcEur = mBtcEur
	}
}

//go:generate mockery --name=BalanceService --output=../handler/mock --outpkg=mock
type BalanceService interface {
	Create(input CreateBalanceInput) (*domain.Balance, error)
	GetByID(id uint) (*domain.Balance, error)
	Update(id uint, input UpdateBalanceInput) (*domain.Balance, error)
	Delete(id uint) error
	DeleteAll() error
	List(filter domain.BalanceFilter, liveBtcPrice float64) ([]domain.Balance, error)
	GetLatest(liveBtcPrice float64) (*domain.Balance, error)
	GetProjected(liveBtcPrice float64) (*domain.Balance, error)
	UpsertProjected(liveBtcPrice float64) error
	GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error)
	GetAllocation() ([]domain.AccountAllocation, error)
}

type balanceService struct {
	repo   repository.BalanceRepository
	txRepo repository.TransactionRepository
}

func NewBalanceService(repo repository.BalanceRepository, txRepo repository.TransactionRepository) BalanceService {
	return &balanceService{repo: repo, txRepo: txRepo}
}

func (s *balanceService) Create(input CreateBalanceInput) (*domain.Balance, error) {
	date, err := timeutil.ParseDate(input.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}

	b := &domain.Balance{
		Date:       date,
		Total:      input.Total,
		Seb:        input.Seb,
		Swed:       input.Swed,
		SwedETF:    input.SwedETF,
		SebPen:     input.SebPen,
		Luminor:    input.Luminor,
		Art:        input.Art,
		Cash:       input.Cash,
		RevM:       input.RevM,
		RevR:       input.RevR,
		RBTC:       input.RBTC,
		MBTC:       input.MBTC,
		BtcPrice:   input.BtcPrice,
		RevStocks:  input.RevStocks,
		IBKRStocks: input.IBKRStocks,
	}

	// Auto-calculate total if not provided
	// BTC fields are stored in BTC units; multiply by btc_price to get EUR contribution
	if b.Total == 0 {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks + b.IBKRStocks
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
	b.SebPen = input.SebPen
	b.Luminor = input.Luminor
	b.Art = input.Art
	b.Cash = input.Cash
	b.RevM = input.RevM
	b.RevR = input.RevR
	b.RBTC = input.RBTC
	b.MBTC = input.MBTC
	b.BtcPrice = input.BtcPrice
	b.RevStocks = input.RevStocks
	b.IBKRStocks = input.IBKRStocks

	// Recalculate total; BTC stored as BTC units × btc_price = EUR contribution
	if input.Total != 0 {
		b.Total = input.Total
	} else {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks + b.IBKRStocks
	}

	if err := s.repo.Update(b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *balanceService) DeleteAll() error {
	return s.repo.DeleteAll()
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

func (s *balanceService) UpsertProjected(liveBtcPrice float64) error {
	projected, err := s.GetProjected(liveBtcPrice)
	if err != nil {
		return err
	}
	if projected.ID == 0 && projected.Total == 0 {
		return nil // no manual snapshot exists yet; nothing to persist
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	projected.Date = today
	projected.IsAuto = true
	return s.repo.UpsertAuto(projected)
}

// applyAccountDelta adds delta to the named account field in b.
func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

func applyAccountDelta(b *domain.Balance, account string, delta float64) {
	switch account {
	case "seb":
		b.Seb += delta
	case "swed":
		b.Swed += delta
	case "swed_etf":
		b.SwedETF += delta
	case "seb_pen":
		b.SebPen += delta
	case "luminor":
		b.Luminor += delta
	case "art":
		b.Art += delta
	case "rev_m":
		b.RevM += delta
	case "rev_r":
		b.RevR += delta
	case "rev_stocks":
		b.RevStocks += delta
	case "ibkr_stocks":
		b.IBKRStocks += delta
	case "cash":
		b.Cash += delta
	}
}

func (s *balanceService) GetProjected(liveBtcPrice float64) (*domain.Balance, error) {
	latest, err := s.repo.GetLatestManual()
	if err != nil {
		return &domain.Balance{}, nil
	}
	txs, err := s.txRepo.ListSince(latest.Date)
	if err != nil {
		return nil, err
	}
	projected := *latest
	latestDay := latest.Date.Truncate(24 * time.Hour)
	for _, tx := range txs {
		txDay := tx.Date.Truncate(24 * time.Hour)
		if txDay.Before(latestDay) {
			continue
		}

		debit := tx.DebitAccount
		credit := tx.CreditAccount

		// Backward compat: old rows only have source_account set.
		// Derive debit/credit from type so existing data keeps working.
		if debit == "" && credit == "" && tx.SourceAccount != "" {
			if tx.Type == domain.TransactionTypeExpense {
				debit = tx.SourceAccount
			} else {
				// income and legacy investments: the source_account was the receiving/destination account
				credit = tx.SourceAccount
			}
		}

		if debit == "" && credit == "" {
			continue // no account info — skip
		}

		if debit != "" {
			applyAccountDelta(&projected, debit, -tx.Amount)
		}
		if credit != "" {
			applyAccountDelta(&projected, credit, tx.Amount)
		}
	}
	projected.Seb = roundCents(projected.Seb)
	projected.Swed = roundCents(projected.Swed)
	projected.SwedETF = roundCents(projected.SwedETF)
	projected.SebPen = roundCents(projected.SebPen)
	projected.Luminor = roundCents(projected.Luminor)
	projected.Art = roundCents(projected.Art)
	projected.Cash = roundCents(projected.Cash)
	projected.RevM = roundCents(projected.RevM)
	projected.RevR = roundCents(projected.RevR)
	projected.RevStocks = roundCents(projected.RevStocks)
	projected.IBKRStocks = roundCents(projected.IBKRStocks)

	if liveBtcPrice >= minValidBtcPrice {
		applyBtcEur(&projected, liveBtcPrice)
	} else {
		btcEur := projected.BtcPrice * (projected.RBTC + projected.MBTC)
		projected.Total = projected.Seb + projected.Swed + projected.SwedETF + projected.SebPen + projected.Luminor + projected.Art + projected.Cash + projected.RevM + projected.RevR + btcEur + projected.RevStocks + projected.IBKRStocks
	}
	projected.ID = 0
	return &projected, nil
}

func (s *balanceService) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	return s.repo.GetTrend(filter)
}

func (s *balanceService) GetAllocation() ([]domain.AccountAllocation, error) {
	latest, err := s.repo.GetLatest()
	if err != nil {
		return []domain.AccountAllocation{}, nil
	}

	// BTC fields are stored as BTC units; convert to EUR using snapshot price for allocation
	rBtcEur := latest.BtcPrice * latest.RBTC
	mBtcEur := latest.BtcPrice * latest.MBTC

	accounts := map[string]float64{
		"Seb":                         latest.Seb,
		"Swedbank":                    latest.Swed,
		"Swedbank ETF":                latest.SwedETF,
		"SEB 2nd pillar pension": latest.SebPen,
		"Luminor":                     latest.Luminor,
		"Artea 3rd pillar pension":    latest.Art,
		"Cash":                        latest.Cash,
		"Revolut M account":           latest.RevM,
		"Revolut R account":           latest.RevR,
		"Revolut R account BTC":       rBtcEur,
		"Revolut M account BTC":       mBtcEur,
		"Revolut M account stocks": latest.RevStocks,
		"IBKR stocks":              latest.IBKRStocks,
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
