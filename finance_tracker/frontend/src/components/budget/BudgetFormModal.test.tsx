import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BudgetFormModal from './BudgetFormModal'
import type { Budget } from '../../types'

vi.mock('../../api/budgets', () => ({ budgetsApi: { labels: vi.fn().mockResolvedValue([]) } }))

const health: Budget = {
  id: 9, name: 'Health', kind: 'spending', label: '', category: 'Health', amount: 50,
  period: 'monthly', fund: false, start_month: '', created_at: '', updated_at: '',
}

describe('BudgetFormModal', () => {
  it('turning a line into a fund applies the new amount to every month since the start', async () => {
    const onSave = vi.fn()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <BudgetFormModal budget={health} prefill={null} error={null} onSave={onSave} onClose={() => {}} />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('checkbox'))
    const amount = screen.getByLabelText(/Set aside per month/)
    await userEvent.clear(amount)
    await userEvent.type(amount, '470')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSave.mock.calls[0][0]).toMatchObject({ fund: true, amount: 470, amount_from: 'all' })
  })
})
