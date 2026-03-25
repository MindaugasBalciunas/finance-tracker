package domain

// Currency represents an ISO 4217 currency code or crypto identifier
type Currency string

const (
	CurrencyEUR Currency = "EUR"
	CurrencyUSD Currency = "USD"
)

// Money pairs a numeric amount with its currency.
// Used in API responses wherever the currency of an amount is not implicit.
type Money struct {
	Value    float64  `json:"value"`
	Currency Currency `json:"currency"`
}
