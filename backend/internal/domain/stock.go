package domain

import "time"

// StockAction represents whether a trade is a buy or sell
type StockAction string

const (
	StockActionBuy  StockAction = "buy"
	StockActionSell StockAction = "sell"
)

// StockTrade represents a single stock buy or sell transaction
type StockTrade struct {
	ID            uint        `json:"id" gorm:"primaryKey;autoIncrement"`
	Date          time.Time   `json:"date" gorm:"not null;index"`
	Action        StockAction `json:"action" gorm:"not null"`
	Ticker        string      `json:"ticker" gorm:"not null;index"`
	Shares        float64     `json:"shares" gorm:"not null"`
	PricePerShare float64     `json:"-" gorm:"not null"`    // DB column; use PricePerShareMoney in responses
	Currency      string      `json:"currency" gorm:"default:'USD'"` // currency of PricePerShare
	Notes         string      `json:"notes"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`

	// Computed for API response (not persisted)
	PricePerShareMoney Money `json:"price_per_share" gorm:"-"`
}

// StockHolding represents the computed position for one ticker
// All amounts are in the currency specified by the Currency field.
type StockHolding struct {
	Ticker       string  `json:"ticker"`
	Currency     string  `json:"currency"` // currency of all Money fields below
	Shares       float64 `json:"shares"`
	AvgCost      Money   `json:"avg_cost"`      // average cost per share
	TotalCost    Money   `json:"total_cost"`    // total amount invested
	RealizedGain Money   `json:"realized_gain"` // gain/loss from closed positions
}

// StockPortfolio represents the full computed portfolio with all holdings
type StockPortfolio struct {
	Holdings          []StockHolding `json:"holdings"`
	TotalCost         Money          `json:"total_cost"`          // sum of all holdings' cost basis
	TotalRealizedGain Money          `json:"total_realized_gain"` // sum of all realized gains
}
