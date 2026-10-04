import { describe, expect, it } from 'vitest'
import type { MonthReview } from '../api/review'
import { buildHighlights, shorten } from './reviewHighlights'

const totals = (income: number, spending: number) => ({
  income, spending, invested: 0, net_saved: income - spending,
  savings_rate: income > 0 ? Math.round(((income - spending) / income) * 1000) / 10 : undefined,
})

function review(over: Partial<MonthReview> = {}): MonthReview {
  return {
    month: '2026-09', complete: true, ...totals(5000, 4000),
    previous: totals(5000, 4000), six_month_avg: totals(5000, 4000),
    categories: [], top_expenses: [], owed_to_you: 0, checks: [],
    ...over,
  }
}

describe('buildHighlights', () => {
  it('a normal month says so plainly', () => {
    const h = buildHighlights(review())
    expect(h.headline).toMatch(/^You saved .+ — 20% of income$/)
    expect(h.tone).toBe('good')
    expect(h.bullets[0].text).toMatch(/^Spending was about usual/)
  })

  it('an overspent month names the driver and the payment behind it', () => {
    const h = buildHighlights(review({
      ...totals(5000, 6200),
      by_category: [
        { category: 'Entertainment', spent: 1400, average: 100, delta: 1300, top: [
          { id: 1, date: '2026-09-26', amount: 1113.5, category: 'Entertainment', comment: 'Restoranas Smelyne, Panevėžys. Family reunion' },
        ] },
      ],
      top_expenses: [{ id: 1, date: '2026-09-26', amount: 1113.5, category: 'Entertainment', comment: 'Restoranas Smelyne, Panevėžys. Family reunion' }],
    }))
    expect(h.tone).toBe('bad')
    expect(h.headline).toMatch(/^You spent .+ more than you earned$/)
    expect(h.bullets[0].text).toContain('Mostly Entertainment')
    expect(h.bullets[0].text).toContain('Restoranas Smelyne')
    expect(h.bullets.some((b) => b.icon === '✨'), 'already named above, not repeated as a one-off').toBe(false)
  })

  it('a re-filed regular payment is called that, and its old category is not good news', () => {
    const h = buildHighlights(review({
      ...totals(5000, 5300),
      by_category: [
        { category: 'Finance', spent: 2400, average: 1400, delta: 1000, top: [
          { id: 7, date: '2026-09-17', amount: 1000, category: 'Finance', comment: 'Aliments 2026.09 Evelina', recurring: true, moved_from: 'Kids' },
        ] },
        { category: 'Kids', spent: 180, average: 1000, delta: -820, top: [] },
      ],
    }))
    expect(h.bullets[0].text).toContain('Aliments 2026.09 Evelina')
    expect(h.bullets[0].text).toContain('instead of Kids')
    expect(h.bullets.some((b) => b.icon === '👍' && b.text.startsWith('Kids'))).toBe(false)
  })

  it('regular payments are never one-offs', () => {
    const h = buildHighlights(review({
      top_expenses: [
        { id: 1, date: '2026-09-17', amount: 1000, category: 'Finance', comment: 'Aliments', recurring: true },
        { id: 2, date: '2026-09-21', amount: 920, category: 'Vacation', comment: 'Final payment for Navaturas Egypt trip' },
      ],
    }))
    const oneOff = h.bullets.find((b) => b.icon === '✨')!
    expect(oneOff.text).toContain('Navaturas')
    expect(oneOff.text).not.toContain('Aliments')
  })

  it('budget and fixed obligations', () => {
    const h = buildHighlights(review({
      fixed: 2000,
      budget: { over: [], within_count: 0, lines: [
        { name: 'Food', budgeted: 200, spent: 650 },
        { name: 'Fun', budgeted: 150, spent: 100 },
      ] },
    }))
    expect(h.bullets.find((b) => b.icon === '🔒')!.text).toContain('(50%)')
    expect(h.bullets.find((b) => b.icon === '🎯')!.text).toMatch(/^1 of 2 budget lines went over — most of all Food/)
  })

  it('caps the list', () => {
    expect(buildHighlights(review({ fixed: 1, owed_to_you: 5 }), 2).bullets).toHaveLength(2)
  })
})

describe('shorten', () => {
  it('keeps dotted numbers, cuts at a clause', () => {
    expect(shorten('Aliments 2026.09 Evelina')).toBe('Aliments 2026.09 Evelina')
    expect(shorten('Restoranas Smelyne, Panevėžys. Family reunion')).toBe('Restoranas Smelyne')
    expect(shorten('Final payment for Navaturas Egypt trip 2026-10-18')).toBe('Final payment for Navaturas Eg…')
  })
})
