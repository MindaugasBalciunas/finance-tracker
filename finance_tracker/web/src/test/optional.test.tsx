import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import { OptionalView } from '../pages/insights/Optional'
import { ToastProvider } from '../components/ui'
import type { OptionalReport } from '../lib/types'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })
// Charts measure their box; jsdom has no ResizeObserver.
const stubResize = () => vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })

const months = Array.from({ length: 13 }, (_, i) => ({ month: `2026-${String(i + 1).padStart(2, '0')}`.replace('2026-13', '2026-12'), essential: 500, optional: 200 }))
const report: OptionalReport = {
  month: '2026-11', spent: 250, usual: 200, free: 2488, year: 2400, months,
  categories: [{ key: 'food.restaurants', month: 50, usual: 100, year: 1200, count: 24, months: 12 }],
  merchants: [{ key: 'Wolt', month: 60, usual: 40, year: 480, count: 12, months: 9 }],
  leaks: {
    while_over: { year: 160, this_month: 25, months: [{ month: '2026-05', from: 20, amount: 10 }, { month: '2026-06', from: 20, amount: 10 }, { month: '2026-07', from: 20, amount: 10 }, { month: '2026-08', from: 20, amount: 10 }, { month: '2026-09', from: 20, amount: 20 }, { month: '2026-10', from: 11, amount: 100 }, { month: '2026-11', from: 9, amount: 25 }],
      items: [{ id: 7, date: '2026-10-20', merchant: '', note: 'Reunion dinner', category: 'food.restaurants', amount: 40, over: true }] },
    small_often: [{ merchant: 'Caffeine', count: 8, spent: 32, yearly: 129.77 }],
    payday: { after: 60, usual: 22.8, extra: 446.4, months: 6 },
    creeping: [{ merchant: 'Wolt', before: 10, now: 60, yearly: 600 }],
    convenience: { convenience: 60, groceries: 350, yearly: 720 },
    one_offs: { year: 1200, count: 2, this_month: 0, items: [{ id: 9, date: '2026-03-15', merchant: 'Apple', category: 'shopping.electronics', amount: 900 }, { id: 10, date: '2026-10-20', merchant: 'Sofa Shop', category: 'housing.furnishing', amount: 300, over: true }] },
  },
  judge: {
    worth: 360, cut: 600, unsure: 0, open: 632,
    items: [
      { kind: 'merchant', key: 'CAFFEINE', name: 'Caffeine', year: 32, usual: 2.67, month: 0, count: 8, months: 1, suggest: 'cut', why: 'small and often' },
      { kind: 'merchant', key: 'BISTRO', name: 'Bistro', year: 600, usual: 50, month: 40, count: 6, months: 6, verdict: 'cut' },
    ],
    cutting: [{ kind: 'merchant', key: 'BISTRO', name: 'Bistro', usual: 100, this_month: 40, last_month: 60, judged_on: '2026-09-02' }],
  },
}
const calls: { method: string; url: string; body: unknown }[] = []
const show = (r: OptionalReport) => {
  stubResize()
  calls.length = 0
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ method: init?.method ?? 'GET', url, body: init?.body ? JSON.parse(init.body as string) : null })
    const path = url.replace(/^.*?api/, '').split('?')[0]
    return new Response(JSON.stringify(path === '/insights/optional' ? r : path === '/categories' ? [] : { ok: true }), { status: 200 })
  }))
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><ToastProvider><HashRouter><OptionalView /></HashRouter></ToastProvider></QueryClientProvider>)
}

describe('Optional spending insights', () => {
  it('shows the month, where it goes and each leak with a way to its payments', async () => {
    show(report)
    expect(await screen.findByText('€250')).toBeInTheDocument()
    expect(screen.getByText(/10% of this month's free money/)).toBeInTheDocument()
    expect(screen.getByText('25% over so far')).toHaveClass('text-warn')
    for (const t of ['Spent after the money ran out', 'Small and often', 'Payday splurge', 'Creeping up', 'Convenience food', 'Big one-offs']) {
      expect(screen.getByText(t)).toBeInTheDocument()
    }
    expect(screen.getByText('≈ €446/yr')).toBeInTheDocument()
    expect(screen.getAllByText('Caffeine').map((e) => e.closest('a')).find(Boolean)!.getAttribute('href')).toMatch(/^#\/ledger\?merchant=Caffeine&need=optional&max=14\.99&from=/)
    expect(screen.getByText('October 2026').closest('a')).toHaveAttribute('href', '#/ledger?need=optional&from=2026-10-11&to=2026-10-31')
    expect(screen.getByText('€10 → €60 a month')).toBeInTheDocument()
  })

  it('lets you judge a place, shows what the data suggests, and follows up a cut', async () => {
    show(report)
    expect(await screen.findByText('Worth it or a leak?')).toBeInTheDocument()
    expect(screen.getByText('Looks like a leak — small and often')).toHaveClass('text-warn')
    // Bistro is already a cut: its Cut button is on; tapping it clears the verdict.
    const bistro = screen.getByRole('group', { name: 'Your verdict on Bistro' })
    const cut = [...bistro.querySelectorAll('button')].find((b) => b.textContent === 'Cut')!
    expect(cut).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(cut)
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.includes('insights/optional/verdicts') && c.url.includes('key=BISTRO'))).toBe(true))
    const caffeine = screen.getByRole('group', { name: 'Your verdict on Caffeine' })
    fireEvent.click([...caffeine.querySelectorAll('button')].find((b) => b.textContent === 'Worth it')!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ kind: 'merchant', key: 'CAFFEINE', verdict: 'worth' }))
    // Cutting back: Bistro was €100 a month, €60 last month.
    expect(screen.getByText('Cutting back')).toBeInTheDocument()
    expect(screen.getByText('(−40%)')).toHaveClass('text-good')
  })

  // Every figure says what it covers and agrees with its neighbours: a
  // monthly pace only for what is paid most months, the leak cards' totals
  // are the twelve complete months with this month apart, their lists add
  // up to them, and a payment in two cards says so.
  it('keeps the figures consistent and traceable', async () => {
    show(report)
    expect(await screen.findByText('€2,400')).toBeInTheDocument()
    expect(screen.getByText('€200 a month')).toBeInTheDocument()
    expect(screen.getByText(/Last 12 months is Jan .* – Dec .*, this month left out/)).toBeInTheDocument()
    // Bistro: monthly-ish, so a pace; Caffeine: eight payments in one month, so none.
    expect(screen.getByText('€600 in 12 months · 6 payments · €50 a month on average · €40 this month')).toBeInTheDocument()
    expect(screen.getByText('€32 in 12 months · 8 payments in 1 month')).toBeInTheDocument()
    // After the money ran out: €160 is the six complete months; November stands apart.
    expect(screen.getByText('November 2026 so far')).toBeInTheDocument()
    expect(screen.queryByText('May 2026')).toBeNull()
    fireEvent.click(screen.getByText('Show all 6 months'))
    expect(screen.getByText('May 2026')).toBeInTheDocument()
    // A payment without a merchant goes by its note and opens itself in the Ledger.
    expect(screen.getByText('Reunion dinner').closest('a')).toHaveAttribute('href', '#/ledger?need=optional&min=40&max=40&from=2026-10-20&to=2026-10-20')
    // Big one-offs: both listed, the one that is also in the other card marked.
    expect(screen.getByText(/^2 optional payments of €250 or more in the last 12 months/)).toBeInTheDocument()
    expect(screen.getAllByText('· after the money ran out').length).toBe(1)
    expect(screen.getByText(/don't add them up/)).toBeInTheDocument()
  })

  // A new book: only this month ran out or had a big one — the cards show
  // this month's figure, never "€0 last 12 months" above a list.
  it('headlines this month when the twelve months had nothing', async () => {
    const l = report.leaks
    show({ ...report, leaks: { ...l, while_over: { ...l.while_over, year: 0, months: [{ month: '2026-11', from: 9, amount: 25 }] }, one_offs: { year: 0, count: 0, this_month: 300, items: [{ id: 11, date: '2026-11-05', merchant: 'Sofa Shop', category: 'housing.furnishing', amount: 300 }] } } })
    expect(await screen.findByText('November 2026 so far')).toBeInTheDocument()
    expect(screen.getAllByText('so far this month')).toHaveLength(2)
    expect(screen.queryByText(/of the last 12 months\./)).toBeNull()
    expect(screen.getByText(/^Optional payments of €250 or more that no fund/)).toBeInTheDocument()
  })

  it('says so when nothing looks like a leak', async () => {
    show({ ...report, leaks: { while_over: { year: 0, this_month: 0, months: [], items: [] }, small_often: [], creeping: [], one_offs: { year: 0, count: 0, this_month: 0, items: [] } } })
    expect(await screen.findByText('No leak signals')).toBeInTheDocument()
    expect(screen.queryByText('Payday splurge')).toBeNull()
  })
})
