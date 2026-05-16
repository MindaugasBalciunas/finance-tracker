import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { balancesApi } from '../api/balances'
import type { BalanceFilter, CreateBalanceInput } from '../types'

export const BALANCES_KEY = 'balances'
export const BALANCE_LATEST_KEY = 'balance-latest'
export const BALANCE_TREND_KEY = 'balance-trend'
export const BALANCE_ALLOCATION_KEY = 'balance-allocation'

export function useBalances(filter: BalanceFilter = {}, btcPrice?: number | null) {
  return useQuery({
    queryKey: [BALANCES_KEY, filter, btcPrice ?? 0],
    queryFn: () => balancesApi.list(filter, btcPrice ?? undefined),
  })
}

export function useLatestBalance(btcPrice?: number | null) {
  return useQuery({
    queryKey: [BALANCE_LATEST_KEY, btcPrice ?? 0],
    queryFn: () => balancesApi.getLatest(btcPrice ?? undefined),
    retry: false,
  })
}

export function useBalanceTrend(filter: BalanceFilter = {}) {
  return useQuery({
    queryKey: [BALANCE_TREND_KEY, filter],
    queryFn: () => balancesApi.getTrend(filter),
  })
}

export function useAccountAllocation() {
  return useQuery({
    queryKey: [BALANCE_ALLOCATION_KEY],
    queryFn: () => balancesApi.getAllocation(),
  })
}

export function useCreateBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateBalanceInput) => balancesApi.create(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BALANCES_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_LATEST_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_TREND_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_ALLOCATION_KEY] })
    },
  })
}

export function useUpdateBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: CreateBalanceInput }) =>
      balancesApi.update(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BALANCES_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_LATEST_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_TREND_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_ALLOCATION_KEY] })
    },
  })
}

export function useDeleteBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => balancesApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BALANCES_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_LATEST_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_TREND_KEY] })
      qc.invalidateQueries({ queryKey: [BALANCE_ALLOCATION_KEY] })
    },
  })
}
