import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { bankingApi, type SaveBankSettingsInput, type StagedEdit, type StagedFilter } from '../api/banking'
import { transactionsApi } from '../api/transactions'
import { invalidateTransactionQueries } from './useTransactions'

export const BANK_SETTINGS_KEY = 'bank-settings'
export const BANK_CONNECTIONS_KEY = 'bank-connections'
export const BANK_STAGED_KEY = 'bank-staged'
export const BANK_ASPSPS_KEY = 'bank-aspsps'

// A sync, a commit and a dismiss all change the review list and the pending
// badge on every connection card. Keeping that in one helper is the same
// lesson invalidateTransactionQueries encodes: a missed key reads as "it
// didn't work" for five minutes, because staleTime is 5 min and there is no
// refetch on focus.
function invalidateBanking(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: [BANK_STAGED_KEY] })
  qc.invalidateQueries({ queryKey: [BANK_CONNECTIONS_KEY] })
}

export function useBankSettings() {
  return useQuery({
    queryKey: [BANK_SETTINGS_KEY],
    queryFn: () => bankingApi.getSettings(),
    staleTime: 60_000,
  })
}

export function useSaveBankSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SaveBankSettingsInput) => bankingApi.saveSettings(input),
    onSuccess: (data) => {
      qc.setQueryData([BANK_SETTINGS_KEY], data)
      // Credentials appearing (or going) flips the whole feature between 404
      // and live, so everything downstream has to be re-asked.
      qc.invalidateQueries({ queryKey: [BANK_CONNECTIONS_KEY] })
      qc.invalidateQueries({ queryKey: [BANK_ASPSPS_KEY] })
    },
  })
}

export function useBankConnections(enabled = true) {
  return useQuery({
    queryKey: [BANK_CONNECTIONS_KEY],
    queryFn: () => bankingApi.connections(),
    enabled,
  })
}

// The bank list is only needed while actually adding a bank — it is a
// provider round-trip, so it is not fetched on page load.
export function useASPSPs(enabled: boolean, country = 'LT') {
  return useQuery({
    queryKey: [BANK_ASPSPS_KEY, country],
    queryFn: () => bankingApi.aspsps(country),
    enabled,
    staleTime: 10 * 60_000,
  })
}

export function useConnectBank() {
  return useMutation({
    mutationFn: ({ name, country }: { name: string; country?: string }) =>
      bankingApi.connect(name, country),
  })
}

export function useBankCallback() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { code?: string; state?: string; url?: string }) => bankingApi.callback(input),
    onSuccess: () => invalidateBanking(qc),
  })
}

export function useDisconnectBank() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => bankingApi.disconnect(id),
    onSuccess: () => invalidateBanking(qc),
  })
}

export function useMapBankAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, accountKey }: { id: number; accountKey: string }) =>
      bankingApi.mapAccount(id, accountKey),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BANK_CONNECTIONS_KEY] }),
  })
}

export function useSyncBankAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, days }: { id: number; days?: number }) => bankingApi.sync(id, days),
    onSuccess: () => invalidateBanking(qc),
  })
}

export function useSyncAllBanks() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (days?: number) => bankingApi.syncAll(days),
    onSuccess: () => invalidateBanking(qc),
  })
}

export function useStagedTransactions(filter: StagedFilter = {}, enabled = true) {
  return useQuery({
    queryKey: [BANK_STAGED_KEY, filter],
    queryFn: () => bankingApi.staged(filter),
    enabled,
  })
}

// Editing touches only the review list — nothing has reached the ledger yet,
// so the transaction queries stay untouched.
export function useUpdateStaged() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, edit }: { id: number; edit: StagedEdit }) => bankingApi.updateStaged(id, edit),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BANK_STAGED_KEY] }),
  })
}

export function useCommitStaged() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => bankingApi.commit(ids),
    onSuccess: () => {
      invalidateBanking(qc)
      // A commit cuts one balance snapshot for everything it added, so the
      // stored balances, the trend and the allocation all move with it.
      invalidateTransactionQueries(qc)
    },
  })
}

export function useDismissStaged() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => bankingApi.dismiss(ids),
    onSuccess: () => invalidateBanking(qc),
  })
}

// A merge touches both the review list and the ledger row it links to.
export function useMergeStaged() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, transactionId }: { id: number; transactionId: number }) =>
      bankingApi.merge(id, transactionId),
    onSuccess: () => {
      invalidateBanking(qc)
      // false: a merge adds no transaction and moves no money, so the stored
      // balances are untouched — only the row's external id changed.
      invalidateTransactionQueries(qc, false)
    },
  })
}

export function useRestoreStaged() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => bankingApi.restore(ids),
    onSuccess: () => invalidateBanking(qc),
  })
}

// Undo after a commit: deleting the transactions puts their staged rows back
// on the review list, which the backend does on DELETE /transactions/batch.
export function useUndoCommit() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => transactionsApi.deleteBatch(ids),
    onSuccess: () => {
      invalidateBanking(qc)
      // Deleting the transactions does NOT remove the snapshot the commit
      // cut — balance history is a series of observations, and undoing the
      // rows does not un-observe it. Refresh anyway so what is shown is what
      // is stored, and say so in the UI.
      invalidateTransactionQueries(qc)
    },
  })
}
