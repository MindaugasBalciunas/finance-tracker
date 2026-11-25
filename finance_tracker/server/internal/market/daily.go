package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// Bar is one trading day: open, high, low and close in the listing's
// currency. The dip plan needs highs and lows (not only closes) to find the
// price gaps a chart leaves behind.
type Bar struct {
	Date  string  `json:"date"`
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

// Series is a listing's daily bars and the currency they are quoted in.
type Series struct {
	Currency string
	Bars     []Bar
}

// Daily returns up to ten years of daily bars for an exact Yahoo symbol
// (VWCE.DE, not VWCE), oldest first. History's "max" range is monthly and
// its long ranges are weekly, which cannot give a 200-day average. Cached
// for 30 minutes, failures for negativeTTL. The slice is shared — copy it
// before changing it.
func Daily(symbol string) (*Series, error) {
	key := "daily:" + symbol
	if v, ok := yahooCacheGet(key); ok {
		if err, failed := v.(error); failed {
			return nil, err
		}
		return v.(*Series), nil
	}
	s, err := fetchDaily(symbol)
	if err == nil && len(s.Bars) == 0 {
		err = fmt.Errorf("no daily prices for %s", symbol)
	}
	if err != nil {
		yahooCacheSet(key, err, negativeTTL)
		return nil, err
	}
	sort.SliceStable(s.Bars, func(i, j int) bool { return s.Bars[i].Date < s.Bars[j].Date })
	yahooCacheSet(key, s, 30*time.Minute)
	return s, nil
}

// fetchDaily is the network call behind Daily (replaced in tests).
var fetchDaily = fetchYahooDailyRaw

func fetchYahooDailyRaw(symbol string) (*Series, error) {
	apiURL := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=10y", yahooBaseURL, url.PathEscape(symbol))
	client := &http.Client{Timeout: 15 * time.Second}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Currency string `json:"currency"`
				} `json:"meta"`
				Timestamps []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open  []*float64 `json:"open"`
						High  []*float64 `json:"high"`
						Low   []*float64 `json:"low"`
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}
	if len(result.Chart.Result) == 0 || len(result.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("no daily prices for %s", symbol)
	}
	r := result.Chart.Result[0]
	q := r.Indicators.Quote[0]
	at := func(s []*float64, i int) float64 {
		if i < len(s) && s[i] != nil {
			return *s[i]
		}
		return 0
	}
	out := make([]Bar, 0, len(r.Timestamps))
	for i, ts := range r.Timestamps {
		b := Bar{Date: time.Unix(ts, 0).UTC().Format("2006-01-02"), Open: at(q.Open, i), High: at(q.High, i), Low: at(q.Low, i), Close: at(q.Close, i)}
		if b.Close <= 0 { // a holiday row Yahoo sends with nulls
			continue
		}
		// A bar with a missing high or low still has a close; never let a
		// zero low look like a gap fill.
		if b.High <= 0 {
			b.High = b.Close
		}
		if b.Low <= 0 {
			b.Low = b.Close
		}
		out = append(out, b)
	}
	return &Series{Currency: r.Meta.Currency, Bars: out}, nil
}
