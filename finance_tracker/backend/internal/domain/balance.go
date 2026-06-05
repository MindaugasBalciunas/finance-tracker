package domain

import "time"

// Balance represents a point-in-time snapshot of all accounts
//
// All EUR-denominated fields are in EUR currency.
// BTC fields are stored as BTC units with corresponding price at snapshot time.
// Enhanced fields with Money types (prefixed "M") are computed on read and not persisted.
type Balance struct {
	ID     uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Date   time.Time `json:"date" gorm:"not null;uniqueIndex:idx_balances_date_auto"`
	IsAuto bool      `json:"is_auto" gorm:"not null;default:false;uniqueIndex:idx_balances_date_auto"`
	Total     float64   `json:"total"`      // EUR - sum of all account balances
	Seb       float64   `json:"seb"`        // EUR
	Swed      float64   `json:"swed"`       // EUR
	SwedETF   float64   `json:"swed_etf"`   // EUR
	SebPen    float64   `json:"seb_pen"`    // EUR
	Luminor   float64   `json:"luminor"`    // EUR
	Art       float64   `json:"art"`        // EUR
	Cash      float64   `json:"cash"`       // EUR
	RevM      float64   `json:"rev_m"`      // EUR
	RevR      float64   `json:"rev_r"`      // EUR
	RBTC      float64   `json:"r_btc"`      // stored in BTC units
	MBTC      float64   `json:"m_btc"`      // stored in BTC units
	BtcPrice  float64   `json:"btc_price"`  // EUR/BTC rate at snapshot time (0 = legacy EUR row or price unknown)
	RevStocks  float64   `json:"rev_stocks"`  // EUR - Revolut stocks portfolio
	IBKRStocks float64   `json:"ibkr_stocks"` // EUR - IBKR portfolio
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed EUR values for BTC fields (not persisted, only set when holding > 0)
	RBtcEur float64 `json:"r_btc_eur,omitempty" gorm:"-"` // r_btc converted to EUR at live price
	MBtcEur float64 `json:"m_btc_eur,omitempty" gorm:"-"` // m_btc converted to EUR at live price
}

// BalanceFilter holds filtering options for querying balances
type BalanceFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
}

// BalanceTrend holds time-series data for charting
type BalanceTrend struct {
	Dates    []string             `json:"dates"`
	Totals   []float64            `json:"totals"`
	Accounts map[string][]float64 `json:"accounts"`
}

// AccountAllocation holds the latest allocation across accounts
type AccountAllocation struct {
	Account    string  `json:"account"`
	Amount     float64 `json:"amount"`
	Percentage float64 `json:"percentage"`
}
