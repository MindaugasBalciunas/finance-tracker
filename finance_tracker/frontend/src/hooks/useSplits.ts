import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { splitsApi, type SplitPart } from '../api/splits'
import { invalidateTransactionQueries } from './useTransactions'

export const OWED_KEY = 'owed'

export function useOwed() {
  return useQuery({ queryKey: [OWED_KEY], queryFn: splitsApi.owed })
}

// Splits and repayments re-describe money already in the ledger; balances
// never move, so only the transaction views and the owed list refresh.
function useInvalidate() {
  const qc = useQueryClient()
  return () => {
    invalidateTransactionQueries(qc, false)
    qc.invalidateQueries({ queryKey: [OWED_KEY] })
  }
}

export function useSplitTransaction() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: ({ id, parts }: { id: number; parts: SplitPart[] }) => splitsApi.split(id, parts),
    onSuccess: invalidate,
  })
}

export function useUnsplitTransaction() {
  const invalidate = useInvalidate()
  return useMutation({ mutationFn: (id: number) => splitsApi.unsplit(id), onSuccess: invalidate })
}

export function useMarkRepayment() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: ({ id, person }: { id: number; person: string }) => splitsApi.repayment(id, person),
    onSuccess: invalidate,
  })
}
