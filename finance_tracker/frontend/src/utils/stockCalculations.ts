import type { StockTrade } from '../types'

export interface SellPnLEntry {
  id: number
  date: string
  ticker: string
  shares: number
  sellPrice: number
  avgCost: number
  pnl: number
  currency: string
}

// Below this, a share count is float residue from a full sell, not a position.
const SHARE_EPSILON = 1e-6

export function computeSellPnL(trades: StockTrade[]): SellPnLEntry[] {
  const byTicker: Record<string, StockTrade[]> = {}
  for (const t of trades) {
    if (!byTicker[t.ticker]) byTicker[t.ticker] = []
    byTicker[t.ticker].push(t)
  }
  const results: SellPnLEntry[] = []
  for (const tickerTrades of Object.values(byTicker)) {
    // Tie-break same-day trades on id so a buy entered before a sell on the
    // same date is processed in that order regardless of input order.
    const sorted = [...tickerTrades].sort(
      (a, b) => a.date.localeCompare(b.date) || a.id - b.id
    )
    let runningShares = 0
    let runningCost = 0
    for (const t of sorted) {
      if (t.action === 'buy') {
        runningShares += t.shares
        runningCost += t.shares * t.price_per_share.value
      } else if (t.action === 'sell' && runningShares > 0) {
        const avgCost = runningCost / runningShares
        // Clamp to what's actually held — an oversell (e.g. a position partly
        // opened outside the app) must never drive runningShares negative.
        const sold = Math.min(t.shares, runningShares)
        const pnl = sold * (t.price_per_share.value - avgCost)
        results.push({
          id: t.id,
          date: t.date.slice(0, 10),
          ticker: t.ticker,
          shares: sold,
          sellPrice: t.price_per_share.value,
          avgCost,
          pnl,
          currency: t.currency,
        })
        runningCost -= sold * avgCost
        runningShares -= sold
        // Float residue after a full sell would poison the next buy's avg
        // cost — snap a near-zero position to exactly zero.
        if (runningShares < SHARE_EPSILON) {
          runningShares = 0
          runningCost = 0
        }
      }
    }
  }
  return results.sort((a, b) => a.date.localeCompare(b.date))
}
