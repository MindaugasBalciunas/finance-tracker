import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { StressView } from '../pages/wealth/Stress'
import { cleanShock, lasts, shockEmpty } from '../lib/stress'
import type { StressReport, StressResult } from '../lib/types'

const result = (p: Partial<StressResult>): StressResult => ({
  id: 'job_loss', name: 'Job loss', blurb: 'No pay for 6 months.', shock: { months: 6, income_pct: -100 },
  months: [{ month: '2026-11', income: 0, out: 2500, end: 1000, base: 4000 }, { month: '2026-12', income: 0, out: 2500, sold: 1500, end: 0, base: 4500 }],
  level: 'sell', lowest: 0, lowest_on: '2026-12', cash_out: '2026-12', sold: [{ key: 'investments', label: 'Investments', amount: 1500, net: 1500 }], sold_total: 1500, short: 0,
  market_hit: 0, loan_extra: 0, monthly_gap: -2500, net_worth_end: 100000, base_net_worth_end: 118000, cost: 18000,
  summary: ['Cash runs out in Dec 2026; selling €1,500 of reserves gets you through.'], levers: [{ label: 'Cut everyday spending by a fifth', amount: 400, monthly: true }], ...p,
})
const report = (p: Partial<StressReport> = {}): StressReport => ({
  start: '2026-10', horizon: 24, cash: 5000, floor: 3750, net_worth: 130000,
  reserves: [{ key: 'investments', label: 'Investments', value: 10000, net: 10000 }, { key: 'pillar3', label: 'III pillar', value: 4000, net: 3400 }],
  locked: 20000, real_assets: 100000, runway: { burn: 2500, cash_months: 2, reserve_months: 7.4 },
  scenarios: [result({}), result({ id: 'market_crash', name: 'Market crash', level: 'ok', lowest: 3532, cost: 10300, sold: [], levers: [], summary: ['Cash stays above your €3,750 emergency target throughout.'] })],
  notes: ['Starts from the cash forecast.'], ...p,
})

function serve(get: StressReport, post?: StressReport) {
  const calls: { method: string; body: any }[] = []
  vi.stubGlobal('fetch', vi.fn(async (_url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    calls.push({ method, body: init?.body ? JSON.parse(init.body as string) : null })
    return new Response(JSON.stringify(method === 'POST' ? post : get), { status: 200 })
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

describe('stress test helpers', () => {
  it('treats a shock with only a length as no shock', () => {
    expect(shockEmpty({ months: 6 })).toBe(true)
    expect(shockEmpty({ months: 6, one_off: 1000 })).toBe(false)
    expect(cleanShock({ months: 0, income_pct: -100, pause_goals: false })).toEqual({ income_pct: -100 })
    expect(lasts({ months: 1 }, 24)).toBe('1 month')
    expect(lasts({}, 24)).toBe('all 24 months')
  })
})

describe('stress test page', () => {
  it('leads with the no-income runway and opens on a lost job', async () => {
    serve(report())
    wrap(<StressView />)
    expect(await screen.findByText('7.4 months')).toBeInTheDocument()
    // The III pillar counts after tax; the II pillar and the house don't count.
    expect(screen.getByText(/III pillar \(after tax\)/)).toBeInTheDocument()
    expect(screen.getByText(/in the II pillar \(locked until retirement\)/)).toBeInTheDocument()
    expect(screen.getByText('Cash runs out in Dec 2026; selling €1,500 of reserves gets you through.').closest('li')).toHaveClass('text-warn')
    expect(screen.getByText("What you'd sell")).toBeInTheDocument()
    // Tapping another crisis shows it.
    fireEvent.click(screen.getAllByRole('button', { name: /Market crash/ }).find((b) => b.hasAttribute('aria-pressed'))!)
    expect(screen.getByText('Cash stays above your €3,750 emergency target throughout.')).toBeInTheDocument()
    expect(screen.queryByText("What you'd sell")).not.toBeInTheDocument()
  })

  it('says so when there is no usual month to go by', async () => {
    serve(report({ runway: undefined }))
    wrap(<StressView />)
    expect(await screen.findByText(/Not enough history yet/)).toBeInTheDocument()
    expect(screen.queryByText(/0\.0 months/)).not.toBeInTheDocument()
  })

  it('posts a crisis of your own and shows it beside the others', async () => {
    const mine = result({ id: 'custom', name: 'Your crisis', blurb: '', level: 'short', short: 2200, broke: '2027-04', summary: ['Cash runs out in Dec 2026 and the reserves are gone by Apr 2027 — €2,200 short at worst.'] })
    const calls = serve(report(), report({ custom: mine }))
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    fireEvent.change(screen.getByLabelText('Unexpected bill, €'), { target: { value: '9000' } })
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true), { timeout: 2000 })
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ months: 6, one_off: 9000 })
    expect(await screen.findByText('€2,200 short')).toHaveClass('text-bad')
    expect(screen.getByText(/€2,200 short at worst/)).toBeInTheDocument()
  })
})
