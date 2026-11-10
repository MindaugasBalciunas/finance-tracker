import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ToastProvider } from '../components/ui'
import { StressView } from '../pages/wealth/Stress'
import { cleanShock, lasts, shockEmpty } from '../lib/stress'
import type { StressReport, StressResult } from '../lib/types'

const result = (p: Partial<StressResult>): StressResult => ({
  id: 'job_loss', group: 'income', name: 'Job loss', blurb: 'No pay for 6 months.', shock: { months: 6, income_pct: -100 },
  months: [{ month: '2026-11', income: 0, out: 2500, end: 1000, base: 4000 }, { month: '2026-12', income: 0, out: 2500, sold: 1500, end: 0, base: 4500 }],
  level: 'sell', lowest: 0, lowest_on: '2026-12', cash_out: '2026-12', sold: [{ key: 'investments', label: 'Investments', amount: 1500, net: 1500 }], sold_total: 1500, short: 0,
  market_hit: 0, loan_extra: 0, monthly_gap: -2500, net_worth_end: 100000, base_net_worth_end: 118000, cost: 18000, paper_loss: 0, locked_in: 0, saving_cut: 0,
  summary: ['Cash runs out in Dec 2026; selling €1,500 of reserves gets you through.'], levers: [{ label: 'Cut everyday spending by a fifth', amount: 400, monthly: true }], ...p,
})
const report = (p: Partial<StressReport> = {}): StressReport => ({
  start: '2026-10', horizon: 24, cash: 5000, floor: 3750, net_worth: 130000,
  reserves: [{ key: 'investments', label: 'Investments', value: 10000, net: 10000 }, { key: 'pillar3', label: 'III pillar', value: 4000, net: 3400 }],
  locked: 20000, real_assets: 100000, runway: { burn: 2500, cash_months: 2, reserve_months: 7.4 }, floor_months: 1.5, target_months: 3,
  groups: [{ id: 'income', name: 'Losing income' }, { id: 'markets', name: 'Markets falling' }, { id: 'costs', name: 'Costs rising' }],
  scenarios: [result({}), result({ id: 'rates', group: 'costs', name: 'Rate rise', level: 'pause', saving_cut: 2400, sold_total: 0, sold: [], cost: 6000, summary: ['Cash would run short in Mar 2027; investing €2,400 less covers it — nothing has to be sold.'] }), result({ id: 'market_crash', group: 'markets', name: 'Market crash', level: 'ok', lowest: 3532, cost: 0, paper_loss: 10300, market_hit: 10300, sold: [], levers: [], summary: ['Cash stays above your €3,750 emergency fund throughout.', 'Nothing is lost for good: the €10,300 drop is on paper and comes back if prices recover.'] }), result({ id: 'rates_all', group: 'costs', name: 'Rates for good', shock: { rate_pp: 3 }, level: 'sell' })],
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
  it('leads with how many crises you get through and how long the money lasts', async () => {
    serve(report())
    wrap(<StressView />)
    expect(await screen.findByText('7.4 months')).toBeInTheDocument()
    // Four crises, none runs out, two need selling.
    expect(screen.getByRole('heading', { name: /get through all 4 crises — 2 of them by selling/ })).toBeInTheDocument()
    expect(screen.getByText(/more by selling what you hold/)).toBeInTheDocument()
    // The fine print is folded away but there.
    expect(screen.getByText(/€3,400/).parentElement!.textContent).toMatch(/III pillar €3,400 \(after tax\)/)
    expect(screen.getByText(/in the II pillar \(locked until retirement\)/)).toBeInTheDocument()
  })

  it('shows one line per crisis and opens it in place', async () => {
    serve(report())
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    const income = within(screen.getByRole('region', { name: 'Losing income' }))
    expect(income.getByText('Sell €1,500 of investments')).toHaveClass('text-warn')
    expect(income.getByText('Must sell')).toBeInTheDocument()
    expect(screen.queryByText("You'd sell")).not.toBeInTheDocument()
    const row = income.getByRole('button', { name: /Job loss/ })
    fireEvent.click(row)
    expect(row).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Cash runs out in Dec 2026; selling €1,500 of reserves gets you through.')).toHaveClass('text-warn')
    expect(screen.getByText("You'd sell")).toBeInTheDocument()
    expect(screen.getByText('€18,000')).toHaveClass('text-bad')
    // Another crisis closes the first.
    fireEvent.click(within(screen.getByRole('region', { name: 'Markets falling' })).getByRole('button', { name: /Market crash/ }))
    expect(screen.queryByText("You'd sell")).not.toBeInTheDocument()
    expect(screen.getByText('Cash stays above your €3,750 emergency fund throughout.')).toBeInTheDocument()
    expect(screen.getByText('€10,300')).toBeInTheDocument()
    // The cost sentence is shown as the two figures, not repeated.
    expect(screen.queryByText(/Nothing is lost for good/)).not.toBeInTheDocument()
    // A kind with no crises is left out.
    expect(screen.queryByRole('region', { name: 'Several at once' })).not.toBeInTheDocument()
  })

  it('says when investing less is enough', async () => {
    serve(report())
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    const costs = within(screen.getByRole('region', { name: 'Costs rising' }))
    expect(costs.getByText('Invest €2,400 less, sell nothing')).toHaveClass('text-warn')
    expect(costs.getByText('Invest less')).toBeInTheDocument()
  })

  it('warns when the emergency fund covers fewer real months than meant', async () => {
    serve(report())
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    // €3,750 is 3 months of essentials but 1.5 of a €2,500 month: 3 real months take €7,500.
    expect(screen.getByText('1.5 months')).toBeInTheDocument()
    expect(screen.getByText('€7,500')).toBeInTheDocument()
  })

  it('says so when there is no usual month to go by', async () => {
    serve(report({ runway: undefined }))
    wrap(<StressView />)
    expect(await screen.findByText(/Not enough history yet/)).toBeInTheDocument()
    expect(screen.queryByText(/0\.0 months/)).not.toBeInTheDocument()
  })

  it('runs a crisis of your own, starting from a ready-made one', async () => {
    const mine = result({ id: 'custom', name: 'Your crisis', blurb: '', level: 'short', short: 2200, broke: '2027-04', cost: 0, summary: ['Cash runs out in Dec 2026 and the reserves are gone by Apr 2027 — €2,200 short at worst.'] })
    const calls = serve(report(), report({ custom: mine }))
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    // Nothing is asked of the server until you start.
    expect(screen.queryByLabelText('Unexpected bill, €')).not.toBeInTheDocument()
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: /Job loss/ }))
    fireEvent.click(screen.getByRole('button', { name: /Change the numbers/ }))
    expect(screen.getByLabelText('Pay change, %')).toHaveValue('-100')
    fireEvent.change(screen.getByLabelText('Unexpected bill, €'), { target: { value: '9000' } })
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.body.one_off === 9000)).toBe(true), { timeout: 2000 })
    expect(calls.filter((c) => c.method === 'POST').pop()!.body).toEqual({ months: 6, income_pct: -100, one_off: 9000 })
    expect(await screen.findByText(/€2,200 short at worst/)).toBeInTheDocument()
    expect(screen.getByText('Runs out')).toBeInTheDocument()
  })

  it('carries a crisis that lasts all along into your own as 0 months', async () => {
    const calls = serve(report(), report({ custom: result({ id: 'custom', name: 'Your crisis' }) }))
    wrap(<StressView />)
    await screen.findByText('7.4 months')
    fireEvent.click(screen.getByRole('button', { name: /Rates for good/ }))
    fireEvent.click(screen.getByRole('button', { name: /Change the numbers/ }))
    expect(screen.getByLabelText('Lasts, months')).toHaveValue('0')
    expect(screen.getByLabelText('Euribor, points')).toHaveValue('3')
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true), { timeout: 2000 })
    expect(calls.find((c) => c.method === 'POST')!.body).toEqual({ rate_pp: 3 })
  })
})
