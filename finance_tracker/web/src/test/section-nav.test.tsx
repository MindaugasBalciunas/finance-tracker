import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import Insights from '../pages/Insights'
import Wealth from '../pages/Wealth'
import Plan from '../pages/Plan'

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>
}

// Old links (notifications, AI answers, bookmarks) must open their view under
// the tab it moved to, with the sub-view toggle showing where you are.
function open(path: string) {
  // The views stay loading: this is about which tab and toggle are showing.
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => {})))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/insights/*" element={<><Insights /><Where /></>} />
          <Route path="/wealth/*" element={<><Wealth /><Where /></>} />
          <Route path="/plan/*" element={<><Plan /><Where /></>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}
const current = () => screen.getAllByRole('button').filter((b) => b.getAttribute('aria-current') === 'page').map((b) => b.textContent)
const view = () => screen.getAllByRole('button').filter((b) => b.className.includes('bg-raised')).map((b) => b.textContent)

describe('section navigation', () => {
  afterEach(() => vi.restoreAllMocks())

  it('merchants sits under Spending and the toggle moves between its views', async () => {
    open('/insights/merchants')
    expect(current()).toEqual(['Spending'])
    expect(view()).toEqual(['Merchants'])
    fireEvent.click(screen.getByRole('button', { name: 'Optional' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/insights/optional'))
    expect(current()).toEqual(['Spending'])
  })

  it('habits opens trends, drift sits beside it', async () => {
    open('/insights/habits')
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/insights/trends'))
    expect(current()).toEqual(['Habits'])
    fireEvent.click(screen.getByRole('button', { name: 'Drift' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/insights/drift'))
  })

  it('Year chosen on Cash flow opens Year, not Month', async () => {
    open('/insights/cashflow')
    expect(current()).toEqual(['Review'])
    fireEvent.click(screen.getByRole('button', { name: 'Year' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/insights/review'))
    expect(screen.getByRole('button', { name: 'Year' })).toHaveClass('bg-raised')
  })

  it('loans open under Property & loans', () => {
    open('/wealth/loans')
    expect(current()).toEqual(['Property & loans'])
  })

  it('stress test keeps its own tab', () => {
    open('/wealth/stress')
    expect(current()).toEqual(['Stress test'])
  })

  it('trips sit under Goals, beside the wish list', async () => {
    open('/plan/trips')
    expect(current()).toEqual(['Goals'])
    fireEvent.click(screen.getByRole('button', { name: 'Wish list' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/plan/wishlist'))
  })
})
