// PROBE (temporary — delete after run): demonstrates that when loan payment
// replay is active, the summary tiles (backend values) and the asset cards
// (frontend estimates) show contradictory Loans/Net-equity numbers on the
// same page at the same time.
import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Assets from './Assets'
import { assetsApi } from '../api/assets'
import { transactionsApi } from '../api/transactions'
import { formatEuro } from '../utils/format'
import type { Asset, AssetSummary, Transaction } from '../types'

vi.mock('../api/assets', () => ({
  assetsApi: {
    listAll: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    getSummary: vi.fn(),
  },
}))
vi.mock('../api/transactions', () => ({
  transactionsApi: {
    listAll: vi.fn(),
  },
}))

const mockedAssets = vi.mocked(assetsApi)
const mockedTxs = vi.mocked(transactionsApi)

const house: Asset = {
  id: 2,
  name: 'House',
  type: 'real_estate',
  purchase_date: '2022-08-17T00:00:00Z',
  purchase_price: 355000,
  current_value: 410000,
  notes: '',
  loan_remaining: 263568.67,
  loan_remaining_date: '2026-01-01T00:00:00Z',
  loan_account: 'seb',
  loan_margin: 1.3,
  loan_base_rate: 2.0,
  loan_label: 'homeloan',
  loan_monthly_payment: 1500,
  created_at: '',
  updated_at: '',
  equity: 146431.33, // backend: current_value - loan_remaining
}

// Backend summary — sums stored loan_remaining (asset_service.go GetSummary)
const summary: AssetSummary = {
  count: 1,
  total_purchase_price: 355000,
  total_value: 410000,
  total_loans: 263568.67,
  net_equity: 146431.33,
}

function pay(id: number, date: string): Transaction {
  return {
    id,
    date,
    type: 'expense',
    amount: { value: 1500, currency: 'EUR' },
    comment: 'mortgage payment',
    category: 'other',
    labels: 'homeloan',
    source_account: '',
    debit_account: 'seb',
    credit_account: '',
    created_at: '',
    updated_at: '',
  } as Transaction
}

const payments = [pay(1, '2026-02-01T00:00:00Z'), pay(2, '2026-03-01T00:00:00Z'), pay(3, '2026-04-01T00:00:00Z')]

// Mirror of Assets.tsx loanEstimate replay math
function expectedRemaining(): number {
  const r = (1.3 + 2.0) / 100 / 12
  let remaining = 263568.67
  for (let i = 0; i < 3; i++) {
    const interest = remaining * r
    const principal = Math.max(1500 - interest, 0)
    remaining = Math.max(remaining - principal, 0)
  }
  return remaining
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
  mockedAssets.listAll.mockResolvedValue([house])
  mockedAssets.getSummary.mockResolvedValue(summary)
  mockedTxs.listAll.mockResolvedValue({ data: payments, total: 3, page: 1, page_size: 3, total_pages: 1 })
})

describe('PROBE: summary tiles vs card estimates', () => {
  it('renders two different loan-outstanding numbers simultaneously', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(1))
    await waitFor(() => expect(screen.getByText(/est\./)).toBeInTheDocument())

    const est = expectedRemaining()
    const staleLoanStr = formatEuro(263568.67) // summary tile "Loans outstanding"
    const estLoanStr = formatEuro(est) // card "Loan outstanding"
    expect(staleLoanStr).not.toBe(estLoanStr)

    // BOTH appear on the page at once — the contradiction
    expect(screen.getByText(staleLoanStr)).toBeInTheDocument()
    expect(screen.getByText((t) => t.trim() === estLoanStr)).toBeInTheDocument()

    // eslint-disable-next-line no-console
    console.log('[PROBE] tile Loans outstanding =', staleLoanStr, '| card Loan outstanding =', estLoanStr,
      '| diff =', formatEuro(263568.67 - est))
  })

  it('renders two different net-equity numbers simultaneously', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(1))
    await waitFor(() => expect(screen.getByText(/est\./)).toBeInTheDocument())

    const est = expectedRemaining()
    const tileEquityStr = formatEuro(146431.33) // summary tile "Net equity" (backend)
    const cardEquityStr = formatEuro(410000 - est) // card "Net equity" (estimate)
    expect(tileEquityStr).not.toBe(cardEquityStr)

    expect(screen.getByText(tileEquityStr)).toBeInTheDocument()
    expect(screen.getByText(cardEquityStr)).toBeInTheDocument()

    // The tile claims "Value − loans" but does not match the single card below it
    expect(screen.getByText('Value − loans')).toBeInTheDocument()

    // eslint-disable-next-line no-console
    console.log('[PROBE] tile Net equity =', tileEquityStr, '| card Net equity =', cardEquityStr,
      '| diff =', formatEuro((410000 - est) - 146431.33))
  })

  it('card Net equity carries NO est. marker (only the loan line does)', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('asset-card')).toHaveLength(1))
    await waitFor(() => expect(screen.getByText(/est\./)).toBeInTheDocument())
    // exactly one est. tag on the page: the loan-outstanding line.
    // The card's switched Net equity value is unmarked.
    expect(screen.getAllByText(/est\./)).toHaveLength(1)
  })
})
