package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
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
	// Extra holds added accounts by key. On update, nil keeps what the
	// snapshot has — a client that predates added accounts must not wipe them.
	Extra map[string]float64 `json:"extra"`
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
	// Extra holds added accounts by key. On update, nil keeps what the
	// snapshot has — a client that predates added accounts must not wipe them.
	Extra map[string]float64 `json:"extra"`
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
	b.Total = componentsTotal(b, rBtcEur+mBtcEur)
	if b.RBTC > 0 {
		b.RBtcEur = rBtcEur
	}
	if b.MBTC > 0 {
		b.MBtcEur = mBtcEur
	}
}

// componentsTotal is every account summed, with BTC already in EUR.
func componentsTotal(b *domain.Balance, btcEur float64) float64 {
	return b.Seb + b.Swed + b.SwedETF + b.SebPen + b.Luminor + b.Art + b.Cash + b.RevM + b.RevR + btcEur + b.RevStocks + b.IBKRStocks + b.Extra.Sum()
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
	// SnapshotFromDeltas applies dated per-account movements as a single new
	// snapshot. Returns the reason when nothing was adjusted, which is never
	// an error — the caller says so rather than failing the write that
	// caused it.
	SnapshotFromDeltas(deltas []AccountDelta) (string, error)
	// SnapshotFromAbsolute sets accounts to the given values (as stated by
	// the bank) in one new snapshot, and reports what each one was before.
	// No snapshot is written when every account already matches.
	SnapshotFromAbsolute(values map[string]float64, at time.Time) ([]AccountSet, error)
	GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error)
	GetAllocation() ([]domain.AccountAllocation, error)
}

type balanceService struct {
	repo   repository.BalanceRepository
	snapMu sync.Mutex
	// accounts names added accounts in the allocation; optional.
	accounts repository.AccountRepository
}

// WithAccounts lets the allocation name added accounts by their label.
func WithAccounts(svc BalanceService, accounts repository.AccountRepository) BalanceService {
	if s, ok := svc.(*balanceService); ok {
		s.accounts = accounts
	}
	return svc
}

func (s *balanceService) accountLabels() map[string]string {
	out := map[string]string{}
	if s.accounts == nil {
		return out
	}
	accs, err := s.accounts.List()
	if err != nil {
		return out
	}
	for _, a := range accs {
		out[a.Key] = a.Label
	}
	return out
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
		Extra:      extraValues(input.Extra),
	}

	// The form sends the live BTC price, or 0 when the price feed hasn't
	// answered — a snapshot holding BTC at price 0 stores a total missing
	// the coins (2026-09-29 lost ~€1.9k this way). Carry the previous
	// snapshot's price instead; it is the best observation available.
	if b.BtcPrice < minValidBtcPrice && b.RBTC+b.MBTC > 0 {
		if prev, err := s.repo.GetLatest(); err == nil && prev.BtcPrice >= minValidBtcPrice {
			b.BtcPrice = prev.BtcPrice
			b.Total = 0 // the client's total was computed without the coins
		}
	}

	if b.Total == 0 {
		btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
		b.Total = roundCents(componentsTotal(b, btcEur))
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
	if input.Extra != nil {
		b.Extra = extraValues(input.Extra)
	}

	// Always recompute the total from the components at the SNAPSHOT's BTC
	// price ("Total is auto-calculated" in the UI). Trusting input.Total let
	// a GET→PUT edit cycle persist the live-priced total the read path
	// computes (applyBtcEur), silently rewriting a historical snapshot with
	// the market price of the moment. The one exception: a legacy total-only
	// row (no component breakdown) keeps its explicit total.
	btcEur := b.BtcPrice * (b.RBTC + b.MBTC)
	b.Total = roundCents(componentsTotal(b, btcEur))
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
// transaction's debit/credit to the current latest snapshot values. Called on
// transaction Create, and on an Update that first adds account linkage;
// Delete leaves balances untouched.
//
// Every path that declines to move the balance writes the reason to
// tx.BalanceNote instead of returning nil silently — the skips are all
// legitimate, but a user who photographed a receipt expecting their balance
// to follow needs to be told it didn't.
// AccountDelta is one dated movement on one balance-sheet account. Signed:
// money leaving an account is negative.
type AccountDelta struct {
	Date    time.Time
	Account string
	Amount  float64
}

func (s *balanceService) SnapshotFromTransaction(tx *domain.Transaction) error {
	debit, credit := resolveAccounts(tx)
	var deltas []AccountDelta
	if debit != "" {
		deltas = append(deltas, AccountDelta{Date: tx.Date, Account: debit, Amount: -tx.Amount})
	}
	if credit != "" {
		deltas = append(deltas, AccountDelta{Date: tx.Date, Account: credit, Amount: tx.Amount})
	}

	// Serialize the read-modify-write: two concurrent creates would otherwise
	// both clone the same "latest" and each lose the other's delta.
	s.snapMu.Lock()
	defer s.snapMu.Unlock()

	note, err := s.snapshotLocked(deltas)
	if err != nil {
		// Name the side the bad code came from — "unknown debit_account" is
		// what the create path says, and a mismatch here would send the user
		// looking at the wrong field.
		var ua *unknownAccountError
		if errors.As(err, &ua) {
			side := "credit_account"
			if ua.Account == debit {
				side = "debit_account"
			}
			return fmt.Errorf("unknown %s %q — balance not adjusted", side, ua.Account)
		}
		return err
	}
	tx.BalanceNote = note
	return nil
}

// unknownAccountError names a code that matches no balance column, so callers
// can re-word it for the field the code actually came from.
type unknownAccountError struct{ Account string }

func (e *unknownAccountError) Error() string {
	return fmt.Sprintf("unknown account %q — balance not adjusted", e.Account)
}

// SnapshotFromDeltas is the batch form: one net movement, one new snapshot.
//
// A bank import commits several rows at once. Cutting a snapshot per row
// would plant a step in the net-worth trend for every row added, so the whole
// commit lands as a single point — which is also what it is: one moment at
// which the account balance was brought up to date.
func (s *balanceService) SnapshotFromDeltas(deltas []AccountDelta) (string, error) {
	s.snapMu.Lock()
	defer s.snapMu.Unlock()
	return s.snapshotLocked(deltas)
}

// snapshotLocked clones the latest snapshot, applies the per-account deltas
// and stores the result as a NEW snapshot dated `date`. The existing snapshot
// is never rewritten: balance history is a series of observations, and
// editing a past one silently restates what the trend already showed.
//
// Callers hold snapMu.
func (s *balanceService) snapshotLocked(deltas []AccountDelta) (string, error) {
	if len(deltas) == 0 {
		return "balance not adjusted: no account was set on this entry", nil
	}

	latest, err := s.repo.GetLatest()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No snapshot to base on — legitimate skip.
			return "no balance snapshot exists yet, so there was nothing to adjust — add one on the Balances page and later entries will follow it", nil
		}
		return "", err
	}

	// A backdated entry must not clone TODAY's balances under a past date —
	// that cuts a wrong point into the net-worth trend. Balances only move
	// forward; past entries are already reflected in later snapshots.
	//
	// Per entry, not per batch: a bank import routinely spans the latest
	// snapshot, and folding its older rows into the total would double-count
	// money the snapshot already saw leave the account.
	cutoff := latest.Date.Truncate(24 * time.Hour)
	net := map[string]float64{}
	var newest time.Time
	stale := 0
	for _, d := range deltas {
		if d.Date.Before(cutoff) {
			stale++
			continue
		}
		net[d.Account] += d.Amount
		if d.Date.After(newest) {
			newest = d.Date
		}
	}
	if len(net) == 0 {
		return fmt.Sprintf(
			"balance not adjusted: this entry is dated %s, before your latest snapshot (%s), which may already include it — update it on the Balances page if it doesn't",
			oldestDate(deltas).Format("2006-01-02"), latest.Date.Format("2006-01-02")), nil
	}

	snap := *latest
	snap.Extra = latest.Extra.Clone()
	// An account code that matches no column would write a snapshot that is
	// a byte-for-byte clone of the previous one — the balance looks
	// "adjusted" in the trend while nothing moved. Fail loudly instead; the
	// caller logs it and the bad linkage becomes visible.
	for _, account := range sortedKeys(net) {
		if !applyAccountDelta(&snap, account, net[account]) {
			return "", &unknownAccountError{Account: account}
		}
	}
	roundAllAccounts(&snap)

	btcEur := snap.BtcPrice * (snap.RBTC + snap.MBTC)
	snap.Total = roundCents(componentsTotal(&snap, btcEur))

	// A fresh row, never an edit of the one it was cloned from.
	snap.ID = 0
	snap.Date = newest
	snap.CreatedAt = time.Time{}
	snap.UpdatedAt = time.Time{}

	if err := s.repo.Create(&snap); err != nil {
		return "", err
	}
	if stale > 0 {
		return fmt.Sprintf(
			"%d entr%s dated before your latest snapshot (%s) were left out of the balance — it already includes them",
			stale, plural(stale, "y", "ies"), latest.Date.Format("2006-01-02")), nil
	}
	return "", nil
}

// AccountSet is one account moved to an absolute value.
type AccountSet struct {
	Account string  `json:"account"`
	Before  float64 `json:"before"`
	After   float64 `json:"after"`
	Changed bool    `json:"changed"`
}

func (s *balanceService) SnapshotFromAbsolute(values map[string]float64, at time.Time) ([]AccountSet, error) {
	s.snapMu.Lock()
	defer s.snapMu.Unlock()

	latest, err := s.repo.GetLatest()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no balance snapshot exists yet — add one on the Balances page first")
		}
		return nil, err
	}
	snap := *latest
	snap.Extra = latest.Extra.Clone()
	var out []AccountSet
	changed := false
	for _, account := range sortedKeys(values) {
		before, ok := accountValue(&snap, account)
		if !ok {
			return nil, &unknownAccountError{Account: account}
		}
		after := roundCents(values[account])
		set := AccountSet{Account: account, Before: before, After: after, Changed: math.Abs(after-before) >= 0.005}
		if set.Changed {
			applyAccountDelta(&snap, account, after-before)
			changed = true
		}
		out = append(out, set)
	}
	if !changed {
		return out, nil
	}
	roundAllAccounts(&snap)
	btcEur := snap.BtcPrice * (snap.RBTC + snap.MBTC)
	snap.Total = roundCents(componentsTotal(&snap, btcEur))
	snap.ID = 0
	// Never behind the snapshot it was cloned from, so it really is the
	// latest and later commits are measured against it.
	if at.Before(latest.Date) {
		at = latest.Date
	}
	snap.Date = at
	snap.CreatedAt = time.Time{}
	snap.UpdatedAt = time.Time{}
	if err := s.repo.Create(&snap); err != nil {
		return nil, err
	}
	return out, nil
}

// accountValue reads one account column by its code.
func accountValue(b *domain.Balance, account string) (float64, bool) {
	switch account {
	case "seb":
		return b.Seb, true
	case "swed":
		return b.Swed, true
	case "swed_etf":
		return b.SwedETF, true
	case "seb_pen":
		return b.SebPen, true
	case "luminor":
		return b.Luminor, true
	case "art":
		return b.Art, true
	case "rev_m":
		return b.RevM, true
	case "rev_r":
		return b.RevR, true
	case "rev_stocks":
		return b.RevStocks, true
	case "ibkr_stocks":
		return b.IBKRStocks, true
	case "cash":
		return b.Cash, true
	}
	if domain.IsCustomAccountKey(account) {
		return b.Extra[account], true
	}
	return 0, false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// oldestDate names the entry the "too old" note is about.
func oldestDate(deltas []AccountDelta) time.Time {
	oldest := deltas[0].Date
	for _, d := range deltas[1:] {
		if d.Date.Before(oldest) {
			oldest = d.Date
		}
	}
	return oldest
}

// sortedKeys keeps the apply order deterministic, so an unknown account code
// fails the same way every run instead of depending on map iteration.
func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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

// applyAccountDelta adds delta to the named account column, reporting
// whether the code matched one. An unmatched code is a caller error, not a
// silent no-op.
func applyAccountDelta(b *domain.Balance, account string, delta float64) bool {
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
	default:
		if !domain.IsCustomAccountKey(account) {
			return false
		}
		if b.Extra == nil {
			b.Extra = domain.AccountValues{}
		}
		b.Extra[account] += delta
	}
	return true
}

// extraValues keeps only well-formed added-account keys.
func extraValues(in map[string]float64) domain.AccountValues {
	out := domain.AccountValues{}
	for k, v := range in {
		if domain.IsCustomAccountKey(k) {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	for k, v := range b.Extra {
		b.Extra[k] = roundCents(v)
	}
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
	labels := s.accountLabels()
	for k, v := range latest.Extra {
		name := labels[k]
		if name == "" {
			name = k
		}
		accounts[name] += v
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
