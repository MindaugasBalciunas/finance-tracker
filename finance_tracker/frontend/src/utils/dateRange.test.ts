import { describe, it, expect, afterEach, vi } from 'vitest'
import { presetRange } from './dateRange'

// presetRange must format from LOCAL date parts. The old toISOString() path
// converted to UTC first, so between 00:00 and 03:00 local time in a
// positive-offset zone (UTC+2/+3) every preset slid back a day. Setting the
// clock to just past local midnight on a month boundary catches that class
// of bug in any non-UTC test environment.

afterEach(() => {
  vi.useRealTimers()
})

function setLocalTime(iso: string) {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(iso)) // no zone suffix → parsed as local time
}

describe('presetRange', () => {
  it('this-month starts on the 1st of the current local month', () => {
    setLocalTime('2026-03-01T00:30:00')
    expect(presetRange('this-month')).toEqual({ date_from: '2026-03-01' })
  })

  it('last-month spans the full previous local month', () => {
    setLocalTime('2026-03-01T00:30:00')
    expect(presetRange('last-month')).toEqual({
      date_from: '2026-02-01',
      date_to: '2026-02-28',
    })
  })

  it('3m / 6m / 1y walk whole months back, crossing year boundaries', () => {
    setLocalTime('2026-03-01T00:30:00')
    expect(presetRange('3m')).toEqual({ date_from: '2025-12-01' })
    expect(presetRange('6m')).toEqual({ date_from: '2025-09-01' })
    expect(presetRange('1y')).toEqual({ date_from: '2025-03-01' })
  })

  it('ytd starts on January 1st of the current local year', () => {
    setLocalTime('2026-03-01T00:30:00')
    expect(presetRange('ytd')).toEqual({ date_from: '2026-01-01' })
  })

  it('month-end does not overflow into the wrong month (Jan 31 → Dec 1)', () => {
    // setDate(1) before setMonth() must protect against 31 → short-month overflow.
    setLocalTime('2026-01-31T12:00:00')
    expect(presetRange('this-month')).toEqual({ date_from: '2026-01-01' })
    expect(presetRange('last-month')).toEqual({
      date_from: '2025-12-01',
      date_to: '2025-12-31',
    })
  })

  it('all and custom return an empty range', () => {
    setLocalTime('2026-03-01T00:30:00')
    expect(presetRange('all')).toEqual({})
    expect(presetRange('custom')).toEqual({})
  })
})
