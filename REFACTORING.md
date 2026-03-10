# Data Structure Refactoring Guide

## Overview

This refactoring introduces robust, type-safe money and cryptocurrency amount handling throughout the codebase. It maintains **backward compatibility** by keeping legacy fields while adding new enriched computed fields.

## Key Changes

### 1. New Domain Types (Backend)

#### `Money` Type
```go
type Money struct {
    Value    float64 // Numeric amount
    Currency Currency // EUR, USD, BTC, etc.
}
```

Used for:
- Balance totals and account balances
- Transaction amounts
- Stock prices and costs

#### `CryptoAmount` Type
```go
type CryptoAmount struct {
    Amount            float64    // BTC quantity
    Unit              Currency   // "BTC"
    PricePerUnit      float64    // EUR/BTC rate
    PriceValidAt      *time.Time // When price was captured
    ConvertedValue    float64    // Amount in EUR (computed)
    ConvertedCurrency Currency   // "EUR"
}
```

Used for:
- BTC holdings with price context
- Explicit tracking of conversion rates
- Clear indication of price validity

### 2. Updated Models

#### Balance
**Old fields (kept for backward compatibility):**
- `RBtcEur`, `MBtcEur` - Float values without context

**New computed fields:**
- `RBtcComputed` - CryptoAmount with full conversion context
- `MBtcComputed` - CryptoAmount with full conversion context
- `TotalEUR` - Money with explicit EUR currency

#### Transaction
**Old fields (kept):**
- `Amount` - Float value

**New computed fields:**
- `AmountMoney` - Money{value: Amount, currency: EUR}

#### StockTrade
**Old fields (kept):**
- `PricePerShare`, `Currency` 

**New computed fields:**
- `PricePerShareMoney` - Money wrapper with currency
- `TotalCostMoney` - Money{value: Shares*Price, currency: Currency}

#### StockHolding
**Old fields (kept):**
- `AvgCostUSD`, `TotalCostUSD`, `RealizedGain`

**New computed fields:**
- `AvgCostMoney`, `TotalCostMoney`, `RealizedGainMoney` - Money wrappers

### 3. Frontend Types (TypeScript)

New base types in `frontend/src/types/index.ts`:
```typescript
type Currency = 'EUR' | 'USD' | 'BTC'

interface Money {
  value: number
  currency: Currency
}

interface CryptoAmount {
  amount: number
  unit: Currency
  price_per_unit: number
  price_valid_at?: string
  converted_value?: number
  converted_currency?: Currency
}
```

All domain interfaces updated to include optional Money-typed computed fields.

### 4. Frontend Utilities

New utility module: `frontend/src/utils/money.ts`
- `formatMoney(money: Money): string` - Format with appropriate currency
- `formatCryptoAmount(crypto: CryptoAmount): string` - Format with conversion
- `isValidBtcPrice(price: number): boolean` - Check price validity
- `getMoneyValue()` - Safe value extraction

## Migration Guide

### For Backend Developers

#### Using Money Types in Services

```go
// Old way
balance.RBtcEur = btcAmount * pricePerUnit
balance.Total = sum

// New way - service automatically populates:
balance.RBtcComputed = &CryptoAmount{
    Amount:            btcAmount,
    Unit:              CurrencyBTC,
    PricePerUnit:      pricePerUnit,
    ConvertedValue:    btcAmount * pricePerUnit,
    ConvertedCurrency: CurrencyEUR,
}
balance.TotalEUR = Money{
    Value:    total,
    Currency: CurrencyEUR,
}
```

#### Service Layer Pattern

All services follow this pattern:

```go
// Fetch data
entity, err := s.repo.Get(id)

// Populate Money fields
populateMoney(entity)

return entity, nil
```

Helper functions handle all Money field population:
- `populateTransactionMoney(tx *Transaction)`
- `populateStockTradeMoney(trade *StockTrade)`
- `populateStockHoldingMoney(holding *StockHolding)`

### For Frontend Developers

#### Using Money Types in Components

```typescript
// Old way
const eurValue = balance.r_btc_eur + balance.m_btc_eur

// New way - use computed fields
const eurValue = getCryptoConverted(balance.r_btc_computed) + 
                 getCryptoConverted(balance.m_btc_computed)

// Display with proper formatting
<span>{formatMoney(balance.total_eur)}</span>
<span>{formatCryptoAmount(balance.r_btc_computed)}</span>
```

#### Backward Compatibility

Old fields still available:
```typescript
// Both work (use new way preferably)
const old = balance.r_btc_eur
const new = balance.r_btc_computed?.converted_value
```

## Database Migration

### Safe (Non-Breaking)

✅ New `Money` and `CryptoAmount` types are **NOT persisted**
✅ Existing database schema remains unchanged
✅ Computed fields generated on read
✅ No migrations needed
✅ Zero risk of data loss

### No Schema Changes Required

All new fields have `gorm:"-"` tag, meaning:
- Not mapped to database columns
- Computed in Go after retrieval
- Can be safely added/modified without migration

## Testing Strategy

### 1. API Response Validation

Verify new Money fields are populated:
```bash
GET /api/v1/balances/latest
# Response includes: r_btc_computed, m_btc_computed, total_eur

GET /api/v1/stocks/portfolio
# Response includes: holdings[].avg_cost_money, total_cost_money

GET /api/v1/transactions
# Response includes: transactions[].amount_money
```

### 2. Backward Compatibility

Verify old fields still work:
```bash
GET /api/v1/balances/latest
# Response still includes: r_btc_eur, m_btc_eur (unchanged)

GET /api/v1/stocks
# Response still includes: avg_cost_usd, total_cost_usd
```

### 3. Computed Field Accuracy

- BTC conversion uses correct live price
- Money currency matches source field currency
- Totals accurately reflect USD/EUR distinction

## Usage Examples

### Example 1: Display Balance with BTC

```typescript
// Get balance with computed Money fields
const { data: balance } = useLatestBalance(btcPrice)

// Display total with explicit currency
<span>{formatMoney(balance.total_eur)}</span>

// Display BTC with conversion
<span>{formatCryptoAmount(balance.r_btc_computed)}</span>
// Output: "0.017 BTC (€680.25)"
```

### Example 2: Stock Portfolio Costs

```typescript
// Portfolio now has Money-typed totals
const { data: portfolio } = useStockPortfolio()

portfolio.holdings.forEach(h => {
  // Use Money type
  const cost = formatMoney(h.total_cost_money)
  const gain = formatMoney(h.realized_gain_money)
  
  // Or use legacy fields (for migration period)
  const legacyCost = h.total_cost_usd // in USD
})
```

### Example 3: Transaction Amount

```typescript
const transaction = useTr ransaction(id)

// New way - explicit currency
const amount = formatMoney(transaction.amount_money)

// Old way - implicit EUR (still works)
const amount = formatEuro(transaction.amount)
```

## Benefits

1. **Type Safety**: Currency always explicit
2. **Clarity**: Money objects bundle amount + currency
3. **Conversions**: CryptoAmount includes price context
4. **Flexibility**: Easy to add other currencies later
5. **Maintainability**: Single source of Money formatting logic
6. **Backward Compatible**: Old code continues to work

## Deprecation Timeline

- **Current**: New Money fields available alongside legacy fields
- **Future**: Mark legacy fields as deprecated in comments
- **Roadmap**: Eventually remove legacy fields in major version

## Future Enhancements

With Money types in place, easy to add:
- Multi-currency portfolio summaries
- Currency conversion utilities
- Exchange rate tracking
- Audit logs with prices
- Real-time conversion in UI
