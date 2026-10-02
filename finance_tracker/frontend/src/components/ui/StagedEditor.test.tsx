import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import StagedEditor from './StagedEditor'
import { bankingApi, type StagedTx } from '../../api/banking'
import { budgetsApi } from '../../api/budgets'
import { transactionsApi } from '../../api/transactions'
import { aiApi } from '../../api/insights'

vi.mock('../../api/banking', () => ({
  bankingApi: { updateStaged: vi.fn() },
}))

vi.mock('../../api/budgets', () => ({
  budgetsApi: { labels: vi.fn(), rules: vi.fn(), previewLabel: vi.fn(), applyLabel: vi.fn() },
}))

vi.mock('../../api/transactions', () => ({
  transactionsApi: { getComments: vi.fn(), suggestCategory: vi.fn() },
}))

vi.mock('../../api/insights', () => ({
  aiApi: { getSettings: vi.fn(), assistTransaction: vi.fn() },
  insightsApi: {},
}))

const bank = vi.mocked(bankingApi)
const budgets = vi.mocked(budgetsApi)
const txs = vi.mocked(transactionsApi)
const ai = vi.mocked(aiApi)

function row(over: Partial<StagedTx> = {}): StagedTx {
  return {
    id: 7,
    link_id: 1,
    external_id: 'eb:1:9001',
    raw_payee: 'BARBORA UAB',
    raw_details: 'PIRKINYS 516793******2950 2026.09.09 33.15 EUR',
    raw_amount: 33.15,
    raw_dk: 'D',
    raw_currency: 'EUR',
    booking_date: '2026-09-10T00:00:00Z',
    amount: 33.15,
    amount_money: { value: 33.15, currency: 'EUR' },
    date: '2026-09-09T00:00:00Z',
    type: 'expense',
    category: 'Entertainment',
    comment: 'Barbora',
    labels: '',
    debit_account: 'swed',
    credit_account: '',
    verdict: 'new',
    verdict_note: '',
    enrich_note: '',
    matched_tx_id: null,
    state: 'staged',
    imported_tx_id: null,
    preticked: true,
    first_seen_at: '',
    last_seen_at: '',
    ...over,
  }
}

function renderEditor(r: StagedTx = row()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <StagedEditor row={r} validAccountKeys={['swed', 'seb', 'cash']} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('StagedEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    budgets.labels.mockResolvedValue(['groceries', 'kids'])
    budgets.rules.mockResolvedValue([])
    budgets.previewLabel.mockResolvedValue({ matches: 0, unlabeled: 0 })
    txs.getComments.mockResolvedValue(['Barbora', 'Barbora delivery'])
    txs.suggestCategory.mockResolvedValue({ category: '', matches: 0, basis: '' })
    ai.getSettings.mockResolvedValue({ enabled: true, has_key: true, model: 'claude-opus-5' } as never)
    bank.updateStaged.mockImplementation(async (_id, edit) => ({ ...row(), ...edit }) as StagedTx)
  })

  // The date was not editable at all before — a card row re-dated to the
  // wrong day could only be fixed after importing it.
  it('offers the date as an editable field', () => {
    renderEditor()
    expect(screen.getByLabelText(/Date/)).toHaveValue('2026-09-09')
  })

  // The label chips are the whole point of reusing the form's editor: the
  // old field was a bare comma-separated text box.
  it('applies a known label with one tap', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(await screen.findByRole('button', { name: '+ groceries' }))
    await waitFor(() => expect(bank.updateStaged).toHaveBeenCalledWith(7, { labels: 'groceries' }))
  })

  // The same history-based suggestion the create form shows — the bank's
  // classifier guessed Entertainment, the user's own ledger says Food.
  it('offers the category similar transactions actually got', async () => {
    const user = userEvent.setup()
    txs.suggestCategory.mockResolvedValue({ category: 'Food', matches: 7, basis: 'comment' })
    renderEditor()

    expect(await screen.findByText(/7 similar transactions/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Use it' }))
    await waitFor(() => expect(bank.updateStaged).toHaveBeenCalledWith(7, { category: 'Food' }))
  })

  // Enrichment happens server-side at sync; the row says so rather than
  // silently presenting a learned guess as the bank's own word.
  it('says where a learned proposal came from', () => {
    renderEditor(row({ enrich_note: 'category Food — 7 past transactions matching "Barbora"' }))
    expect(screen.getByText(/7 past transactions matching/)).toBeInTheDocument()
  })

  // The raw bank narrative is the only place an unrecognised merchant is
  // still named, so it travels with the assist request.
  it('sends the raw bank narrative to AI assist and merges the labels', async () => {
    const user = userEvent.setup()
    ai.assistTransaction.mockResolvedValue({ labels: ['groceries'], comment: 'Barbora groceries' })
    renderEditor()

    await user.click(await screen.findByRole('button', { name: /AI suggest/ }))

    await waitFor(() =>
      expect(ai.assistTransaction).toHaveBeenCalledWith(
        expect.objectContaining({ context: expect.stringContaining('BARBORA UAB') })
      )
    )
    await waitFor(() => expect(bank.updateStaged).toHaveBeenCalledWith(7, { labels: 'groceries' }))

    // The cleaner description is offered, never applied behind the user's back.
    expect(await screen.findByText(/Clearer description/)).toBeInTheDocument()
    expect(bank.updateStaged).not.toHaveBeenCalledWith(7, { comment: 'Barbora groceries' })
  })

  // A hand-applied label no rule explains is a rule waiting to be born —
  // and a rule taught here pays off on every future sync.
  it('offers to turn a label into a rule', async () => {
    renderEditor(row({ labels: 'groceries' }))
    expect(await screen.findByText(/Make/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create rule' })).toBeInTheDocument()
  })
})
