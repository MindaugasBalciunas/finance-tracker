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

export function computeSellPnL(trades: StockTrade[]): SellPnLEntry[] {
  const byTicker: Record<string, StockTrade[]> = {}
  for (const t of trades) {
    if (!byTicker[t.ticker]) byTicker[t.ticker] = []
    byTicker[t.ticker].push(t)
  }
  const results: SellPnLEntry[] = []
  for (const tickerTrades of Object.values(byTicker)) {
    const sorted = [...tickerTrades].sort((a, b) => a.date.localeCompare(b.date))
    let runningShares = 0
    let runningCost = 0
    for (const t of sorted) {
      if (t.action === 'buy') {
        runningShares += t.shares
        runningCost += t.shares * t.price_per_share.value
      } else if (t.action === 'sell' && runningShares > 0) {
        const avgCost = runningCost / runningShares
        const pnl = t.shares * (t.price_per_share.value - avgCost)
        results.push({
          id: t.id,
          date: t.date.slice(0, 10),
          ticker: t.ticker,
          shares: t.shares,
          sellPrice: t.price_per_share.value,
          avgCost,
          pnl,
          currency: t.currency,
        })
        runningCost -= t.shares * avgCost
        runningShares -= t.shares
      }
    }
  }
  return results.sort((a, b) => a.date.localeCompare(b.date))
}
