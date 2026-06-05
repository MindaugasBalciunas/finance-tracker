import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { transactionsApi } from '../api/transactions'
import type {
  TransactionFilter,
  CreateTransactionInput,
  UpdateTransactionInput,
} from '../types'

export const TRANSACTIONS_KEY = 'transactions'
export const SUMMARY_KEY = 'transactions-summary'
export const COMMENTS_KEY = 'transactions-comments'

export function useTransactions(filter: TransactionFilter = {}) {
  return useQuery({
    queryKey: [TRANSACTIONS_KEY, filter],
    queryFn: () => transactionsApi.list(filter),
  })
}

export function useAllExpenses(filter: Pick<TransactionFilter, 'date_from' | 'date_to'> = {}) {
  return useQuery({
    queryKey: [TRANSACTIONS_KEY, 'all-expenses', filter],
    queryFn: () => transactionsApi.list({ type: 'expense', page: 1, page_size: 1000, ...filter }),
  })
}

export function useTransactionSummary(filter: Pick<TransactionFilter, 'date_from' | 'date_to'> = {}) {
  return useQuery({
    queryKey: [SUMMARY_KEY, filter],
    queryFn: () => transactionsApi.getSummary(filter),
  })
}

export function useTransactionComments() {
  return useQuery({
    queryKey: [COMMENTS_KEY],
    queryFn: () => transactionsApi.getComments(),
    staleTime: 60_000,
  })
}

export function useCreateTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTransactionInput) => transactionsApi.create(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
      qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
      qc.invalidateQueries({ queryKey: [COMMENTS_KEY] })
    },
  })
}

export function useUpdateTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateTransactionInput }) =>
      transactionsApi.update(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
      qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
    },
  })
}

export function useDeleteTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => transactionsApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
      qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
    },
  })
}
