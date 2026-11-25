package dip

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/testutil"
)

// flat builds n days of bars at one price, each day's range ±0.1%.
func flat(n int, price float64) []market.Bar {
	out := make([]market.Bar, n)
	for i := range out {
		out[i] = market.Bar{Date: day(i), Open: price, High: price * 1.001, Low: price * 0.999, Close: price}
	}
	return out
}

func day(i int) string { return fmt.Sprintf("2020-%04d", i) } // sortable, unique

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// Four years at 100 then a slide to 60: the 2-year and 4-year averages sit
// above the price, so the first two tranches are due and the reserve's
// thirds and the shares they buy follow.
func TestLadderReachedAfterALongFall(t *testing.T) {
	bars := append(flat(1000, 100), flat(200, 60)...)
	r := Analyze(bars, bars, money.FromFloat(3000))
	if !near(r.SMA200, 60) || !near(r.SMA2y, (304*100+200*60)/504.0) || !near(r.SMA4y, (808*100+200*60)/1008.0) {
		t.Fatalf("averages %v %v %v", r.SMA200, r.SMA2y, r.SMA4y)
	}
	if !near(r.Drawdown, -0.4) || r.Zone != "t2" {
		t.Fatalf("drawdown %v zone %s", r.Drawdown, r.Zone)
	}
	byKey := map[string]Level{}
	for _, l := range r.Levels {
		byKey[l.Key] = l
	}
	t1, t3 := byKey["t1"], byKey["t3"]
	if !t1.Reached || t1.Tranche != money.FromFloat(1000) || t1.Shares != int(1000/t1.Price) {
		t.Fatalf("t1 %+v", t1)
	}
	if t3.Reached || !near(t3.Price, 56) || t3.Tranche != money.FromFloat(1000) {
		t.Fatalf("t3 %+v", t3)
	}
	if byKey["dip"].Tranche != 0 {
		t.Fatal("the −10% rung releases nothing")
	}
}

// A rising market sits above every rung: calm, and the plan tells how far
// each one is.
func TestCalmMarketShowsDistanceToEachRung(t *testing.T) {
	bars := make([]market.Bar, 1100)
	for i := range bars {
		p := 50 + float64(i)/10
		bars[i] = market.Bar{Date: day(i), High: p + 0.01, Low: p - 0.01, Close: p}
	}
	r := Analyze(bars, bars, 0)
	if r.Zone != "calm" || r.DaysUnder4y != 0 || r.PlanBroken {
		t.Fatalf("%+v", r)
	}
	for _, l := range r.Levels {
		if l.Reached || l.Away >= 0 {
			t.Fatalf("rung %s should be below the price: %+v", l.Key, l)
		}
	}
}

// Half a year of closes under the 4-year average means the plan no longer
// fits (the 2001 case).
func TestHalfAYearUnderTheFourYearAverageBreaksThePlan(t *testing.T) {
	bars := append(flat(1100, 100), flat(130, 70)...)
	r := Analyze(bars, bars, 0)
	if r.DaysUnder4y != 130 || !r.PlanBroken {
		t.Fatalf("days %d broken %v", r.DaysUnder4y, r.PlanBroken)
	}
}

// A jump that later days never traded back into is a gap; one they did
// trade back into is gone, and a partial fill shrinks it from the top.
func TestGapsAreTheJumpsNotYetFilled(t *testing.T) {
	bars := []market.Bar{
		{Date: "d0", High: 10, Low: 9, Close: 9.5},
		{Date: "d1", High: 13, Low: 12, Close: 12.5}, // gap 10–12, later dipped to 11 → 10–11
		{Date: "d2", High: 12, Low: 11, Close: 11.5},
		{Date: "d3", High: 16, Low: 15, Close: 15.5}, // gap 12–15, later filled to 12 → gone
		{Date: "d4", High: 15, Low: 12, Close: 14},
		{Date: "d5", High: 20, Low: 18, Close: 19}, // gap 15–18, open
		{Date: "d6", High: 21, Low: 19, Close: 20},
	}
	g := Gaps(bars, 20)
	if len(g) != 2 || g[0] != (Gap{"d5", 15, 18}) || g[1] != (Gap{"d1", 10, 11}) {
		t.Fatalf("%+v", g)
	}
}

// A young fund borrows a proxy's history: levels keep the proxy's shape
// and come out in the fund's own price.
func TestProxyLevelsAreInTheFundsPrice(t *testing.T) {
	proxy := append(flat(1000, 100), flat(200, 60)...)
	own := flat(1200, 3)[1150:] // a €3 listing of the same index, trading the last 50 days
	a := Analyze(proxy, proxy, 0)
	b := Analyze(proxy, own, 0)
	if !near(b.Price, 3) || math.Abs(b.SMA4y-a.SMA4y*3/60) > 1e-9 {
		t.Fatalf("own %v proxy %v", b.SMA4y, a.SMA4y)
	}
	if b.Levels[1].Price >= 10 || b.Levels[1].Price != tick(a.SMA4y*3/60) /* inverted: the 4-year average is the first rung */ {
		t.Fatalf("limit %v", b.Levels[1].Price)
	}
}

// Limit prices round down to the listing's tick, so an order never sits
// above the level.
func TestLimitRoundsDown(t *testing.T) {
	if tick(602.4999) != 602.49 || tick(4.12399) != 4.123 {
		t.Fatal(tick(602.4999), tick(4.12399))
	}
}

// One fund's failed download doesn't hide the others.
func TestBuildKeepsTheFundsThatLoaded(t *testing.T) {
	s := Settings{Funds: []Fund{{Symbol: "OK.DE"}, {Symbol: "YOUNG.AS", Proxy: "OK.DE"}, {Symbol: "DOWN.DE"}}}
	p := Build(s, func(sym string) (*market.Series, error) {
		switch sym {
		case "OK.DE":
			return &market.Series{Currency: "EUR", Bars: flat(1100, 100)}, nil
		case "YOUNG.AS":
			return &market.Series{Currency: "EUR", Bars: flat(20, 5)}, nil
		}
		return nil, errors.New("down")
	})
	if p.Funds[0].Error != "" || p.Funds[0].Currency != "EUR" || len(p.Funds[0].Levels) != 4 {
		t.Fatalf("%+v", p.Funds[0])
	}
	if p.Funds[1].Proxy != "OK.DE" || !near(p.Funds[1].SMA4y, 5) || p.Funds[1].Note == "" {
		t.Fatalf("%+v", p.Funds[1])
	}
	if p.Funds[2].Error == "" || p.Funds[2].Levels == nil || p.Funds[2].Gaps == nil {
		t.Fatalf("%+v", p.Funds[2])
	}
}

// The plan starts from the owner's five ETFs and keeps what is saved.
func TestSettingsDefaultThenSaved(t *testing.T) {
	d := testutil.DB(t)
	if got := LoadSettings(d); len(got.Funds) != 5 || got.Funds[1].Proxy != "VWCE.DE" {
		t.Fatalf("%+v", got)
	}
	s := Settings{Funds: []Fund{{Symbol: " vwce.de ", Reserve: money.FromFloat(3000)}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(d, s); err != nil {
		t.Fatal(err)
	}
	if got := LoadSettings(d); len(got.Funds) != 1 || got.Funds[0].Symbol != "VWCE.DE" || got.Funds[0].Reserve != money.FromFloat(3000) {
		t.Fatalf("%+v", got)
	}
}

func TestSettingsRejectNonsense(t *testing.T) {
	for _, s := range []Settings{
		{Funds: []Fund{{Symbol: "VWCE.DE; drop"}}},
		{Funds: []Fund{{Symbol: "A"}, {Symbol: "a"}}},
		{Funds: []Fund{{Symbol: "A", Reserve: -1}}},
	} {
		if s.Validate() == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
}

// When an unfilled gap sits by the average, the limit goes to the gap's top,
// where a fall first meets buyers (the plan's QQQ 602 beside a 604 average).
func TestGapBesideTheAverageSetsTheLimit(t *testing.T) {
	bars := flat(1000, 100)
	// a jump from 98 to 102 that never filled, then a long stretch above
	bars[600] = market.Bar{Date: day(600), High: 98, Low: 97, Close: 97.5}
	for i := 601; i < 1000; i++ {
		bars[i] = market.Bar{Date: day(i), High: 104, Low: 102, Close: 103}
	}
	r := Analyze(bars, bars, 0)
	var t1 Level
	for _, l := range r.Levels {
		if l.Key == "t1" {
			t1 = l
		}
	}
	if t1.Gap == nil || t1.Price != 102 || near(t1.Basis, 102) {
		t.Fatalf("%+v", t1)
	}
}

// After a long fall the 2-year average sits under the 4-year one: the
// first third goes in at the higher price, whatever the rung is called.
func TestThirdsFollowPriceWhenAveragesInvert(t *testing.T) {
	bars := append(flat(1000, 100), flat(200, 80)...)
	bars = append(bars, flat(10, 88)...) // between the averages (≈84 and ≈92)
	for i := range bars {
		bars[i].Date = day(i)
	}
	r := Analyze(bars, bars, money.FromFloat(3000))
	var t1, t2 Level
	for _, l := range r.Levels {
		switch l.Key {
		case "t1":
			t1 = l
		case "t2":
			t2 = l
		}
	}
	if t1.Label != "4-year average" || !t1.Reached || t2.Label != "2-year average" || t2.Reached || r.Zone != "t1" {
		t.Fatalf("zone %s t1 %+v t2 %+v", r.Zone, t1, t2)
	}
}

// The gap beside the 4-year average still sets its limit when many small
// gaps sit between it and the price (only six are listed).
func TestDeepGapFoundBehindManySmallOnes(t *testing.T) {
	bars := flat(1100, 90)
	// a gap 90.09–100.4 around the 4-year average (~97), then a jump to 106
	// and a staircase of ten more gaps above it
	p := 100.4
	for i := 500; i < 1100; i++ {
		if i == 1001 {
			p = 106
		} else if i > 1001 && (i-1001)%9 == 0 {
			p *= 1.012
		}
		bars[i] = market.Bar{Date: day(i), High: p + 0.2, Low: p, Close: p + 0.1}
	}
	for i := range bars {
		bars[i].Date = day(i)
	}
	r := Analyze(bars, bars, 0)
	if len(r.Gaps) != 6 {
		t.Fatalf("listed %d", len(r.Gaps))
	}
	for _, l := range r.Levels {
		if l.Label == "4-year average" && (l.Gap == nil || l.Gap.High != 100.4 || l.Price != 100.4) {
			t.Fatalf("%+v %+v", l, l.Gap)
		}
	}
}

// fallen: years at 100, then 100 days at 60 (under half a year): the 2-year
// and 4-year rungs (t1, t2) are reached, −44% (t3 at 56) is not.
func fallen() []market.Bar {
	bars := append(flat(1100, 100), flat(100, 60)...)
	for i := range bars { // weekdays from 2022-01-03 (an exchange's days)
		bars[i].Date = weekday(i)
	}
	return bars
}

// weekday is the i-th weekday from Monday 3 Jan 2022.
func weekday(i int) string {
	return time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i/5*7+i%5).Format("2006-01-02")
}

// resetLast empties the shared last-prices cache between tests.
func resetLast(t *testing.T) {
	t.Helper()
	last.mu.Lock()
	last.r = map[string]reading{}
	last.mu.Unlock()
	t.Cleanup(func() {
		last.mu.Lock()
		last.r = map[string]reading{}
		last.mu.Unlock()
	})
}

func todoOf(t *testing.T, f Fund, trades ...TradeIn) Report {
	t.Helper()
	r := levels(fallen(), fallen())
	r.Symbol = "VWCE.DE"
	apply(&r, f, trades)
	return r
}

// Reached rungs whose third isn't in yet are money to put in now, with the
// shares it buys at today's price; the rest are limit orders to keep.
func TestReachedRungsAreMoneyDueNow(t *testing.T) {
	r := todoOf(t, Fund{Symbol: "VWCE.DE", Reserve: money.FromFloat(3000)})
	if r.Todo.Kind != "buy" || r.Todo.Amount != money.FromFloat(2000) || r.Todo.Shares != 33 || r.Left != money.FromFloat(3000) {
		t.Fatalf("%+v left %v", r.Todo, r.Left)
	}
	st := map[string]string{}
	for _, l := range r.Levels {
		st[l.Key] = l.Status
	}
	if st["dip"] != "watch" || st["t1"] != "buy" || st["t2"] != "buy" || st["t3"] != "order" {
		t.Fatalf("%v", st)
	}
}

// A buy in the trades during this fall, at or under a rung's limit and
// worth at least half its third, takes that rung: the plan stops asking and
// the reserve left shrinks. A buy from before the fall began, a small one
// (the monthly amount) or one above every limit takes nothing.
func TestTradesTakeTheRungsTheyBought(t *testing.T) {
	f := Fund{Symbol: "VWCE.DE", Reserve: money.FromFloat(3000)}
	r := todoOf(t, f, TradeIn{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 60, Shares: 17})
	if r.Todo.Kind != "buy" || r.Todo.Amount != money.FromFloat(1000) || r.Left != money.FromFloat(2000) {
		t.Fatalf("%+v left %v", r.Todo, r.Left)
	}
	var t1 Level
	for _, l := range r.Levels {
		if l.Key == "t1" {
			t1 = l
		}
	}
	if t1.Status != "bought" || t1.BoughtOn != "2026-06-01" || t1.BoughtShares != 17 || t1.BoughtPrice != 60 {
		t.Fatalf("%+v", t1)
	}
	for _, tr := range []TradeIn{
		{Date: "2021-06-01", Ticker: "VWCE", Action: "buy", Price: 60, Shares: 17},  // an earlier fall
		{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 60, Shares: 3},   // €180: the monthly amount
		{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 99, Shares: 17},  // above every limit
		{Date: "2026-06-01", Ticker: "VWCE", Action: "sell", Price: 60, Shares: 17}, // a sell
		{Date: "2026-06-01", Ticker: "VALL", Action: "buy", Price: 60, Shares: 17},  // another fund
	} {
		if r := todoOf(t, f, tr); r.Todo.Amount != money.FromFloat(2000) {
			t.Fatalf("%+v took a rung: %+v", tr, r.Todo)
		}
	}
	// two lump buys take both reached rungs: nothing left to buy
	r = todoOf(t, f, TradeIn{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 60, Shares: 17}, TradeIn{Date: "2026-06-02", Ticker: "VWCE", Action: "buy", Price: 60, Shares: 17})
	if r.Todo.Kind != "wait" || r.Left != money.FromFloat(1000) {
		t.Fatalf("%+v left %v", r.Todo, r.Left)
	}
}

// No reserve: nothing to buy, a nudge to set one. Plan broken: hold.
func TestNoReserveOrBrokenPlanNeverSaysBuy(t *testing.T) {
	if r := todoOf(t, Fund{Symbol: "VWCE.DE"}); r.Todo.Kind != "setup" {
		t.Fatalf("%+v", r.Todo)
	}
	bars := append(flat(1100, 100), flat(130, 60)...)
	r := levels(bars, bars)
	apply(&r, Fund{Reserve: money.FromFloat(3000)}, nil)
	if r.Todo.Kind != "stop" {
		t.Fatalf("%+v", r.Todo)
	}
	for _, l := range r.Levels {
		if l.Status == "buy" {
			t.Fatalf("broken plan still buys %+v", l)
		}
	}
}

// Home and the phone read the last prices fetched: a fund with money due
// shows once fetched, a fund without a reserve never does.
func TestDueNowFromTheLastPrices(t *testing.T) {
	resetLast(t)
	d := testutil.DB(t)
	s := Settings{Funds: []Fund{{Symbol: "DUE.DE", Reserve: money.FromFloat(3000)}, {Symbol: "NORES.DE"}}}
	if err := SaveSettings(d, s); err != nil {
		t.Fatal(err)
	}
	Build(s, func(string) (*market.Series, error) { return &market.Series{Currency: "EUR", Bars: fallen()}, nil })
	due := DueNow(d)
	if len(due) != 1 || due[0].Symbol != "DUE.DE" || due[0].Amount != money.FromFloat(2000) || strings.Join(due[0].Rungs, ",") != "t1,t2" {
		t.Fatalf("%+v", due)
	}
	if _, err := d.Exec(`INSERT INTO trades(date,account_id,action,ticker,shares,price,currency,created_at) VALUES('2026-06-01','ibkr','buy','DUE',17,60,'EUR','x')`); err != nil {
		t.Fatal(err)
	}
	if due := DueNow(d); len(due) != 1 || due[0].Amount != money.FromFloat(1000) {
		t.Fatalf("after the buy: %+v", due)
	}
}

func TestPricesReadLikeMoney(t *testing.T) {
	if price(1256.6, "EUR") != "€1,256.60" || price(4.1234, "EUR") != "€4.123" || price(602.12, "USD") != "$602.12" || eurS(money.FromFloat(12500)) != "€12,500" {
		t.Fatal(price(1256.6, "EUR"), price(4.1234, "EUR"), price(602.12, "USD"), eurS(money.FromFloat(12500)))
	}
}

// A fund whose prices stop loading is forgotten: the bell never says "buy"
// on its old prices while the other funds refresh.
func TestFailedFetchForgetsTheFund(t *testing.T) {
	resetLast(t)
	d := testutil.DB(t)
	s := Settings{Funds: []Fund{{Symbol: "A.DE", Reserve: money.FromFloat(3000)}, {Symbol: "B.DE", Reserve: money.FromFloat(3000)}}}
	SaveSettings(d, s)
	ok := func(string) (*market.Series, error) { return &market.Series{Currency: "EUR", Bars: fallen()}, nil }
	Build(s, ok)
	if len(DueNow(d)) != 2 {
		t.Fatal("both due")
	}
	p := Build(s, func(sym string) (*market.Series, error) {
		if sym == "B.DE" {
			return nil, errors.New("down")
		}
		return ok(sym)
	})
	if due := DueNow(d); len(due) != 1 || due[0].Symbol != "A.DE" {
		t.Fatalf("%+v", due)
	}
	// and the page's totals leave B out rather than call its reserve untouched
	if p.Missing != 1 || p.Reserve != money.FromFloat(3000) {
		t.Fatalf("missing %d reserve %v", p.Missing, p.Reserve)
	}
}

// A rung sitting within 3% of the high (a flat market) is not a fall:
// nothing is due, so a mark can't expire the next day and ask again.
func TestRungNearTheHighIsNotDue(t *testing.T) {
	bars := flat(1300, 100)
	r := levels(bars, bars)
	apply(&r, Fund{Reserve: money.FromFloat(3000)}, nil)
	if r.Todo.Kind == "buy" {
		t.Fatalf("%+v", r.Todo)
	}
	for _, l := range r.Levels {
		if l.Status == "buy" {
			t.Fatalf("%+v", l)
		}
	}
}

// The reserve comes from cash above the safety net: buying now is capped at
// that cash, and the part of the reserve it doesn't cover is said.
func TestBuyingIsCappedAtCashAboveTheSafetyNet(t *testing.T) {
	p := Plan{BuyNow: money.FromFloat(2000), Left: money.FromFloat(3000)}
	p.Cash(money.FromFloat(9000), money.FromFloat(1200))
	if !p.HasNet || p.Afford != money.FromFloat(1200) || p.Short != money.FromFloat(1800) {
		t.Fatalf("%+v", p)
	}
	p.Cash(money.FromFloat(9000), money.FromFloat(-50)) // under the net
	if p.Afford != 0 || p.Short != money.FromFloat(3000) {
		t.Fatalf("%+v", p)
	}
}

// The reserve is in euros and a US stock in dollars: a €1,000 third at a
// $50 limit with $1.15 to the euro is 23 shares, not 20.
func TestSharesAreCountedInTheListingsCurrency(t *testing.T) {
	r := levels(fallen(), fallen())
	r.FX = 1.15
	apply(&r, Fund{Reserve: money.FromFloat(3000)}, nil)
	for _, l := range r.Levels {
		if l.Key == "t1" && l.Shares != int(1000*1.15/l.Price) {
			t.Fatalf("%+v", l)
		}
	}
	if r.Todo.Shares != int(2000*1.15/r.Price) {
		t.Fatalf("%+v", r.Todo)
	}
	r.FX = 0 // no rate: no share counts rather than wrong ones
	apply(&r, Fund{Reserve: money.FromFloat(3000)}, nil)
	for _, l := range r.Levels {
		if l.Shares != 0 {
			t.Fatalf("%+v", l)
		}
	}
}

// The owner's buys of a fund land on the chart's nearest point at or after
// the day; sells, other tickers and buys older than the chart don't.
func TestBuysLandOnTheChart(t *testing.T) {
	r := Report{Symbol: "VWCE.DE", Buys: []Buy{}, Chart: []Point{{Date: "2026-03-02"}, {Date: "2026-03-09"}, {Date: "2026-03-16"}}}
	addBuys(&r, []TradeIn{
		{Date: "2026-03-04", Ticker: "VWCE", Action: "buy", Price: 148.9, Shares: 6.5},
		{Date: "2026-03-05", Ticker: "VWCE", Action: "sell", Price: 150, Shares: 1},
		{Date: "2026-03-05", Ticker: "VALL", Action: "buy", Price: 4, Shares: 100},
		{Date: "2025-01-01", Ticker: "VWCE", Action: "buy", Price: 110, Shares: 1},
		{Date: "2026-04-01", Ticker: "vwce", Action: "buy", Price: 160, Shares: 2},
	})
	b := r.Buys
	if len(b) != 2 || b[0].ChartDate != "2026-03-09" || b[0].Price != 148.9 || b[1].ChartDate != "2026-03-16" {
		t.Fatalf("%+v", b)
	}
}

// Bitcoin trades every day: its "2-year" and "4-year" averages are 730 and
// 1,460 days, not 504 and 1,008 trading days.
func TestCryptoCountsCalendarDays(t *testing.T) {
	bars := make([]market.Bar, 1600)
	for i := range bars {
		p := 100.0
		if i >= 1600-730 {
			p = 50 // the last two calendar years at 50
		}
		bars[i] = market.Bar{Date: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02"), High: p, Low: p, Close: p}
	}
	if barsPerYear(bars) != 365 || barsPerYear(fallen()[:0]) != 252 {
		t.Fatal("year length")
	}
	r := levels(bars, bars)
	if !near(r.SMA2y, 50) || !near(r.SMA4y, (730*100+730*50)/1460.0) {
		t.Fatalf("2y %v 4y %v", r.SMA2y, r.SMA4y)
	}
}

// Without an exchange rate a bought rung stays bought (it can't judge the
// size, but it mustn't ask to buy again); a trade in another currency than
// the listing's is not compared with its limits; exactly half a third counts.
func TestBoughtRungsSurviveNoFXAndIgnoreOtherCurrencies(t *testing.T) {
	f := Fund{Symbol: "OKLO", Reserve: money.FromFloat(3000)}
	buy := TradeIn{Date: "2026-06-01", Ticker: "OKLO", Action: "buy", Price: 60, Shares: 17, Currency: "USD"}
	r := levels(fallen(), fallen())
	r.Symbol, r.Currency, r.FX = "OKLO", "USD", 0
	apply(&r, f, []TradeIn{buy})
	if r.Left != money.FromFloat(2000) {
		t.Fatalf("no FX: left %v", r.Left)
	}
	r = levels(fallen(), fallen())
	r.Symbol, r.Currency = "OKLO.DE", "EUR"
	apply(&r, f, []TradeIn{buy})
	if r.Left != money.FromFloat(3000) {
		t.Fatalf("a dollar trade took a euro listing's rung: left %v", r.Left)
	}
	half := TradeIn{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 50, Shares: 10} // €500 = half of €1,000
	if r := todoOf(t, Fund{Symbol: "VWCE.DE", Reserve: money.FromFloat(3000)}, half); r.Left != money.FromFloat(2000) {
		t.Fatalf("half a third: left %v", r.Left)
	}
	under := TradeIn{Date: "2026-06-01", Ticker: "VWCE", Action: "buy", Price: 50, Shares: 9.98}
	if r := todoOf(t, Fund{Symbol: "VWCE.DE", Reserve: money.FromFloat(3000)}, under); r.Left != money.FromFloat(3000) {
		t.Fatalf("under half a third: left %v", r.Left)
	}
}

// Bitcoin bought is money moved into a crypto account: it is priced at the
// day's close and takes the rung like a trade.
func TestCryptoTransferIsABuy(t *testing.T) {
	resetLast(t)
	d := testutil.DB(t)
	s := Settings{Funds: []Fund{{Symbol: "BTC-EUR", Reserve: money.FromFloat(3000)}}}
	if err := SavePlan(d, Crypto, s); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO transactions(date,kind,amount,account_id,to_account_id,category,created_at,updated_at) VALUES('2026-06-01','transfer',100000,'swed','btc_m','transfer.invest','x','x')`); err != nil {
		t.Fatal(err)
	}
	p := Build(LoadPlan(d, Crypto), func(string) (*market.Series, error) { return &market.Series{Currency: "EUR", Bars: fallen()}, nil })
	r := p.Funds[0]
	if r.Left != money.FromFloat(2000) || len(r.Buys) != 0 && r.Buys[0].Price != 60 {
		t.Fatalf("left %v buys %+v", r.Left, r.Buys)
	}
}
