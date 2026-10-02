import client from './client'
import type {
  Transaction,
  CreateTransactionInput,
  UpdateTransactionInput,
  PaginatedTransactions,
  TransactionSummary,
  TransactionFilter,
} from '../types'

export const transactionsApi = {
  list: async (filter: TransactionFilter = {}): Promise<PaginatedTransactions> => {
    const { data } = await client.get<PaginatedTransactions>('/transactions', { params: filter })
    return data
  },

  // Fetches every page matching the filter, so callers get the complete set
  // regardless of how many transactions exist.
  listAll: async (filter: Omit<TransactionFilter, 'page' | 'page_size'> = {}): Promise<PaginatedTransactions> => {
    const pageSize = 1000
    const first = await transactionsApi.list({ ...filter, page: 1, page_size: pageSize })
    if (first.total_pages <= 1) return first
    const rest = await Promise.all(
      Array.from({ length: first.total_pages - 1 }, (_, i) =>
        transactionsApi.list({ ...filter, page: i + 2, page_size: pageSize })
      )
    )
    return { ...first, data: [...first.data, ...rest.flatMap((r) => r.data)] }
  },

  getById: async (id: number): Promise<Transaction> => {
    const { data } = await client.get<Transaction>(`/transactions/${id}`)
    return data
  },

  create: async (input: CreateTransactionInput): Promise<Transaction> => {
    const { data } = await client.post<Transaction>('/transactions', input)
    return data
  },

  update: async (id: number, input: UpdateTransactionInput): Promise<Transaction> => {
    const { data } = await client.put<Transaction>(`/transactions/${id}`, input)
    return data
  },

  delete: async (id: number): Promise<void> => {
    await client.delete(`/transactions/${id}`)
  },

  // Used by undo after a bulk add (JSON restore, bank import). Deleting rows
  // that came from a bank sync also returns them to the review list.
  deleteBatch: async (ids: number[]): Promise<void> => {
    await client.delete('/transactions/batch', { data: { ids } })
  },

  getSummary: async (filter: Pick<TransactionFilter, 'date_from' | 'date_to'> = {}): Promise<TransactionSummary> => {
    const { data } = await client.get<TransactionSummary>('/transactions/summary', { params: filter })
    return data
  },

  suggestCategory: async (params: { type?: string; comment?: string; amount?: number }): Promise<{ category: string; matches: number; basis: string }> => {
    const { data } = await client.get('/transactions/suggest-category', { params })
    return data
  },

  // What similar past transactions were labelled. Offered, never applied —
  // same contract as suggestCategory.
  suggestLabels: async (params: { type?: string; comment?: string }): Promise<{ labels: string[]; matches: number }> => {
    const { data } = await client.get('/transactions/suggest-labels', { params })
    return data
  },

  getComments: async (): Promise<string[]> => {
    const { data } = await client.get<string[]>('/transactions/comments')
    return data
  },
}
