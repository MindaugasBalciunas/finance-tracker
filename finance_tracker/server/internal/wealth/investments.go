package wealth

import (
	"database/sql"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ft/internal/db"
	"ft/internal/market"
)

type Trade struct {
	ID        int64   `json:"id"`
	Date      string  `json:"date"`
	AccountID string  `json:"account_id"`
	Action    string  `json:"action"` // buy | sell
	Ticker    string  `json:"ticker"`
	Shares    float64 `json:"shares"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	Notes     string  `json:"notes"`
}

var tickerRe = regexp.MustCompile(`^[A-Z0-9.\-=^]{1,15}$`)

func ValidTicker(t string) bool { return tickerRe.MatchString(t) }

func ListTrades(q *sql.DB) ([]Trade, error) {
	rows, err := q.Query(`SELECT id,date,COALESCE(account_id,''),action,ticker,shares,price,currency,notes FROM trades ORDER BY date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trade{}
	for rows.Next() {
		var t Trade
		rows.Scan(&t.ID, &t.Date, &t.AccountID, &t.Action, &t.Ticker, &t.Shares, &t.Price, &t.Currency, &t.Notes)
		out = append(out, t)
	}
	return out, nil
}

func SaveTrade(d *sql.DB, t *Trade) error {
	t.Ticker = strings.ToUpper(strings.TrimSpace(t.Ticker))
	t.Action = strings.ToLower(t.Action)
	t.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
	if t.Currency == "" {
		t.Currency = "USD"
	}
	if !ValidTicker(t.Ticker) {
		return errors.New("invalid ticker")
	}
	if t.Action != "buy" && t.Action != "sell" {
		return errors.New("action must be buy or sell")
	}
	if t.Shares <= 0 || t.Price <= 0 {
		return errors.New("shares and price must be positive")
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return errors.New("invalid date")
	}
	var acct any
	if t.AccountID != "" {
		acct = t.AccountID
	}
	if t.ID == 0 {
		res, err := d.Exec(`INSERT INTO trades(date,account_id,action,ticker,shares,price,currency,notes,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			t.Date, acct, t.Action, t.Ticker, t.Shares, t.Price, t.Currency, t.Notes, db.Now())
		if err != nil {
			return err
		}
		t.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := d.Exec(`UPDATE trades SET date=?,account_id=?,action=?,ticker=?,shares=?,price=?,currency=?,notes=? WHERE id=?`,
		t.Date, acct, t.Action, t.Ticker, t.Shares, t.Price, t.Currency, t.Notes, t.ID)
	return err
}

func DeleteTrade(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM trades WHERE id=?`, id)
	return err
}

// Holding is one position, average-cost basis, in its trade currency, plus
// a live valuation in EUR when the quote is available.
type Holding struct {
	Ticker       string   `json:"ticker"`
	Name         string   `json:"name,omitempty"`
	AccountID    string   `json:"account_id"`
	Currency     string   `json:"currency"`
	Shares       float64  `json:"shares"`
	AvgCost      float64  `json:"avg_cost"`
	Cost         float64  `json:"cost"`
	Realized     float64  `json:"realized"`
	FirstBuy     string   `json:"first_buy"`
	Price        *float64 `json:"price,omitempty"`
	PriceAsOf    string   `json:"price_as_of,omitempty"`
	Value        *float64 `json:"value,omitempty"`
	Gain         *float64 `json:"gain,omitempty"`
	GainPct      *float64 `json:"gain_pct,omitempty"`
	ValueEUR     *float64 `json:"value_eur,omitempty"`
	CostEUR      *float64 `json:"cost_eur,omitempty"`
	Week52High   float64  `json:"week52_high,omitempty"`
	Week52Low    float64  `json:"week52_low,omitempty"`
	CostShare    float64  `json:"cost_share"` // of total book cost (EUR)
	DayPct       *float64 `json:"day_pct,omitempty"`        // today's move vs the previous close
	DayChangeEUR *float64 `json:"day_change_eur,omitempty"` // that move on this position, EUR
	Weight       float64  `json:"weight"`                   // share of market value (EUR)
	QuoteError   string   `json:"quote_error,omitempty"`
}

type Portfolio struct {
	Holdings   []Holding `json:"holdings"`
	Closed     []Holding `json:"closed"`
	CostEUR    float64   `json:"cost_eur"`
	ValueEUR   float64   `json:"value_eur"`
	GainEUR    float64   `json:"gain_eur"`
	RealizedEUR float64  `json:"realized_eur"`
	USDPerEUR  float64   `json:"usd_per_eur"`
	DayChangeEUR float64 `json:"day_change_eur"`
	Live       bool      `json:"live"`
}

// fx converts a currency amount to EUR using Yahoo rates (cached).
type fx struct {
	mu    sync.Mutex
	rates map[string]float64
}

func (f *fx) toEUR(amount float64, cur string) (float64, bool) {
	if cur == "EUR" || cur == "" {
		return amount, true
	}
	if cur == "GBp" || cur == "GBX" {
		amount, cur = amount/100, "GBP"
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rate, ok := f.rates[cur]
	if !ok {
		if q, err := market.Fetch("EUR" + cur + "=X"); err == nil && q.Price > 0 {
			rate = q.Price
		}
		f.rates[cur] = rate
	}
	if rate <= 0 {
		return 0, false
	}
	return amount / rate, true
}

// BuildPortfolio replays trades (average cost) and, when live, prices every
// open position in parallel.
func BuildPortfolio(trades []Trade, live bool) Portfolio {
	type pos struct {
		ticker, acct, cur, first string
		shares, cost, realized  float64
	}
	positions := map[string]*pos{}
	var order []string
	for _, t := range trades {
		p := positions[t.Ticker]
		if p == nil {
			p = &pos{ticker: t.Ticker, acct: t.AccountID, cur: t.Currency, first: t.Date}
			positions[t.Ticker] = p
			order = append(order, t.Ticker)
		}
		switch t.Action {
		case "buy":
			if p.shares == 0 {
				p.first = t.Date
			}
			p.cost += t.Shares * t.Price
			p.shares += t.Shares
		case "sell":
			sold := math.Min(t.Shares, p.shares)
			if sold > 0 {
				avg := p.cost / p.shares
				p.realized += sold * (t.Price - avg)
				p.cost -= sold * avg
				p.shares -= sold
			}
			if p.shares < 1e-6 {
				p.shares, p.cost = 0, 0
			}
		}
	}
	out := Portfolio{Holdings: []Holding{}, Closed: []Holding{}, Live: live}
	rates := &fx{rates: map[string]float64{}}
	var wg sync.WaitGroup
	hs := make([]Holding, len(order))
	for i, tk := range order {
		p := positions[tk]
		h := Holding{Ticker: tk, AccountID: p.acct, Currency: p.cur, Shares: p.shares, Cost: round2(p.cost), Realized: round2(p.realized), FirstBuy: p.first}
		if p.shares > 0 {
			h.AvgCost = p.cost / p.shares
		}
		hs[i] = h
		if live && p.shares > 0 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				h := &hs[i]
				q, err := market.Fetch(h.Ticker)
				if err != nil || q.Price <= 0 {
					if err != nil {
						h.QuoteError = err.Error()
					}
					return
				}
				price := q.Price
				if q.Currency == "GBp" && h.Currency == "GBP" {
					price /= 100
				}
				v := round2(price * h.Shares)
				g := round2(v - h.Cost)
				h.Price, h.Value, h.Gain = &price, &v, &g
				if h.Cost > 0 {
					pct := round3(g / h.Cost)
					h.GainPct = &pct
				}
				h.Week52High, h.Week52Low = q.Week52High, q.Week52Low
				h.Name = q.Name
				if q.PrevClose > 0 {
					prev := q.PrevClose
					if q.Currency == "GBp" && h.Currency == "GBP" {
						prev /= 100
					}
					d := round3(price/prev - 1)
					h.DayPct = &d
					dv := round2((price - prev) * h.Shares) // trade currency; EUR below
					h.DayChangeEUR = &dv
				}
				if !q.AsOf.IsZero() {
					h.PriceAsOf = q.AsOf.Format(time.RFC3339)
				}
			}(i)
		}
	}
	wg.Wait()
	for i := range hs {
		h := &hs[i]
		if c, ok := rates.toEUR(h.Cost, h.Currency); ok {
			c = round2(c)
			h.CostEUR = &c
			out.CostEUR += c
		}
		if r, ok := rates.toEUR(h.Realized, h.Currency); ok {
			out.RealizedEUR += r
		}
		if h.DayChangeEUR != nil {
			if v, ok := rates.toEUR(*h.DayChangeEUR, h.Currency); ok {
				v = round2(v)
				h.DayChangeEUR = &v
				out.DayChangeEUR += v
			} else {
				h.DayChangeEUR = nil
			}
		}
		if h.Value != nil {
			if v, ok := rates.toEUR(*h.Value, h.Currency); ok {
				v = round2(v)
				h.ValueEUR = &v
				out.ValueEUR += v
			}
		} else if h.CostEUR != nil && h.Shares > 0 {
			out.ValueEUR += *h.CostEUR
		}
		if h.Shares > 0 {
			out.Holdings = append(out.Holdings, *h)
		} else {
			out.Closed = append(out.Closed, *h)
		}
	}
	for i := range out.Holdings {
		h := &out.Holdings[i]
		if h.CostEUR != nil && out.CostEUR > 0 {
			h.CostShare = round3(*h.CostEUR / out.CostEUR)
		}
		v := h.ValueEUR
		if v == nil {
			v = h.CostEUR
		}
		if v != nil && out.ValueEUR > 0 {
			h.Weight = round3(*v / out.ValueEUR)
		}
	}
	out.DayChangeEUR = round2(out.DayChangeEUR)
	sort.Slice(out.Holdings, func(i, j int) bool { return out.Holdings[i].Cost > out.Holdings[j].Cost })
	out.GainEUR = round2(out.ValueEUR - out.CostEUR)
	out.CostEUR, out.ValueEUR, out.RealizedEUR = round2(out.CostEUR), round2(out.ValueEUR), round2(out.RealizedEUR)
	if r, ok := rates.rates["USD"]; ok {
		out.USDPerEUR = r
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// Scenario values a position at analyst targets (or its 52-week range for
// ETFs without coverage), in EUR.
type Scenario struct {
	Ticker    string   `json:"ticker"`
	ValueEUR  float64  `json:"value_eur"`
	LowEUR    *float64 `json:"low_eur,omitempty"`
	MeanEUR   *float64 `json:"mean_eur,omitempty"`
	HighEUR   *float64 `json:"high_eur,omitempty"`
	Analysts  int      `json:"analysts"`
	Basis     string   `json:"basis"`
}

type Scenarios struct {
	Positions []Scenario `json:"positions"`
	ValueEUR  float64    `json:"value_eur"`
	LowEUR    float64    `json:"low_eur"`
	MeanEUR   float64    `json:"mean_eur"`
	HighEUR   float64    `json:"high_eur"`
	Covered   float64    `json:"covered"` // share of value with analyst targets (not 52-week fallbacks)
}

// AnalystFunc looks up targets; injectable for tests.
type AnalystFunc func(ticker string) (low, mean, high float64, analysts int, basis string, err error)

// BuildScenarios applies each position's target range to its share count.
// Positions without targets count at today's value in every scenario.
func BuildScenarios(p Portfolio, lookup AnalystFunc) Scenarios {
	var out Scenarios
	var covered float64
	for _, h := range p.Holdings {
		if h.ValueEUR == nil || h.Value == nil || *h.Value == 0 {
			continue
		}
		rate := *h.ValueEUR / *h.Value // EUR per unit of trade currency
		s := Scenario{Ticker: h.Ticker, ValueEUR: *h.ValueEUR}
		low, mean, high := *h.ValueEUR, *h.ValueEUR, *h.ValueEUR
		if lo, me, hi, n, basis, err := lookup(h.Ticker); err == nil && me > 0 {
			l, m, x := round2(lo*h.Shares*rate), round2(me*h.Shares*rate), round2(hi*h.Shares*rate)
			s.LowEUR, s.MeanEUR, s.HighEUR, s.Analysts, s.Basis = &l, &m, &x, n, basis
			low, mean, high = l, m, x
			if n > 0 { // only real analyst coverage counts; a 52-week range is not a target
				covered += *h.ValueEUR
			}
		}
		out.Positions = append(out.Positions, s)
		out.ValueEUR += *h.ValueEUR
		out.LowEUR += low
		out.MeanEUR += mean
		out.HighEUR += high
	}
	if out.ValueEUR > 0 {
		out.Covered = round3(covered / out.ValueEUR)
	}
	out.ValueEUR, out.LowEUR, out.MeanEUR, out.HighEUR = round2(out.ValueEUR), round2(out.LowEUR), round2(out.MeanEUR), round2(out.HighEUR)
	return out
}

// PerfPoint is the stock book on one day: what it was worth and what the
// open positions cost (average cost), both in EUR.
type PerfPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value_eur"`
	Cost  float64 `json:"cost_eur"`
}

// PriceHistory returns daily/weekly closes for a ticker (injectable for tests).
type PriceHistory func(ticker string) ([]market.HistoryPoint, error)

// PortfolioHistory replays the trades over time and values the holdings at
// each close. FX uses today's rates (toEUR), so the line shows what the
// positions did, not currency noise. Before a ticker's first fetched close
// its earliest close is used; with no prices at all it counts at cost. Dates: every close in [from, today], plus today.
func PortfolioHistory(trades []Trade, from, today string, prices PriceHistory, toEUR func(amount float64, cur string) float64) []PerfPoint {
	if len(trades) == 0 {
		return []PerfPoint{}
	}
	ts := append([]Trade(nil), trades...)
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].Date < ts[j].Date })
	cur := map[string]string{}
	for _, t := range ts {
		cur[t.Ticker] = t.Currency
	}
	closes := map[string][]market.HistoryPoint{}
	dateSet := map[string]bool{today: true}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for tk := range cur {
		wg.Add(1)
		go func(tk string) {
			defer wg.Done()
			h, err := prices(tk)
			if err != nil {
				return
			}
			sort.Slice(h, func(i, j int) bool { return h[i].Date < h[j].Date })
			mu.Lock()
			closes[tk] = h
			for _, p := range h {
				if p.Date >= from && p.Date <= today {
					dateSet[p.Date] = true
				}
			}
			mu.Unlock()
		}(tk)
	}
	wg.Wait()
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		if d >= from {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	closeAt := func(tk, d string) (float64, bool) {
		h := closes[tk]
		i := sort.Search(len(h), func(i int) bool { return h[i].Date > d })
		if i == 0 {
			// Before the fetched window starts: the earliest close is the
			// best estimate (a few days off at most), better than cost.
			if len(h) > 0 && h[0].Close > 0 {
				return h[0].Close, true
			}
			return 0, false
		}
		return h[i-1].Close, h[i-1].Close > 0
	}
	type pos struct{ shares, cost float64 }
	held := map[string]*pos{}
	next := 0
	out := make([]PerfPoint, 0, len(dates))
	for _, d := range dates {
		for ; next < len(ts) && ts[next].Date <= d; next++ {
			t := ts[next]
			p := held[t.Ticker]
			if p == nil {
				p = &pos{}
				held[t.Ticker] = p
			}
			if t.Action == "buy" {
				p.shares += t.Shares
				p.cost += t.Shares * t.Price
			} else if p.shares > 0 {
				sold := math.Min(t.Shares, p.shares)
				p.cost -= sold * p.cost / p.shares
				p.shares -= sold
				if p.shares < 1e-6 {
					p.shares, p.cost = 0, 0
				}
			}
		}
		var value, cost float64
		for tk, p := range held {
			if p.shares <= 0 {
				continue
			}
			c := cur[tk]
			cost += toEUR(p.cost, c)
			if px, ok := closeAt(tk, d); ok {
				value += toEUR(px*p.shares, c)
			} else {
				value += toEUR(p.cost, c)
			}
		}
		if value == 0 && cost == 0 && len(out) == 0 {
			continue // nothing held yet
		}
		out = append(out, PerfPoint{Date: d, Value: round2(value), Cost: round2(cost)})
	}
	return out
}

// LiveEUR converts with today's Yahoo FX (cached), 1:1 when unavailable.
func LiveEUR() func(float64, string) float64 {
	f := &fx{rates: map[string]float64{}}
	return func(a float64, c string) float64 {
		if v, ok := f.toEUR(a, c); ok {
			return v
		}
		return a
	}
}
