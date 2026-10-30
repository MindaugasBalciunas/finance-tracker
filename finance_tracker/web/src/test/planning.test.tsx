import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { NextEuroCard } from '../pages/plan/NextEuro'
import { LineRow } from '../pages/Plan'
import type { PlanLine } from '../lib/types'
import { Wishlist } from '../pages/plan/Wishlist'
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

describe('budget lines carry the payday checklist', () => {
  const line = (p: Partial<PlanLine>): PlanLine => ({ id: 3, name: 'ETF', kind: 'saving', categories: ['transfer.invest'], tag: '', period: 'monthly', fund: false, start_month: '',
    archived: false, sort: 0, amount: 1000, steps: [], monthly_share: 1000, budgeted: 1000, spent: 1000, remaining: 0, month_spent: 1000, history: [], ...p } as PlanLine)

  it('shows where a saving line’s money went, ticked when done', () => {
    serve({ '/categories': [] })
    wrap(<LineRow l={line({})} month="2026-10" onEdit={() => {}} extra={{ route: 'Swedbank → IBKR', done: true }} />)
    expect(screen.getByText('Swedbank → IBKR')).toBeInTheDocument()
    expect(screen.getByText('Swedbank → IBKR').parentElement).toHaveClass('text-good')
  })

  it('shows when a fixed obligation is due', () => {
    serve({ '/categories': [] })
    wrap(<LineRow l={line({ name: 'Mortgage', kind: 'fixed', spent: 0, remaining: 1400 })} month="2026-10" onEdit={() => {}} extra={{ note: 'due 19 Oct' }} />)
    expect(screen.getByText('due 19 Oct')).toBeInTheDocument()
  })
})

describe('next euros (on the wish list)', () => {
  it('ranks safety first, then returns', async () => {
    serve({
      '/plan/next-euro': { sum: 500, notes: ['This year\'s 3rd-pillar refund room is used.'], options: [
        { key: 'buffer', title: 'Emergency buffer', why: 'Cash covers 2.5 of 3 months.', return: 0, sure: true, room: 300, amount: 300 },
        { key: 'invest', title: 'Invest', why: 'Expected about 7.5%.', return: 5.5, sure: false, amount: 200 },
      ] },
    })
    wrap(<NextEuroCard />)
    expect(await screen.findByText('safety first')).toBeInTheDocument()
    expect(screen.getByText('5.5% a year · expected')).toBeInTheDocument()
    expect(screen.getByText(/refund room is used/)).toBeInTheDocument()
  })
})

// The decision journal lives on the wish list: one page from "thinking of
// buying" to "was it worth it?".
const month = { month: '2026-09', income: 6000, invested: 1200, spending: 4000, left_over: 800, funded: 0, available: 800, complete: true }
const wish = (goals: any[] = []) => ({ goals, funding: month, proposal: [], history: [month], avg_left_over: 800, total_saved: 0, total_needed: 0, pots: [] })
const rec = { saving: 0, skipped: 0, skipped_sum: 0, bought: 0, yes: 0, meh: 0, no: 0, regretted_sum: 0, by_category: [] }
const dec = (p: any) => ({ category: '', cool_until: '2026-01-01', decided_on: '', review_on: '', worth: '', note: '', created_at: 'x', ...p })

describe('wish list with the decision journal', () => {
  it('writes a purchase down from the wish list, with its impact as you type', async () => {
    const calls = serve({
      '/goals': wish(), '/plan/decisions': { items: [], record: rec },
      '/plan/decisions/impact': { amount: 600, cooling_days: 7, lines: [{ key: 'month', tone: 'warn', text: 'Takes 60% of the €1,000 left to spend this month.' }] },
      '/categories': [], '/plan/next-euro': { sum: 500, options: [], notes: [] },
    })
    wrap(<Wishlist />)
    fireEvent.click(await screen.findByText('Thinking of buying…'))
    fireEvent.change(screen.getByPlaceholderText('Robot vacuum'), { target: { value: 'Desk' } })
    fireEvent.change(screen.getByPlaceholderText('€'), { target: { value: '600' } })
    expect(await screen.findByText(/Takes 60%/, {}, { timeout: 2000 })).toBeInTheDocument()
    fireEvent.click(screen.getByText('Write it down'))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST' && c.url.includes('/plan/decisions'))?.body).toMatchObject({ title: 'Desk', amount: 600 }))
  })

  it('lists what is slept on above the goals, Save for it makes a goal, and a goal from a decision shows once when done', async () => {
    const calls = serve({
      '/goals': wish([{ id: 9, name: 'Sofa', target: 2400, priority: 1, target_date: '', note: '', url: '', tag: '', status: 'bought', done_on: '2026-10-30', saved: 2400, remaining: 0 }]),
      '/plan/decisions': { items: [
        dec({ id: 4, title: 'Kayak', amount: 900, status: 'thinking' }),
        dec({ id: 5, title: 'Sofa', amount: 2400, status: 'bought', goal_id: 9, decided_on: '2026-10-30', review_on: '2026-01-01' }),
      ], record: { ...rec, bought: 1 } },
      '/plan/decisions/*': { id: 10, name: 'Kayak' },
      '/categories': [], '/plan/next-euro': { sum: 500, options: [], notes: [] },
    })
    wrap(<Wishlist />)
    expect(await screen.findByText('Sleeping on it')).toBeInTheDocument()
    expect(screen.getAllByText('Sofa')).toHaveLength(1) // the goal and its decision: one row
    expect(screen.getByText('Was it worth it?')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Save for it'))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.includes('/plan/decisions/4/save-for'))).toBe(true))
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
