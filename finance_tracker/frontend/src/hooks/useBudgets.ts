import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { budgetsApi } from '../api/budgets'
import { TRANSACTIONS_KEY, SUMMARY_KEY } from './useTransactions'
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
    // Rewrites transaction labels, so every label-derived cache is stale.
    onSuccess: () => invalidateLabelWorld(qc),
  })
}

export function useApplyLabel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { label: string; category?: string; comment_match?: string; create_rule?: boolean }) =>
      budgetsApi.applyLabel(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
      qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
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

export function useLabelStats() {
  return useQuery({
    queryKey: ['label-stats'],
    queryFn: () => budgetsApi.labelStats(),
  })
}

export function useLabelSuggestions() {
  return useQuery({
    queryKey: ['label-suggestions'],
    queryFn: () => budgetsApi.labelSuggestions(),
  })
}

// A rename/delete rewrites transactions, rules AND budgets — invalidate all
// label-derived caches so every page reflects the new vocabulary.
function invalidateLabelWorld(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: [TRANSACTIONS_KEY] })
  qc.invalidateQueries({ queryKey: [SUMMARY_KEY] })
  qc.invalidateQueries({ queryKey: [BUDGETS_KEY] })
  qc.invalidateQueries({ queryKey: ['labels'] })
  qc.invalidateQueries({ queryKey: ['label-rules'] })
  qc.invalidateQueries({ queryKey: ['label-stats'] })
  qc.invalidateQueries({ queryKey: ['label-suggestions'] })
}

export function useRenameLabel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { from: string; to: string }) => budgetsApi.renameLabel(input),
    onSuccess: () => invalidateLabelWorld(qc),
  })
}

export function useDeleteLabel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (label: string) => budgetsApi.deleteLabel(label),
    onSuccess: () => invalidateLabelWorld(qc),
  })
}
