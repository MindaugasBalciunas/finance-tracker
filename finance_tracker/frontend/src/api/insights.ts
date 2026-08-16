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

  generate: async (): Promise<AIInsight> => {
    const { data } = await client.post<AIInsight>('/insights/generate')
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

  chat: async (messages: ChatMessage[]): Promise<string> => {
    const { data } = await client.post<{ reply: string }>('/ai/chat', { messages })
    return data.reply
  },
}
