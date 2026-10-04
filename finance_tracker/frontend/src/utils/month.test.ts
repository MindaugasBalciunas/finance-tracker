import { beforeEach, describe, expect, it, vi } from 'vitest'
import { lastCompleteMonth, markReviewNudgeSeen, reviewNudgeSeen } from './month'

describe('review nudge', () => {
  // Node 25's own global localStorage shadows jsdom's and is unusable
  // without a backing file — stub a plain in-memory one.
  beforeEach(() => {
    const store = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    })
  })

  it('last complete month crosses the year boundary', () => {
    expect(lastCompleteMonth(new Date(2026, 0, 3))).toBe('2025-12')
    expect(lastCompleteMonth(new Date(2026, 9, 4))).toBe('2026-09')
  })

  it('closing hides it for that month only', () => {
    expect(reviewNudgeSeen('2026-09')).toBe(false)
    markReviewNudgeSeen('2026-09')
    expect(reviewNudgeSeen('2026-09')).toBe(true)
    expect(reviewNudgeSeen('2026-10')).toBe(false)
  })
})

describe('review nudge without storage', () => {
  it('shows rather than crashing', () => {
    vi.stubGlobal('localStorage', { getItem: () => { throw new Error('denied') }, setItem: () => { throw new Error('denied') } })
    expect(reviewNudgeSeen('2026-09')).toBe(false)
    expect(() => markReviewNudgeSeen('2026-09')).not.toThrow()
  })
})
