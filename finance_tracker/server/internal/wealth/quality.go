package wealth

import (
	"database/sql"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

// Quality answers what a professional asks of a portfolio: what it really
// earned a year (money-weighted, so the timing of each buy counts), whether
// the hand-picked stocks beat simply holding the core fund, where it is
// concentrated, and where the next contribution should go to move toward the
// target mix — by buying, not selling (gains are taxed when sold).
type Quality struct {
	Value     float64      `json:"value_eur"`
	Invested  float64      `json:"invested_eur"` // net money put in (buys − sells)
	Return    *float64     `json:"annual_return,omitempty"`
	Since     string       `json:"since"`
	Groups    []GroupStats `json:"groups"` // core funds vs picks
	Benchmark *Benchmark   `json:"benchmark,omitempty"`
	Heavy     []Weight     `json:"heavy,omitempty"` // single positions above the concentration limit
	Targets   []Target     `json:"targets,omitempty"`
	NextBuy   float64      `json:"next_buy,omitempty"` // the contribution the split is for
}

type GroupStats struct {
	Name     string   `json:"name"` // core | picks
	Tickers  []string `json:"tickers"`
	Value    float64  `json:"value_eur"`
	Invested float64  `json:"invested_eur"`
	Return   *float64 `json:"annual_return,omitempty"`
}

// Benchmark is the same money, on the same days, into one fund instead.
type Benchmark struct {
	Ticker string   `json:"ticker"`
	Value  float64  `json:"value_eur"`
	Return *float64 `json:"annual_return,omitempty"`
	Of     string   `json:"of"` // which money: all | picks
}

type Weight struct {
	Ticker string  `json:"ticker"`
	Weight float64 `json:"weight"`
}

type Target struct {
	Ticker string  `json:"ticker"`
	Target float64 `json:"target"` // share 0–1
	Actual float64 `json:"actual"`
	Buy    float64 `json:"buy_eur"` // of the next contribution
}

// InvestSettings: which tickers are the core (broad funds), the benchmark,
// the target mix and the monthly contribution it is spread over.
type InvestSettings struct {
	Core      []string           `json:"core"`
	Benchmark string             `json:"benchmark"`
	Targets   map[string]float64 `json:"targets,omitempty"` // ticker → share 0–1
	Monthly   float64            `json:"monthly,omitempty"`
	MaxSingle float64            `json:"max_single,omitempty"` // concentration limit for one stock, default 10%
}

type flow struct {
	date   time.Time
	amount float64 // money in (+) to the portfolio, EUR
}

// XIRR is the annual rate r with Σ amount·(1+r)^(−years) = 0, where money
// put in is negative and the end value positive. nil when it can't be found
// or the money has been in for under 90 days (a few weeks' move annualised
// says nothing).
func XIRR(flows []flow, end time.Time, value float64) *float64 {
	if len(flows) == 0 {
		return nil
	}
	first := flows[0].date
	for _, f := range flows {
		if f.date.Before(first) {
			first = f.date
		}
	}
	if end.Sub(first) < 90*24*time.Hour {
		return nil
	}
	npv := func(r float64) float64 {
		s := value / math.Pow(1+r, end.Sub(first).Hours()/24/365.25)
		for _, f := range flows {
			s -= f.amount / math.Pow(1+r, f.date.Sub(first).Hours()/24/365.25)
		}
		return s
	}
	lo, hi := -0.99, 10.0
	if npv(lo)*npv(hi) > 0 {
		return nil
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if npv(lo)*npv(mid) <= 0 {
			hi = mid
		} else {
			lo = mid
		}
	}
	r := math.Round((lo+hi)/2*1000) / 1000
	return &r
}

// BuildQuality values the portfolio p (live prices) against its trades.
func BuildQuality(trades []Trade, p Portfolio, s InvestSettings, prices PriceHistory, toEUR func(float64, string) float64, today time.Time) Quality {
	q := Quality{Value: round2(p.ValueEUR), Groups: []GroupStats{}}
	if len(trades) == 0 {
		return q
	}
	core := map[string]bool{}
	for _, t := range s.Core {
		core[t] = true
	}
	value := map[string]float64{}
	for _, h := range p.Holdings {
		if h.ValueEUR != nil {
			value[h.Ticker] = *h.ValueEUR
		}
	}
	ts := append([]Trade(nil), trades...)
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].Date < ts[j].Date })
	q.Since = ts[0].Date
	groupOf := func(t string) string {
		if core[t] {
			return "core"
		}
		return "picks"
	}
	all := []flow{}
	by := map[string][]flow{}
	for _, t := range ts {
		d, err := time.Parse("2006-01-02", t.Date)
		if err != nil {
			continue
		}
		amt := toEUR(t.Shares*t.Price, t.Currency)
		if t.Action == "sell" {
			amt = -amt
		}
		all = append(all, flow{d, amt})
		by[groupOf(t.Ticker)] = append(by[groupOf(t.Ticker)], flow{d, amt})
		q.Invested += amt
	}
	q.Invested = round2(q.Invested)
	q.Return = XIRR(all, today, p.ValueEUR)
	for _, g := range []string{"core", "picks"} {
		fs := by[g]
		if len(fs) == 0 {
			continue
		}
		gs := GroupStats{Name: g, Tickers: []string{}}
		seen := map[string]bool{}
		for _, t := range ts {
			if groupOf(t.Ticker) == g && !seen[t.Ticker] {
				seen[t.Ticker] = true
				gs.Tickers = append(gs.Tickers, t.Ticker)
				gs.Value += value[t.Ticker]
			}
		}
		for _, f := range fs {
			gs.Invested += f.amount
		}
		gs.Value, gs.Invested = round2(gs.Value), round2(gs.Invested)
		gs.Return = XIRR(fs, today, gs.Value)
		q.Groups = append(q.Groups, gs)
	}

	// The picks' money into the benchmark on the same days.
	if s.Benchmark != "" && prices != nil && len(by["picks"]) > 0 {
		if hist, err := prices(s.Benchmark); err == nil && len(hist) > 0 {
			closeOn := func(d time.Time) float64 {
				day := d.Format("2006-01-02")
				c := hist[0].Close
				for _, h := range hist {
					if h.Date > day {
						break
					}
					c = h.Close
				}
				return c
			}
			var units float64
			for _, f := range by["picks"] {
				if c := closeOn(f.date); c > 0 {
					units += f.amount / c
				}
			}
			v := units * hist[len(hist)-1].Close
			q.Benchmark = &Benchmark{Ticker: s.Benchmark, Of: "picks", Value: round2(v), Return: XIRR(by["picks"], today, v)}
		}
	}

	// Concentration: one stock (not a broad fund) above the limit.
	limit := s.MaxSingle
	if limit <= 0 {
		limit = 0.10
	}
	for _, h := range p.Holdings {
		if !core[h.Ticker] && h.Weight > limit {
			q.Heavy = append(q.Heavy, Weight{h.Ticker, h.Weight})
		}
	}

	// Toward the target mix with the next contribution, by buying only.
	if len(s.Targets) > 0 && s.Monthly > 0 {
		q.NextBuy = s.Monthly
		total := p.ValueEUR + s.Monthly
		var need float64
		gaps := map[string]float64{}
		for t, w := range s.Targets {
			if g := total*w - value[t]; g > 0 {
				gaps[t] = g
				need += g
			}
		}
		for t, w := range s.Targets {
			tg := Target{Ticker: t, Target: w}
			if p.ValueEUR > 0 {
				tg.Actual = math.Round(value[t]/p.ValueEUR*1000) / 1000
			}
			if need > 0 {
				tg.Buy = math.Round(s.Monthly*gaps[t]/need*100) / 100
			}
			q.Targets = append(q.Targets, tg)
		}
		sort.Slice(q.Targets, func(i, j int) bool { return q.Targets[i].Target > q.Targets[j].Target })
	}
	return q
}

// LoadInvestSettings reads the owner's choices; with none saved, broad funds
// (by name) are the core and the largest of them the benchmark.
func LoadInvestSettings(d *sql.DB, p Portfolio) InvestSettings {
	var s InvestSettings
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='invest'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	if s.Core == nil {
		best := 0.0
		for _, h := range p.Holdings {
			if IsFund(h.Ticker, h.Name) {
				s.Core = append(s.Core, h.Ticker)
				if h.Weight > best {
					best = h.Weight
					s.Benchmark = h.Ticker
				}
			}
		}
	}
	if s.Benchmark == "" && len(s.Core) > 0 {
		s.Benchmark = s.Core[0]
	}
	return s
}

func SaveInvestSettings(d *sql.DB, s InvestSettings) error {
	raw, _ := json.Marshal(s)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('invest',?)`, string(raw))
	return err
}

// IsFund guesses a broad fund from its name or a well-known ticker.
func IsFund(ticker, name string) bool {
	n := strings.ToUpper(name)
	for _, w := range []string{"ETF", "UCITS", "VANGUARD", "ISHARES", "SPDR", "XTRACKERS", "AMUNDI", "INDEX"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	switch strings.ToUpper(ticker) {
	case "VWCE", "VWRL", "VWRA", "IWDA", "SWDA", "SPYL", "SPY", "VOO", "VTI", "CSPX", "SXR8", "EUNL", "XDWD":
		return true
	}
	return false
}
