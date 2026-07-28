import { describe, it, expect } from 'vitest'
import { computeMonthPlan, medianMonthlyIncome, budgetMatches } from './budget'
import type { Transaction, Budget, MonthlySummary } from '../types'

function tx(type: string, category: string, value: number, labels = '', comment = ''): Transaction {
  return {
    id: Math.round(Math.random() * 1e9),
    date: '2026-07-10',
    type,
    category,
    labels,
    comment,
    amount: { value, currency: 'EUR' },
    created_at: '',
    updated_at: '',
  } as Transaction
}

function budget(kind: string, name: string, amount: number, opts: { label?: string; category?: string } = {}): Budget {
  return {
    id: Math.round(Math.random() * 1e9),
    name,
    kind,
    label: opts.label ?? '',
    category: opts.category ?? '',
    amount,
    created_at: '',
    updated_at: '',
  } as Budget
}

const PLAN: Budget[] = [
  budget('fixed', 'Loan', 1285, { label: 'loan' }),
  budget('fixed', 'Alimony', 1000, { label: 'alimony' }),
  budget('investment', 'VWCE', 1000, { category: 'Stocks & ETF' }),
  budget('investment', 'Artea', 200, { category: 'Pension' }),
  budget('spending', 'Food', 500, { category: 'Food' }),
]

describe('budgetMatches', () => {
  it('matches by label when set, category otherwise', () => {
    const loanTx = tx('expense', 'Finance', 755, 'loan')
    expect(budgetMatches(PLAN[0], loanTx)).toBe(true)
    expect(budgetMatches(PLAN[4], loanTx)).toBe(false)
    const foodTx = tx('expense', 'Food', 50)
    expect(budgetMatches(PLAN[4], foodTx)).toBe(true)
  })

  it('a comma-list label matches any label in the group', () => {
    const eatingOut = budget('spending', 'Eating out', 300, { label: 'restaurant,fast food,delivery' })
    expect(budgetMatches(eatingOut, tx('expense', 'Food', 20, 'restaurant'))).toBe(true)
    expect(budgetMatches(eatingOut, tx('expense', 'Food', 12, 'fast food,kids'))).toBe(true)
    expect(budgetMatches(eatingOut, tx('expense', 'Food', 30, 'wolt,delivery'))).toBe(true)
    expect(budgetMatches(eatingOut, tx('expense', 'Food', 80, 'groceries'))).toBe(false)
  })
})

describe('medianMonthlyIncome', () => {
  it('takes the median of complete months, skipping the current one', () => {
    const byMonth = [
      { year: 2026, month: 5, income: 5000, expenses: 0, investments: 0 },
      { year: 2026, month: 6, income: 6000, expenses: 0, investments: 0 },
      { year: 2026, month: 7, income: 100, expenses: 0, investments: 0 }, // current
    ] as MonthlySummary[]
    expect(medianMonthlyIncome(byMonth, new Date(2026, 6, 20))).toBe(5500)
  })

  it('returns null with no history', () => {
    expect(medianMonthlyIncome([], new Date())).toBeNull()
  })
})

describe('computeMonthPlan', () => {
  const monthTxs = [
    tx('expense', 'Finance', 755.1, 'loan', 'Loan interest'),
    tx('expense', 'Finance', 530.04, 'loan', 'Loan return'),
    tx('expense', 'Kids', 1000, 'alimony', 'Aliments'),
    tx('expense', 'Food', 320),
    tx('expense', 'Transport', 80),
    tx('investment', 'Stocks & ETF', 600),
    tx('investment', 'Pension', 200),
  ]

  it('computes fixed, investment and discretionary correctly', () => {
    const plan = computeMonthPlan(PLAN, monthTxs, 5500)

    expect(plan.fixed.find((s) => s.budget.name === 'Loan')!.actual).toBeCloseTo(1285.14)
    expect(plan.fixed.find((s) => s.budget.name === 'Alimony')!.actual).toBe(1000)
    expect(plan.investments.find((s) => s.budget.name === 'VWCE')!.actual).toBe(600)
    expect(plan.investments.find((s) => s.budget.name === 'Artea')!.actual).toBe(200)
    expect(plan.spending.find((s) => s.budget.name === 'Food')!.actual).toBe(320)

    // Discretionary excludes loan + alimony: 320 + 80
    expect(plan.discretionarySpent).toBeCloseTo(400)
    // Unbudgeted: Transport only (Food has a budget)
    expect(plan.unbudgeted).toEqual([{ category: 'Transport', spent: 80 }])
    // Safe to spend: 5500 − 2285 fixed − 1200 targets − 400 spent
    expect(plan.safeToSpend).toBeCloseTo(5500 - 2285 - 1200 - 400)
  })

  it('handles missing income base', () => {
    const plan = computeMonthPlan(PLAN, monthTxs, null)
    expect(plan.safeToSpend).toBeNull()
  })

  it('does not count expense transactions toward investment budgets', () => {
    const plan = computeMonthPlan(
      [budget('investment', 'VWCE', 1000, { category: 'Stocks & ETF' })],
      [tx('expense', 'Stocks & ETF', 50)],
      null
    )
    expect(plan.investments[0].actual).toBe(0)
  })

  it('spending limits ignore fixed obligations sharing the category', () => {
    // Alimony lives inside the unified Kids category but has its own fixed
    // budget — the Kids spending limit only measures discretionary spend.
    const plan = computeMonthPlan(
      [
        budget('fixed', 'Alimony', 1000, { label: 'alimony' }),
        budget('spending', 'Kids', 250, { category: 'Kids' }),
      ],
      [
        tx('expense', 'Kids', 1000, 'alimony', 'Aliments'),
        tx('expense', 'Kids', 60, 'kids,entertainment', '360 arena'),
      ],
      null
    )
    expect(plan.fixed[0].actual).toBe(1000)
    expect(plan.spending[0].actual).toBe(60)
    expect(plan.unbudgeted).toEqual([])
  })

  it('label-group spending budgets aggregate their labels', () => {
    const plan = computeMonthPlan(
      [budget('spending', 'Eating out', 300, { label: 'restaurant,fast food,delivery' })],
      [
        tx('expense', 'Food', 45, 'restaurant', 'Sushi'),
        tx('expense', 'Food', 12, 'fast food', 'McDonalds'),
        tx('expense', 'Food', 28, 'delivery,wolt', 'Wolt'),
        tx('expense', 'Food', 90, 'groceries', 'Maxima'),
      ],
      null
    )
    expect(plan.spending[0].actual).toBe(85)
    expect(plan.unbudgeted).toEqual([{ category: 'Food', spent: 90 }])
  })

  it('label-scoped spending budgets hide only their slice from unbudgeted', () => {
    const plan = computeMonthPlan(
      [budget('spending', 'Gifts', 100, { label: 'gift' })],
      [
        tx('expense', 'Gifts', 40, 'gift', 'Birthday present'),
        tx('expense', 'Gifts', 25, '', 'Donation'),
      ],
      null
    )
    expect(plan.spending[0].actual).toBe(40)
    expect(plan.unbudgeted).toEqual([{ category: 'Gifts', spent: 25 }])
  })
})
