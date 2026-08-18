import { describe, it, expect } from 'vitest'
import { computeSellPnL } from './stockCalculations'
import type { StockTrade } from '../types'

let nextId = 1
function trade(
  action: string,
  date: string,
  shares: number,
  price: number,
  opts: { id?: number; ticker?: string } = {}
): StockTrade {
  return {
    id: opts.id ?? nextId++,
    date,
    action,
    ticker: opts.ticker ?? 'VWCE',
    shares,
    price_per_share: { value: price, currency: 'EUR' },
    currency: 'EUR',
    source: 'IBKR',
    notes: '',
    created_at: '',
    updated_at: '',
  } as StockTrade
}

describe('computeSellPnL', () => {
  it('averages cost across multiple buys', () => {
    const entries = computeSellPnL([
      trade('buy', '2026-01-05', 10, 10),
      trade('buy', '2026-01-10', 10, 20),
      trade('sell', '2026-02-01', 5, 30),
    ])
    expect(entries).toHaveLength(1)
    expect(entries[0].avgCost).toBeCloseTo(15)
    expect(entries[0].pnl).toBeCloseTo(5 * (30 - 15)) // 75
  })

  it('keeps avg cost stable across partial sells (realized P&L correctness)', () => {
    const entries = computeSellPnL([
      trade('buy', '2026-01-05', 10, 10),
      trade('buy', '2026-01-10', 10, 20),
      trade('sell', '2026-02-01', 5, 30),
      trade('sell', '2026-03-01', 15, 15),
    ])
    expect(entries).toHaveLength(2)
    expect(entries[1].avgCost).toBeCloseTo(15)
    expect(entries[1].pnl).toBeCloseTo(0) // sold the rest exactly at avg cost
  })

  it('resets residue after a full sell so a later buy has a clean avg cost', () => {
    // 0.1 + 0.2 = 0.30000000000000004 in floats — selling 0.3 leaves residue
    // that must not leak into the next position's average cost.
    const entries = computeSellPnL([
      trade('buy', '2026-01-05', 0.1, 100),
      trade('buy', '2026-01-10', 0.2, 100),
      trade('sell', '2026-02-01', 0.3, 120),
      trade('buy', '2026-03-01', 1, 50),
      trade('sell', '2026-04-01', 1, 60),
    ])
    expect(entries).toHaveLength(2)
    expect(entries[0].pnl).toBeCloseTo(0.3 * 20) // 6
    expect(entries[1].avgCost).toBe(50) // exactly the fresh buy price
    expect(entries[1].pnl).toBeCloseTo(10)
  })

  it('clamps an oversell to the shares actually held', () => {
    const entries = computeSellPnL([
      trade('buy', '2026-01-05', 1, 100),
      trade('sell', '2026-01-10', 5, 110), // only 1 share is tracked
      trade('buy', '2026-02-01', 2, 50),
      trade('sell', '2026-02-10', 2, 55),
    ])
    expect(entries).toHaveLength(2)
    expect(entries[0].shares).toBe(1)
    expect(entries[0].pnl).toBeCloseTo(1 * (110 - 100)) // 10, not 50
    // Position after the oversell is flat, so the next cycle starts clean.
    expect(entries[1].avgCost).toBe(50)
    expect(entries[1].pnl).toBeCloseTo(10)
  })

  it('processes a same-day buy before the sell via the id tie-break', () => {
    // Sell listed first in the input — the id tie-break must reorder it.
    const entries = computeSellPnL([
      trade('sell', '2026-01-05', 10, 12, { id: 2 }),
      trade('buy', '2026-01-05', 10, 10, { id: 1 }),
    ])
    expect(entries).toHaveLength(1)
    expect(entries[0].avgCost).toBeCloseTo(10)
    expect(entries[0].pnl).toBeCloseTo(20)
  })

  it('ignores a sell with no prior position', () => {
    expect(computeSellPnL([trade('sell', '2026-01-05', 5, 100)])).toHaveLength(0)
  })
})
