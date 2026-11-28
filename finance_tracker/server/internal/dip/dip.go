// Package dip is the owner's crash plan for index ETFs: a written-down
// ladder of prices at which a cash reserve goes into the market, so a fall
// is met with a plan and not with "buy the dip" guesses. The ladder follows
// the "turbulence plan" the owner adopted:
//
//   - always: the same monthly amount into the world fund, whatever happens;
//   - −10% under the 200-day average: nothing extra, such dips usually mend;
//   - the 2-year average (often where an old unfilled gap sits): first third
//     of the reserve;
//   - the 4-year average (another old gap): second third;
//   - −44% from the high, the deepest wave zone: last third;
//   - the plan no longer fits when the price stays under the 4-year average
//     for more than half a year (as in 2001) — then it is rewritten.
//
// Averages move, so the levels are worked out from daily prices each time
// and the page says when a limit order should be moved. The app shows the
// levels; the owner places the orders at the broker.
package dip

import (
	"math"
	"sort"
	"time"

	"ft/internal/market"
	"ft/internal/money"
)

// Trading days in the windows the plan names.
const (
	days200 = 200
	// A stock-exchange year has ~252 trading days; crypto trades every day,
	// so its year is 365 bars (see barsPerYear). The 2- and 4-year windows
	// and "half a year" scale with it; the 200-day average stays 200 bars.
	tradingYear = 252
	// deepDrop is the deepest wave zone, measured from the all-time high.
	deepDrop = 0.44
	// gapNear: an unfilled gap counts as backing a level when its middle is
	// within this share of the level.
	gapNear = 0.05
	// nearHigh: within this share of the high no fall has begun, and a new
	// fall starts after the last day there.
	nearHigh = 0.03
	// minGap ignores the many tiny overnight gaps a European listing of a US
	// index leaves every day: only a jump of at least this share counts.
	minGap = 0.005
)

// Gap is a price range a chart jumped over and has not traded back into
// since: buyers there never had their chance, and prices often come back
// to it. Low is the earlier day's high, High the later day's low, shrunk
// by any later partial fill.
type Gap struct {
	Date string  `json:"date"`
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// Level is one rung of the ladder, in the fund's own price.
type Level struct {
	Key    string `json:"key"`   // dip, t1, t2, t3
	Label  string `json:"label"` // "2-year average"
	Action string `json:"action"`
	// Price is the limit price, rounded down to the tick: the top of an
	// unfilled gap backing the average when there is one (where the fall
	// first meets buyers: the plan's 602 for QQQ is the April 2026 gap's top
	// beside a 2-year average of 604), else the average itself.
	Price float64 `json:"price"`
	Basis float64 `json:"basis"` // the average or drop level the rung is named after
	// Away is how far the price must fall to reach it (−0.12 = 12% lower);
	// zero or above means it is reached.
	Away    float64     `json:"away"`
	Reached bool        `json:"reached"`
	Gap     *Gap        `json:"gap,omitempty"` // an unfilled gap near the level
	Tranche money.Cents `json:"tranche"`       // the share of the reserve it releases
	Shares  int         `json:"shares"`        // whole shares the tranche buys at Price
	// Exact is the tranche in fractional shares at Price, for brokers that
	// sell fractions.
	Exact float64 `json:"exact,omitempty"`
	// Status says what to do with this rung today (see apply): watch (the
	// −10% line, no money), order (a limit buy to keep at the broker), buy
	// (reached and not bought yet), bought (a buy in the trades took it in
	// this fall), hold (reached, but the plan is broken).
	Status string `json:"status"`
	// The buy in the trades that took this rung's third (see apply).
	BoughtOn     string  `json:"bought_on,omitempty"`
	BoughtShares float64 `json:"bought_shares,omitempty"`
	BoughtPrice  float64 `json:"bought_price,omitempty"`
}

// Todo is a fund's one-line answer to "what do I do today".
type Todo struct {
	Kind   string      `json:"kind"` // buy | wait | stop | setup | calm
	Text   string      `json:"text"`
	Amount money.Cents `json:"amount,omitempty"` // to buy now
	Shares int         `json:"shares,omitempty"` // whole shares that buys at today's price
}

// Point is one day on the fund's chart (fund price units).
type Point struct {
	Date   string  `json:"date"`
	Close  float64 `json:"close"`
	SMA200 float64 `json:"sma200,omitempty"`
	SMA2y  float64 `json:"sma2y,omitempty"`
	SMA4y  float64 `json:"sma4y,omitempty"`
}

// Report is one fund read against the plan.
type Report struct {
	Symbol   string      `json:"symbol"`
	Name     string      `json:"name"`
	Proxy    string      `json:"proxy,omitempty"` // whose history the levels come from
	Currency string      `json:"currency"`
	AsOf     string      `json:"as_of"`
	Price    float64     `json:"price"`
	High     float64     `json:"high"`     // all-time high close (in fund units)
	Drawdown float64     `json:"drawdown"` // from that high, ≤ 0
	SMA200   float64     `json:"sma200"`
	SMA2y    float64     `json:"sma2y"`
	SMA4y    float64     `json:"sma4y"`
	Zone     string      `json:"zone"` // calm, dip, t1, t2, t3
	Reserve  money.Cents `json:"reserve"`
	Levels   []Level     `json:"levels"`
	Gaps     []Gap       `json:"gaps"`
	// DaysUnder4y counts the latest trading days in a row under the 4-year
	// average; PlanBroken is set from half a year.
	DaysUnder4y int     `json:"days_under_4y"`
	PlanBroken  bool    `json:"plan_broken"`
	Chart       []Point `json:"chart"`
	Note        string  `json:"note,omitempty"`
	Error       string  `json:"error,omitempty"`
	// CycleStart is the last day the price stood within 3% of its high:
	// the fall now running began after it. A rung marked bought before it
	// belongs to an earlier fall, so the reserve is whole again.
	CycleStart string      `json:"cycle_start,omitempty"`
	Left       money.Cents `json:"left"` // reserve not yet used in this fall
	Todo       Todo        `json:"todo"`
	// FX is units of the listing's currency per euro (1 for EUR; 0 when
	// the rate couldn't be fetched). The reserve is in euros and OKLO is
	// quoted in dollars: a €1,000 third at $50 is ~23 shares, not 20.
	FX float64 `json:"fx"`
	// Buys are the owner's own purchases of this fund (from the trades),
	// for the chart's dots. ChartDate is the full chart's point the dot sits
	// on; the web places dots on its own window (3M–3Y) from Date, so this is
	// for other readers of the API.
	Buys []Buy `json:"buys"`
}

// Buy is one purchase drawn on the chart.
type Buy struct {
	Date      string  `json:"date"`
	ChartDate string  `json:"chart_date"`
	Price     float64 `json:"price"`
	Shares    float64 `json:"shares"`
}

// Analyze reads a fund against the plan. src is the history the levels are
// computed on: the fund's own, or a long-lived proxy tracking the same index
// when the fund is too young for a 4-year average. own is the fund's bars
// (only its last close is needed) and may be the same slice as src. Levels
// on a proxy are carried over by the ratio of the two latest prices, so a
// limit order is always in the fund's own price.
func Analyze(src, own []market.Bar, reserve money.Cents) Report {
	r := levels(src, own)
	apply(&r, Fund{Reserve: reserve}, nil)
	return r
}

// levels works out the prices: averages, gaps, rungs and zone. What to do
// with money (tranches, shares, status) is apply's, so a saved reserve or a
// new trade can be applied to a cached reading without new prices.
func levels(src, own []market.Bar) Report {
	r := Report{Levels: []Level{}, Gaps: []Gap{}, Chart: []Point{}, Buys: []Buy{}, FX: 1}
	if len(src) == 0 || len(own) == 0 {
		r.Error = "no prices"
		return r
	}
	last := src[len(src)-1]
	r.AsOf = own[len(own)-1].Date
	r.Price = own[len(own)-1].Close
	k := scale(src, own) // proxy → fund units; 1 on own history

	n := len(src)
	sum := make([]float64, n+1)
	for i, b := range src {
		sum[i+1] = sum[i] + b.Close
	}
	sma := func(win, i int) float64 { // average of the win closes ending at i
		if i+1 < win {
			return 0
		}
		return (sum[i+1] - sum[i+1-win]) / float64(win)
	}
	yr := barsPerYear(src)
	days2y, days4y, halfYear := 2*yr, 4*yr, yr/2
	r.SMA200 = sma(days200, n-1) * k
	r.SMA2y = sma(days2y, n-1) * k
	r.SMA4y = sma(days4y, n-1) * k

	high := 0.0
	for _, b := range src {
		if b.Close >= high {
			high = b.Close
		}
		if b.Close >= high*(1-nearHigh) {
			r.CycleStart = b.Date
		}
	}
	r.High = high * k
	r.Drawdown = last.Close/high - 1

	// Every open gap backs a rung, however far down; only the six nearest
	// are listed. Cutting first hid the gap beside the 4-year average behind
	// the many small ones above it.
	all := []Gap{}
	for _, g := range Gaps(src, last.Close) {
		all = append(all, Gap{Date: g.Date, Low: round3(g.Low * k), High: round3(g.High * k)})
	}
	r.Gaps = all[:min(len(all), 6)]

	if r.SMA4y > 0 {
		for i := n - 1; i >= 0 && src[i].Close < sma(days4y, i); i-- {
			r.DaysUnder4y++
		}
	}
	r.PlanBroken = r.DaysUnder4y >= halfYear

	mk := func(key, label string, at float64) *Level {
		if at <= 0 {
			return nil
		}
		lv := Level{Key: key, Label: label, Basis: at, Price: tick(at)}
		if key != "dip" && key != "t3" {
			if lv.Gap = nearGap(all, at); lv.Gap != nil {
				lv.Price = tick(lv.Gap.High)
			}
		}
		lv.Away = lv.Price/r.Price - 1
		lv.Reached = r.Price <= lv.Price
		return &lv
	}
	if lv := mk("dip", "−10% under the 200-day average", r.SMA200*0.9); lv != nil {
		lv.Action = "Nothing extra: such dips usually mend on their own"
		r.Levels = append(r.Levels, *lv)
	}
	// The buying rungs take the thirds top down by price, not by name: after
	// a long fall the 2-year average sits under the 4-year one, and the
	// first third must still go in at the first price reached.
	rungs := []Level{}
	for _, lv := range []*Level{mk("avg2y", "2-year average", r.SMA2y), mk("avg4y", "4-year average", r.SMA4y), mk("t3", "−44% from the high", r.High*(1-deepDrop))} {
		if lv != nil {
			rungs = append(rungs, *lv)
		}
	}
	sort.SliceStable(rungs, func(i, j int) bool { return rungs[i].Price > rungs[j].Price })
	for i := range rungs {
		rungs[i].Key = []string{"t1", "t2", "t3"}[i]
		rungs[i].Action = []string{"First third of the reserve", "Second third of the reserve", "Last third of the reserve"}[i]
	}
	r.Levels = append(r.Levels, rungs...)

	r.Zone = "calm"
	for _, lv := range r.Levels { // in price order, so the deepest reached wins
		if lv.Reached && (lv.Key != "dip" || r.Zone == "calm") {
			r.Zone = lv.Key
		}
	}

	r.Chart = chart(src, sma, k, yr)
	if n < days4y {
		r.Note = "Less than 4 years of prices: the 4-year level is missing until there are."
	}
	return r
}

// barsPerYear is 365 for a market that trades at weekends (crypto) and
// 252 otherwise, judged from the last few weeks of dates.
func barsPerYear(bars []market.Bar) int {
	weekend := 0
	for _, b := range bars[max(0, len(bars)-30):] {
		if t, err := time.Parse("2006-01-02", b.Date); err == nil && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
			weekend++
		}
	}
	if weekend >= 4 {
		return 365
	}
	return tradingYear
}

// scale is the ratio that carries a proxy's levels into the fund's price,
// taken on the latest day both traded: VALL in Amsterdam and VWCE on Xetra
// can end on different days (a holiday, or a cache filled minutes apart),
// and a day's move would otherwise shift every limit.
func scale(src, own []market.Bar) float64 {
	at := make(map[string]float64, len(src))
	for _, b := range src {
		at[b.Date] = b.Close
	}
	for i := len(own) - 1; i >= 0; i-- {
		if c, ok := at[own[i].Date]; ok && c > 0 {
			return own[i].Close / c
		}
	}
	return own[len(own)-1].Close / src[len(src)-1].Close
}

// Gaps lists the unfilled up-gaps under price, nearest first. A gap opens
// when a day's low is above the day before's high; later days that trade
// back into it shrink it from the top, and one that reaches its bottom
// closes it.
func Gaps(bars []market.Bar, price float64) []Gap {
	n := len(bars)
	minAfter := make([]float64, n+1) // lowest low from i on
	minAfter[n] = math.Inf(1)
	for i := n - 1; i >= 0; i-- {
		minAfter[i] = math.Min(minAfter[i+1], bars[i].Low)
	}
	out := []Gap{}
	for i := n - 1; i >= 1; i-- {
		lo, hi := bars[i-1].High, bars[i].Low
		if hi <= lo {
			continue
		}
		hi = math.Min(hi, minAfter[i+1])
		if hi <= lo || (hi-lo)/lo < minGap || lo >= price {
			continue
		}
		out = append(out, Gap{Date: bars[i].Date, Low: lo, High: math.Min(hi, price)})
	}
	// nearest under the price first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].High > out[j-1].High; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func nearGap(gaps []Gap, level float64) *Gap {
	var best *Gap
	bestD := gapNear
	for i := range gaps {
		g := gaps[i]
		var d float64 // 0 when the level is inside the gap
		switch {
		case level < g.Low:
			d = (g.Low - level) / level
		case level > g.High:
			d = (level - g.High) / level
		}
		if d <= bestD {
			best, bestD = &gaps[i], d
		}
	}
	if best == nil {
		return nil
	}
	g := *best
	return &g
}

// tick rounds a limit price down to the listing's tick: cents, or tenths of
// a cent under €10 (VALL trades around €4).
func tick(p float64) float64 {
	per := 100.0
	if p < 10 {
		per = 1000
	}
	return math.Floor(p*per+1e-6) / per // dividing keeps 3.812, not 3.8120000000000003
}

// chart samples three years: every day of the last year, so the 3-month and
// 6-month views show the dip day by day, and weekly (every fifth trading day)
// before that. It always keeps the latest day. The web thins the daily year
// back to weekly for the three-year view.
func chart(src []market.Bar, sma func(win, i int) float64, k float64, yr int) []Point {
	days2y, days4y := 2*yr, 4*yr
	n := len(src)
	from := max(0, n-3*yr)
	out := []Point{}
	for i := n - 1; i >= from; {
		b := src[i]
		out = append(out, Point{Date: b.Date, Close: round3(b.Close * k), SMA200: round3(sma(days200, i) * k), SMA2y: round3(sma(days2y, i) * k), SMA4y: round3(sma(days4y, i) * k)})
		if i > n-1-yr {
			i--
		} else {
			i -= 5
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
