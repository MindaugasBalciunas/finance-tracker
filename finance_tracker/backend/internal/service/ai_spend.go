package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// The AI spend ledger.
//
// Anthropic publishes no balance endpoint — GET /v1/organizations/balance is a
// 404, and the Admin API's usage and cost reports describe spend, not what is
// left (and need an admin key individual accounts cannot create). So the only
// way this app can show "remaining" is to keep its own books: price every call
// as it happens, let the user record top-ups, and subtract.
//
// Two honest limits, surfaced in the UI rather than buried here:
//   - Costs are list-price estimates on a direct-Claude install (see
//     estimateCostUSD), so the total drifts from a real invoice.
//   - Anything else billed to the same API key is invisible. Another app on
//     the same key makes the remaining figure read high, never low.

// spendKindKey carries which feature made a call down to the one function
// every call passes through. Recording at that choke point is deliberate: a
// ledger that misses calls is worse than no ledger, and there are nine call
// sites that would each have to remember.
type spendKindKey struct{}

func withSpendKind(ctx context.Context, kind string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, spendKindKey{}, kind)
}

func spendKindFrom(ctx context.Context) string {
	if ctx == nil {
		return "other"
	}
	if k, ok := ctx.Value(spendKindKey{}).(string); ok && k != "" {
		return k
	}
	return "other"
}

// spendRecorder is set once, at service construction. It is package-level
// because callGatewayFull is a free function — the single point every gateway
// call funnels through, and therefore the only place a recording cannot be
// forgotten.
var (
	spendMu       sync.RWMutex
	spendRecorder func(*domain.AISpend)
)

func setSpendRecorder(f func(*domain.AISpend)) {
	spendMu.Lock()
	defer spendMu.Unlock()
	spendRecorder = f
}

func recordSpend(s *domain.AISpend) {
	spendMu.RLock()
	f := spendRecorder
	spendMu.RUnlock()
	if f != nil {
		f(s)
	}
}

var errTopUpAmount = errors.New("a top-up has to be a positive amount in USD")

// SpendSummary is what the app knows about AI cost. Every figure is USD.
type SpendSummary struct {
	ThisMonth float64 `json:"this_month"`
	Last30    float64 `json:"last_30_days"`
	AllTime   float64 `json:"all_time"`
	// Remaining is nil until at least one top-up has been recorded — there is
	// nothing to count down from before that, and showing 0 would read as
	// "you are out of credit".
	Remaining   *float64           `json:"remaining"`
	ToppedUp    float64            `json:"topped_up"`
	SinceDate   string             `json:"since_date,omitempty"`
	ByKind      map[string]float64 `json:"by_kind"`
	TopUps      []domain.AITopUp   `json:"top_ups"`
	HasEstimate bool               `json:"has_estimate"`
}

func (s *insightService) SpendReport() (SpendSummary, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var out SpendSummary
	var err error
	if out.ThisMonth, err = s.repo.SpendSince(monthStart); err != nil {
		return out, err
	}
	if out.Last30, err = s.repo.SpendSince(now.AddDate(0, 0, -30)); err != nil {
		return out, err
	}
	if out.AllTime, err = s.repo.SpendSince(time.Time{}); err != nil {
		return out, err
	}
	if out.ByKind, err = s.repo.SpendByKindSince(time.Time{}); err != nil {
		return out, err
	}
	if out.TopUps, err = s.repo.ListTopUps(); err != nil {
		return out, err
	}

	if len(out.TopUps) == 0 {
		return out, nil
	}
	// Count down from the first top-up, not from all time: spend before the
	// user started recording top-ups was paid for by money this ledger never
	// saw, and charging it against the recorded balance would understate it.
	earliest := out.TopUps[0].OccurredOn
	for _, t := range out.TopUps {
		out.ToppedUp += t.AmountUSD
		if t.OccurredOn.Before(earliest) {
			earliest = t.OccurredOn
		}
	}
	spentSince, err := s.repo.SpendSince(earliest)
	if err != nil {
		return out, err
	}
	remaining := out.ToppedUp - spentSince
	out.Remaining = &remaining
	out.SinceDate = earliest.Format("2006-01-02")
	return out, nil
}

func (s *insightService) AddTopUp(amountUSD float64, note string, on time.Time) (*domain.AITopUp, error) {
	if amountUSD <= 0 {
		return nil, errTopUpAmount
	}
	if on.IsZero() {
		on = time.Now()
	}
	t := &domain.AITopUp{AmountUSD: amountUSD, Note: note, OccurredOn: on}
	if err := s.repo.SaveTopUp(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *insightService) DeleteTopUp(id uint) error { return s.repo.DeleteTopUp(id) }
