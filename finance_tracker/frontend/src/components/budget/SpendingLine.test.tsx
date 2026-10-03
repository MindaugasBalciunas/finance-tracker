import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import SpendingLine from './SpendingLine'
import type { BudgetLineStatus } from '../../types'

function line(over: Partial<BudgetLineStatus>): BudgetLineStatus {
  return {
    id: 1, name: 'Entertainment', kind: 'spending', category: 'Entertainment', period: 'monthly', fund: false,
    amount: 200, monthly_share: 200, budgeted: 200, spent: 50, remaining: 150, month_spent: 50, history: [],
    ...over,
  }
}

const noop = () => {}

describe('SpendingLine', () => {
  it('shows a fund by what is available, not by a monthly cap', () => {
    render(<SpendingLine
      line={line({
        name: 'Vacation', fund: true, monthly_share: 400, budgeted: 600, spent: 900, remaining: -300,
        fund_state: { start_month: '2026-01', opening: 200, contribution: 400, spent: 900, available: -300, contributed: 1600, drawn: 1900 },
      })}
      daysLeft={10} onView={noop} onEdit={noop} onDelete={noop} onApply={noop} applying={false}
    />)
    expect(screen.getByText(/available/)).toBeInTheDocument()
    expect(screen.getByText(/overdrawn/)).toBeInTheDocument()
    expect(screen.getByText(/since 2026-01/)).toBeInTheDocument()
  })

  it('judges a yearly line against the year with pace and projection', () => {
    render(<SpendingLine
      line={line({
        name: 'Clothing', period: 'yearly', amount: 1200, monthly_share: 100, budgeted: 1200, spent: 900, remaining: 300,
        year: { year: 2026, budget: 1200, spent: 900, pace: 600, projected: 1800, elapsed: 0.5 },
      })}
      daysLeft={null} onView={noop} onEdit={noop} onDelete={noop} onApply={noop} applying={false}
    />)
    expect(screen.getByText(/projected/)).toHaveClass('text-red-600')
    expect(screen.getByText('yearly')).toBeInTheDocument()
  })

  it('offers to turn a lumpy monthly cap into a fund from January', async () => {
    const onApply = vi.fn()
    render(<SpendingLine
      line={line({ suggestion: { amount: 540, median: 110, p75: 295, mean: 532, max: 3827, months: 12, lumpy: true, basis: '' } })}
      daysLeft={null} onView={noop} onEdit={noop} onDelete={noop} onApply={onApply} applying={false}
    />)
    await userEvent.click(screen.getByRole('button', { name: /Make it a .*fund/ }))
    const [input] = onApply.mock.calls[0]
    expect(input).toMatchObject({ fund: true, amount: 540, amount_from: 'all', start_month: `${new Date().getFullYear()}-01` })
  })

  it('applies a steady suggestion from this month only', async () => {
    const onApply = vi.fn()
    render(<SpendingLine
      line={line({ name: 'Food', suggestion: { amount: 500, median: 416, p75: 499, mean: 428, max: 769, months: 12, lumpy: false, basis: 'p75' } })}
      daysLeft={null} onView={noop} onEdit={noop} onDelete={noop} onApply={onApply} applying={false}
    />)
    await userEvent.click(screen.getByRole('button', { name: /Apply from this month/ }))
    const [input] = onApply.mock.calls[0]
    expect(input).toMatchObject({ amount: 500, fund: false })
    expect(input.amount_from).toBeUndefined()
  })
})
