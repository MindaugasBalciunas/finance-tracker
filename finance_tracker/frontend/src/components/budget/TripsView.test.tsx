import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import TripsView from './TripsView'
import { budgetsApi } from '../../api/budgets'
import type { TripSummary } from '../../types'

vi.mock('../../api/budgets', () => ({
  budgetsApi: { trips: vi.fn(), renameTrip: vi.fn(), assignTrip: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() },
}))

function trip(name: string, from: string): TripSummary {
  return { label: `trip:${name}`, name, from, to: from, days: 1, count: 1, total: 100, per_day: 100, by_category: [], by_label: [] }
}

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}><TripsView /></QueryClientProvider>)
}

describe('TripsView rename', () => {
  beforeEach(() => {
    vi.mocked(budgetsApi.trips).mockResolvedValue({ trips: [trip('2026-08', '2026-08-17'), trip('egypt-2026', '2026-10-18')], suggestions: [] })
    vi.mocked(budgetsApi.renameTrip).mockResolvedValue({ label: 'trip:x' })
  })

  it('renames a trip to a new name', async () => {
    renderView()
    await userEvent.click(await screen.findByRole('button', { name: 'Rename 2026-08' }))
    const input = screen.getByLabelText('New trip name')
    await userEvent.clear(input)
    await userEvent.type(input, 'Bulgaria 2027')
    expect(screen.getByText('Saved as "bulgaria-2027"')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(budgetsApi.renameTrip).toHaveBeenCalledWith({ from: 'trip:2026-08', to: 'Bulgaria 2027' }))
  })

  it('merges into an existing trip after confirming', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderView()
    await userEvent.click(await screen.findByRole('button', { name: 'Rename 2026-08' }))
    const input = screen.getByLabelText('New trip name')
    await userEvent.clear(input)
    await userEvent.type(input, 'egypt-2026')
    await userEvent.click(screen.getByRole('button', { name: 'Merge' }))
    expect(confirmSpy).toHaveBeenCalled()
    await waitFor(() => expect(budgetsApi.renameTrip).toHaveBeenCalledWith({ from: 'trip:2026-08', to: 'egypt-2026' }))
    confirmSpy.mockRestore()
  })
})

describe('TripsView year count', () => {
  it('counts a trip booked last year but taken this year', async () => {
    const y = new Date().getFullYear()
    const early: TripSummary = { ...trip('italy-spring', `${y - 1}-11-19`), to: `${y}-03-22` }
    vi.mocked(budgetsApi.trips).mockResolvedValue({ trips: [early, trip('egypt', `${y}-08-17`), { ...trip('old', `${y - 1}-05-01`) }], suggestions: [] })
    renderView()
    expect(await screen.findByText(`Trips in ${y}`)).toBeInTheDocument()
    expect(screen.getByText('2', { selector: 'p' })).toBeInTheDocument()
  })
})
