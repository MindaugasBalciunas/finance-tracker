package dip

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"ft/internal/market"
	"ft/internal/money"
)

// apply turns a fund's price ladder into what to do with money: each
// buying rung gets a third of the reserve (the last one the remainder),
// the share counts at its limit, and a status; the fund gets one line for
// today. A rung is bought when the trades show it (boughtRungs), so nothing
// is marked by hand: the buying happens at the broker, and the trade
// reaches the app by the IBKR sync or by adding it.
func apply(r *Report, f Fund, trades []TradeIn) {
	r.Reserve = f.Reserve
	third := money.Cents(int64(f.Reserve) / 3)
	took := boughtRungs(r, third, trades)
	var due money.Cents
	var used money.Cents
	reached, orders := 0, 0
	var nearest *Level
	for i := range r.Levels {
		lv := &r.Levels[i]
		lv.Tranche, lv.Shares, lv.Exact, lv.BoughtOn, lv.BoughtShares, lv.BoughtPrice = 0, 0, 0, "", 0, 0
		if lv.Key == "dip" {
			lv.Status = "watch"
			continue
		}
		lv.Tranche = third
		if lv.Key == "t3" {
			lv.Tranche = f.Reserve - 2*third
		}
		if lv.Tranche > 0 && lv.Price > 0 && r.FX > 0 {
			local := lv.Tranche.Float() * r.FX // the third in the listing's currency
			lv.Exact = math.Floor(local/lv.Price*100) / 100
			lv.Shares = int(math.Floor(local / lv.Price))
		}
		t, bought := took[lv.Key]
		switch {
		case bought:
			lv.Status, lv.BoughtOn, lv.BoughtShares, lv.BoughtPrice = "bought", t.Date, t.Shares, t.Price
			used += lv.Tranche
		case lv.Reached && lv.Tranche <= 0:
			lv.Status = "watch" // reached, but no reserve set: nothing to buy with
		case lv.Reached && r.PlanBroken:
			lv.Status = "hold"
		case lv.Reached && r.Drawdown > -nearHigh:
			// A rung sitting within 3% of the high (a flat market): no fall
			// has begun, so nothing is due — and a mark made now would expire
			// the next day, asking again every day.
			lv.Status = "order"
			orders++
		case lv.Reached:
			lv.Status = "buy"
			due += lv.Tranche
			reached++
		default:
			lv.Status = "order"
			orders++
			if nearest == nil {
				nearest = lv
			}
		}
	}
	r.Left = f.Reserve - used

	name := shortName(r.Symbol)
	switch {
	case r.Error != "":
		r.Todo = Todo{Kind: "wait", Text: "Prices unavailable right now"}
	case r.PlanBroken:
		r.Todo = Todo{Kind: "stop", Text: fmt.Sprintf("Plan broken: %s has been under its 4-year average for over half a year. Keep the monthly amount, hold the reserve and rewrite the plan.", name)}
	case f.Reserve <= 0:
		r.Todo = Todo{Kind: "setup", Text: "Set a reserve for this fund to get amounts to buy."}
	case due > 0:
		rungs := "a rung"
		if reached > 1 {
			rungs = fmt.Sprintf("%d rungs", reached)
		}
		r.Todo = Todo{Kind: "buy", Amount: due, Shares: int(math.Floor(due.Float() * r.FX / r.Price)),
			Text: fmt.Sprintf("Buy %s of %s now: the price has reached %s of your plan.", eurS(due), name, rungs)}
	case nearest != nil:
		kind := "wait"
		if r.Zone == "calm" {
			kind = "calm"
		}
		r.Todo = Todo{Kind: kind, Text: fmt.Sprintf("Nothing extra to buy. Keep a limit buy for %s at %s (%.1f%% lower).", eurS(nearest.Tranche), price(nearest.Price, r.Currency), -nearest.Away*100)}
	default:
		r.Todo = Todo{Kind: "wait", Text: "Every third of the reserve is in for this fall. Keep the monthly amount going."}
	}
}

// boughtRungs finds the rungs the owner's trades already took in the
// running fall: a buy of the fund dated from the fall's start, at or within
// 1% above a rung's limit (a market order on the day), worth at least half
// a third — so the regular monthly amount doesn't count as a rung. Each buy
// takes the highest such rung still open.
func boughtRungs(r *Report, third money.Cents, trades []TradeIn) map[string]TradeIn {
	took := map[string]TradeIn{}
	if third <= 0 {
		return took
	}
	for _, t := range trades {
		if !mine(r, t) || t.Date < r.CycleStart {
			continue
		}
		// Worth at least half a third. Without an exchange rate the size
		// can't be judged, so any buy at a limit counts: a failed rate fetch
		// must not turn bought rungs back into "buy".
		if r.FX > 0 && t.Shares*t.Price/r.FX < third.Float()/2 {
			continue
		}
		var best *Level
		for i := range r.Levels {
			lv := &r.Levels[i]
			if _, done := took[lv.Key]; done || lv.Key == "dip" || t.Price > lv.Price*1.01 {
				continue
			}
			if best == nil || lv.Price > best.Price {
				best = lv
			}
		}
		if best != nil {
			took[best.Key] = t
		}
	}
	return took
}

// shortName is VWCE for VWCE.DE.
// Ticker is the ticker a listing's trades are recorded under (VWCE for
// VWCE.DE, BTC for BTC-EUR).
func Ticker(symbol string) string { return shortName(symbol) }

// shortName is VWCE for VWCE.DE and BTC for BTC-EUR: the ticker trades
// are recorded under.
func shortName(symbol string) string {
	n, _, _ := strings.Cut(symbol, ".")
	n, _, _ = strings.Cut(n, "-")
	return strings.ToUpper(n)
}

// mine says whether a trade is of this fund and in its currency: a trade
// in another currency would be compared with the wrong limit (OKLO bought in
// dollars against an OKLO.DE listing in euros).
func mine(r *Report, t TradeIn) bool {
	return t.Action == "buy" && strings.ToUpper(t.Ticker) == shortName(r.Symbol) && t.Price > 0 &&
		(t.Currency == "" || r.Currency == "" || strings.EqualFold(t.Currency, r.Currency))
}

func eurS(c money.Cents) string { return "€" + thousands(fmt.Sprint(int64(math.Round(c.Float())))) }

// thousands puts commas into the whole part: 1256.60 → 1,256.60.
func thousands(s string) string {
	whole, frac, dot := strings.Cut(s, ".")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if dot {
		return whole + "." + frac
	}
	return whole
}

func price(p float64, currency string) string {
	sym := map[string]string{"": "€", "EUR": "€", "USD": "$", "GBP": "£"}[currency]
	if sym == "" {
		sym = currency + " "
	}
	if p < 10 {
		return fmt.Sprintf("%s%.3f", sym, p)
	}
	return sym + thousands(fmt.Sprintf("%.2f", p))
}

// ── the last reading, for Home and the phone ───────────────────────
//
// Home's bell and the push alerts must not wait on Yahoo, so they read the
// last prices any reading fetched (the page, or Watch every few hours),
// kept in memory only: nothing is written to the database in the background.

var last = struct {
	mu sync.Mutex
	r  map[string]reading // by symbol + "|" + proxy
}{r: map[string]reading{}}

type reading struct {
	at time.Time
	r  Report // levels only
}

// remember keeps a fund's reading, each with its own time: one global time
// let a fund whose prices stopped loading keep saying "buy" on weeks-old
// prices while the others refreshed. A failed fetch forgets the fund.
func remember(f Fund, r Report) {
	last.mu.Lock()
	defer last.mu.Unlock()
	key := f.Symbol + "|" + f.Proxy
	if r.Error != "" {
		delete(last.r, key)
		return
	}
	r.Chart = nil                                // Home doesn't draw it
	r.Levels = append([]Level(nil), r.Levels...) // the caller applies money to its own copy
	last.r[key] = reading{at: time.Now(), r: r}
}

// Due is a fund whose price reached rungs not bought yet in this fall.
type Due struct {
	Plan   string      `json:"plan"` // funds | bets
	Symbol string      `json:"symbol"`
	Name   string      `json:"name"`
	Amount money.Cents `json:"amount"`
	Rungs  []string    `json:"rungs"` // t1…t3 reached and not bought
	Since  string      `json:"since"` // the fall's start, so an alert is sent once per fall and rung
	Text   string      `json:"text"`
}

// DueNow lists the funds with money to put in today, from the last prices
// read in the past 30 hours (none yet → nothing). Funds without a reserve
// are left out: in demo mode, or before the owner sets one up, the bell
// stays quiet.
func DueNow(q queryRower) []Due {
	last.mu.Lock()
	defer last.mu.Unlock()
	out := []Due{}
	for _, pf := range planFunds(q) {
		f := pf.f
		got, ok := last.r[f.Symbol+"|"+f.Proxy]
		if !ok || f.Reserve <= 0 || time.Since(got.at) > 30*time.Hour {
			continue
		}
		cached := got.r
		r := cached
		r.Levels = append([]Level(nil), cached.Levels...)
		r.Symbol, r.Name = f.Symbol, f.Name
		apply(&r, f, pf.trades)
		if r.Todo.Kind != "buy" {
			continue
		}
		d := Due{Plan: pf.plan, Symbol: f.Symbol, Name: f.Name, Amount: r.Todo.Amount, Since: r.CycleStart, Text: r.Todo.Text}
		for _, lv := range r.Levels {
			if lv.Status == "buy" {
				d.Rungs = append(d.Rungs, lv.Key)
			}
		}
		out = append(out, d)
	}
	return out
}

type planFund struct {
	plan   string
	f      Fund
	trades []TradeIn
}

func planFunds(q queryRower) []planFund {
	var out []planFund
	for _, plan := range Plans {
		s := LoadPlan(q, plan)
		for _, f := range s.Funds {
			out = append(out, planFund{plan, f, s.Trades})
		}
	}
	return out
}

// FreeAfter is the cash above the safety net left for a plan once the
// plans before it (Plans' order) have their reserve left: funds first,
// then bets, then crypto.
func FreeAfter(q queryRower, plan string, spare money.Cents) money.Cents {
	for _, p := range Plans {
		if p == plan {
			break
		}
		spare -= LeftNow(q, p)
	}
	return spare
}

// LeftNow is a plan's reserve not yet used in the running fall, from the
// last prices read (a fund without a reading counts its whole reserve, the
// safe side for "is there cash for the bets after the funds?").
func LeftNow(q queryRower, plan string) money.Cents {
	s := LoadPlan(q, plan)
	last.mu.Lock()
	defer last.mu.Unlock()
	var left money.Cents
	for _, f := range s.Funds {
		got, ok := last.r[f.Symbol+"|"+f.Proxy]
		if !ok {
			left += f.Reserve
			continue
		}
		r := got.r
		r.Levels = append([]Level(nil), got.r.Levels...)
		apply(&r, f, s.Trades)
		left += r.Left
	}
	return left
}

// Watch reads the plan every few hours so Home and the phone hear about a
// reached rung without the page being opened. It only fetches prices.
func Watch(d *sql.DB, daily func(string) (*market.Series, error)) {
	time.Sleep(3 * time.Minute) // let start-up settle
	for {
		for _, plan := range Plans {
			s := LoadPlan(d, plan)
			for _, f := range s.Funds {
				if f.Reserve > 0 { // without a reserve nothing can be due
					Build(s, daily)
					break
				}
			}
		}
		time.Sleep(2 * time.Hour)
	}
}
