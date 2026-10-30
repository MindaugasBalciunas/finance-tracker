package market

import (
	"encoding/json"
	"sync"
	"time"
)

// Official inflation: Eurostat's HICP annual rate for Lithuania, the
// yardstick for "is it me, or is everything more expensive?". One small
// public request a day at most; when Eurostat can't be reached the page just
// goes without the comparison.

type Inflation struct {
	Rate   float64 `json:"rate"`  // % year on year
	Month  string  `json:"month"` // YYYY-MM it describes
	Source string  `json:"source"`
}

var hicp struct {
	mu      sync.Mutex
	val     *Inflation
	fetched time.Time
}

const hicpURL = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/prc_hicp_minr?geo=LT&coicop18=TOTAL&unit=RCH_A&lastTimePeriod=1"

// HICPFetch is replaceable in tests.
var HICPFetch = func() ([]byte, error) { return ctxGET(hicpURL, "") }

// LithuaniaInflation returns the latest official rate, or nil if unknown.
func LithuaniaInflation() *Inflation {
	hicp.mu.Lock()
	defer hicp.mu.Unlock()
	if time.Since(hicp.fetched) < 24*time.Hour {
		return hicp.val
	}
	hicp.fetched = time.Now()
	raw, err := HICPFetch()
	if err != nil {
		if hicp.val == nil {
			hicp.fetched = time.Now().Add(-23 * time.Hour) // try again in an hour
		}
		return hicp.val
	}
	if v := parseHICP(raw); v != nil {
		hicp.val = v
	}
	return hicp.val
}

func parseHICP(raw []byte) *Inflation {
	var r struct {
		Value     map[string]float64 `json:"value"`
		Dimension struct {
			Time struct {
				Category struct {
					Index map[string]int `json:"index"`
				} `json:"category"`
			} `json:"time"`
		} `json:"dimension"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	for month, i := range r.Dimension.Time.Category.Index {
		for k, v := range r.Value {
			if k == itoa(i) {
				return &Inflation{Rate: v, Month: month, Source: "Eurostat HICP, Lithuania"}
			}
		}
	}
	return nil
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
