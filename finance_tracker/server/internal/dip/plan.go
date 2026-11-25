package dip

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"ft/internal/market"
	"ft/internal/money"
)

// Fund is one ETF on the plan. Symbol is the exact Yahoo listing the owner
// buys (VWCE.DE); Proxy, when set, is a long-lived listing of the same index
// whose history gives the averages (VALL started in 2026, so it borrows
// VWCE's). Reserve is the cash set aside for this fund's falls, split in
// three tranches.
type Fund struct {
	Symbol  string      `json:"symbol"`
	Name    string      `json:"name"`
	Proxy   string      `json:"proxy,omitempty"`
	Reserve money.Cents `json:"reserve"`
}

// Settings is the plan: which funds, and how much reserve each has.
type Settings struct {
	Funds []Fund `json:"funds"`
	Saved bool   `json:"-"`
	// Trades are the owner's trades, read with the plan: they show which
	// rungs were bought and put the buys on the charts.
	Trades []TradeIn `json:"-"`
}

// Defaults are the owner's ETFs (2026-11): the world fund, its all-cap
// twin, the S&P 500 and two European Nasdaq-100 listings (UBS's BCFP only
// trades since March 2025, so it borrows SXRV's history), all in EUR on
// Xetra/Amsterdam.
func Defaults() Settings {
	return Settings{Funds: []Fund{
		{Symbol: "VWCE.DE", Name: "Vanguard FTSE All-World"},
		{Symbol: "VALL.AS", Name: "Vanguard FTSE Global All-Cap", Proxy: "VWCE.DE"},
		{Symbol: "SPYL.DE", Name: "SPDR S&P 500", Proxy: "SXR8.DE"},
		{Symbol: "SXRV.DE", Name: "iShares Nasdaq-100"},
		{Symbol: "BCFP.DE", Name: "UBS Nasdaq-100", Proxy: "SXRV.DE"},
	}}
}

// Three plans run on the same ladder, each with its own funds and reserve:
// the index ETFs ("funds", the plan as written), the owner's single-stock
// bets at Revolut ("bets") and bitcoin ("crypto"), added 2026-11-25.
const (
	Funds  = "funds"
	Bets   = "bets"
	Crypto = "crypto"
)

// Plans in the order they claim the cash above the safety net.
var Plans = []string{Funds, Bets, Crypto}

var settingsKey = map[string]string{Funds: "dip_plan", Bets: "dip_bets", Crypto: "dip_crypto"}

// ValidPlan says whether a plan name from a request is one of the two.
func ValidPlan(plan string) bool { _, ok := settingsKey[plan]; return ok }

type queryRower interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

// loadTrades reads the trades the plan needs, oldest first.
func loadTrades(q queryRower) []TradeIn {
	out := []TradeIn{}
	rows, err := q.Query(`SELECT date, ticker, action, price, shares, currency FROM trades ORDER BY date, id`)
	if err != nil {
		return out
	}
	for rows.Next() {
		var t TradeIn
		if rows.Scan(&t.Date, &t.Ticker, &t.Action, &t.Price, &t.Shares, &t.Currency) == nil {
			t.Action = strings.ToLower(t.Action)
			out = append(out, t)
		}
	}
	rows.Close()
	// Bitcoin isn't in the trades: money moved into a crypto account is the
	// buy (the BTC accounts hold the coins). Priced later from the day's close.
	crypto, err := q.Query(`SELECT t.date, t.amount FROM transactions t JOIN accounts a ON a.id = t.to_account_id
		WHERE t.kind = 'transfer' AND a.kind = 'crypto' AND t.pending = 0 ORDER BY t.date, t.id`)
	if err != nil {
		return out
	}
	defer crypto.Close()
	for crypto.Next() {
		var date string
		var cents money.Cents
		if crypto.Scan(&date, &cents) == nil {
			out = append(out, TradeIn{Date: date, Ticker: "BTC", Action: "buy", Currency: "EUR", EUR: cents.Float()})
		}
	}
	return out
}

// LoadSettings returns the saved ETF plan, or the defaults before one is saved.
func LoadSettings(q queryRower) Settings { return LoadPlan(q, Funds) }

// LoadPlan returns a saved plan. Before one is saved the ETF plan has its
// defaults and the bets plan is empty (its defaults come from the holdings,
// which the API reads); Saved says which.
func LoadPlan(q queryRower, plan string) Settings {
	s := loadPlan(q, plan)
	s.Trades = loadTrades(q)
	return s
}

func loadPlan(q queryRower, plan string) Settings {
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key=?`, settingsKey[plan]).Scan(&raw) == nil {
		var s Settings
		if json.Unmarshal([]byte(raw), &s) == nil && s.Funds != nil {
			s.Saved = true
			return s
		}
	}
	switch plan {
	case Funds:
		return Defaults()
	case Crypto:
		return Settings{Funds: []Fund{{Symbol: "BTC-EUR", Name: "Bitcoin"}}}
	}
	return Settings{Funds: []Fund{}}
}

var symbolRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.\-]{0,14}$`)

// Validate cleans the plan and says what is wrong in a sentence.
func (s *Settings) Validate() error {
	if len(s.Funds) > 8 {
		return fmt.Errorf("keep the plan to 8 funds or fewer")
	}
	seen := map[string]bool{}
	for i := range s.Funds {
		f := &s.Funds[i]
		f.Symbol = strings.ToUpper(strings.TrimSpace(f.Symbol))
		f.Proxy = strings.ToUpper(strings.TrimSpace(f.Proxy))
		f.Name = strings.TrimSpace(f.Name)
		if !symbolRe.MatchString(f.Symbol) {
			return fmt.Errorf("%q is not a ticker — use the exchange listing, like VWCE.DE", f.Symbol)
		}
		if f.Proxy != "" && !symbolRe.MatchString(f.Proxy) {
			return fmt.Errorf("%q is not a ticker", f.Proxy)
		}
		if f.Proxy == f.Symbol {
			f.Proxy = ""
		}
		if seen[f.Symbol] {
			return fmt.Errorf("%s is on the plan twice", f.Symbol)
		}
		seen[f.Symbol] = true
		if r := []rune(f.Name); len(r) > 60 { // by letter: a byte cut splits "ė"
			f.Name = string(r[:60])
		}
		if f.Reserve < 0 {
			return fmt.Errorf("a reserve can't be negative")
		}
	}
	return nil
}

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

// SaveSettings stores the ETF plan after Validate.
func SaveSettings(e execer, s Settings) error { return SavePlan(e, Funds, s) }

// SavePlan stores a plan after Validate.
func SavePlan(e execer, plan string, s Settings) error {
	raw, _ := json.Marshal(s)
	_, err := e.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, settingsKey[plan], string(raw))
	return err
}

// fxRate is units of a currency per euro: 1 for EUR, the live rate
// otherwise, 0 when it can't be had (share counts are then left out rather
// than wrong). Replaced in tests.
var fxRate = func(ccy string) float64 {
	if ccy == "" || ccy == "EUR" {
		return 1
	}
	if q, err := market.Fetch("EUR" + ccy + "=X"); err == nil && q.Price > 0 {
		return q.Price
	}
	return 0
}

// addBuys puts the owner's purchases of a fund on its chart: trades of the
// same ticker (VWCE for VWCE.DE), each on the chart's nearest point at or
// after its day. Older than the chart's three years, they are left off.
func addBuys(r *Report, trades []TradeIn) {
	{
		for _, t := range trades {
			if !mine(r, t) || len(r.Chart) == 0 || t.Date < r.Chart[0].Date {
				continue
			}
			at := r.Chart[len(r.Chart)-1].Date
			for _, pt := range r.Chart {
				if pt.Date >= t.Date {
					at = pt.Date
					break
				}
			}
			r.Buys = append(r.Buys, Buy{Date: t.Date, ChartDate: at, Price: t.Price, Shares: t.Shares})
		}
	}
}

// pricedBuys gives a euro-only buy of this fund (a crypto transfer) the
// close of its day, or the next one traded, and the units that bought.
func pricedBuys(trades []TradeIn, base string, bars []market.Bar) []TradeIn {
	out := make([]TradeIn, 0, len(trades))
	for _, t := range trades {
		if t.Price == 0 && t.EUR > 0 && strings.EqualFold(t.Ticker, base) {
			for _, b := range bars {
				if b.Date >= t.Date && b.Close > 0 {
					t.Price, t.Shares = b.Close, t.EUR/b.Close
					break
				}
			}
		}
		out = append(out, t)
	}
	return out
}

// TradeIn is the part of a trade the chart needs (wealth.Trade's fields).
type TradeIn struct {
	Date, Ticker, Action string
	Price, Shares        float64
	Currency             string
	// EUR is set (and Price left 0) for a crypto buy, which the ledger keeps
	// as a euro transfer into a crypto account: its price and amount come
	// from that day's close (pricedBuys).
	EUR float64
}

// Plan is the page: every fund read against the ladder, and the totals
// across them: money to put in now, reserve left, and the monthly amount
// (from Investments' settings) that keeps going whatever happens.
type Plan struct {
	Settings Settings    `json:"settings"`
	Funds    []Report    `json:"funds"`
	BuyNow   money.Cents `json:"buy_now"`
	Reserve  money.Cents `json:"reserve"`
	Left     money.Cents `json:"left"`
	Orders   int         `json:"orders"` // limit buys to keep at the broker
	Monthly  money.Cents `json:"monthly"`
	// MonthlyFrom says where Monthly comes from: "settings" (Investments'
	// "Invested each month") or "transfers" (UsualMonthly).
	MonthlyFrom string `json:"monthly_from,omitempty"`
	// Missing counts funds whose prices didn't load: their reserve is left
	// out of Reserve and Left rather than shown as untouched.
	Missing int `json:"missing"`
	// The reserve is money above the safety net (the emergency fund's
	// target), never the safety net itself. Spare is what is above it today;
	// Afford is what of BuyNow that cash covers, Short how much of the
	// reserve left it doesn't. HasNet is false when no emergency fund is set.
	HasNet    bool        `json:"has_net"`
	SafetyNet money.Cents `json:"safety_net"`
	Spare     money.Cents `json:"spare"`
	Afford    money.Cents `json:"afford"`
	Short     money.Cents `json:"short"`
}

// Cash sets what is above the safety net, and what of the plan it covers.
func (p *Plan) Cash(safetyNet, spare money.Cents) {
	p.HasNet, p.SafetyNet, p.Spare = true, safetyNet, spare
	p.Afford = min(p.BuyNow, max(0, spare))
	p.Short = max(0, p.Left-max(0, spare))
}

// Build reads each fund in parallel. A fund whose prices can't be fetched
// gets an Error and the rest still show.
func Build(s Settings, daily func(symbol string) (*market.Series, error)) Plan {
	out := make([]Report, len(s.Funds))
	var wg sync.WaitGroup
	for i, f := range s.Funds {
		wg.Add(1)
		go func(i int, f Fund) {
			defer wg.Done()
			out[i] = one(f, daily, s.Trades)
		}(i, f)
	}
	wg.Wait()
	p := Plan{Settings: s, Funds: out}
	for _, r := range out {
		if r.Error != "" {
			p.Missing++
			continue
		}
		p.Reserve += r.Reserve
		p.Left += r.Left
		if r.Todo.Kind == "buy" {
			p.BuyNow += r.Todo.Amount
		}
		for _, lv := range r.Levels {
			if lv.Status == "order" && lv.Tranche > 0 {
				p.Orders++
			}
		}
	}
	return p
}

func one(f Fund, daily func(string) (*market.Series, error), trades []TradeIn) Report {
	fail := func(err error) Report {
		remember(f, Report{Error: "no prices"})
		r := Report{Symbol: f.Symbol, Name: f.Name, Proxy: f.Proxy, Levels: []Level{}, Gaps: []Gap{}, Chart: []Point{}, Buys: []Buy{},
			Error: fmt.Sprintf("Prices for %s couldn't be fetched right now (%v).", f.Symbol, err)}
		apply(&r, f, trades)
		return r
	}
	own, err := daily(f.Symbol)
	if err != nil {
		return fail(err)
	}
	src := own
	if f.Proxy != "" {
		p, err := daily(f.Proxy)
		if err != nil {
			return fail(err)
		}
		src = p
	}
	r := levels(src.Bars, own.Bars)
	r.Symbol, r.Name, r.Proxy, r.Currency = f.Symbol, f.Name, f.Proxy, own.Currency
	r.FX = fxRate(own.Currency)
	if f.Proxy != "" {
		r.Note = fmt.Sprintf("Averages and gaps come from %s, which tracks the same index with a longer history, scaled to %s's price.", f.Proxy, f.Symbol)
	}
	remember(f, r)
	trades = pricedBuys(trades, shortName(f.Symbol), own.Bars)
	addBuys(&r, trades)
	apply(&r, f, trades)
	return r
}
