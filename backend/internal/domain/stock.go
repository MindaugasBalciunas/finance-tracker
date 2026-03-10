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
	PricePerShare float64     `json:"price_per_share" gorm:"not null"`
	Currency      string      `json:"currency" gorm:"default:'USD'"`
	Notes         string      `json:"notes"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// StockHolding represents the computed position for one ticker
type StockHolding struct {
	Ticker        string  `json:"ticker"`
	Currency      string  `json:"currency"`       // native currency of the holding (USD or EUR)
	Shares        float64 `json:"shares"`         // current shares held
	AvgCostUSD    float64 `json:"avg_cost_usd"`   // average cost per share (in native currency)
	TotalCostUSD  float64 `json:"total_cost_usd"` // total invested in native currency
	RealizedGain  float64 `json:"realized_gain"`  // gain/loss from sells in native currency
}

// StockPortfolio represents the full computed portfolio
type StockPortfolio struct {
	Holdings      []StockHolding `json:"holdings"`
	TotalCostUSD  float64        `json:"total_cost_usd"`
	TotalRealizedGain float64    `json:"total_realized_gain"`
}
