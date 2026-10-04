import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import type { Account, Category, Overview, PlanReport, Rule, Tx, TxList, Budget, InboxRow, Snapshot, Flow } from './types'

export const useOverview = () => useQuery({ queryKey: ['overview'], queryFn: () => api.get<Overview>('/overview') })
export const useCategories = () => useQuery({ queryKey: ['categories'], queryFn: () => api.get<Category[]>('/categories'), staleTime: 300_000 })
export const useAccounts = () => useQuery({ queryKey: ['accounts'], queryFn: () => api.get<Account[]>('/accounts'), staleTime: 60_000 })
export const useTags = () => useQuery({ queryKey: ['tags'], queryFn: () => api.get<{ tag: string; count: number; last: string }[]>('/tags'), staleTime: 60_000 })
export const useRules = () => useQuery({ queryKey: ['rules'], queryFn: () => api.get<Rule[]>('/rules') })
export const useBudgets = () => useQuery({ queryKey: ['budgets'], queryFn: () => api.get<Budget[]>('/budgets', { archived: '1' }) })
export const usePlan = (month: string) => useQuery({ queryKey: ['plan', month], queryFn: () => api.get<PlanReport>('/plan', { month }) })
export const useInbox = (state = 'open') => useQuery({ queryKey: ['inbox', state], queryFn: () => api.get<InboxRow[]>('/bank/inbox', { state }) })
export const useNetWorthHistory = (from: string, step = 'month', accounts = false) =>
  useQuery({ queryKey: ['nw-history', from, step, accounts], queryFn: () => api.get<Snapshot[]>('/networth/history', { from, step, accounts }) })
export const useCashflow = (from: string, to: string, granularity: 'month' | 'year') =>
  useQuery({ queryKey: ['cashflow', from, to, granularity], queryFn: () => api.get<Flow[]>('/insights/cashflow', { from, to, granularity }) })
export const useMerchants = () =>
  useQuery({ queryKey: ['merchants'], queryFn: () => api.get<{ merchant: string; count: number; category: string }[]>('/merchants'), staleTime: 300_000 })

export interface TxFilter {
  from?: string; to?: string; kind?: string; category?: string[]; account?: string[]; tag?: string[]; tags_all?: boolean
  merchant?: string; q?: string; min?: string; max?: string; sort?: string; limit?: number; offset?: number
}
export const useTransactions = (f: TxFilter) =>
  useQuery({ queryKey: ['transactions', f], queryFn: () => api.get<TxList>('/transactions', f as any), placeholderData: (p) => p })

/** Invalidate everything a ledger change can move. */
export function useRefresh() {
  const qc = useQueryClient()
  return () => {
    for (const k of ['transactions', 'overview', 'plan', 'inbox', 'tags', 'merchants', 'cashflow', 'breakdown', 'trends', 'recurring', 'fi', 'review', 'trips', 'accounts', 'nw-history', 'loans', 'networth', 'owed', 'tx', 'balances', 'portfolio', 'trades'])
      qc.invalidateQueries({ queryKey: [k] })
  }
}

export function useSaveTx() {
  const refresh = useRefresh()
  return useMutation({
    mutationFn: (t: Partial<Tx> & { auto_fill?: boolean; suppressed_tags?: string[] }) =>
      t.id ? api.put<Tx>(`/transactions/${t.id}`, t) : api.post<Tx>('/transactions', t),
    onSuccess: refresh,
  })
}
export function useDeleteTx() {
  const refresh = useRefresh()
  return useMutation({ mutationFn: (id: number) => api.del(`/transactions/${id}`), onSuccess: refresh })
}
