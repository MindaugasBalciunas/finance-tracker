import client from './client'
import type { Money, TransactionType } from '../types'

export type BankSettings = {
  environment: 'sandbox' | 'production'
  redirect_url: string
  application_id: string
  has_key: boolean
  configured: boolean
  updated_at: string
}

export type SaveBankSettingsInput = {
  application_id?: string
  private_key_pem?: string
  clear_key?: boolean
  environment?: 'sandbox' | 'production'
  redirect_url?: string
}

export type ASPSP = {
  name: string
  country: string
  logo: string
  beta: boolean
  sandbox: boolean
  max_consent_days: number
  redirect: boolean
}

export type BankAccountLink = {
  id: number
  connection_id: number
  iban: string
  display_name: string
  account_key: string
  last_synced_at: string | null
  last_tx_date: string | null
  pending: number
  // What the bank last stated; null until it has said.
  bank_balance?: number | null
  bank_balance_currency?: string
  bank_balance_at?: string | null
}

// One balance-sheet account set to the bank's own figure by a sync.
export type BankBalanceResult = {
  account: string
  before: number
  after: number
  changed: boolean
  skipped?: string
}

export type BankConnection = {
  id: number
  aspsp_name: string
  aspsp_country: string
  status: 'pending' | 'authorized' | 'expired' | 'revoked'
  valid_until: string
  days_until_expiry: number
  last_error?: string
  accounts: BankAccountLink[]
}

export type StagedVerdict =
  | 'new'
  | 'duplicate_exact'
  | 'duplicate_content'
  | 'internal'
  | 'needs_review'
  | 'pending'

export type StagedTx = {
  id: number
  link_id: number
  external_id: string
  raw_payee: string
  raw_details: string
  raw_amount: number
  raw_dk: 'D' | 'K' | ''
  raw_currency: string
  booking_date: string
  amount: number
  amount_money: Money
  date: string
  type: TransactionType
  category: string
  comment: string
  labels: string
  debit_account: string
  credit_account: string
  verdict: StagedVerdict
  verdict_note: string
  // Where a learned proposal came from — "category Food — 7 past
  // transactions matching \"Barbora\"". Empty when the classifier's own
  // answer stood.
  enrich_note: string
  matched_tx_id: number | null
  // A card reservation: authorised by the bank, not booked. Visible days
  // early, editable, never committable — the amount can still change.
  pending: boolean
  committable: boolean
  superseded_by: number | null
  // A transaction already in the ledger that this row is plainly the bank's
  // version of — same amount, account and day, different description.
  merge_candidate?: {
    id: number
    date: string
    comment: string
    amount: number
    labels: string
  }
  state: 'staged' | 'imported' | 'dismissed' | 'superseded'
  imported_tx_id: number | null
  preticked: boolean
  first_seen_at: string
  last_seen_at: string
}

export type SyncResult = {
  fetched: number
  staged_new: number
  unchanged: number
  auto_skipped: number
  duplicate_exact: number
  duplicate_content: number
  needs_review: number
  internal: number
  pending: number
  superseded: number
  released: number
  date_from: string
  date_to: string
  balances?: BankBalanceResult[]
  auto_linked?: number
}

// One line per mapped account, plus the totals. Per-account detail stays
// because the answer is rarely uniform — one bank's consent expires while
// the other syncs fine.
export type AccountSyncResult = {
  link_id: number
  bank: string
  name: string
  result?: SyncResult
  skipped?: string
  error?: string
}

export type SyncAllResult = {
  accounts: AccountSyncResult[]
  totals: SyncResult
  synced: number
  skipped: number
  failed: number
  balances?: BankBalanceResult[]
  auto_linked?: number
}

export type CommitResult = {
  imported: number
  skipped: number
  imported_tx_ids: number[]
  notes?: string[]
  balance_note: string
}

export type StagedPage = {
  transactions: StagedTx[]
  total: number
  page: number
  page_size: number
  counts: Record<string, number>
}

export type StagedEdit = {
  date?: string
  type?: TransactionType
  amount?: number
  category?: string
  comment?: string
  labels?: string
  debit_account?: string
  credit_account?: string
}

export type StagedFilter = {
  state?: string
  verdict?: string
  link_id?: number
  page?: number
  page_size?: number
}

export const bankingApi = {
  getSettings: async (): Promise<BankSettings> => {
    const { data } = await client.get<BankSettings>('/banking/settings')
    return data
  },

  saveSettings: async (input: SaveBankSettingsInput): Promise<BankSettings> => {
    const { data } = await client.put<BankSettings>('/banking/settings', input)
    return data
  },

  aspsps: async (country = 'LT'): Promise<ASPSP[]> => {
    const { data } = await client.get<{ aspsps: ASPSP[] }>('/banking/aspsps', { params: { country } })
    return data.aspsps ?? []
  },

  connections: async (): Promise<{ connections: BankConnection[]; valid_account_keys: string[] }> => {
    const { data } = await client.get<{ connections: BankConnection[]; valid_account_keys: string[] }>(
      '/banking/connections'
    )
    return { connections: data.connections ?? [], valid_account_keys: data.valid_account_keys ?? [] }
  },

  // Returns the bank's authorisation URL to open. The bank sends the browser
  // back to the registered redirect; nothing calls our server.
  connect: async (aspsp_name: string, country = 'LT'): Promise<{ connection_id: number; url: string }> => {
    const { data } = await client.post<{ connection_id: number; url: string }>('/banking/connections', {
      aspsp_name,
      country,
    })
    return data
  },

  // Either {code, state} from the auto-redirect, or {url} pasted by hand.
  callback: async (input: { code?: string; state?: string; url?: string }): Promise<{ connection_id: number }> => {
    const { data } = await client.post<{ connection_id: number }>('/banking/connections/callback', input)
    return data
  },

  disconnect: async (id: number): Promise<void> => {
    await client.delete(`/banking/connections/${id}`)
  },

  mapAccount: async (id: number, account_key: string): Promise<BankAccountLink> => {
    const { data } = await client.put<BankAccountLink>(`/banking/accounts/${id}`, { account_key })
    return data
  },

  // days narrows the window — the cautious first look before the full 90.
  sync: async (id: number, days?: number): Promise<SyncResult> => {
    const { data } = await client.post<SyncResult>(`/banking/accounts/${id}/sync`, null, {
      params: days ? { days } : undefined,
    })
    return data
  },

  // Every mapped account on every live connection, in one press. days
  // narrows the window the same way the per-account sync does.
  syncAll: async (days?: number): Promise<SyncAllResult> => {
    const { data } = await client.post<SyncAllResult>('/banking/sync', null, {
      params: days ? { days } : undefined,
    })
    return data
  },

  staged: async (filter: StagedFilter = {}): Promise<StagedPage> => {
    const { data } = await client.get<StagedPage>('/banking/staged', { params: filter })
    return data
  },

  // Correct the classifier's proposal before committing. Only the
  // ledger-bound fields move — the raw bank columns stay as sent.
  updateStaged: async (id: number, input: StagedEdit): Promise<StagedTx> => {
    const { data } = await client.put<StagedTx>(`/banking/staged/${id}`, input)
    return data
  },

  commit: async (ids: number[]): Promise<CommitResult> => {
    const { data } = await client.post<CommitResult>('/banking/staged/commit', { ids })
    return data
  },

  dismiss: async (ids: number[]): Promise<void> => {
    await client.post('/banking/staged/dismiss', { ids })
  },

  restore: async (ids: number[]): Promise<void> => {
    await client.post('/banking/staged/restore', { ids })
  },

  // Link a bank row to a transaction already entered by hand. The ledger row
  // keeps its own description and labels; only the bank reference moves, so
  // later syncs recognise it instead of offering it again.
  merge: async (id: number, transactionId: number): Promise<{ merged_into: number; note: string }> => {
    const { data } = await client.post(`/banking/staged/${id}/merge`, { transaction_id: transactionId })
    return data
  },

  unmerge: async (id: number): Promise<void> => {
    await client.post(`/banking/staged/${id}/unmerge`)
  },
}

