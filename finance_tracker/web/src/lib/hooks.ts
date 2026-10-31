import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import { byBank } from './brand'
import type { Account, Category, Overview, PlanReport, Rule, Tx, TxList, Budget, InboxRow, Snapshot, Flow, MerchantReport } from './types'

export const useOverview = () => useQuery({ queryKey: ['overview'], queryFn: () => api.get<Overview>('/overview') })
export const useCategories = () => useQuery({ queryKey: ['categories'], queryFn: () => api.get<Category[]>('/categories'), staleTime: 300_000 })
// Every picker and list shows Swedbank first, then SEB, Revolut, the rest.
export const useAccounts = () => useQuery({ queryKey: ['accounts'], queryFn: () => api.get<Account[]>('/accounts'), staleTime: 60_000, select: byBank })
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
// Not under ['merchants']: that key is the autocomplete list, kept for minutes.
export const useMerchantInsights = (preset: string) =>
  useQuery({ queryKey: ['merchant-insights', preset], queryFn: () => api.get<MerchantReport>('/insights/merchants', { preset }), placeholderData: (p) => p })

export interface Prefs { liquid_only: boolean; periods?: Record<string, string>; hidden_accounts?: string[]; history_hidden?: string[]; usage_off?: boolean }

/** View choices stored on the server so they follow the owner across devices. */
export function usePrefs() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['prefs'], queryFn: () => api.get<Prefs>('/prefs'), staleTime: Infinity })
  const set = useMutation({
    mutationFn: (p: Partial<Prefs>) => api.put<Prefs>('/prefs', p),
    onMutate: (p) => qc.setQueryData<Prefs>(['prefs'], (old) => ({ ...(old ?? { liquid_only: false }), ...p, periods: { ...(old?.periods ?? {}), ...(p.periods ?? {}) } })),
    onSuccess: (p) => qc.setQueryData(['prefs'], p),
    onError: () => qc.invalidateQueries({ queryKey: ['prefs'] }),
  })
  return { prefs: q.data ?? { liquid_only: false }, set: set.mutate }
}

/** A chart's period, remembered on the server per chart key. */
export function usePeriod(key: string, fallback: string, allowed?: string[]): [string, (v: string) => void] {
  const { prefs, set } = usePrefs()
  const saved = prefs.periods?.[key]
  const value = saved && (!allowed || allowed.includes(saved)) ? saved : fallback
  return [value, (v: string) => set({ periods: { [key]: v } })]
}

/** Demo mode: the whole app over fictional data (the real data is untouched). */
export function useDemo() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['demo'], queryFn: () => api.get<{ on: boolean; protected?: boolean }>('/demo'), staleTime: Infinity })
  // Never show one database's cache in the other: every other query is reset
  // (data dropped, active ones refetched) — clear() would also detach the
  // live subscriptions, so the banner wouldn't notice the switch.
  const resetData = () => qc.resetQueries({ predicate: (x) => x.queryKey[0] !== 'demo' })
  // Leaving a PIN-protected demo needs the PIN (checked by the server).
  const switchTo = async (on: boolean, pin?: string) => {
    await api.put('/demo', { on, pin })
    qc.setQueryData(['demo'], { on, protected: q.data?.protected })
    await resetData()
  }
  const reset = async () => {
    await api.post('/demo/reset', {})
    await resetData()
  }
  return { on: !!q.data?.on, ready: q.isSuccess, protected: !!q.data?.protected, switchTo, reset }
}

export interface TxFilter {
  from?: string; to?: string; kind?: string; category?: string[]; account?: string[]; tag?: string[]; tags_all?: boolean
  merchant?: string; q?: string; min?: string; max?: string; sort?: string; limit?: number; offset?: number
  choices?: string // '1': obligations left out
  salary?: string // 'month': a salary counts in the month it pays for
}
export const useTransactions = (f: TxFilter) =>
  useQuery({ queryKey: ['transactions', f], queryFn: () => api.get<TxList>('/transactions', f as any), placeholderData: (p) => p })

// Views a ledger change can't move: settings and security, the chat, price
// charts and the editor's own suggestions. Everything else is refreshed — a
// hand-kept list of what *does* move kept missing new pages (tax, the month
// close, the balance table, portfolio quality after a trade…), which then
// showed old numbers after an edit.
const UNMOVED = new Set(['ai-models', 'ai-settings', 'analyst', 'auth-status', 'backups', 'bank-connections', 'bank-settings', 'categories', 'chat',
  'demo', 'hist', 'logos', 'notify', 'oauth-grants', 'oauth-request', 'passkeys', 'plan-settings', 'prefs', 'rules', 'topups',
  'usage', 'rule-preview', 'cat-suggest', 'note-suggest', 'tag-suggest'])

/** Invalidate everything a ledger change can move (only views on screen refetch). */
export function useRefresh() {
  const qc = useQueryClient()
  // Not awaited: a mutation's onSuccess would otherwise hold the editor open
  // until every view on screen has refetched.
  return () => {
    void qc.invalidateQueries({ predicate: (q) => !UNMOVED.has(String(q.queryKey[0])) })
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

/** Is the screen wide enough for a desktop arrangement (Tailwind's xl)?
 *  For layouts CSS alone can't reorder without changing the phone order. */
export function useWide(query = '(min-width: 1280px)') {
  const get = () => typeof window !== 'undefined' && !!window.matchMedia?.(query).matches
  const [wide, setWide] = useState(get)
  useEffect(() => {
    const m = window.matchMedia?.(query)
    if (!m) return
    const on = () => setWide(m.matches)
    m.addEventListener('change', on)
    return () => m.removeEventListener('change', on)
  }, [query])
  return wide
}
