package domain

import "time"

// AssetType categorises a physical asset
type AssetType string

const (
	AssetTypeVehicle    AssetType = "vehicle"
	AssetTypeRealEstate AssetType = "real_estate"
	AssetTypeSolar      AssetType = "solar"
	AssetTypeOther      AssetType = "other"
)

// Asset represents a physical asset (vehicle, real estate, solar installation…).
// All monetary amounts are in EUR. Zero loan fields mean the asset is owned outright.
type Asset struct {
	ID            uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name          string     `json:"name" gorm:"not null"`
	Type          AssetType  `json:"type" gorm:"not null;default:'other';index"`
	PurchaseDate  *time.Time `json:"purchase_date,omitempty"`
	PurchasePrice float64    `json:"purchase_price" gorm:"not null"`
	CurrentValue  float64    `json:"current_value"` // latest valuation; defaults to purchase price
	ValuationDate *time.Time `json:"valuation_date,omitempty"`
	Notes         string     `json:"notes"`

	// Financing
	LoanRemaining     float64    `json:"loan_remaining"` // outstanding balance
	LoanRemainingDate *time.Time `json:"loan_remaining_date,omitempty"`
	LoanRate          string     `json:"loan_rate,omitempty"`    // legacy free text, e.g. "6M EURIBOR + 1.3%"
	LoanAccount       string     `json:"loan_account,omitempty"` // account payments are deducted from
	LoanPaidOffDate   *time.Time `json:"loan_paid_off_date,omitempty"`
	// Structured interest: total rate = bank margin + variable base
	// (EURIBOR); the base resets on LoanRateResetDate. LoanMonthlyPayment
	// drives the amortization projection of the remaining balance.
	LoanMargin         float64    `json:"loan_margin"`          // % p.a., fixed part
	LoanLabel          string     `json:"loan_label,omitempty"` // transaction label identifying this loan's payments
	LoanBaseRate       float64    `json:"loan_base_rate"`       // % p.a., variable part (EURIBOR)
	LoanRateResetDate  *time.Time `json:"loan_rate_reset_date,omitempty"`
	LoanMonthlyPayment float64    `json:"loan_monthly_payment"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed for API response (not persisted)
	Equity float64 `json:"equity" gorm:"-"`
}

// ComputeEquity returns current value minus outstanding loan.
func (a *Asset) ComputeEquity() float64 {
	return a.CurrentValue - a.LoanRemaining
}

// AssetSummary aggregates all assets for the overview cards.
type AssetSummary struct {
	Count              int     `json:"count"`
	TotalPurchasePrice float64 `json:"total_purchase_price"`
	TotalValue         float64 `json:"total_value"`
	TotalLoans         float64 `json:"total_loans"`
	NetEquity          float64 `json:"net_equity"` // total value − total loans
}
