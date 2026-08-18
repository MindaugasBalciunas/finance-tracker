package marketdata

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// resetCache empties the shared quote cache so tests don't see each other.
func resetCache() {
	cache.mu.Lock()
	cache.m = make(map[string]cacheEntry)
	cache.mu.Unlock()
}

// pointAt swaps yahooBaseURL to a test server for the duration of the test.
func pointAt(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := yahooBaseURL
	yahooBaseURL = srv.URL
	t.Cleanup(func() { yahooBaseURL = orig })
}

func chartBody(price float64, marketTime int64) string {
	return fmt.Sprintf(`{"chart":{"result":[{"meta":{"regularMarketPrice":%g,"regularMarketTime":%d,"fiftyTwoWeekHigh":210.5,"fiftyTwoWeekLow":150.25,"currency":"USD"}}],"error":null}}`,
		price, marketTime)
}

func TestFetchHappyPath(t *testing.T) {
	resetCache()
	marketTime := time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC).Unix()
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, chartBody(198.5, marketTime))
	}))
	defer srv.Close()
	pointAt(t, srv)

	q, err := Fetch("AAPL")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if q.Price != 198.5 || q.Currency != "USD" || q.ResolvedTicker != "AAPL" {
		t.Errorf("unexpected quote: %+v", q)
	}
	if q.Week52High != 210.5 || q.Week52Low != 150.25 {
		t.Errorf("unexpected 52wk range: %+v", q)
	}
	if !q.AsOf.Equal(time.Unix(marketTime, 0).UTC()) {
		t.Errorf("AsOf = %v, want %v", q.AsOf, time.Unix(marketTime, 0).UTC())
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1 (bare symbol should resolve first)", got)
	}

	// Second call within TTL is served from cache — no extra request.
	if _, err := Fetch("AAPL"); err != nil {
		t.Fatalf("cached Fetch: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests after cached call = %d, want 1", got)
	}
}

func TestFetchMetaNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "<html>server error</html>")
	}))
	defer srv.Close()
	pointAt(t, srv)

	_, err := FetchMeta("AAPL")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("want *StatusError, got %v", err)
	}
	if statusErr.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want 500", statusErr.Code)
	}
	if errors.Is(err, ErrRateLimited) {
		t.Error("500 must not match ErrRateLimited")
	}
}

func TestFetchMeta429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "<html>rate limited</html>")
	}))
	defer srv.Close()
	pointAt(t, srv)

	_, err := FetchMeta("AAPL")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestFetchNegativeCache(t *testing.T) {
	resetCache()
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	pointAt(t, srv)

	_, err := Fetch("NOPE")
	if err == nil {
		t.Fatal("want error for unknown ticker")
	}
	// bare symbol + 5 European suffixes
	first := requests.Load()
	if first != int64(1+len(euroSuffixes)) {
		t.Errorf("requests = %d, want %d", first, 1+len(euroSuffixes))
	}

	// Second call within the negative TTL is answered from cache.
	_, err2 := Fetch("NOPE")
	if err2 == nil {
		t.Fatal("want cached error for unknown ticker")
	}
	if got := requests.Load(); got != first {
		t.Errorf("requests after cached miss = %d, want %d", got, first)
	}
}

func TestFetchMetaRegularMarketTime(t *testing.T) {
	marketTime := int64(1755460800)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chartBody(42, marketTime))
	}))
	defer srv.Close()
	pointAt(t, srv)

	m, err := FetchMeta("AAPL")
	if err != nil {
		t.Fatalf("FetchMeta: %v", err)
	}
	if m.RegularMarketTime != marketTime {
		t.Errorf("RegularMarketTime = %d, want %d", m.RegularMarketTime, marketTime)
	}
}

func TestCacheSetBounded(t *testing.T) {
	resetCache()
	for i := 0; i < maxCacheEntries+50; i++ {
		cacheSet(fmt.Sprintf("t%d", i), cacheEntry{
			q:       &Quote{Ticker: "X"},
			expires: time.Now().Add(time.Duration(i) * time.Second),
		})
	}
	cache.mu.Lock()
	n := len(cache.m)
	_, newestKept := cache.m[fmt.Sprintf("t%d", maxCacheEntries+49)]
	cache.mu.Unlock()
	if n > maxCacheEntries {
		t.Errorf("cache size = %d, want <= %d", n, maxCacheEntries)
	}
	if !newestKept {
		t.Error("newest entry should survive eviction")
	}
	resetCache()
}
