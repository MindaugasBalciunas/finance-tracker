package service

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
	"gorm.io/gorm"
)

// CreateBalanceInput is the input DTO for creating a balance snapshot
type CreateBalanceInput struct {
	Date       string  `json:"date" binding:"required"`
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
func btcToEur(btcAmount, snapshotPrice, livePrice float64) float64 {
	if snapshotPrice >= minValidBtcPrice {
		price := snapshotPrice
		if livePrice >= minValidBtcPrice {
			price = livePrice
		}
		return btcAmount * price
	}
	if btcAmount > 0 && btcAmount < 1 && livePrice >= minValidBtcPrice {
		return btcAmount * livePrice
	}
	return btcAmount
}

// applyBtcEur recalculates Total using live BTC price.
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

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
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
	SnapshotFromTransaction(tx *domain.Transaction) error
	GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error)
	GetAllocation() ([]domain.AccountAllocation, error)
}

type balanceService struct {
	repo   repository.BalanceRepository
	snapMu sync.Mutex
}

func NewBalanceService(repo repository.BalanceRepository, _ repository.TransactionRepository) BalanceService {
	return &balanceService{repo: repo}
}

func (s *balanceService) Create(input CreateBalanceInput) (*domain.Balance, error) {
	date, err := timeutil.ParseDatetime(input.Date)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD or YYYY-MM-DDTHH:MM")
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

	if b.Total == 0 {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = roundCents(b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks + b.IBKRStocks)
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
		date, err := timeutil.ParseDatetime(input.Date)
		if err != nil {
			return nil, errors.New("invalid date format, use YYYY-MM-DD or YYYY-MM-DDTHH:MM")
		}
		b.Date = date
	}

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

	// Always recompute the total from the components at the SNAPSHOT's BTC
	// price ("Total is auto-calculated" in the UI). Trusting input.Total let
	// a GET→PUT edit cycle persist the live-priced total the read path
	// computes (applyBtcEur), silently rewriting a historical snapshot with
	// the market price of the moment. The one exception: a legacy total-only
	// row (no component breakdown) keeps its explicit total.
	btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
	b.Total = roundCents(b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks + b.IBKRStocks)
	if b.Total == 0 && input.Total != 0 {
		b.Total = input.Total
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

// GetProjected returns the latest snapshot with live BTC price applied.
// With manual multi-snapshot-per-day entries this is equivalent to GetLatest.
func (s *balanceService) GetProjected(liveBtcPrice float64) (*domain.Balance, error) {
	b, err := s.repo.GetLatest()
	if err != nil {
		return &domain.Balance{}, nil
	}
	result := *b
	applyBtcEur(&result, liveBtcPrice)
	result.ID = 0
	return &result, nil
}

// SnapshotFromTransaction creates a new balance snapshot by applying a single
// transaction's debit/credit to the current latest snapshot values.
// Called only on transaction Create — Update and Delete leave balances untouched.
func (s *balanceService) SnapshotFromTransaction(tx *domain.Transaction) error {
	// Serialize the read-modify-write: two concurrent creates would otherwise
	// both clone the same "latest" and each lose the other's delta.
	s.snapMu.Lock()
	defer s.snapMu.Unlock()

	latest, err := s.repo.GetLatest()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // no snapshot to base on — legitimate skip
		}
		return err
	}

	// A backdated transaction must not clone TODAY's balances under a past
	// date — that cuts a wrong point into the net-worth trend. Balances only
	// move forward; past entries are already reflected in later snapshots.
	if tx.Date.Before(latest.Date.Truncate(24 * time.Hour)) {
		return nil
	}

	debit, credit := resolveAccounts(tx)
	if debit == "" && credit == "" {
		return nil // no account info — nothing to apply
	}

	snap := *latest
	if debit != "" {
		applyAccountDelta(&snap, debit, -tx.Amount)
	}
	if credit != "" {
		applyAccountDelta(&snap, credit, tx.Amount)
	}
	roundAllAccounts(&snap)

	btcEur := snap.BtcPrice * (snap.RBTC + snap.MBTC)
	snap.Total = roundCents(snap.Seb + snap.Swed + snap.SwedETF + snap.SebPen + snap.Luminor + snap.Art + snap.Cash + snap.RevM + snap.RevR + btcEur + snap.RevStocks + snap.IBKRStocks)

	snap.ID = 0
	snap.Date = tx.Date
	snap.CreatedAt = time.Time{}
	snap.UpdatedAt = time.Time{}

	return s.repo.Create(&snap)
}

func resolveAccounts(tx *domain.Transaction) (debit, credit string) {
	debit = tx.DebitAccount
	credit = tx.CreditAccount
	if debit == "" && credit == "" && tx.SourceAccount != "" {
		if tx.Type == domain.TransactionTypeExpense {
			debit = tx.SourceAccount
		} else {
			credit = tx.SourceAccount
		}
	}
	return debit, credit
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

func roundAllAccounts(b *domain.Balance) {
	b.Seb = roundCents(b.Seb)
	b.Swed = roundCents(b.Swed)
	b.SwedETF = roundCents(b.SwedETF)
	b.SebPen = roundCents(b.SebPen)
	b.Luminor = roundCents(b.Luminor)
	b.Art = roundCents(b.Art)
	b.Cash = roundCents(b.Cash)
	b.RevM = roundCents(b.RevM)
	b.RevR = roundCents(b.RevR)
	b.RevStocks = roundCents(b.RevStocks)
	b.IBKRStocks = roundCents(b.IBKRStocks)
}

func (s *balanceService) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	return s.repo.GetTrend(filter)
}

func (s *balanceService) GetAllocation() ([]domain.AccountAllocation, error) {
	latest, err := s.repo.GetLatest()
	if err != nil {
		return []domain.AccountAllocation{}, nil
	}

	rBtcEur := latest.BtcPrice * latest.RBTC
	mBtcEur := latest.BtcPrice * latest.MBTC

	accounts := map[string]float64{
		"Seb":                      latest.Seb,
		"Swedbank":                 latest.Swed,
		"Swedbank ETF":             latest.SwedETF,
		"SEB 2nd pillar pension":   latest.SebPen,
		"Luminor":                  latest.Luminor,
		"Artea 3rd pillar pension": latest.Art,
		"Cash":                     latest.Cash,
		"Revolut M account":        latest.RevM,
		"Revolut R account":        latest.RevR,
		"Revolut R account BTC":    rBtcEur,
		"Revolut M account BTC":    mBtcEur,
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
