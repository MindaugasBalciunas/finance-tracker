import type { BudgetInput, BudgetLineStatus } from '../../types'

export function ym(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
}

export function monthRange(month: string): { date_from: string; date_to: string } {
  const [y, m] = month.split('-').map(Number)
  const last = new Date(y, m, 0).getDate()
  return { date_from: `${month}-01`, date_to: `${month}-${String(last).padStart(2, '0')}` }
}

export function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(y, m - 1).toLocaleDateString('en', { month: 'long', year: 'numeric' })
}

export function shortMonth(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(y, m - 1).toLocaleDateString('en', { month: 'short' })
}

export function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  return ym(new Date(y, m - 1 + delta))
}

// Funds start accruing in January of the current year: a fund set up in
// October already knows what the year's holidays cost.
export function defaultFundStart(): string {
  return `${new Date().getFullYear()}-01`
}

// The input that re-saves a line unchanged — the base for one-tap edits.
export function lineInput(l: BudgetLineStatus): BudgetInput {
  return {
    name: l.name,
    kind: l.kind,
    label: l.label ?? '',
    category: l.category ?? '',
    amount: l.amount,
    period: l.period,
    fund: l.fund,
    start_month: l.fund_state?.start_month ?? '',
  }
}

// "€1.2k" for dense cells.
export function compactEuro(v: number): string {
  const a = Math.abs(v)
  const sign = v < 0 ? '−' : ''
  if (a >= 10000) return `${sign}${Math.round(a / 1000)}k`
  if (a >= 1000) return `${sign}${(a / 1000).toFixed(1)}k`
  return `${sign}${Math.round(a)}`
}
