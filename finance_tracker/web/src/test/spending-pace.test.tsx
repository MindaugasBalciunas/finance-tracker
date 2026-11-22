import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { OutPaceCard } from '../pages/insights/Spending'
import type { OutPacePoint } from '../lib/types'

afterEach(() => { vi.unstubAllGlobals() })

const points: OutPacePoint[] = Array.from({ length: 31 }, (_, i) => ({
  day: i + 1, last_month: 100 * (i + 1), last_month_invested: 40 * (i + 1), typical: 90 * (i + 1), typical_invested: 30 * (i + 1),
  ...(i < 10 ? { current: 1200 + i, current_invested: 800 } : {}),
}))

describe('Spending → this month so far, with investing', () => {
  it('splits today’s total into spent and invested', async () => {
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(points), { status: 200 })))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><OutPaceCard /></QueryClientProvider>)
    expect(await screen.findByText('This month so far, with investing')).toBeInTheDocument()
    // Day 10: €1,209 out, of which €800 invested.
    expect(screen.getByText('€409')).toBeInTheDocument()
    expect(screen.getByText('€800')).toBeInTheDocument()
    expect(vi.mocked(fetch).mock.calls[0][0]).toMatch(/insights\/pace-out$/)
  })
})
