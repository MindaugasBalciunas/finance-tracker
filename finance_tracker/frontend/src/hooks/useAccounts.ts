import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { accountsApi, type AccountInput } from '../api/accounts'
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

// Every account key → display name: the built-in labels plus added accounts.
export function useAccountLabels(): Record<string, string> {
  const added = useAddedAccounts()
  return useMemo(() => {
    const out: Record<string, string> = { ...ACCOUNT_LABELS }
    for (const a of added) out[a.key] = a.label
    return out
  }, [added])
}

function useInvalidateAccounts() {
  const qc = useQueryClient()
  return () => {
    qc.invalidateQueries({ queryKey: [ACCOUNTS_KEY] })
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
