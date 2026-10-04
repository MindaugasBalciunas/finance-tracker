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
		if out.Holdings[i].CostEUR != nil && out.CostEUR > 0 {
			out.Holdings[i].CostShare = round3(*out.Holdings[i].CostEUR / out.CostEUR)
		}
	}
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
