import { describe, expect, it } from 'vitest'
import { awayText, closeness, rungColor, SERIES, nextRung, orderCost, orderLine, ordersByFund, px, standingOrders, yearTicks } from '../lib/dips'
import type { DipFund, DipLevel } from '../lib/types'

const lv = (key: DipLevel['key'], price: number, reached: boolean, tranche = 1000): DipLevel =>
  ({ key, label: key, action: '', price, basis: price, away: price / 100 - 1, reached, tranche: key === 'dip' ? 0 : tranche, shares: Math.floor(tranche / price), status: key === 'dip' ? 'watch' : reached ? 'buy' : 'order' })
const fund = (levels: DipLevel[]) => ({ levels } as unknown as DipFund)

describe('dip plan', () => {
  it('names the highest unreached rung that releases money as next', () => {
    expect(nextRung(fund([lv('dip', 95, false), lv('t1', 90, false), lv('t2', 80, false)]))?.key).toBe('t1')
    expect(nextRung(fund([lv('dip', 95, true), lv('t1', 90, true), lv('t2', 80, false)]))?.key).toBe('t2')
    expect(nextRung(fund([lv('t1', 90, true)]))).toBeUndefined()
    // no reserve set yet: the next rung is still the next rung
    expect(nextRung(fund([lv('t1', 90, false, 0)]))?.key).toBe('t1')
  })

  it('puts one tick on each new year', () => {
    expect(yearTicks(['2024-11-01', '2024-12-20', '2025-01-03', '2025-06-01', '2026-01-02'])).toEqual(['2025-01-03', '2026-01-02'])
  })

  it('prints a €4 listing to the tenth of a cent and the rest to the cent', () => {
    expect(px(4.1234)).toBe('€4.123')
    expect(px(602.49)).toBe('€602.49')
    expect(px(undefined)).toBe('—')
  })

  it('says how far a rung is, and an order never costs more than its third', () => {
    expect(awayText(lv('t1', 88, false))).toBe('12% lower')
    expect(awayText(lv('t1', 101, true))).toBe('reached')
    const l = lv('t2', 602.49, false)
    expect(l.shares).toBe(1)
    expect(orderCost(l)).toBeLessThanOrEqual(l.tranche)
  })

  it('lists the limit orders to keep: unreached money rungs, not bought ones', () => {
    const f = { symbol: 'VWCE.DE', currency: 'EUR', levels: [lv('dip', 95, false), lv('t1', 90, true), { ...lv('t2', 80, false), status: 'bought' as const }, lv('t3', 56, false)] } as unknown as DipFund
    const orders = standingOrders([f])
    expect(orders.map((o) => o.level.key)).toEqual(['t3'])
    expect(orderLine(f, orders[0].level)).toBe('VWCE limit buy 17 @ €56.00 (VWCE.DE)')
  })

  it('puts each fund’s orders on one line, highest first', () => {
    const a = { symbol: 'VWCE.DE', levels: [lv('t1', 90, false), lv('t2', 80, false)] } as unknown as DipFund
    const b = { symbol: 'SPYL.DE', levels: [lv('t1', 14, false)] } as unknown as DipFund
    const g = ordersByFund(standingOrders([a, b]))
    expect(g.map((x) => [x.fund.symbol, x.levels.map((l) => l.key)])).toEqual([['VWCE.DE', ['t1', 't2']], ['SPYL.DE', ['t1']]])
  })

  it('colours a rung green when due, amber within 5%, plain further down', () => {
    expect(closeness(lv('t1', 101, true))).toBe('buy')
    expect(closeness(lv('t1', 96, false))).toBe('near')
    expect(closeness(lv('t2', 80, false))).toBe('far')
    expect(closeness({ ...lv('dip', 99, false), away: -0.01 })).toBe('far') // the −10% line buys nothing
  })

  it('gives each rung its average’s chart colour', () => {
    expect(rungColor({ label: '2-year average' })).toBe(SERIES.sma2y)
    expect(rungColor({ label: '4-year average' })).toBe(SERIES.sma4y)
    expect(rungColor({ label: '−10% under the 200-day average' })).toBe(SERIES.sma200)
    expect(rungColor({ label: '−44% from the high' })).toBe(SERIES.deep)
  })
})
