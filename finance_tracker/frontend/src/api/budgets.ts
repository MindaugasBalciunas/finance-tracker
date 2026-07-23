import client from './client'
import type { Budget, BudgetInput, BudgetSettings, BudgetSettingsInput } from '../types'

export const budgetsApi = {
  list: async (): Promise<Budget[]> => {
    const { data } = await client.get<Budget[]>('/budgets')
    return data
  },

  create: async (input: BudgetInput): Promise<Budget> => {
    const { data } = await client.post<Budget>('/budgets', input)
    return data
  },

  update: async (id: number, input: BudgetInput): Promise<Budget> => {
    const { data } = await client.put<Budget>(`/budgets/${id}`, input)
    return data
  },

  delete: async (id: number): Promise<void> => {
    await client.delete(`/budgets/${id}`)
  },

  getSettings: async (): Promise<BudgetSettings> => {
    const { data } = await client.get<BudgetSettings>('/budgets/settings')
    return data
  },

  saveSettings: async (input: BudgetSettingsInput): Promise<BudgetSettings> => {
    const { data } = await client.put<BudgetSettings>('/budgets/settings', input)
    return data
  },

  labels: async (): Promise<string[]> => {
    const { data } = await client.get<string[]>('/labels')
    return data
  },

  rules: async (): Promise<{ id: number; label: string; category: string; comment_match: string }[]> => {
    const { data } = await client.get('/labels/rules')
    return data
  },

  // Runs every saved rule over all transactions — deterministic re-labeling
  // for historical records.
  reapplyRules: async (): Promise<{ relabeled: number; rules: number }> => {
    const { data } = await client.post('/labels/reapply')
    return data
  },

  // Bulk-tags matching historic transactions; optionally saves the rule so
  // future transactions are labeled automatically.
  applyLabel: async (input: {
    label: string
    category?: string
    comment_match?: string
    create_rule?: boolean
  }): Promise<{ labeled: number }> => {
    const { data } = await client.post('/labels/apply', input)
    return data
  },
}
