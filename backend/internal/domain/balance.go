package domain

import "time"

// Balance represents a point-in-time snapshot of all accounts
type Balance struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Date      time.Time `json:"date" gorm:"not null;uniqueIndex"`
	Total     float64   `json:"total"`
	Seb       float64   `json:"seb"`
	Swed      float64   `json:"swed"`
	SwedETF   float64   `json:"swed_etf"`
	SwedPen   float64   `json:"swed_pen"`
	Luminor   float64   `json:"luminor"`
	Art       float64   `json:"art"`
	Cash      float64   `json:"cash"`
	RevM      float64   `json:"rev_m"`
	RevR      float64   `json:"rev_r"`
	RBTC      float64   `json:"r_btc"`      // stored in BTC units
	MBTC      float64   `json:"m_btc"`      // stored in BTC units
	BtcPrice  float64   `json:"btc_price"`  // EUR/BTC rate at snapshot time (0 = legacy EUR row)
	RevStocks float64   `json:"rev_stocks"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BalanceFilter holds filtering options for querying balances
type BalanceFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
}

// BalanceTrend holds time-series data for charting
type BalanceTrend struct {
	Dates     []string             `json:"dates"`
	Totals    []float64            `json:"totals"`
	Accounts  map[string][]float64 `json:"accounts"`
}

// AccountAllocation holds the latest allocation across accounts
type AccountAllocation struct {
	Account    string  `json:"account"`
	Amount     float64 `json:"amount"`
	Percentage float64 `json:"percentage"`
}
