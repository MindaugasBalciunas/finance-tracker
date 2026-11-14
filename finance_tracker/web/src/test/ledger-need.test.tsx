import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ToastProvider } from '../components/ui'
import Ledger from '../pages/Ledger'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

// The Ledger opened from the Free money card's Optional row: the request
// carries need=optional, and the page shows one total with a switch.
it('shows optional spending as one total, and switches to essential', async () => {
  const urls: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    urls.push(url)
    const path = url.replace(/^.*?api/, '').split('?')[0]
    const body = path === '/transactions'
      ? { items: [{ id: 1, date: '2026-11-06', kind: 'expense', amount: 45, category: 'food.restaurants', account_id: 'swed', merchant: 'Bistro', tags: [] }], total: 1, income: 0, expense: 45, transfer: 0 }
      : path === '/categories' || path === '/accounts' ? [] : {}
    return new Response(JSON.stringify(body), { status: 200 })
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ToastProvider>
        <MemoryRouter initialEntries={['/ledger?need=optional&from=2026-11-01&to=2026-11-30']}>
          <Routes><Route path="/ledger/*" element={<Ledger />} /></Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>)
  expect(await screen.findByText('Optional spending · 1 payment')).toBeInTheDocument()
  expect(screen.getByText('optional spending')).toBeInTheDocument() // the removable chip
  expect(urls.some((u) => u.includes('/transactions?') && u.includes('need=optional') && u.includes('from=2026-11-01'))).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: 'Essential' }))
  await waitFor(() => expect(urls.some((u) => u.includes('/transactions?') && u.includes('need=essential'))).toBe(true))
})
