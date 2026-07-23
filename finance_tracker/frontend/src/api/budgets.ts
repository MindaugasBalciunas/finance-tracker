import client from './client'
import type { Budget, BudgetInput } from '../types'

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

  labels: async (): Promise<string[]> => {
    const { data } = await client.get<string[]>('/labels')
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
