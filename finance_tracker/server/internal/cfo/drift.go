package cfo

import (
	"database/sql"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// PriceView is price creep with the official rate beside the personal one;
// the page, Ask CFO and the MCP tool all read this one view.
type PriceView struct {
	*insights.Prices
	Official *market.Inflation `json:"official"` // nil when Eurostat can't be reached
}

func Prices(d *sql.DB, now time.Time) (*PriceView, error) {
	p, err := insights.PriceCreep(d, now)
	if err != nil {
		return nil, err
	}
	return &PriceView{p, market.LithuaniaInflation()}, nil
}

// Ownership is the true cost of every property and vehicle, with money tied
// up in them valued at the plan's expected return (made nominal).
func Ownership(d *sql.DB, now time.Time) ([]wealth.OwnCost, error) {
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-1, -1, 0).Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	return wealth.Ownership(d, book, txs, plan.LoadSettings(d).ExpectedReturn+inflation, now)
}

// Fees gathers what is invested and what it costs: positions from trades
// (live prices when asked, else cost) and the accounts that are funds as a
// whole — pensions, and broker accounts without trades (bank funds).
func Fees(d *sql.DB, now time.Time, live bool) (*wealth.Fees, error) {
	trades, err := wealth.ListTrades(d)
	if err != nil {
		return nil, err
	}
	p := wealth.BuildPortfolio(trades, live)
	var holdings []wealth.FeeLine
	for _, h := range p.Holdings {
		v := 0.0
		switch {
		case h.ValueEUR != nil:
			v = *h.ValueEUR
		case h.CostEUR != nil:
			v = *h.CostEUR
		}
		if v <= 0 {
			continue
		}
		holdings = append(holdings, wealth.FeeLine{ID: h.Ticker, Name: firstNonEmpty(h.Name, h.Ticker), Value: money.FromFloat(v)})
	}
	traded := map[string]bool{}
	for _, t := range trades {
		traded[t.AccountID] = true
	}
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	snap := book.SnapshotAt(now.Format("2006-01-02"), true)
	var funds []wealth.FeeLine
	for id, a := range book.Accounts {
		if a.Archived || !(a.Kind == "pension" || (a.Kind == "brokerage" && !traded[id])) {
			continue
		}
		if v := snap.ByAccount[id]; v > 0 {
			funds = append(funds, wealth.FeeLine{ID: id, Name: a.Name, Value: v})
		}
	}
	core := wealth.LoadInvestSettings(d, p).Core
	return wealth.AnalyseFees(wealth.LoadFeeSettings(d), holdings, funds, plan.LoadSettings(d).ExpectedReturn+inflation, core...), nil
}
