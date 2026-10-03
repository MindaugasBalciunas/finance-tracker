import type { Transaction, Budget, MonthlySummary } from '../types'
import { txLabels } from './labels'

export function txHasLabel(tx: Transaction, label: string): boolean {
  if (!label) return false
  return txLabels(tx).includes(label)
}

// A budget's label may be a comma list ("restaurant,fast food,delivery") —
// the budget then covers the whole label group: any of them matches.
export function budgetLabels(budget: Pick<Budget, 'label'>): string[] {
  return (budget.label ?? '')
    .split(',')
    .map((l) => l.trim())
    .filter(Boolean)
}

// Rule-style matching: every matcher that is set must hold. Label-only and
// category-only budgets behave as before; a budget with BOTH narrows to
// transactions of that category carrying one of the labels (e.g. category
// Pension + label artea tracks just the Artea contributions).
export function budgetMatches(budget: Budget, tx: Transaction): boolean {
  const labels = budgetLabels(budget)
  if (labels.length === 0 && !budget.category) return false
  const labelOk = labels.length === 0 || labels.some((l) => txHasLabel(tx, l))
  const categoryOk = !budget.category || budget.category === (tx.category as string)
  return labelOk && categoryOk
}

// Median income over complete months — a stable "what I earn monthly" base.
export function medianMonthlyIncome(byMonth: MonthlySummary[], now: Date): number | null {
  const complete = byMonth.filter(
    (m) => !(m.year === now.getFullYear() && m.month === now.getMonth() + 1) && m.income > 0
  )
  if (complete.length === 0) return null
  const sorted = complete.map((m) => m.income).sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 1 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}
