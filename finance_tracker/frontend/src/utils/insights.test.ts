import { describe, it, expect } from 'vitest'
import {
  spendPaceInsight,
  categoryMoverInsights,
  topPayeeInsight,
  largestExpenseInsight,
} from './insights'
import type { Transaction, MonthlySummary } from '../types'

function tx(date: string, value: number, category = 'Food', comment = ''): Transaction {
  return {
    id: Math.round(Math.random() * 1e9),
    date,
    type: 'expense',
    category,
    amount: { value, currency: 'EUR' },
    comment,
    created_at: '',
    updated_at: '',
  } as Transaction
}

function month(year: number, m: number, expenses: number): MonthlySummary {
  return { year, month: m, income: 5000, expenses, investments: 0 } as MonthlySummary
}

const NOW = new Date(2026, 6, 20) // 2026-07-20

describe('spendPaceInsight', () => {
  it('projects current month spend against the average', () => {
    const byMonth = [month(2026, 5, 4000), month(2026, 6, 4000), month(2026, 7, 2000)]
    const ins = spendPaceInsight(byMonth, NOW)
    // 2000 / 20 days * 31 days = 3100 → 22.5% below the 4000 average
    expect(ins).not.toBeNull()
    expect(ins!.tone).toBe('good')
    expect(ins!.text).toContain('below your average')
  })

  it('flags a month tracking far above average', () => {
    const byMonth = [month(2026, 5, 3000), month(2026, 6, 3000), month(2026, 7, 4000)]
    const ins = spendPaceInsight(byMonth, NOW)
    // 4000 / 20 * 31 = 6200 → ~107% above 3000
    expect(ins!.tone).toBe('bad')
    expect(ins!.text).toContain('above your average')
  })

  it('returns null without a current month or history', () => {
    expect(spendPaceInsight([month(2026, 5, 3000)], NOW)).toBeNull()
    expect(spendPaceInsight([month(2026, 7, 1000)], NOW)).toBeNull()
  })
})

describe('categoryMoverInsights', () => {
  it('reports categories far above their average', () => {
    const expenses = [
      tx('2026-05-10', 400, 'Food'),
      tx('2026-06-10', 400, 'Food'),
      tx('2026-07-10', 900, 'Food'),
      tx('2026-05-10', 100, 'Transport'),
      tx('2026-06-10', 100, 'Transport'),
      tx('2026-07-10', 105, 'Transport'),
    ]
    const movers = categoryMoverInsights(expenses, NOW)
    expect(movers).toHaveLength(1)
    expect(movers[0].text).toContain('Food is up')
    expect(movers[0].tone).toBe('warn')
  })

  it('reports categories far below their average as good', () => {
    const expenses = [
      tx('2026-05-10', 600, 'Vacation'),
      tx('2026-06-10', 600, 'Vacation'),
      tx('2026-07-10', 100, 'Vacation'),
    ]
    const movers = categoryMoverInsights(expenses, NOW)
    expect(movers).toHaveLength(1)
    expect(movers[0].text).toContain('Vacation is down')
    expect(movers[0].tone).toBe('good')
  })

  it('ignores small absolute or relative changes', () => {
    const expenses = [
      tx('2026-06-10', 100, 'Food'),
      tx('2026-07-10', 130, 'Food'), // +30% but only €30
      tx('2026-06-10', 1000, 'Housing'),
      tx('2026-07-10', 1100, 'Housing'), // +€100 but only 10%
    ]
    expect(categoryMoverInsights(expenses, NOW)).toHaveLength(0)
  })
})

describe('topPayeeInsight', () => {
  it('aggregates by comment case-insensitively', () => {
    const expenses = [
      tx('2026-07-01', 50, 'Food', 'Maxima'),
      tx('2026-07-08', 60, 'Food', 'maxima'),
      tx('2026-07-15', 70, 'Food', 'MAXIMA'),
      tx('2026-07-02', 120, 'Health', 'Pharmacy'),
    ]
    const ins = topPayeeInsight(expenses)
    expect(ins!.text).toContain('Maxima')
    expect(ins!.text).toContain('3 payments')
  })

  it('returns null when no comment repeats', () => {
    expect(topPayeeInsight([tx('2026-07-01', 50, 'Food', 'One-off')])).toBeNull()
  })
})

describe('largestExpenseInsight', () => {
  it('finds the biggest single expense', () => {
    const expenses = [
      tx('2026-07-01', 90, 'Food', 'small'),
      tx('2026-07-05', 1250, 'Housing', 'Loan payment'),
    ]
    const ins = largestExpenseInsight(expenses)
    expect(ins!.text).toContain('Loan payment')
    expect(ins!.text).toContain('2026-07-05')
  })

  it('stays quiet when everything is small', () => {
    expect(largestExpenseInsight([tx('2026-07-01', 40, 'Food', 'coffee')])).toBeNull()
  })
})
