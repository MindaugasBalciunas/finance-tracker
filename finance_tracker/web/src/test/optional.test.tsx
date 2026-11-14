import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import { OptionalView } from '../pages/insights/Optional'
import type { OptionalReport } from '../lib/types'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })
// Charts measure their box; jsdom has no ResizeObserver.
const stubResize = () => vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })

const months = Array.from({ length: 13 }, (_, i) => ({ month: `2026-${String(i + 1).padStart(2, '0')}`.replace('2026-13', '2026-12'), essential: 500, optional: 200 }))
const report: OptionalReport = {
  month: '2026-11', spent: 250, usual: 200, free: 2488, year: 2400, months,
  categories: [{ key: 'food.restaurants', month: 50, usual: 100, year: 1200, count: 24 }],
  merchants: [{ key: 'Wolt', month: 60, usual: 40, year: 480, count: 12 }],
  leaks: {
    while_over: { year: 160, this_month: 0, months: [{ month: '2026-10', from: 11, amount: 100 }], items: [{ id: 7, date: '2026-10-20', merchant: 'Concert', category: 'leisure.events', amount: 40 }] },
    small_often: [{ merchant: 'Caffeine', count: 8, spent: 32, yearly: 129.77 }],
    payday: { after: 60, usual: 22.8, extra: 446.4, months: 6 },
    creeping: [{ merchant: 'Wolt', before: 10, now: 60, yearly: 600 }],
    convenience: { convenience: 60, groceries: 350, yearly: 720 },
    one_offs: { year: 900, items: [{ id: 9, date: '2026-03-15', merchant: 'Apple', category: 'shopping.electronics', amount: 900 }] },
  },
}
const show = (r: OptionalReport) => {
  stubResize()
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    const path = url.replace(/^.*?api/, '').split('?')[0]
    return new Response(JSON.stringify(path === '/insights/optional' ? r : path === '/categories' ? [] : {}), { status: 200 })
  }))
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><HashRouter><OptionalView /></HashRouter></QueryClientProvider>)
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
    expect(screen.getByText('Caffeine').closest('a')!.getAttribute('href')).toMatch(/^#\/ledger\?merchant=Caffeine&need=optional&max=14\.99&from=/)
    expect(screen.getByText('October 2026').closest('a')).toHaveAttribute('href', '#/ledger?need=optional&from=2026-10-11&to=2026-10-31')
    expect(screen.getByText('€10 → €60 a month')).toBeInTheDocument()
  })

  it('says so when nothing looks like a leak', async () => {
    show({ ...report, leaks: { while_over: { year: 0, this_month: 0, months: [], items: [] }, small_often: [], creeping: [], one_offs: { year: 0, items: [] } } })
    expect(await screen.findByText('No leak signals')).toBeInTheDocument()
    expect(screen.queryByText('Payday splurge')).toBeNull()
  })
})
