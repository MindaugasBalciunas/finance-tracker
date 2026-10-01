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
  matched_tx_id: number | null
  state: 'staged' | 'imported' | 'dismissed'
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
  date_from: string
  date_to: string
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
}
