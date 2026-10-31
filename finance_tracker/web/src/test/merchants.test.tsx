import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { MerchantsView, trendLabel } from '../pages/insights/Merchants'
import { SubscriptionsCard } from '../pages/plan/Subscriptions'
import { accountLogo, MerchantMark } from '../components/MerchantLogo'
import type { MerchantReport, MerchantStat, Subscriptions } from '../lib/types'

function serve(routes: Record<string, unknown>) {
  const calls: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    calls.push(url)
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const hit = Object.entries(routes).find(([k]) => path === k)
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

const months = (last: number) => Array.from({ length: 12 }, (_, i) => ({ month: `2026-${String(i + 1).padStart(2, '0')}`, amount: i === 9 ? last : 0 }))
const stat = (p: Partial<MerchantStat>): MerchantStat => ({ name: 'Maxima', category: 'food.groceries', total: 300, share: 0.6, count: 12, average: 25, previous: 250, change: 0.2,
  first: '2024-01-03', last: '2026-10-28', new: false, months: months(300), ...p })
const report: MerchantReport = {
  from: '2026-10-01', to: '2026-10-31', prev_from: '2026-08-31', prev_to: '2026-09-30', total: 500, unnamed: 42, count: 2, new_count: 1, new_total: 200,
  merchants: [stat({}), stat({ name: 'IKEA', category: 'home', total: 200, share: 0.4, count: 1, average: 200, previous: 0, change: 0, first: '2026-10-12', new: true })],
  lapsed: [{ name: 'Gym+', category: 'health', last: '2026-07-03', monthly: 15 }],
}

describe('merchant trend', () => {
  it('reads more spent as bad, less as good, small moves as neutral', () => {
    expect(trendLabel({ previous: 100, change: 0.25, new: false })).toEqual({ text: '+25%', tone: 'bad' })
    expect(trendLabel({ previous: 100, change: -0.3, new: false })).toEqual({ text: '-30%', tone: 'good' })
    expect(trendLabel({ previous: 100, change: 0.05, new: false })?.tone).toBe('muted')
  })
  it('says new for a first visit, nothing without an earlier window', () => {
    expect(trendLabel({ previous: 0, change: 0, new: true })?.text).toBe('new')
    expect(trendLabel({ previous: 0, change: 0, new: false })).toBeNull()
    expect(trendLabel({ previous: 100, change: 0.5, new: false }, false)).toBeNull()
  })
})

describe('merchants page', () => {
  it('ranks who was paid and opens a merchant with links into the ledger', async () => {
    const calls = serve({ '/insights/merchants': report, '/prefs': { liquid_only: false }, '/categories': [], '/logos': {} })
    wrap(<MerchantsView />)
    expect(await screen.findByText('Maxima')).toBeInTheDocument()
    expect(calls.some((u) => u.includes('/insights/merchants') && u.includes('preset=12m'))).toBe(true)
    expect(screen.getByText('+20%')).toHaveClass('text-bad')
    expect(screen.getByText('Gym+').closest('a')).toHaveAttribute('href', expect.stringContaining('merchant=Gym%2B'))
    fireEvent.click(screen.getByText('Maxima'))
    const sheet = screen.getByRole('button', { name: 'Close' }).closest('.fixed') as HTMLElement
    const tx = within(sheet).getByText('Transactions').closest('a')!
    expect(tx.getAttribute('href')).toContain('merchant=Maxima')
    expect(tx.getAttribute('href')).toContain('from=2026-10-01')
    expect(within(sheet).getByText('All time').closest('a')!.getAttribute('href')).toContain('period=all')
  })

  it('filters to new places', async () => {
    serve({ '/insights/merchants': report, '/prefs': { liquid_only: false }, '/categories': [], '/logos': {} })
    wrap(<MerchantsView />)
    await screen.findByText('Maxima')
    fireEvent.click(screen.getByText('New (1)'))
    expect(screen.queryByText('Maxima')).not.toBeInTheDocument()
    expect(screen.getByText('IKEA')).toBeInTheDocument()
  })
})

describe('subscription review', () => {
  it('opens a subscription’s transactions', async () => {
    const subs: Subscriptions = { monthly: 12, yearly: 144, to_cancel: 0, items: [
      { key: 'netflix', merchant: 'Netflix', category: 'leisure.streaming', cadence: 'monthly', amount: 12, yearly: 144, since: '2025-01-05', last: '2026-10-05', new: false },
      { key: 'gym', merchant: 'Gym+', category: 'health', cadence: 'monthly', amount: 30, yearly: 360, since: '2025-01-05', last: '2026-10-05', new: false, verdict: { verdict: 'keep', decided_on: '2026-09-01' } },
    ] }
    serve({ '/plan/subscriptions': subs, '/categories': [], '/logos': {} })
    wrap(<SubscriptionsCard />)
    expect((await screen.findByText('Netflix')).closest('a')).toHaveAttribute('href', expect.stringContaining('merchant=Netflix'))
    // Reviewed ones fold away; only what needs a verdict stays open.
    expect(screen.getByText('Reviewed (1)')).toBeInTheDocument()
  })
})

describe('account logos', () => {
  const index = { swedbank: 'swedbank.lt', seb: 'seb.lt', revolut: 'revolut.me' }
  it('use the bank, by institution or name', () => {
    expect(accountLogo(index, { institution: 'Swedbank', name: 'Main', kind: 'checking' })).toBe('swedbank.lt')
    expect(accountLogo(index, { institution: '', name: 'Revolut savings', kind: 'savings' })).toBe('revolut.me')
  })
  it('never give a house, car or cash a bank', () => {
    expect(accountLogo(index, { institution: 'SEB', name: 'House', kind: 'property' })).toBeUndefined()
    expect(accountLogo(index, { institution: 'Swedbank', name: 'Car', kind: 'vehicle' })).toBeUndefined()
    expect(accountLogo(index, { institution: '', name: 'Cash', kind: 'cash' })).toBeUndefined()
  })
})

describe('merchant marks', () => {
  it('show the logo when the merchant is known, else the given fallback', async () => {
    serve({ '/logos': { telia: 'telia.lt' } })
    const { container } = wrap(<><MerchantMark merchant="Telia" category="utilities" /><MerchantMark merchant="Mortgage principal" fallback={<span>arrows</span>} /></>)
    await screen.findByText('arrows')
    await waitFor(() => expect(container.querySelector('img')).toHaveAttribute('src', 'api/logos/telia.lt'))
  })
})
