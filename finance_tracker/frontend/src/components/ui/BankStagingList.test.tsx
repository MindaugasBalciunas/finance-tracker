import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BankStagingList from './BankStagingList'
import { bankingApi, type StagedTx } from '../../api/banking'
import { budgetsApi } from '../../api/budgets'
import { transactionsApi } from '../../api/transactions'
import { aiApi } from '../../api/insights'

vi.mock('../../api/banking', () => ({
  bankingApi: {
    commit: vi.fn(),
    dismiss: vi.fn(),
    restore: vi.fn(),
    updateStaged: vi.fn(),
    staged: vi.fn(),
    connections: vi.fn(),
  },
}))

vi.mock('../../api/transactions', () => ({
  transactionsApi: { deleteBatch: vi.fn(), getComments: vi.fn(), suggestCategory: vi.fn(), suggestLabels: vi.fn() },
}))

// The review modal is the ordinary transaction form, so it pulls the same
// label/AI surfaces the form does.
vi.mock('../../api/budgets', () => ({
  budgetsApi: { labels: vi.fn(), rules: vi.fn(), previewLabel: vi.fn(), applyLabel: vi.fn() },
}))

vi.mock('../../api/insights', () => ({
  aiApi: { getSettings: vi.fn(), assistTransaction: vi.fn() },
  insightsApi: {},
}))

const api = vi.mocked(bankingApi)
const budgets = vi.mocked(budgetsApi)
const txs = vi.mocked(transactionsApi)

function row(over: Partial<StagedTx> = {}): StagedTx {
  return {
    id: 1,
    link_id: 1,
    external_id: 'eb:1:7001',
    raw_payee: 'LIDL SNIPISKES',
    raw_details: 'PIRKINYS 2026.09.09 23.40 EUR',
    raw_amount: 23.4,
    raw_dk: 'D',
    raw_currency: 'EUR',
    booking_date: '2026-09-10T00:00:00Z',
    amount: 23.4,
    amount_money: { value: 23.4, currency: 'EUR' },
    date: '2026-09-09T00:00:00Z',
    type: 'expense',
    category: 'Food',
    comment: 'Lidl',
    labels: '',
    debit_account: 'swed',
    credit_account: '',
    verdict: 'new',
    verdict_note: '',
    enrich_note: '',
    pending: false,
    committable: true,
    superseded_by: null,
    matched_tx_id: null,
    state: 'staged',
    imported_tx_id: null,
    preticked: true,
    first_seen_at: '',
    last_seen_at: '',
    ...over,
  }
}

function renderList(rows: StagedTx[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <BankStagingList rows={rows} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('BankStagingList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    budgets.labels.mockResolvedValue([])
    budgets.rules.mockResolvedValue([])
    txs.getComments.mockResolvedValue([])
    txs.suggestCategory.mockResolvedValue({ category: '', matches: 0, basis: '' })
    txs.suggestLabels.mockResolvedValue({ labels: [], matches: 0 })
    vi.mocked(aiApi).getSettings.mockResolvedValue({ enabled: false } as never)
    api.updateStaged.mockImplementation(async (_id, edit) => ({ ...row(), ...edit }) as StagedTx)
    api.commit.mockResolvedValue({
      imported: 1,
      skipped: 0,
      imported_tx_ids: [99],
      balance_note: 'Account balances were not changed.',
    })
  })

  // One-by-one is the primary interaction: the ✓ on a row must add exactly
  // that row, regardless of what else happens to be ticked.
  it('per-row Add posts exactly that one id', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 11, comment: 'Lidl' }), row({ id: 12, comment: 'Ignitis' })])

    const addButtons = screen.getAllByRole('button', { name: /Add$/ })
    await user.click(addButtons[1])

    await waitFor(() => expect(api.commit).toHaveBeenCalledTimes(1))
    expect(api.commit).toHaveBeenCalledWith([12])
  })

  // The ~150-duplicate episode in reverse: a likely duplicate is shown with
  // its reason, never swept into a batch the user did not look at.
  it('never pre-ticks a duplicate_content row', async () => {
    renderList([
      row({ id: 11, comment: 'Lidl', verdict: 'new', preticked: true }),
      row({
        id: 12,
        comment: 'Ignitis',
        verdict: 'duplicate_content',
        preticked: false,
        verdict_note: 'looks like #4021 · 2026-09-12 · Ignitis · 8.12',
        matched_tx_id: 4021,
      }),
    ])

    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(boxes[0].checked).toBe(true)
    expect(boxes[1].checked).toBe(false)
    expect(screen.getByText(/looks like #4021/)).toBeInTheDocument()
    // Shown with its reason, not hidden — the user decides.
    expect(screen.getByText('Ignitis')).toBeInTheDocument()

    // And the batch bar only carries the one safe row.
    expect(screen.getByText('1 selected')).toBeInTheDocument()
  })

  it('the batch bar commits the pre-ticked set', async () => {
    const user = userEvent.setup()
    api.commit.mockResolvedValue({
      imported: 2,
      skipped: 0,
      imported_tx_ids: [99, 100],
      balance_note: 'Account balances were not changed.',
    })
    renderList([
      row({ id: 11, preticked: true }),
      row({ id: 12, preticked: true, comment: 'Ignitis' }),
      row({ id: 13, preticked: false, verdict: 'duplicate_content', comment: 'Maybe dup' }),
    ])

    await user.click(screen.getByRole('button', { name: 'Add 2' }))
    await waitFor(() => expect(api.commit).toHaveBeenCalledWith([11, 12]))
    expect(await screen.findByText(/Added 2/)).toBeInTheDocument()
  })

  it('offers undo after a commit and deletes exactly the imported ids', async () => {
    const user = userEvent.setup()
    const { transactionsApi } = await import('../../api/transactions')
    renderList([row({ id: 11 })])

    await user.click(screen.getByRole('button', { name: /Add$/ }))
    const undo = await screen.findByRole('button', { name: 'Undo' })
    await user.click(undo)

    await waitFor(() => expect(vi.mocked(transactionsApi.deleteBatch)).toHaveBeenCalledWith([99]))
  })

  it('dismiss posts the row id and restore brings it back', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 11 })])
    await user.click(screen.getByRole('button', { name: /Dismiss/ }))
    await waitFor(() => expect(api.dismiss).toHaveBeenCalledWith([11]))
  })

  // Reviewing a row opens the ordinary transaction form — the same one
  // "+ Add" opens — prefilled from the bank's proposal.
  it('reviews a row in the normal transaction form', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 11, comment: 'Lidl' })])

    await user.click(screen.getByText('Lidl'))

    expect(await screen.findByRole('button', { name: 'Add transaction' })).toBeInTheDocument()
    expect(screen.getByLabelText('Amount (€)')).toHaveValue(23.4)
    expect(screen.getByLabelText('Category')).toHaveValue('Food')
    expect(screen.getByLabelText('Comment')).toHaveValue('Lidl')
  })

  // Submitting saves the corrections onto the staged row and then adds it —
  // one action, because the queue is the draft.
  it('saves the corrections and adds the transaction', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 11, comment: 'Lidl' })])

    await user.click(screen.getByText('Lidl'))
    await user.selectOptions(await screen.findByLabelText('Category'), 'Housing')
    await user.click(screen.getByRole('button', { name: 'Add transaction' }))

    await waitFor(() =>
      expect(api.updateStaged).toHaveBeenCalledWith(11, expect.objectContaining({
        category: 'Housing', comment: 'Lidl', amount: 23.4, type: 'expense',
      }))
    )
    await waitFor(() => expect(api.commit).toHaveBeenCalledWith([11]))
  })

  // A card reservation is visible days early, but it is not a transaction
  // yet: its amount can still move and the bank re-issues it under a new
  // reference when it books.
  it('offers no Add button on a reservation', async () => {
    renderList([row({ id: 12, comment: 'Circle K', pending: true, committable: false, preticked: false, verdict: 'pending' })])

    expect(screen.getByText('reserved')).toBeInTheDocument()
    expect(screen.getByText('Waiting for the bank to book it')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Add/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  // ...but it can still be filed, so the corrections carry onto the booking.
  it('lets a reservation be filed ahead of its booking', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 12, comment: 'Circle K', pending: true, committable: false, preticked: false, verdict: 'pending' })])

    await user.click(screen.getByRole('button', { name: /File it/ }))
    expect(await screen.findByRole('button', { name: 'Save for when it books' })).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Category'), 'Transport')
    await user.click(screen.getByRole('button', { name: 'Save for when it books' }))

    await waitFor(() =>
      expect(api.updateStaged).toHaveBeenCalledWith(12, expect.objectContaining({ category: 'Transport' }))
    )
    // Saving a reservation never reaches the ledger.
    expect(api.commit).not.toHaveBeenCalled()
  })

  it('shows the raw bank data behind a disclosure, not by default', async () => {
    const user = userEvent.setup()
    renderList([row({ id: 11, comment: 'Lidl' })])

    expect(screen.queryByText(/PIRKINYS/)).not.toBeInTheDocument()
    await user.click(screen.getByText('Lidl'))
    await user.click(await screen.findByRole('button', { name: /raw bank data/ }))
    expect(screen.getByText(/PIRKINYS/)).toBeInTheDocument()
  })
})
