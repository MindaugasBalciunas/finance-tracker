// Package marketdata fetches live stock quotes from Yahoo Finance's public
// chart endpoint (no API key). It is a leaf package so both the stock HTTP
// handler and the AI insight service can share one implementation and one
// cache instead of each carrying their own Yahoo client.
package market

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// yahooBaseURL is a variable so tests can point the client at a local server.
var yahooBaseURL = "https://query1.finance.yahoo.com"

// ErrRateLimited is returned (via errors.Is) when Yahoo answers 429.
var ErrRateLimited = errors.New("yahoo rate limited")

// StatusError is a non-200 HTTP status from Yahoo — before this existed, an
// HTML error page surfaced as a misleading JSON parse error.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("yahoo status %d", e.Code) }

// Is lets errors.Is(err, ErrRateLimited) match a 429 StatusError.
func (e *StatusError) Is(target error) bool {
	return target == ErrRateLimited && e.Code == http.StatusTooManyRequests
}

// Meta is the subset of Yahoo's chart "meta" block we use.
type Meta struct {
	RegularMarketPrice float64 `json:"regularMarketPrice"`
	RegularMarketTime  int64   `json:"regularMarketTime"` // unix seconds of the last trade
	FiftyTwoWeekHigh   float64 `json:"fiftyTwoWeekHigh"`
	FiftyTwoWeekLow    float64 `json:"fiftyTwoWeekLow"`
	Currency           string  `json:"currency"`
}

// Quote is a resolved live quote for a ticker.
type Quote struct {
	Ticker         string    `json:"ticker"`          // as requested
	ResolvedTicker string    `json:"resolved_ticker"` // the symbol Yahoo answered on (may carry an exchange suffix)
	Price          float64   `json:"price"`
	Currency       string    `json:"currency"`
	Week52High     float64   `json:"week_52_high"`
	Week52Low      float64   `json:"week_52_low"`
	AsOf           time.Time `json:"as_of"` // when the price last traded — zero if Yahoo omitted it; callers can judge staleness
}

// euroSuffixes are tried, in order, when a bare ticker returns nothing —
// covers Amsterdam, Xetra, London, Milan and Paris listings (e.g. VWCE).
var euroSuffixes = []string{".AS", ".DE", ".L", ".MI", ".PA"}

var cache = struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}{m: make(map[string]cacheEntry)}

type cacheEntry struct {
	q       *Quote
	err     error // negative entry: the lookup failed and this is why
	expires time.Time
}

// maxCacheEntries bounds the shared cache — tickers can be caller- or
// model-supplied, so an unbounded map is a memory-growth vector.
const maxCacheEntries = 256

// negativeTTL is how long a failed lookup stays cached. A miss costs up to
// six serial round-trips through the suffix fallback, so retrying every call
// would stall callers for ~a minute each time.
const negativeTTL = 5 * time.Minute

// cacheSet inserts an entry, first dropping expired ones and then, if the
// cache is still full, the entries closest to expiry (a proxy for oldest).
func cacheSet(key string, e cacheEntry) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := time.Now()
	for k, old := range cache.m {
		if now.After(old.expires) {
			delete(cache.m, k)
		}
	}
	for len(cache.m) >= maxCacheEntries {
		var oldestKey string
		var oldest time.Time
		for k, old := range cache.m {
			if oldestKey == "" || old.expires.Before(oldest) {
				oldestKey, oldest = k, old.expires
			}
		}
		delete(cache.m, oldestKey)
	}
	cache.m[key] = e
}

// Fetch resolves a ticker (trying European exchange suffixes if a bare
// symbol misses) and returns its latest quote. Results are cached for five
// minutes — quotes move slowly relative to how often a daily status update
// or a chat turn asks for them, and each miss can cost several round-trips.
func Fetch(ticker string) (*Quote, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	cache.mu.Lock()
	if e, ok := cache.m[ticker]; ok && time.Now().Before(e.expires) {
		cache.mu.Unlock()
		if e.err != nil {
			return nil, e.err
		}
		return e.q, nil
	}
	cache.mu.Unlock()

	candidates := []string{ticker}
	if !strings.Contains(ticker, ".") {
		for _, s := range euroSuffixes {
			candidates = append(candidates, ticker+s)
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		m, err := FetchMeta(candidate)
		if err == nil && m.RegularMarketPrice > 0 {
			q := &Quote{
				Ticker: ticker, ResolvedTicker: candidate,
				Price: m.RegularMarketPrice, Currency: m.Currency,
				Week52High: m.FiftyTwoWeekHigh, Week52Low: m.FiftyTwoWeekLow,
			}
			if m.RegularMarketTime > 0 {
				q.AsOf = time.Unix(m.RegularMarketTime, 0).UTC()
			}
			cacheSet(ticker, cacheEntry{q: q, expires: time.Now().Add(5 * time.Minute)})
			return q, nil
		}
		if err != nil {
			lastErr = err
			if errors.Is(err, ErrRateLimited) {
				break // trying more suffixes while throttled only digs the hole deeper
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no price for %s", ticker)
	}
	cacheSet(ticker, cacheEntry{err: lastErr, expires: time.Now().Add(negativeTTL)})
	return nil, lastErr
}

// Price is a convenience wrapper returning just the latest price.
func Price(ticker string) (float64, error) {
	q, err := Fetch(ticker)
	if err != nil {
		return 0, err
	}
	return q.Price, nil
}

// FetchMeta calls Yahoo's v8 chart endpoint for one exact symbol (no suffix
// fallback, no cache) and returns its meta block.
func FetchMeta(ticker string) (*Meta, error) {
	apiURL := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=1d", yahooBaseURL, url.PathEscape(ticker))
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode}
	}

	var result struct {
		Chart struct {
			Result []struct {
				Meta Meta `json:"meta"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}
	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for ticker %s", ticker)
	}
	m := result.Chart.Result[0].Meta
	return &m, nil
}
