import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { logoDomain, logoKey } from '../components/MerchantLogo'
import { FI } from '../pages/insights/FI'
import { FeesCard } from '../pages/wealth/Fees'
import { SubscriptionsCard } from '../pages/plan/Subscriptions'
import { CashCalendarCard } from '../pages/plan/CashCalendar'

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
// Charts measure their box; jsdom has no ResizeObserver.
beforeEach(() => { vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} }) })
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('merchant logos', () => {
  it('match ledger spellings the way the server does, then by first word', () => {
    expect(logoKey('Varlė.lt')).toBe('varlelt')
    expect(logoKey("McDonald's")).toBe('mcdonalds')
    const index = { youtube: 'youtube.com', lidl: 'lidl.lt' }
    expect(logoDomain(index, 'YouTube Premium')).toBe('youtube.com')
    expect(logoDomain(index, 'Lidl')).toBe('lidl.lt')
    expect(logoDomain(index, 'Star Fish Resturant')).toBeUndefined()
  })
})

describe('independence', () => {
  it('shows the payout today, the levels with dates, and coast', async () => {
    serve({
      '/wealth/fi-plan': {
        annual_spend: 36000, spend_source: 'last 12 months', target: 900000, investable: 120000, progress: 0.13, monthly_invested: 1500, expected_return: 5, withdrawal_rate: 4,
        years_to_fi: 21.5, fi_date: '2048-04', emergency: { cash: 20000, monthly_essential: 1500, months: 13.3, target_months: 3, target: 4500 },
        levels: [{ key: 'obligations', title: 'Obligations covered', detail: '', monthly: 1500, capital: 450000, progress: 0.27, date: '2038-01', months: 135, age: 49, done: false }],
        passive_now: 400, obligations: 1500, essentials: 1500, spending: 3000,
        coast: { age: 55, needed: 380000, date: '2036-05', months: 115, done: false, detail: 'Keep adding until then.' },
        payouts: [{ year: 2026, age: 38, investable: 120000, monthly: 400 }], what_ifs: [{ extra: 100, date: '2047-01', sooner: 1.3 }],
        savings_rate: 0.25, locked: 18000, notes: [],
      },
      '/wealth/milestones': [{ key: 'fi', title: 'Financial independence', detail: '', date: '2048-04', months: 258, done: false }],
    })
    wrap(<FI />)
    expect(await screen.findByText('€400')).toBeInTheDocument()
    expect(screen.getByText('Obligations covered')).toBeInTheDocument()
    expect(screen.getByText(/January 2038 · age 49/)).toBeInTheDocument()
    expect(screen.getByText('May 2036')).toBeInTheDocument()
    expect(screen.getByText('1.3 years sooner')).toBeInTheDocument()
  })
})

describe('fees', () => {
  it('asks for a missing fund fee and saves it', async () => {
    const calls = serve({
      '/wealth/fees': { lines: [{ id: 'artea', name: 'Artea', kind: 'account', value: 8000, ter: 0, platform: 0, source: 'missing', yearly: 0, drag_20y: 0, saving_20y: 0 }],
        value: 8000, yearly: 0, weighted: 0, drag_20y: 0, saving_20y: 0, missing: 1, return: 7.5, low_cost: 0.2 },
      '/wealth/fees/*': { ok: true },
    })
    wrap(<FeesCard />)
    fireEvent.click(await screen.findByText('Set fee'))
    fireEvent.change(screen.getByPlaceholderText('0.22'), { target: { value: '1,2' } })
    fireEvent.click(screen.getByText('Save'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ body: { ter: 1.2, platform: 0 } }))
    expect(calls.find((c) => c.method === 'PUT')!.url).toContain('/wealth/fees/artea')
  })
})

describe('subscriptions', () => {
  it('flags a trial and marks one to cancel by a date', async () => {
    const calls = serve({
      '/plan/subscriptions': { items: [{ key: 'max|subscriptions.media', merchant: 'Max', category: 'subscriptions.media', cadence: 'monthly', amount: 9.99, yearly: 119.88,
        since: '2026-08-03', last: '2026-10-03', next: '2026-11-03', new: true, trial_from: '2026-08-03' }], monthly: 9.99, yearly: 119.88, to_cancel: 0 },
      '/logos': {}, '/categories': [],
    })
    wrap(<SubscriptionsCard />)
    expect(await screen.findByText(/began as a trial/)).toBeInTheDocument()
    fireEvent.click(screen.getByText('Cancel by…'))
    fireEvent.click(screen.getByText('Set'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ key: 'max|subscriptions.media', verdict: 'cancel' }))
  })
})

describe('cash calendar', () => {
  it('names the tight day and shows what lands on a picked day', async () => {
    serve({ '/plan/calendar': { start: 3000, floor: 2500, daily: 40, low: 1800, low_date: '2026-11-16', first_below: '2026-11-12',
      days: [{ date: '2026-11-02', balance: 2960, low: false, short: false }, { date: '2026-11-03', balance: 6100, low: false, short: false, events: [{ label: 'Salary (plan)', kind: 'income', amount: 3200 }] }] } })
    wrap(<CashCalendarCard />)
    expect(await screen.findByText(/emergency floor/)).toBeInTheDocument()
    expect(screen.getByText('Salary (plan)')).toBeInTheDocument() // the first day with payments is shown
  })
})
