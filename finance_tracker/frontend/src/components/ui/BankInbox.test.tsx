import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BankInbox from './BankInbox'
import { bankingApi } from '../../api/banking'

vi.mock('../../api/banking', () => ({
  bankingApi: {
    getSettings: vi.fn(),
    connections: vi.fn(),
    staged: vi.fn(),
    syncAll: vi.fn(),
    commit: vi.fn(),
    dismiss: vi.fn(),
    restore: vi.fn(),
    updateStaged: vi.fn(),
  },
}))

vi.mock('../../api/transactions', () => ({
  transactionsApi: { deleteBatch: vi.fn(), getComments: vi.fn(), suggestCategory: vi.fn(), suggestLabels: vi.fn() },
}))

vi.mock('../../api/budgets', () => ({
  budgetsApi: { labels: vi.fn(), rules: vi.fn(), previewLabel: vi.fn(), applyLabel: vi.fn() },
}))

vi.mock('../../api/insights', () => ({
  aiApi: { getSettings: vi.fn(), assistTransaction: vi.fn() },
  insightsApi: {},
}))

const api = vi.mocked(bankingApi)

const emptySync = {
  fetched: 0, staged_new: 0, unchanged: 0, auto_skipped: 0,
  duplicate_exact: 0, duplicate_content: 0, needs_review: 0, internal: 0,
  date_from: '2026-09-01', date_to: '2026-09-30',
}

function connections(accountKey = 'swed') {
  return {
    connections: [
      {
        id: 1, aspsp_name: 'Swedbank', aspsp_country: 'LT', status: 'authorized' as const,
        valid_until: '', days_until_expiry: 90,
        accounts: [
          {
            id: 10, connection_id: 1, iban: 'LT16…1234', display_name: 'Main',
            account_key: accountKey, last_synced_at: null, last_tx_date: null, pending: 0,
          },
        ],
      },
    ],
    valid_account_keys: ['swed', 'seb'],
  }
}

function renderInbox() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <BankInbox />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('BankInbox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getSettings.mockResolvedValue({ configured: true } as never)
    api.connections.mockResolvedValue(connections() as never)
    api.staged.mockResolvedValue({ transactions: [], total: 0, page: 1, page_size: 100, counts: {} })
    api.syncAll.mockResolvedValue({ accounts: [], totals: emptySync, synced: 0, skipped: 0, failed: 0 })
  })

  // A user who never set banking up gets no bar at all on their
  // transactions page.
  it('renders nothing until banking is configured', async () => {
    api.getSettings.mockResolvedValue({ configured: false } as never)
    renderInbox()
    await waitFor(() => expect(api.getSettings).toHaveBeenCalled())
    expect(screen.queryByText('From your bank')).not.toBeInTheDocument()
  })

  // One press, every mapped account — the thing that used to need a trip to
  // the settings page per account.
  it('syncs every bank in one press', async () => {
    const user = userEvent.setup()
    renderInbox()

    await user.click(await screen.findByRole('button', { name: 'Sync all' }))
    await waitFor(() => expect(api.syncAll).toHaveBeenCalledWith(undefined))
  })

  // The cautious first look against a real bank stays one press away.
  it('offers the narrow 7-day window', async () => {
    const user = userEvent.setup()
    renderInbox()

    await user.click(await screen.findByRole('button', { name: 'Last 7 days' }))
    await waitFor(() => expect(api.syncAll).toHaveBeenCalledWith(7))
  })

  // One bank failing while another works is the normal case — consents
  // expire on their own schedules — so the report names the unhappy one.
  it('names the account that could not sync', async () => {
    const user = userEvent.setup()
    api.syncAll.mockResolvedValue({
      accounts: [
        { link_id: 10, bank: 'Swedbank', name: 'Main', result: { ...emptySync, fetched: 12, staged_new: 3 } },
        { link_id: 11, bank: 'SEB', name: 'Savings', skipped: 'the connection to this bank has expired — reconnect it' },
      ],
      totals: { ...emptySync, fetched: 12, staged_new: 3 },
      synced: 1, skipped: 1, failed: 0,
    })
    renderInbox()

    await user.click(await screen.findByRole('button', { name: 'Sync all' }))
    expect(await screen.findByText(/12 fetched from 1 account/)).toBeInTheDocument()
    expect(screen.getByText(/SEB · Savings — the connection to this bank has expired/)).toBeInTheDocument()
  })

  // Nothing is mapped: pressing Sync would be a no-op, so say where to fix it
  // rather than letting the button do nothing.
  it('disables sync when no account is mapped', async () => {
    api.connections.mockResolvedValue(connections('') as never)
    renderInbox()

    expect(await screen.findByRole('button', { name: 'Sync all' })).toBeDisabled()
    expect(screen.getByText(/No account is mapped yet/)).toBeInTheDocument()
  })

  // The queue is the point of the panel: it opens in place, on the
  // transactions page, instead of three taps into the settings menu.
  it('shows the waiting rows in place', async () => {
    const user = userEvent.setup()
    api.staged.mockResolvedValue({
      transactions: [
        {
          id: 1, link_id: 1, external_id: 'eb:1:1', raw_payee: 'LIDL', raw_details: '',
          raw_amount: 23.4, raw_dk: 'D', raw_currency: 'EUR',
          booking_date: '2026-09-10T00:00:00Z', amount: 23.4,
          amount_money: { value: 23.4, currency: 'EUR' }, date: '2026-09-09T00:00:00Z',
          type: 'expense', category: 'Food', comment: 'Lidl', labels: '',
          debit_account: 'swed', credit_account: '', verdict: 'new', verdict_note: '',
          enrich_note: '', matched_tx_id: null, state: 'staged', imported_tx_id: null,
          preticked: true, first_seen_at: '', last_seen_at: '',
        },
      ],
      total: 1, page: 1, page_size: 100, counts: {},
    })
    renderInbox()

    expect(await screen.findByText('1 to review')).toBeInTheDocument()
    await user.click(screen.getByText('From your bank'))
    expect(await screen.findByText('Lidl')).toBeInTheDocument()
  })
})
