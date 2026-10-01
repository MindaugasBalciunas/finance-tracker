import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import AISpendCard from './AISpendCard'
import { aiApi, type SpendReport } from '../../api/insights'

vi.mock('../../api/insights', () => ({
  aiApi: { spend: vi.fn(), addTopUp: vi.fn(), deleteTopUp: vi.fn() },
}))

const api = vi.mocked(aiApi)

function report(over: Partial<SpendReport> = {}): SpendReport {
  return {
    this_month: 7.57,
    last_30_days: 7.57,
    all_time: 31.08,
    remaining: 12.43,
    topped_up: 20,
    since_date: '2026-10-01',
    by_kind: { chat: 5.12, view_summary: 1.98, forecast: 0.47 },
    top_ups: [{ id: 1, amount_usd: 20, note: 'Console', occurred_on: '2026-10-01' }],
    ...over,
  }
}

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <AISpendCard />
    </QueryClientProvider>
  )
}

describe('AISpendCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.spend.mockResolvedValue(report())
  })

  it('shows the remaining balance against what was added', async () => {
    renderCard()
    expect(await screen.findByText(/≈ \$12.43/)).toBeInTheDocument()
    expect(screen.getByText(/left of \$20 added since 2026-10-01/)).toBeInTheDocument()
  })

  // "$0 remaining" would read as "you are out of credit" — a different and
  // alarming claim. With no top-up recorded there is simply nothing to count
  // down from, and the card has to say that instead.
  it('never shows a zero balance when no top-up has been recorded', async () => {
    api.spend.mockResolvedValue(report({ remaining: null, topped_up: 0, top_ups: [], since_date: undefined }))
    renderCard()
    expect(await screen.findByText(/count down from it/)).toBeInTheDocument()
    expect(screen.queryByText(/≈ \$0/)).not.toBeInTheDocument()
    expect(screen.queryByText(/left of/)).not.toBeInTheDocument()
  })

  it('records a top-up', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: /Record a top-up/ }))
    await user.type(screen.getByLabelText(/Amount/), '25')
    await user.click(screen.getByRole('button', { name: 'Save top-up' }))

    await waitFor(() => expect(api.addTopUp).toHaveBeenCalledTimes(1))
    expect(api.addTopUp.mock.calls[0][0].amount_usd).toBe(25)
  })

  // The number is the app's own bookkeeping, not the provider's. Saying so
  // where the number is, is the whole reason it can be shown at all.
  it('states the figure is the app own bookkeeping, not the provider balance', async () => {
    renderCard()
    expect(await screen.findByText(/doesn't publish a balance/)).toBeInTheDocument()
    expect(screen.getByText(/not counted here/)).toBeInTheDocument()
  })
})
