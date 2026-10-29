package cfo

import (
	"database/sql"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/market"
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
