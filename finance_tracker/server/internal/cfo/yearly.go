package cfo

import (
	"database/sql"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

// YearlyCost is a once-a-year cost and whether the plan sets money aside
// for it: a fund line covering its category, and how much more a month it
// would take.
type YearlyCost struct {
	insights.Yearly
	Fund      string      `json:"fund,omitempty"` // the fund line that covers it
	FundID    int64       `json:"fund_id,omitempty"`
	Covered   bool        `json:"covered"`             // the fund's contribution covers every yearly cost in it
	Shortfall money.Cents `json:"shortfall,omitempty"` // more per month the fund would need
}

// YearlyCosts lists the once-a-year costs (fixed obligations left out) and
// how the plan's funds cover them, grouped per fund so two costs in one
// fund are measured against its contribution together.
func YearlyCosts(d *sql.DB, now time.Time) ([]YearlyCost, error) {
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-4, 0, 0).Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	budgets, err := plan.List(d, false)
	if err != nil {
		return nil, err
	}
	month := now.Format("2006-01")
	isFixed := func(t *ledger.Tx) bool {
		for _, b := range budgets {
			if b.Kind == "fixed" && b.Matches(t) {
				return true
			}
		}
		return false
	}
	found := insights.DetectYearly(txs, now, isFixed)
	out := make([]YearlyCost, 0, len(found))
	need := map[int64]money.Cents{} // per fund: what its yearly costs need a month
	for _, y := range found {
		c := YearlyCost{Yearly: y}
		t := ledger.Tx{Kind: "expense", Category: y.Category, Merchant: y.Merchant}
		for _, b := range budgets {
			if b.Kind == "spending" && b.Fund && b.Matches(&t) {
				c.Fund, c.FundID = b.Name, b.ID
				need[b.ID] += y.Monthly
				break
			}
		}
		out = append(out, c)
	}
	for i := range out {
		c := &out[i]
		if c.FundID == 0 {
			c.Shortfall = c.Monthly
			continue
		}
		for _, b := range budgets {
			if b.ID == c.FundID {
				have := b.MonthlyShare(b.AmountFor(month))
				c.Covered = have >= need[b.ID]
				if !c.Covered {
					c.Shortfall = need[b.ID] - have
				}
			}
		}
	}
	return out, nil
}
