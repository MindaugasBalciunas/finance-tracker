import type { Transaction, Budget, MonthlySummary } from '../types'
import { txLabels } from './labels'

export interface BudgetStatus {
  budget: Budget
  actual: number // matched amount this month
}

export interface MonthPlan {
  fixed: BudgetStatus[]
  investments: BudgetStatus[]
  spending: BudgetStatus[]
  fixedPlanned: number
  investmentPlanned: number
  discretionarySpent: number // expenses not matched by fixed budgets
  unbudgeted: { category: string; spent: number }[] // discretionary categories without a spending budget
  safeToSpend: number | null // income base − fixed − investment targets − discretionary spent
  incomeBase: number | null
}

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

export function budgetMatches(budget: Budget, tx: Transaction): boolean {
  const labels = budgetLabels(budget)
  if (labels.length > 0) return labels.some((l) => txHasLabel(tx, l))
  return budget.category === (tx.category as string)
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

// Computes the month's plan status from the month's transactions.
export function computeMonthPlan(
  budgets: Budget[],
  monthTxs: Transaction[],
  incomeBase: number | null
): MonthPlan {
  const fixed = budgets.filter((b) => b.kind === 'fixed')
  const investments = budgets.filter((b) => b.kind === 'investment')
  const spending = budgets.filter((b) => b.kind === 'spending')

  const isFixedTx = (tx: Transaction) => fixed.some((b) => budgetMatches(b, tx))
  const discretionaryTxs = monthTxs.filter((tx) => tx.type === 'expense' && !isFixedTx(tx))
  const discretionarySpent = discretionaryTxs.reduce((s, tx) => s + tx.amount.value, 0)

  const status = (b: Budget): BudgetStatus => {
    const wantType = b.kind === 'investment' ? 'investment' : 'expense'
    // Spending limits measure choices, so fixed obligations don't count
    // against them (alimony matches the Kids category but is not kids
    // discretionary spending — it has its own fixed budget).
    const pool = b.kind === 'spending' ? discretionaryTxs : monthTxs
    const actual = pool
      .filter((tx) => tx.type === wantType && budgetMatches(b, tx))
      .reduce((s, tx) => s + tx.amount.value, 0)
    return { budget: b, actual }
  }

  const fixedStatus = fixed.map(status)
  const investmentStatus = investments.map(status)
  const spendingStatus = spending.map(status)

  // Unbudgeted = discretionary spend no spending budget matches, grouped by
  // category. Matching per transaction (not per category) lets label-scoped
  // budgets (e.g. gift) cover their slice of a category without hiding the
  // rest of it.
  const byCategory: Record<string, number> = {}
  for (const tx of discretionaryTxs) {
    if (spending.some((b) => budgetMatches(b, tx))) continue
    const cat = tx.category as string
    byCategory[cat] = (byCategory[cat] ?? 0) + tx.amount.value
  }
  const unbudgeted = Object.entries(byCategory)
    .map(([category, spent]) => ({ category, spent }))
    .sort((a, b) => b.spent - a.spent)

  const fixedPlanned = fixed.reduce((s, b) => s + b.amount, 0)
  const investmentPlanned = investments.reduce((s, b) => s + b.amount, 0)

  const safeToSpend = incomeBase != null
    ? incomeBase - fixedPlanned - investmentPlanned - discretionarySpent
    : null

  return {
    fixed: fixedStatus,
    investments: investmentStatus,
    spending: spendingStatus,
    fixedPlanned,
    investmentPlanned,
    discretionarySpent,
    unbudgeted,
    safeToSpend,
    incomeBase,
  }
}
