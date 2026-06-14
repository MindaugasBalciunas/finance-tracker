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
	RebuildAutoSnapshots(liveBtcPrice float64) error
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
	_ = s.RebuildAutoSnapshots(0)
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
	_ = s.RebuildAutoSnapshots(0)
	return b, nil
}

func (s *balanceService) DeleteAll() error {
	return s.repo.DeleteAll()
}

func (s *balanceService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	_ = s.RebuildAutoSnapshots(0)
	return nil
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

func (s *balanceService) RebuildAutoSnapshots(liveBtcPrice float64) error {
	latest, err := s.repo.GetLatestManual()
	if err != nil {
		return nil // no manual snapshot — nothing to do
	}
	if err := s.repo.DeleteAllAuto(); err != nil {
		return err
	}
	txs, err := s.txRepo.ListStrictlyAfter(latest.Date)
	if err != nil {
		return err
	}
	running := *latest
	for _, tx := range txs {
		debit, credit := resolveAccounts(&tx)
		if debit == "" && credit == "" {
			continue
		}
		if debit != "" {
			applyAccountDelta(&running, debit, -tx.Amount)
		}
		if credit != "" {
			applyAccountDelta(&running, credit, tx.Amount)
		}
		roundAllAccounts(&running)
		if liveBtcPrice >= minValidBtcPrice {
			applyBtcEur(&running, liveBtcPrice)
		} else {
			btcEur := running.BtcPrice * (running.RBTC + running.MBTC)
			running.Total = running.Seb + running.Swed + running.SwedETF + running.SebPen + running.Luminor + running.Art + running.Cash + running.RevM + running.RevR + btcEur + running.RevStocks + running.IBKRStocks
		}
		snapshot := running
		snapshot.ID = 0
		snapshot.Date = tx.Date.Truncate(24 * time.Hour)
		snapshot.IsAuto = true
		snapshot.CreatedAt = time.Time{}
		snapshot.UpdatedAt = time.Time{}
		if err := s.repo.CreateAuto(&snapshot); err != nil {
			return err
		}
	}
	return nil
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

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
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
	b, err := s.repo.GetLatest()
	if err != nil {
		return &domain.Balance{}, nil
	}
	result := *b
	applyBtcEur(&result, liveBtcPrice)
	result.ID = 0
	return &result, nil
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
