package domain

import "time"

// Currency represents a currency code
type Currency string

const (
	CurrencyEUR Currency = "EUR"
	CurrencyUSD Currency = "USD"
	CurrencyBTC Currency = "BTC"
)

// Money represents an amount of money with currency information
type Money struct {
	// Value is the numeric amount
	Value float64 `json:"value"`
	// Currency is the ISO 4217 currency code
	Currency Currency `json:"currency"`
}

// NewMoney creates a Money value
func NewMoney(value float64, currency Currency) Money {
	return Money{Value: value, Currency: currency}
}

// CryptoAmount represents an amount of cryptocurrency with price context
type CryptoAmount struct {
	// Amount is the quantity of the cryptocurrency
	Amount float64 `json:"amount"`
	// Unit is the cryptocurrency identifier (e.g., "BTC")
	Unit Currency `json:"unit"`
	// PricePerUnit is the exchange rate (e.g., EUR per BTC) - 0 if not available
	PricePerUnit float64 `json:"price_per_unit"`
	// PriceValidAt is when the price was last updated
	PriceValidAt *time.Time `json:"price_valid_at,omitempty"`
	// ConvertedValue is the amount converted to base currency (computed)
	ConvertedValue float64 `json:"converted_value,omitempty"`
	// ConvertedCurrency is the currency of ConvertedValue
	ConvertedCurrency Currency `json:"converted_currency,omitempty"`
}

// NewCryptoAmount creates a CryptoAmount with optional conversion
func NewCryptoAmount(amount float64, unit Currency, pricePerUnit float64) CryptoAmount {
	ca := CryptoAmount{
		Amount:       amount,
		Unit:         unit,
		PricePerUnit: pricePerUnit,
	}
	if pricePerUnit > 0 {
		ca.ConvertedValue = amount * pricePerUnit
		ca.ConvertedCurrency = CurrencyEUR
	}
	return ca
}

// IsValidPrice checks if the price is reasonable (>= minimum threshold)
func (ca CryptoAmount) IsValidPrice() bool {
	const minValidPrice = 100.0
	return ca.PricePerUnit >= minValidPrice
}

// InEUR converts the crypto amount to EUR if a valid price is available
// Prefers livePricePerUnit, falls back to CryptoAmount.PricePerUnit
func (ca CryptoAmount) InEUR(livePricePerUnit float64) (Money, bool) {
	priceToUse := ca.PricePerUnit

	// Prefer live price if it's valid
	if livePricePerUnit > 0 && livePricePerUnit >= 100.0 {
		priceToUse = livePricePerUnit
	}

	// If we have no valid price, can't convert
	if priceToUse < 100.0 {
		return Money{}, false
	}

	return Money{
		Value:    ca.Amount * priceToUse,
		Currency: CurrencyEUR,
	}, true
}
