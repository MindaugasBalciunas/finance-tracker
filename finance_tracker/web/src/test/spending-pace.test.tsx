import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { IncomeUsedCard, OutPaceCard } from '../pages/insights/Spending'
import { HashRouter } from 'react-router-dom'
import type { OutPacePoint, PaceOut } from '../lib/types'

afterEach(() => { vi.unstubAllGlobals() })

const points: OutPacePoint[] = Array.from({ length: 31 }, (_, i) => ({
  day: i + 1, last_month: 100 * (i + 1), last_month_invested: 40 * (i + 1), typical: 90 * (i + 1), typical_invested: 30 * (i + 1),
  ...(i < 10 ? { current: 1200 + i, current_invested: 800 } : {}),
}))

const res: PaceOut = { points, month: { month: '2026-11', income: 3000, expected_salary: 200, spent: 409, invested: 800, payroll_pension: 0, left: 1991,
  into: [{ account_id: 'ibkr', name: 'IBKR', kind: 'brokerage', amount: 1000 }, { account_id: 'mortgage', name: 'Mortgage', kind: 'loan', amount: 100 }, { account_id: 'rev', name: 'Revolut ETF', kind: 'brokerage', amount: -300 }] } }

describe('Spending → this month so far, with investing', () => {
  it('splits today’s total into spent and invested', async () => {
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(res), { status: 200 })))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><HashRouter><OutPaceCard /></HashRouter></QueryClientProvider>)
    expect(await screen.findByText('This month so far, with investing')).toBeInTheDocument()
    // Day 10: €1,209 out, of which €800 invested.
    expect(screen.getAllByText('€409').length).toBeGreaterThan(0)
    expect(screen.getAllByText('€800').length).toBeGreaterThan(0)
    expect(vi.mocked(fetch).mock.calls[0][0]).toMatch(/insights\/pace-out$/)
  })

  it('lists where the month’s income went, by account, with what is left', async () => {
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(res), { status: 200 })))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><HashRouter><IncomeUsedCard /></HashRouter></QueryClientProvider>)
    expect(await screen.findByText(/income went/)).toBeInTheDocument()
    expect(screen.getByText('€3,200')).toBeInTheDocument()
    expect(screen.getByText('incl. €200 still to come')).toBeInTheDocument()
    expect(screen.getByText('IBKR').closest('a')).toHaveAttribute('href', '#/ledger?account=ibkr&from=2026-11-01&to=2026-11-30')
    expect(screen.getByText('· principal')).toBeInTheDocument()
    expect(screen.getByText('-€300').className).toMatch(/text-bad/)
    expect(screen.getByText('Left, once the pay is in')).toBeInTheDocument()
    expect(screen.getByText('€1,991')).toBeInTheDocument()
  })
})
