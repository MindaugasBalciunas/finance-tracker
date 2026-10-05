import { addMonths, eur, eurc, eurk, monthLabel, parseNum, pct, signed } from '../lib/format'
import { qs } from '../lib/api'
import { catColor, GROUPS } from '../lib/categories'

describe('format', () => {
  it('formats euros for overviews, lists and axes', () => {
    expect(eur(1234.4)).toBe('€1,234')
    expect(eur(-0.4)).toBe('€0')
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
