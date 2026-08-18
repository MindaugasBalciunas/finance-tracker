import { describe, it, expect } from 'vitest'
import { formatEuro, formatUsd, formatPercent, formatDate, formatShortDate } from './format'

// format.ts uses the lt-LT locale: digit groups are separated by spaces
// (regular or non-breaking depending on the ICU version), decimals by a comma,
// and month names are Lithuanian. Assertions avoid ICU-specific separators.

describe('formatEuro', () => {
  it('formats positive amount', () => {
    const result = formatEuro(1234.56)
    expect(result.replace(/\s/g, ' ')).toContain('1 234,56')
  })
  it('formats zero', () => {
    expect(formatEuro(0)).toContain('0,00')
  })
  it('includes euro symbol', () => {
    expect(formatEuro(100)).toContain('€')
  })
  it('returns a dash for non-finite input', () => {
    expect(formatEuro(NaN)).toBe('—')
    expect(formatEuro(Infinity)).toBe('—')
    expect(formatEuro(-Infinity)).toBe('—')
  })
})

describe('formatUsd', () => {
  it('formats finite amounts', () => {
    expect(formatUsd(1234.5)).toContain('1,234.50')
  })
  it('returns a dash for non-finite input', () => {
    expect(formatUsd(NaN)).toBe('—')
    expect(formatUsd(Infinity)).toBe('—')
  })
})

describe('formatPercent', () => {
  it('signs positive values and keeps decimals', () => {
    expect(formatPercent(3.456)).toBe('+3.46%')
    expect(formatPercent(-1.5, 1)).toBe('-1.5%')
  })
  it('returns a dash for non-finite input', () => {
    expect(formatPercent(NaN)).toBe('—')
    expect(formatPercent(Infinity)).toBe('—')
    expect(formatPercent(-Infinity)).toBe('—')
  })
})

describe('formatDate', () => {
  it('formats ISO date string', () => {
    const result = formatDate('2026-01-15T00:00:00Z')
    expect(result).toContain('15')
    expect(result).toContain('saus') // Lithuanian "sausio" = January
    expect(result).toContain('2026')
  })
})

describe('formatShortDate', () => {
  it('omits year', () => {
    const result = formatShortDate('2026-03-06T00:00:00Z')
    expect(result).toContain('06')
    expect(result).not.toContain('2026')
  })
})
