import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { accountsApi, type AccountInput } from '../api/accounts'
import { BALANCES_KEY, BALANCE_ALLOCATION_KEY, BALANCE_LATEST_KEY, BALANCE_PROJECTED_KEY } from './useBalances'
import { ACCOUNT_LABELS, type Account, type AccountGroup } from '../types'

export const ACCOUNTS_KEY = 'accounts'

export function useAccounts() {
  return useQuery({ queryKey: [ACCOUNTS_KEY], queryFn: accountsApi.list })
}

// Accounts added beyond the built-in columns, archived ones included —
// their history still shows wherever it has values.
export function useAddedAccounts(): Account[] {
  const { data } = useAccounts()
  return useMemo(() => (data ?? []).filter((a) => !a.builtin), [data])
}

export type AccountNamer = (key: string, fallback?: string) => string

// useAccountName returns the display name for an account key. Added accounts
// use their own label; a built-in uses its label only once the user renamed
// it, otherwise each screen's own wording (fallback) stays.
export function useAccountName(): AccountNamer {
  const { data } = useAccounts()
  return useMemo(() => {
    const byKey = new Map((data ?? []).map((a) => [a.key, a]))
    return (key: string, fallback?: string) => {
      const a = byKey.get(key)
      const builtinDefault = ACCOUNT_LABELS[key as keyof typeof ACCOUNT_LABELS]
      if (a && (!a.builtin || a.label !== builtinDefault)) return a.label
      return fallback ?? builtinDefault ?? key
    }
  }, [data])
}

// useAccountGroupOf returns where each account counts now — what a built-in
// the user moved to another group needs on the per-account lists.
export function useAccountGroupOf(): (key: string) => AccountGroup | undefined {
  const { data } = useAccounts()
  return useMemo(() => {
    const byKey = new Map((data ?? []).map((a) => [a.key, a.group]))
    return (key: string) => byKey.get(key)
  }, [data])
}

// Every account key → display name: built-ins (renamed or not) plus added.
export function useAccountLabels(): Record<string, string> {
  const { data } = useAccounts()
  const name = useAccountName()
  return useMemo(() => {
    const out: Record<string, string> = {}
    for (const k of Object.keys(ACCOUNT_LABELS)) out[k] = name(k)
    for (const a of data ?? []) out[a.key] = name(a.key)
    return out
  }, [data, name])
}

function useInvalidateAccounts() {
  const qc = useQueryClient()
  return () => {
    qc.invalidateQueries({ queryKey: [ACCOUNTS_KEY] })
    // Moving or renaming an account changes every balance's group sums and
    // the allocation names — refetch them, or the cards show the old split.
    for (const k of [BALANCES_KEY, BALANCE_LATEST_KEY, BALANCE_PROJECTED_KEY, BALANCE_ALLOCATION_KEY]) {
      qc.invalidateQueries({ queryKey: [k] })
    }
    // Bank mapping offers added accounts as targets.
    qc.invalidateQueries({ queryKey: ['bank-connections'] })
  }
}

export function useCreateAccount() {
  const invalidate = useInvalidateAccounts()
  return useMutation({
    mutationFn: (input: { label: string; group: AccountGroup }) => accountsApi.create(input),
    onSuccess: invalidate,
  })
}

export function useUpdateAccount() {
  const invalidate = useInvalidateAccounts()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: AccountInput }) => accountsApi.update(id, input),
    onSuccess: invalidate,
  })
}
