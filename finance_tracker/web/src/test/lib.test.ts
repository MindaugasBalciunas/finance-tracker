import { dayTicks } from '../components/MonthChart'
import { barRate } from '../lib/flow'
import { fiSeries } from '../lib/fi'
import type { Flow } from '../lib/types'
import { ledgerLink } from '../lib/links'
import { clickLabel } from '../lib/usage'
import { addMonths, eur, eurc, eurk, monthLabel, parseNum, pct, signed } from '../lib/format'
import { qs } from '../lib/api'
import { catColor, GROUPS } from '../lib/categories'

describe('format', () => {
  it('formats euros for overviews, lists and axes', () => {
    expect(eur(1234.4)).toBe('€1,234')
    // Small amounts keep their cents (Revolut €11.46 must not read €11).
    expect(eur(11.46)).toBe('€11.46')
    expect(eur(707.95)).toBe('€707.95')
    expect(eur(-0.4)).toBe('-€0.40')
    expect(eur(12)).toBe('€12')
    expect(eurk(11.46)).toBe('€11.46')
    expect(eurk(609)).toBe('€609')
    expect(signed(-2.49)).toBe('−€2.49')
    expect(eur(null)).toBe('—')
    expect(eurc(1234.5)).toBe('€1,234.50')
    expect(eurk(1500)).toBe('€1.5k')
    expect(eurk(250000)).toBe('€250k')
    expect(eurk(-2_400_000)).toBe('−€2.4M')
    expect(signed(504)).toBe('+€504')
    expect(signed(-12)).toBe('−€12')
    expect(pct(0.214)).toBe('21%')
    expect(pct(0.0361, 1)).toBe('3.6%')
  })
  it('handles months across year boundaries', () => {
    expect(addMonths('2026-01', -1)).toBe('2025-12')
    expect(addMonths('2026-11', 3)).toBe('2027-02')
    expect(monthLabel('2026-10')).toBe('Oct ’26')
    expect(monthLabel('2026-10', true)).toBe('October 2026')
  })
})

describe('api query strings', () => {
  it('skips empty values and repeats lists', () => {
    expect(qs({ a: 'x', b: '', c: undefined, d: false, e: true, tag: ['kids', 'trip:rome'] })).toBe('?a=x&e=1&tag=kids&tag=trip%3Arome')
    expect(qs({})).toBe('')
  })
})

describe('chart colours', () => {
  it('follow the category, not its rank, and fold the rest into Other', () => {
    expect(catColor('food.groceries')).toBe(catColor('food'))
    expect(catColor('food')).not.toBe(catColor('housing'))
    expect(catColor('dating')).toBe('var(--s-other)')
    expect(new Set(GROUPS.map((g) => g.slot)).size).toBe(GROUPS.length)
  })
})

describe('parseNum', () => {
  it('reads decimals typed either way, never NaN', () => {
    expect(parseNum('12,5')).toBe(12.5)
    expect(parseNum('12.5')).toBe(12.5)
    expect(parseNum('0,0091')).toBe(0.0091)
    expect(parseNum('1 234,56')).toBe(1234.56)
    expect(parseNum('1.234,56')).toBe(1234.56)
    expect(parseNum('1,234.56')).toBe(1234.56)
    expect(parseNum('1,234,567')).toBe(1234567)
    expect(parseNum('€ 9.99')).toBe(9.99)
    expect(parseNum('-3,5')).toBe(-3.5)
    expect(parseNum('.5')).toBe(0.5)
    expect(parseNum('12.')).toBe(12)
    for (const bad of ['', 'abc', '1,2,3.4.5', '12a', '--1', '.']) expect(parseNum(bad)).toBeUndefined()
    expect(parseNum(NaN)).toBeUndefined()
    expect(parseNum(7)).toBe(7)
  })
})

describe('usage click labels', () => {
  it('never record what a drill-down link says, only where it leads', () => {
    const a = document.createElement('a')
    a.href = '#/ledger?merchant=Evelina&from=2026-01-01'
    a.textContent = 'Evelina ×6 €5,452'
    expect(clickLabel(a)).toBe('link → /ledger')
    const b = document.createElement('button')
    b.setAttribute('aria-label', 'Collapse menu')
    expect(clickLabel(b)).toBe('Collapse menu')
  })
})

describe('ledgerLink', () => {
  it('lists the transactions behind a figure', () => {
    expect(ledgerLink({ tag: 'trip:egypt' })).toBe('/ledger?tag=trip%3Aegypt&period=all')
    expect(ledgerLink({ category: ['food', 'kids'], month: '2026-02' })).toBe('/ledger?category=food&category=kids&from=2026-02-01&to=2026-02-28')
    expect(ledgerLink({ kind: 'income', year: '2025' })).toBe('/ledger?kind=income&from=2025-01-01&to=2025-12-31')
  })
})

describe('month chart day labels', () => {
  it('labels every day when wide, and never two neighbours at the end on phones', () => {
    expect(dayTicks(30, true)).toHaveLength(30)
    expect(dayTicks(28, false).slice(-2)).toEqual([25, 28])
    expect(dayTicks(29, false).slice(-2)).toEqual([27, 29])
    expect(dayTicks(30, false).slice(-2)).toEqual([27, 30])
    expect(dayTicks(31, false).slice(-2)).toEqual([29, 31])
    expect(dayTicks(31, false)[0]).toBe(1)
  })
})

describe('savings rate by month', () => {
  const flow = (f: Partial<Flow>): Flow => ({ period: '2026-11', income: 0, spending: 0, essential: 0, discretionary: 0, saved: 0, savings_rate: 0, invested: 0, principal: 0, payroll_pension: 0, income_by: {}, spending_by: {}, ...f })
  it('rates a month whose salary is still due with the expected salary, not as a month with no pay', () => {
    // November on the 5th: €100 in, €257 spent, salary due by 10 Dec.
    expect(barRate(flow({ income: 100, spending: 257, savings_rate: -1.57, expected_salary: 6000, expected_savings_rate: 0.958 }))).toBeCloseTo(95.8)
    expect(barRate(flow({ income: 4125, spending: 4000, savings_rate: 0.03 }))).toBeCloseTo(3)
    expect(barRate(flow({ income: 0, spending: 300 }))).toBeNull()
  })
})

describe('FI projection spans', () => {
  const yearly = Array.from({ length: 31 }, (_, y) => ({ year: 2026 + y, investable: 1000 * y, monthly: 10 * y }))
  const monthly = Array.from({ length: 121 }, (_, m) => ({ year: 2026 + Math.floor((m + 10) / 12), month: `${2026 + Math.floor((m + 10) / 12)}-${String(((m + 10) % 12) + 1).padStart(2, '0')}`, investable: 100 * m, monthly: m }))
  it('reads up to ten years month by month and further a year at a time', () => {
    const one = fiSeries({ payouts: yearly, payouts_monthly: monthly }, 1)
    expect(one).toHaveLength(13)
    expect(one[0].label).toBe('2026-11')
    expect(one[12].label).toBe('2027-11')
    expect(fiSeries({ payouts: yearly, payouts_monthly: monthly }, 10)).toHaveLength(121)
    const thirty = fiSeries({ payouts: yearly, payouts_monthly: monthly }, 30)
    expect(thirty).toHaveLength(31)
    expect(thirty[30].label).toBe('2056')
    // An older server without monthly points still draws the yearly line.
    expect(fiSeries({ payouts: yearly }, 3).map((p) => p.label)).toEqual(['2026', '2027', '2028', '2029'])
  })
})
