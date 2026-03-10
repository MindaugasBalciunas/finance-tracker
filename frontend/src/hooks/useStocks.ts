import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { stocksApi } from '../api/stocks'
import type { CreateStockTradeInput } from '../types'

const TRADES_KEY = ['stock-trades']
const PORTFOLIO_KEY = ['stock-portfolio']

export function useStockTrades() {
  return useQuery({ queryKey: TRADES_KEY, queryFn: stocksApi.listAll })
}

export function useStockPortfolio() {
  return useQuery({ queryKey: PORTFOLIO_KEY, queryFn: stocksApi.getPortfolio })
}

export function useStockPrice(ticker: string, enabled: boolean) {
  return useQuery({
    queryKey: ['stock-price', ticker],
    queryFn: () => stocksApi.getPrice(ticker),
    staleTime: 5 * 60 * 1000,
    enabled: enabled && ticker.length > 0,
  })
}

export function useStockHistory(ticker: string, range: string, enabled: boolean) {
  return useQuery({
    queryKey: ['stock-history', ticker, range],
    queryFn: () => stocksApi.getHistory(ticker, range),
    staleTime: 10 * 60 * 1000,
    enabled: enabled && ticker.length > 0,
  })
}

export function useUsdEurRate() {
  const { data } = useQuery({
    queryKey: ['stock-price', 'EURUSD=X'],
    queryFn: () => stocksApi.getPrice('EURUSD=X'),
    staleTime: 5 * 60 * 1000,
  })
  // data.price is USD per 1 EUR (e.g. 1.08)
  // USD→EUR: divide by rate; EUR→USD: multiply by rate
  const rate = data?.price ?? null
  return {
    usdToEur: (usd: number): number | null => (rate != null ? usd / rate : null),
    eurToUsd: (eur: number): number | null => (rate != null ? eur * rate : null),
  }
}

export function useCreateStockTrade() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateStockTradeInput) => stocksApi.create(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: TRADES_KEY })
      qc.invalidateQueries({ queryKey: PORTFOLIO_KEY })
    },
  })
}

export function useUpdateStockTrade() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: Partial<CreateStockTradeInput> }) =>
      stocksApi.update(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: TRADES_KEY })
      qc.invalidateQueries({ queryKey: PORTFOLIO_KEY })
    },
  })
}

export function useDeleteStockTrade() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => stocksApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: TRADES_KEY })
      qc.invalidateQueries({ queryKey: PORTFOLIO_KEY })
    },
  })
}
