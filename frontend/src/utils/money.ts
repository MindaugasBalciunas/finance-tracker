import type { Money, CryptoAmount } from '../types'

/** Format EUR currency */
export function formatEuro(amount: number | null | undefined): string {
  if (amount == null) return '—'
  return new Intl.NumberFormat('en-EU', { style: 'currency', currency: 'EUR' }).format(amount)
}

/** Format USD currency */
export function formatUsd(amount: number | null | undefined): string {
  if (amount == null) return '—'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount)
}

/** Format Money type with appropriate currency formatter */
export function formatMoney(money: Money | null | undefined): string {
  if (!money) return '—'
  switch (money.currency) {
    case 'EUR':
      return formatEuro(money.value)
    case 'USD':
      return formatUsd(money.value)
    case 'BTC':
      return `₿${money.value.toFixed(8)}`
    default:
      return `${money.value.toFixed(2)} ${money.currency}`
  }
}

/** Format CryptoAmount with optional conversion */
export function formatCryptoAmount(crypto: CryptoAmount | null | undefined, showConverted = true): string {
  if (!crypto) return '—'
  
  const baseValue = `${crypto.amount.toFixed(8)} ${crypto.unit}`
  
  if (showConverted && crypto.converted_value != null && crypto.converted_currency) {
    const convertedValue = formatMoney({ value: crypto.converted_value, currency: crypto.converted_currency as any })
    return `${baseValue} (${convertedValue})`
  }
  
  return baseValue
}

/** Check if a BTC price is valid (reasonable) */
export const MIN_VALID_BTC_PRICE = 100

export function isValidBtcPrice(price: number): boolean {
  return price >= MIN_VALID_BTC_PRICE
}

/** Safe getter for Money value with fallback */
export function getMoneyValue(money: Money | null | undefined, fallback = 0): number {
  return money?.value ?? fallback
}

/** Safe getter for CryptoAmount converted value */
export function getCryptoConverted(crypto: CryptoAmount | null | undefined, fallback = 0): number {
  return crypto?.converted_value ?? fallback
}
