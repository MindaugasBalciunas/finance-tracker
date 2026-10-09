package wealth_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

func TestSnapshotCarriesValuesForward(t *testing.T) {
	d := DB(t)
	Bal(t, d, "swed", "2026-01-31", 1000)
	Bal(t, d, "swed", "2026-03-31", 1500)
	Bal(t, d, "ibkr", "2026-02-15", 5000)
	Bal(t, d, "house", "2022-08-17", 355000)
	Bal(t, d, "mortgage", "2026-01-17", -270000)
	book, err := wealth.LoadBook(d)
	if err != nil {
		t.Fatal(err)
	}
	s := book.SnapshotAt("2026-02-28", true)
	if s.ByAccount["swed"] != E(1000) || s.ByAccount["ibkr"] != E(5000) {
		t.Fatalf("carry forward: %+v", s.ByAccount)
	}
	if s.NetWorth != E(1000+5000+355000-270000) || s.Debt != E(270000) || s.Liquid != E(6000) {
		t.Fatalf("totals: nw %v debt %v liquid %v", s.NetWorth, s.Debt, s.Liquid)
	}
	if s.ByGroup["real_assets"] != E(355000) || s.ByGroup["debt"] != E(-270000) {
		t.Fatal("groups", s.ByGroup)
	}
	if book.SnapshotAt("2025-12-31", false).ByGroup["cash"] != 0 {
		t.Error("nothing before the first value")
	}
	hist := book.History("2026-01-01", "2026-03-31", "month")
	if len(hist) != 3 || hist[2].ByGroup["cash"] != E(1500) {
		t.Fatalf("history %+v", hist)
	}
	// Weekly series always end on the last day (today's update is visible).
	wk := book.History("2026-01-01", "2026-03-31", "week")
	if wk[len(wk)-1].Date != "2026-03-31" || wk[len(wk)-1].ByGroup["cash"] != E(1500) {
		t.Fatalf("weekly tail %+v", wk[len(wk)-1])
	}
	if book.FirstDate() != "2022-08-17" {
		t.Error(book.FirstDate())
	}
}

func TestStaleBalancesFlagged(t *testing.T) {
	d := DB(t)
	Bal(t, d, "swed", "2026-01-01", 10)
	Bal(t, d, "house", "2026-01-01", 10) // valuations don't go stale
	book, _ := wealth.LoadBook(d)
	s := book.SnapshotAt("2026-04-01", true)
	if s.Stale["swed"] != "2026-01-01" || s.Stale["house"] != "" {
		t.Error(s.Stale)
	}
}

func TestApplyDelta(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-10-01", 500)
	// After the last value: the balance moves, on the transaction's date.
	if ok, err := wealth.ApplyDelta(d, "cash", "2026-10-03", E(-20)); !ok || err != nil {
		t.Fatal(ok, err)
	}
	book, _ := wealth.LoadBook(d)
	if p, _ := book.At("cash", "2026-10-03"); p.Value != E(480) {
		t.Fatal(p.Value)
	}
	// Before the last value: already inside it.
	if ok, _ := wealth.ApplyDelta(d, "cash", "2026-09-30", E(-20)); ok {
		t.Error("a back-dated row must not move the balance")
	}
	// Never recorded: nothing to move. Property/crypto: never.
	if ok, _ := wealth.ApplyDelta(d, "seb", "2026-10-03", E(5)); ok {
		t.Error("unrecorded account moved")
	}
	Bal(t, d, "house", "2026-01-01", 1)
	if ok, _ := wealth.ApplyDelta(d, "house", "2026-10-03", E(5)); ok {
		t.Error("property moved")
	}
	// Bank-synced accounts follow the bank, not hand-entered rows.
	Bal(t, d, "swed", "2026-10-01", 100)
	d.Exec(`INSERT INTO bank_connections(id,aspsp_name,created_at,updated_at) VALUES(1,'Swedbank','x','x')`)
	d.Exec(`INSERT INTO bank_accounts(connection_id,uid,account_id,created_at,updated_at) VALUES(1,'uid-1','swed','x','x')`)
	if ok, _ := wealth.ApplyDelta(d, "swed", "2026-10-03", E(-5)); ok {
		t.Error("bank-synced account moved by a manual row")
	}
	deltas := wealth.TxDeltas("transfer", "swed", "cash", E(100))
	if deltas["swed"] != E(-100) || deltas["cash"] != E(100) {
		t.Error(deltas)
	}
	if len(wealth.TxDeltas("expense", "", "", E(1))) != 0 {
		t.Error("no account, no delta")
	}
}

func TestSetAndDeleteBalance(t *testing.T) {
	d := DB(t)
	q := 0.5
	p := money.FromFloat(60000)
	if err := wealth.SetBalance(d, "btc_m", "2026-10-01", money.FromFloat(30000), &q, &p, ""); err != nil {
		t.Fatal(err)
	}
	if err := wealth.SetBalance(d, "btc_m", "bad", 1, nil, nil, ""); err == nil {
		t.Error("bad date accepted")
	}
	book, _ := wealth.LoadBook(d)
	pt, _ := book.At("btc_m", "2026-10-02")
	if *pt.Quantity != 0.5 || *pt.Price != p || pt.Source != "manual" {
		t.Error(pt)
	}
	wealth.DeleteBalance(d, "btc_m", "2026-10-01")
	book, _ = wealth.LoadBook(d)
	if _, ok := book.At("btc_m", "2026-10-02"); ok {
		t.Error("delete")
	}
}

func TestPortfolioAverageCost(t *testing.T) {
	trades := []wealth.Trade{
		{Date: "2025-01-01", Action: "buy", Ticker: "AAA", Shares: 10, Price: 10, Currency: "EUR", AccountID: "ibkr"},
		{Date: "2025-02-01", Action: "buy", Ticker: "AAA", Shares: 10, Price: 20, Currency: "EUR"},
		{Date: "2025-03-01", Action: "sell", Ticker: "AAA", Shares: 5, Price: 30, Currency: "EUR"},
		{Date: "2025-03-02", Action: "buy", Ticker: "BBB", Shares: 1, Price: 100, Currency: "EUR"},
		{Date: "2025-03-03", Action: "sell", Ticker: "BBB", Shares: 3, Price: 120, Currency: "EUR"}, // oversell clamps
	}
	p := wealth.BuildPortfolio(trades, false)
	if len(p.Holdings) != 1 || len(p.Closed) != 1 {
		t.Fatalf("%+v", p)
	}
	h := p.Holdings[0]
	if h.Shares != 15 || h.AvgCost != 15 || h.Cost != 225 || h.Realized != 75 {
		t.Fatalf("avg cost: %+v", h)
	}
	if p.Closed[0].Realized != 20 || p.Closed[0].Shares != 0 {
		t.Fatalf("closed: %+v", p.Closed[0])
	}
	if p.CostEUR != 225 || p.RealizedEUR != 95 {
		t.Fatalf("eur totals %v %v", p.CostEUR, p.RealizedEUR)
	}
	if !wealth.ValidTicker("VWCE.DE") || wealth.ValidTicker("drop table;") {
		t.Error("ticker validation")
	}
}

func TestTradesCRUD(t *testing.T) {
	d := DB(t)
	tr := wealth.Trade{Date: "2026-01-01", Action: "BUY", Ticker: " vwce ", Shares: 2, Price: 120}
	if err := wealth.SaveTrade(d, &tr); err != nil || tr.Ticker != "VWCE" || tr.Currency != "USD" {
		t.Fatal(tr, err)
	}
	if err := wealth.SaveTrade(d, &wealth.Trade{Date: "2026-01-01", Action: "hold", Ticker: "X", Shares: 1, Price: 1}); err == nil {
		t.Error("bad action accepted")
	}
	list, _ := wealth.ListTrades(d)
	if len(list) != 1 {
		t.Fatal(list)
	}
	wealth.DeleteTrade(d, tr.ID)
	if list, _ := wealth.ListTrades(d); len(list) != 0 {
		t.Error("delete")
	}
}

func TestLoans(t *testing.T) {
	d := DB(t)
	Bal(t, d, "house", "2025-12-01", 410000)
	Bal(t, d, "mortgage", "2026-09-17", -262596.03)
	Tx(t, d, ledger.Tx{Date: "2026-09-17", Amount: E(466.21), Category: "transfer.debt", AccountID: "seb", ToAccountID: "mortgage"})
	Tx(t, d, ledger.Tx{Date: "2026-09-17", Amount: E(894.45), Category: "housing.mortgage_interest", AccountID: "seb"})
	book, _ := wealth.LoadBook(d)
	txs, _ := ledger.All(d, ledger.Filter{})
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	loans := wealth.Loans(book, txs, now)
	if len(loans) != 1 {
		t.Fatal(loans)
	}
	l := loans[0]
	if l.Balance != E(262596.03) || l.Rate != 3.95 || l.Equity != E(410000-262596.03) {
		t.Fatalf("%+v", l)
	}
	// One month's interest at 3.95% on the balance, the rest principal.
	if l.NextInterest != E(864.38) || l.NextPrincipal != E(497.28) {
		t.Fatalf("split %v / %v", l.NextInterest, l.NextPrincipal)
	}
	if l.PaidPrincipal12 != E(466.21) || l.PaidInterest12 != E(894.45) {
		t.Fatal("12m paid")
	}
	if l.MonthsLeft < 290 || l.MonthsLeft > 320 || l.LTV < 0.63 || l.LTV > 0.65 {
		t.Fatalf("months %d ltv %v", l.MonthsLeft, l.LTV)
	}
}

func TestMovementAndScenarios(t *testing.T) {
	d := DB(t)
	Bal(t, d, "swed", "2026-01-01", 1000)
	Bal(t, d, "swed", "2026-06-01", 1500)
	Bal(t, d, "ibkr", "2026-01-01", 10000)
	Bal(t, d, "ibkr", "2026-06-01", 8000)
	book, _ := wealth.LoadBook(d)
	mv := book.Movement("2026-01-01", "2026-06-30")
	if len(mv) != 2 || mv[0].AccountID != "ibkr" || mv[0].Change != E(-2000) || mv[1].Change != E(500) {
		t.Fatalf("%+v", mv)
	}
	v, ve := 100.0, 92.0
	p := wealth.Portfolio{Holdings: []wealth.Holding{
		{Ticker: "AAA", Shares: 2, Value: &v, ValueEUR: &ve},
		{Ticker: "ETF", Shares: 1, Value: &v, ValueEUR: &ve},
	}}
	s := wealth.BuildScenarios(p, func(t string) (float64, float64, float64, int, string, error) {
		if t == "AAA" {
			return 40, 60, 80, 12, "analyst targets", nil
		}
		if t == "ETF" {
			return 0, 0, 0, 0, "", errNone // ETFs: no coverage
		}
		return 0, 0, 0, 0, "", errNone
	})
	// AAA: 2 shares × target × 0.92 EUR/USD; ETF counts at today's value.
	if s.LowEUR != 73.6+92 || s.MeanEUR != 110.4+92 || s.HighEUR != 147.2+92 || s.Covered != 0.5 {
		t.Fatalf("%+v", s)
	}
}

func TestPortfolioHistory(t *testing.T) {
	trades := []wealth.Trade{
		{Date: "2026-01-10", Action: "buy", Ticker: "AAA", Shares: 10, Price: 10, Currency: "USD"},
		{Date: "2026-02-10", Action: "buy", Ticker: "BBB", Shares: 1, Price: 100, Currency: "EUR"},
		{Date: "2026-03-10", Action: "sell", Ticker: "AAA", Shares: 5, Price: 20, Currency: "USD"},
	}
	closes := map[string][]market.HistoryPoint{
		"AAA": {{Date: "2026-01-31", Close: 12}, {Date: "2026-02-28", Close: 15}, {Date: "2026-03-31", Close: 18}},
		"BBB": {{Date: "2026-03-31", Close: 110}}, // no close in February: its earliest close stands in
		"CCC": nil,                                // no prices at all: counts at cost
	}
	usd := func(a float64, c string) float64 {
		if c == "USD" {
			return a * 0.5
		}
		return a
	}
	pts := wealth.PortfolioHistory(trades, "2026-01-01", "2026-04-01", func(tk string) ([]market.HistoryPoint, error) { return closes[tk], nil }, usd)
	want := []wealth.PerfPoint{
		{Date: "2026-01-31", Value: 60, Cost: 50},   // 10×12 USD → €60; cost 100 USD → €50
		{Date: "2026-02-28", Value: 185, Cost: 150}, // 10×15×.5 + BBB at its first close 110
		{Date: "2026-03-31", Value: 155, Cost: 125}, // 5×18×.5 + 110; cost 5×10×.5 + 100
		{Date: "2026-04-01", Value: 155, Cost: 125}, // today: last closes carried
	}
	if len(pts) != len(want) {
		t.Fatalf("%+v", pts)
	}
	for i := range want {
		if pts[i] != want[i] {
			t.Errorf("%d: got %+v want %+v", i, pts[i], want[i])
		}
	}
	if len(wealth.PortfolioHistory(nil, "", "2026-04-01", nil, usd)) != 0 {
		t.Error("no trades, no points")
	}
}

func TestRangeFallbackIsNotAnalystCoverage(t *testing.T) {
	v, ve := 100.0, 100.0
	p := wealth.Portfolio{Holdings: []wealth.Holding{{Ticker: "ETF", Shares: 1, Value: &v, ValueEUR: &ve}}}
	s := wealth.BuildScenarios(p, func(string) (float64, float64, float64, int, string, error) {
		return 80, 100, 120, 0, "52-week range", nil
	})
	if s.Covered != 0 || s.MeanEUR != 100 || s.Positions[0].Basis != "52-week range" {
		t.Fatalf("%+v", s)
	}
}

func TestBalanceTablePages(t *testing.T) {
	d := DB(t)
	Bal(t, d, "swed", "2026-01-01", 100)
	Bal(t, d, "swed", "2026-01-03", 300)
	Bal(t, d, "ibkr", "2026-01-02", 1000)
	book, _ := wealth.LoadBook(d)
	tb := book.Table(1, 2)
	if tb.Dates != 3 || tb.Pages != 2 || len(tb.Rows) != 2 || tb.Rows[0].Date != "2026-01-03" {
		t.Fatalf("%+v", tb)
	}
	top := tb.Rows[0]
	// swed recorded that day; ibkr carried from the 2nd.
	if c := top.Cells["swed"]; !c.Recorded || c.Value != E(300) || c.Source != "manual" {
		t.Errorf("recorded cell %+v", c)
	}
	if c := top.Cells["ibkr"]; c.Recorded || c.Value != E(1000) {
		t.Errorf("carried cell %+v", c)
	}
	if top.NetWorth != E(1300) || top.Liquid != E(1300) {
		t.Errorf("totals %v %v", top.NetWorth, top.Liquid)
	}
	last := book.Table(2, 2)
	if last.Page != 2 || len(last.Rows) != 1 || last.Rows[0].Date != "2026-01-01" || len(last.Rows[0].Cells) != 1 {
		t.Fatalf("page 2 %+v", last)
	}
	if over := book.Table(9, 2); over.Page != 2 {
		t.Error("page past the end clamps to the last")
	}
}

func TestRebuildLoanHistory(t *testing.T) {
	d := DB(t)
	det := `{"base_rate":2.65,"margin":1.3,"monthly_payment":1361.66,"payment_day":17,"start_date":"2022-08-17","start_principal":285000}`
	if _, err := d.Exec(`UPDATE accounts SET details=? WHERE id='mortgage'`, det); err != nil {
		t.Fatal(err)
	}
	// An old reconstruction that started too high, and the real balance.
	d.Exec(`INSERT INTO balances(account_id,date,value,source,updated_at) VALUES('mortgage','2022-08-17',-28793013,'computed','x')`)
	Bal(t, d, "mortgage", "2026-09-17", -262596.03)
	// The bank split principal out in August 2026.
	Tx(t, d, ledger.Tx{Date: "2026-08-17", Amount: E(506.43), Category: "transfer.debt", AccountID: "seb", ToAccountID: "mortgage"})
	n, err := wealth.RebuildLoanHistory(d, "mortgage")
	if err != nil || n < 48 {
		t.Fatalf("months %d err %v", n, err)
	}
	book, _ := wealth.LoadBook(d)
	if p, _ := book.At("mortgage", "2022-08-17"); p.Value != -E(285000) || p.Source != "computed" {
		t.Errorf("starts at the original principal: %+v", p)
	}
	if p, _ := book.At("mortgage", "2026-09-17"); p.Value != -E(262596.03) || p.Source != "manual" {
		t.Errorf("real balance untouched: %+v", p)
	}
	// August's recorded principal is exact: July − August = 506.43.
	jul, _ := book.At("mortgage", "2026-07-17")
	aug, _ := book.At("mortgage", "2026-08-17")
	if aug.Value-jul.Value != E(506.43) {
		t.Errorf("recorded principal month: jul %v aug %v", jul.Value, aug.Value)
	}
	// And August + September's principal lands on the anchor.
	if aug.Value >= -E(262596.03) {
		t.Errorf("owed before the anchor should be higher: %v", aug.Value)
	}
	// Owed only ever goes down.
	prev := -E(285001)
	for _, p := range book.Series("mortgage") {
		if p.Value < prev {
			t.Fatalf("owed rose on %s: %v after %v", p.Date, p.Value, prev)
		}
		prev = p.Value
	}
	// Equity explained: down payment + repaid + appreciation = value − owed.
	Bal(t, d, "house", "2025-12-01", 410000)
	d.Exec(`UPDATE accounts SET details='{"purchase_date":"2022-08-17","purchase_price":355000}' WHERE id='house'`)
	d.Exec(`UPDATE accounts SET details=json_set(details,'$.asset_id','house') WHERE id='mortgage'`)
	bk, _ := wealth.LoadBook(d)
	lv := wealth.Loans(bk, nil, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))[0]
	if lv.DownPayment != E(70000) || lv.Appreciation != E(55000) || lv.Repaid != E(22403.97) ||
		lv.DownPayment+lv.Repaid+lv.Appreciation != lv.Equity {
		t.Errorf("equity breakdown: %+v", lv)
	}
	// Saving again is stable.
	n2, _ := wealth.RebuildLoanHistory(d, "mortgage")
	book2, _ := wealth.LoadBook(d)
	if n2 != n || len(book2.Series("mortgage")) != len(book.Series("mortgage")) {
		t.Error("rebuild is idempotent")
	}
}

var errNone = errFn("no targets")

type errFn string

func (e errFn) Error() string { return string(e) }

func TestTodaysReadingsAreLoggedWithTimes(t *testing.T) {
	d := DB(t)
	today := time.Now().Format("2006-01-02")
	set := func(date string, v float64, src string) {
		t.Helper()
		if err := wealth.SetBalance(d, "swed", date, E(v), nil, nil, src); err != nil {
			t.Fatal(err)
		}
	}
	set(today, 1610.79, "bank")
	set(today, 1610.79, "bank") // a repeat sync adds nothing
	set(today, 1559.00, "bank")
	set("2026-01-31", 1000, "manual") // a past day keeps no trail
	rd, err := wealth.Readings(d, "2026-01-01", today)
	if err != nil {
		t.Fatal(err)
	}
	if len(rd[today]) != 2 || rd[today][0].Value != E(1610.79) || rd[today][1].Value != E(1559) || rd[today][1].At == "" {
		t.Fatalf("today's trail: %+v", rd[today])
	}
	if len(rd["2026-01-31"]) != 0 {
		t.Fatalf("past day logged: %+v", rd["2026-01-31"])
	}
	book, _ := wealth.LoadBook(d)
	if p, _ := book.At("swed", today); p.Value != E(1559) || p.At == "" {
		t.Fatalf("daily value should be the latest reading with its time: %+v", p)
	}
	if p, _ := book.At("swed", "2026-01-31"); p.At != "" {
		t.Fatalf("a backdated value has no time of day: %+v", p)
	}
	if err := wealth.DeleteBalance(d, "swed", today); err != nil {
		t.Fatal(err)
	}
	if rd, _ := wealth.Readings(d, today, today); len(rd[today]) != 0 {
		t.Fatal("deleting the day should drop its trail")
	}
}

// Hiding an account closes it at €0 from today (history kept); showing it
// again removes only that closing value.
func TestCloseAndReopenAccount(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	Bal(t, d, "ibkr", "2026-10-05", 1130.51)
	if err := wealth.CloseAccount(d, "ibkr", now); err != nil {
		t.Fatal(err)
	}
	book, _ := wealth.LoadBook(d)
	if book.SnapshotAt("2026-10-09", true).ByAccount["ibkr"] != 0 || book.SnapshotAt("2026-10-08", true).ByAccount["ibkr"] != E(1130.51) {
		t.Fatal("closed from today, history kept")
	}
	if err := wealth.ReopenAccount(d, "ibkr"); err != nil {
		t.Fatal(err)
	}
	book, _ = wealth.LoadBook(d)
	if p, _ := book.At("ibkr", "2026-10-09"); p.Value != E(1130.51) {
		t.Fatalf("reopened: %+v", p)
	}
	// A value entered after closing is the owner's: reopening keeps it.
	wealth.CloseAccount(d, "ibkr", now)
	Bal(t, d, "ibkr", "2026-10-10", 50)
	wealth.ReopenAccount(d, "ibkr")
	book, _ = wealth.LoadBook(d)
	if p, _ := book.At("ibkr", "2026-10-10"); p.Value != E(50) {
		t.Fatal("kept later value")
	}
	// Nothing held: nothing written.
	Bal(t, d, "swed", "2026-10-01", 0)
	wealth.CloseAccount(d, "swed", now)
	if b, _ := wealth.LoadBook(d); len(b.Series("swed")) != 1 {
		t.Fatal("empty account needs no closing value")
	}
}
