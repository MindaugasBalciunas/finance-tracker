package wealth_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
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
		return 0, 0, 0, 0, "", errNone
	})
	// AAA: 2 shares × target × 0.92 EUR/USD; ETF counts at today's value.
	if s.LowEUR != 73.6+92 || s.MeanEUR != 110.4+92 || s.HighEUR != 147.2+92 || s.Covered != 0.5 {
		t.Fatalf("%+v", s)
	}
}

var errNone = errFn("no targets")

type errFn string

func (e errFn) Error() string { return string(e) }
