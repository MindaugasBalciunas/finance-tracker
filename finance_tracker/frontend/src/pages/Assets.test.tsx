import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Assets from './Assets'
import { assetsApi } from '../api/assets'
import type { Asset, AssetSummary } from '../types'

vi.mock('../api/assets', () => ({
  assetsApi: {
    listAll: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    getSummary: vi.fn(),
  },
}))

const mockedApi = vi.mocked(assetsApi)

const rav4: Asset = {
  id: 1,
  name: 'Toyota RAV4 Style Hybrid',
  type: 'vehicle',
  purchase_date: '2020-11-17T00:00:00Z',
  purchase_price: 34000,
  current_value: 34000,
  notes: 'Bought new. Fully paid on 2025-11-17.',
  loan_remaining: 0,
  loan_paid_off_date: '2025-11-17T00:00:00Z',
  created_at: '',
  updated_at: '',
  equity: 34000,
}

const house: Asset = {
  id: 2,
  name: 'House Platiniškių 21A, Platiniškių k.',
  type: 'real_estate',
  purchase_date: '2022-08-17T00:00:00Z',
  purchase_price: 355000,
  current_value: 410000,
  valuation_date: '2025-12-01T00:00:00Z',
  notes: 'Energy class B, built 2017.',
  loan_remaining: 263568.67,
  loan_remaining_date: '2026-07-20T00:00:00Z',
  loan_rate: '6M EURIBOR + 1.3%',
  loan_account: 'seb',
  created_at: '',
  updated_at: '',
  equity: 146431.33,
}

const solar: Asset = {
  id: 3,
  name: 'Solar panels 10.35 kWp (SOLAX X3 Hybrid G4)',
  type: 'solar',
  purchase_price: 4551.5,
  current_value: 4551.5,
  notes: 'Net capex after state subsidy.',
  loan_remaining: 0,
  created_at: '',
  updated_at: '',
  equity: 4551.5,
}

const summary: AssetSummary = {
  count: 3,
  total_purchase_price: 393551.5,
  total_value: 448551.5,
  total_loans: 263568.67,
  net_equity: 184982.83,
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Assets />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedApi.listAll.mockResolvedValue([rav4, house, solar])
  mockedApi.getSummary.mockResolvedValue(summary)
})

describe('Assets page', () => {
  it('renders one card per asset', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(3))
    expect(screen.getByText('Toyota RAV4 Style Hybrid')).toBeInTheDocument()
    expect(screen.getByText('House Platiniškių 21A, Platiniškių k.')).toBeInTheDocument()
    expect(screen.getByText('Solar panels 10.35 kWp (SOLAX X3 Hybrid G4)')).toBeInTheDocument()
  })

  it('shows summary totals', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('Total value')).toBeInTheDocument())
    expect(screen.getByText('Loans outstanding')).toBeInTheDocument()
    // Summary card + one label per asset card
    expect(screen.getAllByText('Net equity').length).toBeGreaterThanOrEqual(1)
    // 3 assets counted
    expect(screen.getByText('3 assets')).toBeInTheDocument()
  })

  it('shows loan details for mortgaged asset and paid-off badge for others', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(3))
    expect(screen.getByText(/6M EURIBOR \+ 1.3%/)).toBeInTheDocument()
    expect(screen.getByText(/from SEB/)).toBeInTheDocument()
    // RAV4 paid off + solar owned outright
    expect(screen.getAllByText(/Owned outright/)).toHaveLength(2)
  })

  it('opens the create form modal', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(3))
    await userEvent.click(screen.getByRole('button', { name: '+ Add' }))
    expect(screen.getByText('New Asset')).toBeInTheDocument()
    expect(screen.getByText('Purchase price (€)')).toBeInTheDocument()
  })

  it('creates an asset through the form', async () => {
    mockedApi.create.mockResolvedValue({ ...solar, id: 9 })
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(3))

    await userEvent.click(screen.getByRole('button', { name: '+ Add' }))
    await userEvent.type(screen.getByPlaceholderText('e.g. Toyota RAV4 Style Hybrid'), 'Garage')
    const priceInputs = screen.getAllByRole('spinbutton')
    await userEvent.type(priceInputs[0], '12000')
    await userEvent.click(screen.getByRole('button', { name: 'Save Asset' }))

    await waitFor(() => expect(mockedApi.create).toHaveBeenCalledTimes(1))
    const payload = mockedApi.create.mock.calls[0][0]
    expect(payload.name).toBe('Garage')
    expect(payload.purchase_price).toBe(12000)
    expect(payload.current_value).toBeUndefined()
    expect(payload.loan_remaining).toBeUndefined()
  })

  it('deletes an asset after confirmation', async () => {
    mockedApi.delete.mockResolvedValue(undefined as never)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(3))

    await userEvent.click(screen.getByRole('button', { name: 'Delete Toyota RAV4 Style Hybrid' }))
    await waitFor(() => expect(mockedApi.delete).toHaveBeenCalledWith(1))
  })

  it('shows empty state when there are no assets', async () => {
    mockedApi.listAll.mockResolvedValue([])
    mockedApi.getSummary.mockResolvedValue({ count: 0, total_purchase_price: 0, total_value: 0, total_loans: 0, net_equity: 0 })
    renderPage()
    await waitFor(() => expect(screen.getByText(/No assets yet/)).toBeInTheDocument())
  })
})
