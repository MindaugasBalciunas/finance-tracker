import { describe, it, expect } from 'vitest'
import { medianMonthlyIncome, budgetMatches } from './budget'
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
