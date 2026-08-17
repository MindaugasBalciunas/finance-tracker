import client from './client'

export interface AIInsight {
  id: number
  content: string
  created_at: string
}

export const insightsApi = {
  getLatest: async (): Promise<AIInsight> => {
    const { data } = await client.get<AIInsight>('/insights/latest')
    return data
  },

  // Optional date range scopes the period-sensitive sections of the overview.
  generate: async (range?: { date_from?: string; date_to?: string }): Promise<AIInsight> => {
    const { data } = await client.post<AIInsight>('/insights/generate', range ?? {})
    return data
  },

  list: async (): Promise<AIInsight[]> => {
    const { data } = await client.get<AIInsight[]>('/insights')
    return data
  },
}

export interface AISettings {
  gateway_url: string
  model: string
  has_key: boolean
  updated_at: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

// LLM gateway (nexos.ai by default): settings, connectivity test, chat.
export const aiApi = {
  getSettings: async (): Promise<AISettings> => {
    const { data } = await client.get<AISettings>('/ai/settings')
    return data
  },

  // Empty api_key keeps the stored key; clear_key removes it.
  saveSettings: async (input: {
    gateway_url: string
    model: string
    api_key?: string
    clear_key?: boolean
  }): Promise<AISettings> => {
    const { data } = await client.put<AISettings>('/ai/settings', input)
    return data
  },

  test: async (): Promise<void> => {
    await client.post('/ai/test')
  },

  // History lives server-side so the conversation follows the user across
  // phone and browser — only the new message travels up.
  chat: async (message: string): Promise<string> => {
    const { data } = await client.post<{ reply: string }>('/ai/chat', { message })
    return data.reply
  },

  chatHistory: async (): Promise<ChatMessage[]> => {
    const { data } = await client.get<{ messages: { role: 'user' | 'assistant'; content: string }[] }>(
      '/ai/chat/history',
    )
    return (data.messages ?? []).map((m) => ({ role: m.role, content: m.content }))
  },

  clearChat: async (): Promise<void> => {
    await client.delete('/ai/chat/history')
  },

  // Single-transaction AI assist: labels + a cleaner description learned
  // from the user's own history. Suggestion only — nothing is written.
  assistTransaction: async (input: {
    date?: string
    type?: string
    category?: string
    amount?: number
    comment?: string
    labels?: string
  }): Promise<{ labels: string[]; comment: string; note?: string }> => {
    const { data } = await client.post('/ai/assist-transaction', input)
    return data
  },

  // Bulk reindex: mode 'unlabeled' tags rows with no labels; mode 'review'
  // audits labeled rows and proposes remaps. Suggestion-only until applied.
  labelReindex: async (mode: 'unlabeled' | 'review', offset = 0): Promise<{
    suggestions: {
      id: number; date: string; type: string; amount: number; category: string; comment: string
      current?: string[]; add: string[]; remove?: string[]; reason?: string
    }[]
    scanned: number
    remaining_unlabeled: number
  }> => {
    const { data } = await client.post('/ai/label-reindex', { mode, offset })
    return data
  },

  // Writes the user-approved changes (fixed labels can never be removed).
  applyLabelSuggestions: async (items: { id: number; add: string[]; remove?: string[] }[]): Promise<number> => {
    const { data } = await client.post<{ applied: number }>('/ai/label-reindex/apply', { items })
    return data.applied
  },

  // Short AI review of the view the user has open; server-cached 15 min per
  // view+period, so tab-hopping is free.
  viewSummary: async (
    view: string,
    range: { date_from?: string; date_to?: string },
    refresh = false,
  ): Promise<string> => {
    const { data } = await client.get<{ summary: string }>('/ai/view-summary', {
      params: { view, ...range, ...(refresh ? { refresh: '1' } : {}) },
    })
    return data.summary
  },
}
