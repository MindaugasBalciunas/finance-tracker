import type { Balance } from '../types'

/** Minimum sensible EUR/BTC price — anything below is treated as invalid/legacy */
export const MIN_VALID_BTC_PRICE = 100

/**
 * Return the EUR value of a single BTC amount given snapshot and live prices.
 * Prefers live price when valid, falls back to snapshot price, handles legacy
 * rows where the field was stored directly in EUR (value >= 1).
 */
export function btcToEur(
  btcAmount: number,
  snapshotPrice: number,
  liveBtcPrice: number | null,
): number {
  const hasSnapshotPrice = snapshotPrice >= MIN_VALID_BTC_PRICE
  if (hasSnapshotPrice) {
    const price =
      liveBtcPrice && liveBtcPrice >= MIN_VALID_BTC_PRICE
        ? liveBtcPrice
        : snapshotPrice
    return btcAmount * price
  }
  // Legacy: small value looks like BTC units, convert with live price
  if (btcAmount > 0 && btcAmount < 1 && liveBtcPrice && liveBtcPrice >= MIN_VALID_BTC_PRICE) {
    return btcAmount * liveBtcPrice
  }
  // Already in EUR or zero
  return btcAmount
}

/**
 * Return total EUR value of both BTC fields from a Balance snapshot.
 */
export function balanceBtcEur(b: Balance, liveBtcPrice: number | null): number {
  return (
    btcToEur(b.r_btc ?? 0, b.btc_price ?? 0, liveBtcPrice) +
    btcToEur(b.m_btc ?? 0, b.btc_price ?? 0, liveBtcPrice)
  )
}
