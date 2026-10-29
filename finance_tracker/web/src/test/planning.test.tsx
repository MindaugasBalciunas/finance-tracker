import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { PaydayView } from '../pages/plan/Payday'
import { DecisionsView } from '../pages/plan/Decisions'
import { DriftView } from '../pages/insights/Drift'
import { Ownership } from '../pages/wealth/Ownership'

/** Answers API calls from a table of path → body; records every request. */
function serve(routes: Record<string, unknown>) {
  const calls: { method: string; url: string; body: any }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ method: init?.method ?? 'GET', url, body: init?.body ? JSON.parse(init.body as string) : null })
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const hit = Object.entries(routes).find(([k]) => path === k || (k.endsWith('*') && path.startsWith(k.slice(0, -1))))
    return new Response(JSON.stringify(hit ? hit[1] : {}), { status: hit ? 200 : 404 })
  }))
  return calls
}
const wrap = (ui: ReactNode) => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <ToastProvider><HashRouter>{ui}</HashRouter></ToastProvider>
  </QueryClientProvider>)

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('payday', () => {
  it('lists what is still to move, with where from and to, and ranks the next euros', async () => {
    serve({
      '/plan/payday': { month: '2026-10', payday: '2026-10-02', expected: '2026-11-03', salary: 3000, total: 600, done_total: 300, open: 1, steps: [
        { key: 's1', kind: 'saving', title: 'Invest', from_name: 'Swedbank', to_name: 'IBKR', amount: 500, done: 200, status: 'partial' },
        { key: 's2', kind: 'saving', title: 'III pillar', amount: 100, done: 100, status: 'done' },
      ] },
      '/plan/next-euro': { sum: 500, notes: [], options: [
        { key: 'buffer', title: 'Emergency buffer', why: 'Cash covers 2.5 of 3 months.', return: 0, sure: true, room: 300, amount: 300 },
        { key: 'invest', title: 'Invest', why: 'Expected about 7.5%.', return: 5.5, sure: false, amount: 200 },
      ] },
    })
    wrap(<PaydayView />)
    expect(await screen.findByText('Swedbank → IBKR')).toBeInTheDocument()
    expect(screen.getByText('€300 left')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument() // steps left
    expect(await screen.findByText('safety first')).toBeInTheDocument()
    expect(screen.getByText('5.5% a year · expected')).toBeInTheDocument()
  })
})

describe('decision journal', () => {
  it('shows a purchase’s impact as you type and writes it down', async () => {
    const calls = serve({
      '/plan/decisions': { items: [], record: { skipped: 0, skipped_sum: 0, bought: 0, yes: 0, meh: 0, no: 0, regretted_sum: 0, by_category: [] } },
      '/plan/decisions/impact': { amount: 600, cooling_days: 7, lines: [{ key: 'month', tone: 'warn', text: 'Takes 60% of the €1,000 left to spend this month.' }] },
      '/categories': [],
    })
    wrap(<DecisionsView />)
    fireEvent.change(await screen.findByPlaceholderText('Robot vacuum'), { target: { value: 'Desk' } })
    fireEvent.change(screen.getByPlaceholderText('€'), { target: { value: '600' } })
    expect(await screen.findByText(/Takes 60%/, {}, { timeout: 2000 })).toBeInTheDocument()
    expect(screen.getByText('Sleep on it: a week.')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Write it down'))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.includes('/plan/decisions'))).toBe(true))
    expect(calls.find((c) => c.method === 'POST')!.body).toMatchObject({ title: 'Desk', amount: 600 })
  })

  it('asks whether a purchase was worth it once its review is due', async () => {
    const calls = serve({
      '/plan/decisions': { items: [{ id: 7, title: 'Bike', amount: 900, category: '', status: 'bought', cool_until: '2026-01-01', decided_on: '2026-01-05', review_on: '2026-03-05', worth: '', note: '', created_at: 'x' }],
        record: { skipped: 0, skipped_sum: 0, bought: 1, yes: 0, meh: 0, no: 0, regretted_sum: 0, by_category: [] } },
      '/plan/decisions/*': { ok: true },
      '/categories': [],
    })
    wrap(<DecisionsView />)
    expect(await screen.findByText('Was it worth it?')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Yes'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ id: 7, worth: 'yes', status: 'bought' }))
  })
})

describe('drift', () => {
  it('sets your inflation beside the official rate and flags a renewal', async () => {
    serve({
      '/insights/creep': { years: 1, then: { from: '2024-10', to: '2025-09', income: 36000, spending: 24000, fixed: 12000 }, now: { from: '2025-10', to: '2026-09', income: 39600, spending: 31200, fixed: 13000 },
        income_growth: 10, spending_growth: 30, fixed_growth: 8.3, gap: 20, tone: 'bad', text: 'Spending grew 30% in 1 year, income 10%.', drivers: [], new: [], new_monthly: 0 },
      '/insights/prices': { rising: 1, extra: 60, personal: 10, basket: 2, weight: 600, official: { rate: 6.1, month: '2026-09', source: 'Eurostat HICP, Lithuania' },
        items: [{ merchant: 'Insurer', category: 'finance.insurance', cadence: 'yearly', now: 360, last_date: '2025-11-10', year_ago: 300, change: 20, yearly: 360, extra: 60, next: '2026-11-10', renews_soon: true, advice: 'Renews soon and cost 20% more last time.' }] },
      '/categories': [],
    })
    wrap(<DriftView />)
    expect(await screen.findByText('Spending grew 30% in 1 year, income 10%.')).toBeInTheDocument()
    expect((await screen.findAllByText('+10%')).length).toBe(2) // income growth, and yours
    expect(screen.getByText('+6.1%')).toBeInTheDocument()
    expect(screen.getByText('Your regular bills are rising faster than prices in general.')).toBeInTheDocument()
    expect(screen.getByText(/renews/)).toBeInTheDocument()
  })
})

describe('ownership', () => {
  it('shows the real monthly cost and saves the figures it is missing', async () => {
    const calls = serve({
      '/wealth/ownership': [{ account_id: 'car', name: 'Golf', kind: 'vehicle', settings: { tag: 'car', km_year: 0, alt_per_km: 0, rent: 0 }, value: 12000, equity: 12000,
        lines: [{ key: 'value', label: 'Lost value', amount: 3000 }], running: [], year: 6000, month: 500, missing: ['km_year', 'alt_per_km'] }],
      '/wealth/ownership/*': { ok: true },
    })
    wrap(<Ownership />)
    expect(await screen.findByText('€500')).toBeInTheDocument()
    fireEvent.click(screen.getByText(/km driven a year, what a km costs without it/))
    fireEvent.change(screen.getByPlaceholderText('14000'), { target: { value: '12000' } })
    fireEvent.change(screen.getByPlaceholderText('0.35'), { target: { value: '0,32' } })
    fireEvent.click(screen.getByText('Save'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ tag: 'car', km_year: 12000, alt_per_km: 0.32 }))
  })
})
