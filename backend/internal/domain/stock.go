package domain

import "time"

// StockAction represents whether a trade is a buy or sell
type StockAction string

const (
	StockActionBuy  StockAction = "buy"
	StockActionSell StockAction = "sell"
)

// StockTrade represents a single stock buy or sell transaction
// All prices and amounts are in the specified currency (usually USD or EUR)
type StockTrade struct {
	ID            uint        `json:"id" gorm:"primaryKey;autoIncrement"`
	Date          time.Time   `json:"date" gorm:"not null;index"`
	Action        StockAction `json:"action" gorm:"not null"`
	Ticker        string      `json:"ticker" gorm:"not null;index"`
	Shares        float64     `json:"shares" gorm:"not null"`
	PricePerShare float64     `json:"price_per_share" gorm:"not null"`
	Currency      string      `json:"currency" gorm:"default:'USD"` // Currency of PricePerShare and Total
	Notes         string      `json:"notes"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`

	// Enhanced field (computed on read)
	PricePerShareMoney Money `json:"price_per_share_money" gorm:"-"` // PricePerShare with explicit currency
	TotalCostMoney     Money `json:"total_cost_money" gorm:"-"`      // Shares * PricePerShare with currency
}

// StockHolding represents the computed position for one ticker
// All USD amounts are in the trade currency (usually USD or EUR)
type StockHolding struct {
	Ticker       string  `json:"ticker"`
	Currency     string  `json:"currency"`       // Currency of all amounts (USD or EUR)
	Shares       float64 `json:"shares"`         // Current shares held (unit-less)
	AvgCostUSD   float64 `json:"avg_cost_usd"`   // Average cost per share in currency (DEPRECATED field name, use AvgCost)
	TotalCostUSD float64 `json:"total_cost_usd"` // Total invested in currency (DEPRECATED field name, use TotalCost)
	RealizedGain float64 `json:"realized_gain"`  // Gain/loss from sells in currency

	// Enhanced fields (computed on read)
	AvgCostMoney      Money `json:"avg_cost_money" gorm:"-"`      // Average cost with currency
	TotalCostMoney    Money `json:"total_cost_money" gorm:"-"`    // Total cost with currency
	RealizedGainMoney Money `json:"realized_gain_money" gorm:"-"` // Realized gain with currency
}

// StockPortfolio represents the full computed portfolio with all holdings
type StockPortfolio struct {
	Holdings          []StockHolding `json:"holdings"`
	TotalCostUSD      float64        `json:"total_cost_usd"`      // Sum in native currency (DEPRECATED, use TotalCostMoney)
	TotalRealizedGain float64        `json:"total_realized_gain"` // Sum in native currency (DEPRECATED, use TotalRealizedGainMoney)

	// Enhanced field (computed on read)
	TotalCostMoney         Money `json:"total_cost_money" gorm:"-"`          // Total cost with currency
	TotalRealizedGainMoney Money `json:"total_realized_gain_money" gorm:"-"` // Total realized gain with currency
}
