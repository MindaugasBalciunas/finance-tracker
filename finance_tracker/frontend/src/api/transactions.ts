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

  getSummary: async (filter: Pick<TransactionFilter, 'date_from' | 'date_to'> = {}): Promise<TransactionSummary> => {
    const { data } = await client.get<TransactionSummary>('/transactions/summary', { params: filter })
    return data
  },

  getComments: async (): Promise<string[]> => {
    const { data } = await client.get<string[]>('/transactions/comments')
    return data
  },
}
