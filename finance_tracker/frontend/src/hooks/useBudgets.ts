import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { budgetsApi } from '../api/budgets'
import { TRANSACTIONS_KEY } from './useTransactions'
import type { BudgetInput, BudgetSettingsInput } from '../types'

export const BUDGETS_KEY = 'budgets'

export function useBudgets() {
  return useQuery({
    queryKey: [BUDGETS_KEY],
    queryFn: () => budgetsApi.list(),
  })
}

export function useCreateBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: BudgetInput) => budgetsApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BUDGETS_KEY] }),
  })
}

export function useUpdateBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: BudgetInput }) => budgetsApi.update(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BUDGETS_KEY] }),
  })
}

export function useDeleteBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => budgetsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BUDGETS_KEY] }),
  })
}

export function useLabels(category?: string) {
  return useQuery({
    queryKey: ['labels', category ?? ''],
    queryFn: () => budgetsApi.labels(category),
    staleTime: 60_000,
  })
}

export function useLabelRules() {
  return useQuery({
    queryKey: ['label-rules'],
    queryFn: () => budgetsApi.rules(),
    staleTime: 60_000,
  })
}

export const BUDGET_SETTINGS_KEY = 'budget-settings'

export function useBudgetSettings() {
  return useQuery({
    queryKey: [BUDGET_SETTINGS_KEY],
    queryFn: () => budgetsApi.getSettings(),
  })
}

export function useSaveBudgetSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: BudgetSettingsInput) => budgetsApi.saveSettings(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: [BUDGET_SETTINGS_KEY] }),
  })
}

export function useReapplyRules() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => budgetsApi.reapplyRules(),
    onSuccess: () => qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] }),
  })
}

export function useApplyLabel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { label: string; category?: string; comment_match?: string; create_rule?: boolean }) =>
      budgetsApi.applyLabel(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
      qc.invalidateQueries({ queryKey: ['label-rules'] })
      qc.invalidateQueries({ queryKey: ['labels'] })
    },
  })
}

export function useDeleteRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => budgetsApi.deleteRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['label-rules'] }),
  })
}
