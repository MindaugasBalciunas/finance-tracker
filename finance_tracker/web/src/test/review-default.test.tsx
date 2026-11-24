import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { ToastProvider } from '../components/ui'
import { ReviewTabs } from '../pages/insights/MonthReview'
import { thisMonth } from '../lib/format'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

function asked(route: string) {
  const months: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url.includes('/insights/month')) months.push(new URL(url, 'http://x').searchParams.get('month') ?? '')
    return new Response(JSON.stringify(url.includes('/categories') || url.includes('/accounts') ? [] : {}), { status: 200 })
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider><MemoryRouter initialEntries={[route]}><ReviewTabs /></MemoryRouter></ToastProvider>
    </QueryClientProvider>)
  return months
}

// Review opens on the month in progress; a link naming a month still opens it.
it('opens on the current month', async () => {
  const months = asked('/insights/review')
  await waitFor(() => expect(months[0]).toBe(thisMonth()))
})

it('opens the month a link names', async () => {
  const months = asked('/insights/review?month=2026-10')
  await waitFor(() => expect(months[0]).toBe('2026-10'))
})
