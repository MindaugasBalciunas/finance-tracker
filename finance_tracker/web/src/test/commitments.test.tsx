import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { CommitmentsLine } from '../components/CommitmentsLine'

const wrap = (ui: ReactNode) => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <HashRouter>{ui}</HashRouter>
  </QueryClientProvider>)

beforeEach(() => { vi.stubGlobal('fetch', vi.fn(async () => new Response('[]', { status: 200 }))) })
afterEach(() => { vi.unstubAllGlobals() })

// Alimony and the mortgage leave the "Biggest expenses" list and show as one
// line under it; each total opens only its own rows (the Ledger's
// fixed-commitments filter), so €400 paid to the ex-partner under Kids isn't
// a link to every kids expense.
describe('fixed commitments line', () => {
  it('shows the four largest, folds the rest and links each to its own rows', () => {
    const { container } = wrap(<CommitmentsLine from="2026-01-01" to="2026-12-31" items={[
      { category: 'housing.mortgage_interest', total: 8693.6, count: 11 },
      { category: 'kids.alimony', total: 7000, count: 7 },
      { category: 'kids.general', total: 400, count: 1 },
      { category: 'finance.fines', total: 90, count: 2 },
      { category: 'finance.taxes', total: 61, count: 4 },
      { category: 'finance.bank_fees', total: 31, count: 21 },
    ]} />)
    expect(screen.getByText('Fixed commitments, not listed:')).toBeInTheDocument()
    const links = container.querySelectorAll('a')
    expect(links).toHaveLength(4)
    expect(links[2].getAttribute('href')).toBe('#/ledger?category=kids.general&commitments=1&from=2026-01-01&to=2026-12-31')
    expect(container.textContent).toContain('other €92')
  })

  it('renders nothing when there are none', () => {
    const { container } = wrap(<CommitmentsLine from="2026-01-01" to="2026-12-31" items={[]} />)
    expect(container.textContent).toBe('')
  })
})
