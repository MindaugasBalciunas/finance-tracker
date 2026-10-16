package wealth

import (
	"math"
	"testing"
	"time"

	"ft/internal/market"
)

// A portfolio's real annual return, the picks against the same money in
// the core fund, the next contribution toward the target mix (buying only),
// and a single stock above the concentration limit.
func TestQuality(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	if r := XIRR([]flow{{day("2025-01-01"), 1000}}, day("2026-01-01"), 1100); r == nil || math.Abs(*r-0.1) > 0.002 {
		t.Fatalf("1000 → 1100 in a year: %v", r)
	}
	if XIRR([]flow{{day("2026-09-01"), 1000}}, day("2026-10-01"), 1100) != nil {
		t.Error("a month's move must not be annualised")
	}

	trades := []Trade{
		{Date: "2025-01-02", Action: "buy", Ticker: "VWCE", Shares: 70, Price: 100, Currency: "EUR"},
		{Date: "2025-01-02", Action: "buy", Ticker: "PICK", Shares: 10, Price: 300, Currency: "EUR"},
	}
	v := func(x float64) *float64 { return &x }
	p := Portfolio{ValueEUR: 10000, Holdings: []Holding{
		{Ticker: "VWCE", ValueEUR: v(7700), Weight: 0.77},
		{Ticker: "PICK", ValueEUR: v(2300), Weight: 0.23},
	}}
	prices := func(tk string) ([]market.HistoryPoint, error) {
		return []market.HistoryPoint{{Date: "2025-01-02", Close: 100}, {Date: "2026-01-02", Close: 110}}, nil
	}
	s := InvestSettings{Core: []string{"VWCE"}, Benchmark: "VWCE", Targets: map[string]float64{"VWCE": 0.8, "PICK": 0.2}, Monthly: 1000}
	q := BuildQuality(trades, p, s, prices, func(a float64, _ string) float64 { return a }, day("2026-01-02"))
	if q.Invested != 10000 || q.Return == nil || math.Abs(*q.Return) > 0.002 {
		t.Errorf("whole portfolio flat over the year: invested %v return %v", q.Invested, q.Return)
	}
	var core, picks GroupStats
	for _, g := range q.Groups {
		if g.Name == "core" {
			core = g
		} else {
			picks = g
		}
	}
	if *core.Return < 0.09 || *picks.Return > -0.22 {
		t.Errorf("core +10%%, picks −23%%: %v / %v", *core.Return, *picks.Return)
	}
	// The picks' €3,000 in VWCE on the same day would be €3,300.
	if q.Benchmark == nil || q.Benchmark.Value != 3300 || q.Benchmark.Of != "picks" {
		t.Errorf("benchmark %+v", q.Benchmark)
	}
	if len(q.Heavy) != 1 || q.Heavy[0].Ticker != "PICK" {
		t.Errorf("concentration %+v", q.Heavy)
	}
	// 11,000 after the buy: VWCE should be 8,800 (has 7,700), PICK 2,200 (has 2,300) → all 1,000 into VWCE.
	for _, tg := range q.Targets {
		if (tg.Ticker == "VWCE" && tg.Buy != 1000) || (tg.Ticker == "PICK" && tg.Buy != 0) {
			t.Errorf("next buy %+v", q.Targets)
		}
	}
}
