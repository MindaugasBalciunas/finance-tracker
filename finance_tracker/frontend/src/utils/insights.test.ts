import { describe, it, expect } from 'vitest'
import {
  spendPaceInsight,
  categoryMoverInsights,
  topPayeeInsight,
  largestExpenseInsight,
  fixedShareInsight,
  eatingOutInsight,
  labelMoverInsights,
  labelCoverageInsight,
} from './insights'
import type { Transaction, MonthlySummary } from '../types'

function tx(date: string, value: number, category = 'Food', comment = '', labels = ''): Transaction {
  return {
    id: Math.round(Math.random() * 1e9),
    date,
    type: 'expense',
    category,
    amount: { value, currency: 'EUR' },
    comment,
    labels,
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

  it('anchors on the latest month in the period, not the calendar month', () => {
    // "Last month" filter: data ends in June, but today is July 20.
    const expenses = [
      tx('2026-04-10', 1800, 'Finance'),
      tx('2026-05-10', 1900, 'Finance'),
      tx('2026-06-10', 950, 'Finance'),
    ]
    const movers = categoryMoverInsights(expenses, NOW)
    expect(movers).toHaveLength(1)
    // June vs avg(Apr, May) = 950 vs 1850 → down ~49%, NOT "down 100%"
    expect(movers[0].text).toContain('Finance is down 49% in June')
    expect(movers[0].text).not.toContain('100%')
  })

  it('says nothing when the period has fewer than two months', () => {
    expect(categoryMoverInsights([tx('2026-06-10', 950, 'Finance')], NOW)).toHaveLength(0)
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

describe('label-aware insights', () => {
  const loanRows = [
    tx('2026-07-05', 1284, 'Finance', 'EVELINA BALČIŪNIENĖ', 'loan,evelina'),
    tx('2026-06-05', 1284, 'Finance', 'EVELINA BALČIŪNIENĖ', 'loan,evelina'),
    tx('2026-05-05', 1284, 'Finance', 'EVELINA BALČIŪNIENĖ', 'loan,evelina'),
  ]
  const shopping = [
    tx('2026-07-06', 120, 'Food', 'LIDL Pilaite', 'groceries,lidl'),
    tx('2026-07-08', 110, 'Food', 'LIDL Pilaite', 'groceries,lidl'),
  ]

  it('topPayee skips fixed obligations — a loan is not a merchant', () => {
    const ins = topPayeeInsight([...loanRows, ...shopping])
    expect(ins).not.toBeNull()
    expect(ins!.text).toContain('LIDL')
    expect(ins!.text).not.toContain('EVELINA')
  })

  it('largestExpense skips fixed obligations', () => {
    const ins = largestExpenseInsight([...loanRows, tx('2026-07-09', 450, 'Housing', 'BigBox: Fridge')])
    expect(ins!.text).toContain('Fridge')
  })

  it('fixedShare reports pre-committed money', () => {
    const ins = fixedShareInsight([...loanRows, ...shopping])
    expect(ins).not.toBeNull()
    expect(ins!.text).toContain('94%') // 3852 of 4082
  })

  it('eatingOut compares out vs groceries', () => {
    const rows = [
      ...shopping,
      tx('2026-07-10', 60, 'Food', 'KAVINE BON PIZZA', 'coffee,restaurant'),
      tx('2026-07-11', 70, 'Food', 'Bolt food', 'delivery'),
    ]
    const ins = eatingOutInsight(rows)
    expect(ins).not.toBeNull()
    expect(ins!.text).toContain('36%') // 130 out of 360 food money
    expect(ins!.tone).toBe('info')
  })

  it('eatingOut warns when eating out beats groceries', () => {
    const rows = [
      tx('2026-07-06', 100, 'Food', 'LIDL', 'groceries,lidl'),
      tx('2026-07-10', 200, 'Food', 'Restoranas', 'restaurant'),
    ]
    expect(eatingOutInsight(rows)!.tone).toBe('warn')
  })
})

describe('categoryMoverInsights with fixed obligations', () => {
  it('a recurring alimony starting mid-period is not a category trend', () => {
    const expenses = [
      // Kids - General discretionary is flat…
      tx('2026-05-10', 500, 'Kids - General', 'toys'),
      tx('2026-06-10', 520, 'Kids - General', 'clothes'),
      tx('2026-07-10', 510, 'Kids - General', 'books'),
      // …but alimony transfers begin in June.
      tx('2026-06-15', 1000, 'Kids - General', 'Aliments 2026.05', 'alimony'),
      tx('2026-07-17', 1000, 'Kids - General', 'Aliments 2026.06', 'alimony'),
    ]
    expect(categoryMoverInsights(expenses, NOW)).toHaveLength(0)
  })
})

describe('labelMoverInsights', () => {
  it('reports labels far above their average across categories', () => {
    const expenses = [
      tx('2026-05-10', 200, 'Food', 'Pizza place', 'restaurant'),
      tx('2026-06-10', 200, 'Entertainment', 'Dinner & show', 'restaurant'),
      tx('2026-07-05', 350, 'Food', 'Tasting menu', 'restaurant'),
      tx('2026-07-06', 150, 'Food', 'Sushi', 'restaurant'),
    ]
    const out = labelMoverInsights(expenses, NOW)
    expect(out).toHaveLength(1)
    expect(out[0].text).toContain('restaurant')
    expect(out[0].text).toContain('up')
    expect(out[0].tone).toBe('warn')
  })

  it('ignores fixed-obligation labels entirely', () => {
    const expenses = [
      tx('2026-05-10', 100, 'Finance', 'Loan', 'loan'),
      tx('2026-06-10', 100, 'Finance', 'Loan', 'loan'),
      tx('2026-07-05', 900, 'Finance', 'Loan extra', 'loan'),
    ]
    expect(labelMoverInsights(expenses, NOW)).toHaveLength(0)
  })

  it('returns nothing for a single month of data', () => {
    const expenses = [tx('2026-07-05', 350, 'Food', '', 'restaurant')]
    expect(labelMoverInsights(expenses, NOW)).toHaveLength(0)
  })
})

describe('labelCoverageInsight', () => {
  it('reports the unlabeled share when meaningful', () => {
    const expenses = [
      tx('2026-07-01', 700, 'Food', '', 'groceries'),
      tx('2026-07-02', 300, 'Other', 'mystery'),
    ]
    const ins = labelCoverageInsight(expenses)
    expect(ins).not.toBeNull()
    expect(ins!.text).toContain('70% of spending is labeled')
    expect(ins!.tone).toBe('warn') // 30% unlabeled ≥ 25%
  })

  it('stays quiet when coverage is high or amounts are tiny', () => {
    const covered = [
      tx('2026-07-01', 990, 'Food', '', 'groceries'),
      tx('2026-07-02', 10, 'Other', ''),
    ]
    expect(labelCoverageInsight(covered)).toBeNull()
  })
})
