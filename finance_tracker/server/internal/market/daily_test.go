package market

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func resetYahooCache() {
	yahooCache.mu.Lock()
	yahooCache.m = map[string]yahooCacheEntry{}
	yahooCache.mu.Unlock()
}

// Daily bars come back oldest first with the listing's currency; a holiday
// row with null prices is dropped, and a missing low takes the close so it
// can never look like a gap being filled down to zero.
func TestDailyBarsFromYahoo(t *testing.T) {
	resetYahooCache()
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path + "?" + r.URL.RawQuery
		fmt.Fprint(w, `{"chart":{"result":[{"meta":{"currency":"EUR"},
			"timestamp":[1764000000,1763913600,1763827200],
			"indicators":{"quote":[{"open":[101,100,null],"high":[102,101,null],"low":[null,99,null],"close":[101.5,100.5,null]}]}}],"error":null}}`)
	}))
	defer srv.Close()
	pointAt(t, srv)

	s, err := Daily("VWCE.DE")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "/v8/finance/chart/VWCE.DE") || !strings.Contains(path, "interval=1d") || !strings.Contains(path, "range=10y") {
		t.Fatalf("asked %s", path)
	}
	if s.Currency != "EUR" || len(s.Bars) != 2 || s.Bars[0].Close != 100.5 || s.Bars[1].Low != 101.5 {
		t.Fatalf("%+v", s)
	}
}

// While Yahoo answers with an error, the failure is remembered for a few
// minutes instead of being asked again on every page load.
func TestDailyRemembersFailures(t *testing.T) {
	resetYahooCache()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"chart":{"result":null,"error":{"description":"No data found, symbol may be delisted"}}}`)
	}))
	defer srv.Close()
	pointAt(t, srv)

	for range 2 {
		if _, err := Daily("GONE.DE"); err == nil || !strings.Contains(err.Error(), "delisted") {
			t.Fatalf("err %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("calls %d", calls.Load())
	}
}

// A 429 from Yahoo is a rate limit, not a parse error.
func TestDailyRateLimited(t *testing.T) {
	resetYahooCache()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	pointAt(t, srv)
	if _, err := Daily("VWCE.DE"); err == nil || !isRateLimited(err) {
		t.Fatalf("err %v", err)
	}
}

func isRateLimited(err error) bool {
	se, ok := err.(*StatusError)
	return ok && se.Is(ErrRateLimited)
}
