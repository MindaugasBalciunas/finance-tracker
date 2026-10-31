package market

import (
	"errors"
	"testing"
)

func fakeHistory(t *testing.T, f func(ticker string) ([]HistoryPoint, error)) *int {
	t.Helper()
	yahooCache.mu.Lock()
	yahooCache.m = map[string]yahooCacheEntry{}
	yahooCache.mu.Unlock()
	calls := 0
	orig := fetchHistory
	fetchHistory = func(ticker, interval, rangeParam string) ([]HistoryPoint, error) {
		calls++
		return f(ticker)
	}
	t.Cleanup(func() { fetchHistory = orig })
	return &calls
}

// While Yahoo is down, the six suffix tries happen once, not on every page
// load: the failure is remembered for a few minutes.
func TestHistoryRemembersFailures(t *testing.T) {
	calls := fakeHistory(t, func(string) ([]HistoryPoint, error) { return nil, errors.New("yahoo down") })
	if _, err := History("VWCE", "1y"); err == nil {
		t.Fatal("expected an error")
	}
	first := *calls
	if first != 6 {
		t.Fatalf("suffix tries: %d", first)
	}
	if _, err := History("VWCE", "1y"); err == nil || err.Error() != "yahoo down" || *calls != first {
		t.Fatalf("second lookup: err %v, calls %d", err, *calls)
	}
}

// Closes come back oldest first, whatever order the source sent.
func TestHistorySortedOldestFirst(t *testing.T) {
	fakeHistory(t, func(string) ([]HistoryPoint, error) {
		return []HistoryPoint{{"2026-03-01", 3}, {"2026-01-01", 1}, {"2026-02-01", 2}}, nil
	})
	h, err := History("AAPL", "1y")
	if err != nil || len(h) != 3 || h[0].Date != "2026-01-01" || h[2].Date != "2026-03-01" {
		t.Fatalf("%v %+v", err, h)
	}
}
