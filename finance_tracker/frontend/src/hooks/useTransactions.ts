import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { transactionsApi } from '../api/transactions'
import {
  BALANCES_KEY,
  BALANCE_ALLOCATION_KEY,
  BALANCE_LATEST_KEY,
  BALANCE_PROJECTED_KEY,
  BALANCE_TREND_KEY,
} from './useBalances'
import type {
  TransactionFilter,
  CreateTransactionInput,
  UpdateTransactionInput,
} from '../types'

export const TRANSACTIONS_KEY = 'transactions'
export const SUMMARY_KEY = 'transactions-summary'
export const COMMENTS_KEY = 'transactions-comments'

// Creating (or account-linking) a transaction writes a new balance snapshot
// server-side, so every balance view is stale the moment it succeeds. With
// staleTime at 5 min and refetchOnWindowFocus off, a missed key here reads as
// "the balance didn't adjust" for up to five minutes — which is exactly how
// it was reported. Keep this list in sync with useBalances' exported keys.
export function invalidateTransactionQueries(qc: QueryClient, balancesMoved = true) {
  qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
  qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
  qc.invalidateQueries({ queryKey: [COMMENTS_KEY] })
  qc.invalidateQueries({ queryKey: [BALANCE_PROJECTED_KEY] })
  if (!balancesMoved) return
  qc.invalidateQueries({ queryKey: [BALANCES_KEY] })
  qc.invalidateQueries({ queryKey: [BALANCE_LATEST_KEY] })
  qc.invalidateQueries({ queryKey: [BALANCE_TREND_KEY] })
  qc.invalidateQueries({ queryKey: [BALANCE_ALLOCATION_KEY] })
}

export function useTransactions(filter: TransactionFilter = {}) {
  return useQuery({
    queryKey: [TRANSACTIONS_KEY, filter],
    queryFn: () => transactionsApi.list(filter),
  })
}

export function useAllExpenses(filter: Pick<TransactionFilter, 'date_from' | 'date_to'> = {}) {
  return useQuery({
    queryKey: [TRANSACTIONS_KEY, 'all-expenses', filter],
    queryFn: () => transactionsApi.listAll({ type: 'expense', ...filter }),
  })
}

export function useAllTransactions(
  filter: Omit<TransactionFilter, 'page' | 'page_size'> = {},
  enabled = true
) {
  return useQuery({
    queryKey: [TRANSACTIONS_KEY, 'all', filter],
    queryFn: () => transactionsApi.listAll(filter),
    enabled,
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
    onSuccess: () => invalidateTransactionQueries(qc),
  })
}

export function useUpdateTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateTransactionInput }) =>
      transactionsApi.update(id, input),
    // An update that first adds account linkage snapshots the balance too.
    onSuccess: () => invalidateTransactionQueries(qc),
  })
}

export function useDeleteTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => transactionsApi.delete(id),
    // Deleting never rewrites a snapshot (balances only move forward), so the
    // stored balances stay valid — only the projection changes.
    onSuccess: () => invalidateTransactionQueries(qc, false),
  })
}
