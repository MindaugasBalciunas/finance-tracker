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
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },

  list: async (): Promise<AIInsight[]> => {
    const { data } = await client.get<AIInsight[]>('/insights')
    return data
  },
}

// 'gateway' is an OpenAI-style gateway behind a Bearer token (nexos.ai and
// friends); 'anthropic' is the first-party Claude API.
export type AIProvider = 'gateway' | 'anthropic'

export interface AISettings {
  gateway_url: string
  model: string
  // Always resolved by the backend — never the empty "infer from URL" value.
  provider: AIProvider
  // Master switch. When false every AI surface is hidden and the AI
  // endpoints 404; only /ai/settings and /ai/models stay reachable.
  enabled: boolean
  has_key: boolean
  updated_at: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface RuleSuggestion {
  action: 'add' | 'update' | 'delete'
  rule_id?: number
  label: string
  comment_match?: string
  category?: string
  old_comment_match?: string
  old_category?: string
  reason?: string
  matches: number
  would_label: number
}

// A draft transaction extracted from a scanned photo. Empty strings / a
// non-positive amount mean the scan couldn't read that field — the form
// keeps its own default rather than overwriting with a blank.
export interface TransactionScan {
  type: 'expense' | 'income' | 'investment'
  date: string
  amount: number
  comment: string
  category: string
  labels: string[]
  // Account recognized from the image (bank-app screenshots), '' when unknown.
  debit_account: string
  credit_account: string
  note: string
}

// LLM provider (nexos.ai gateway by default, or the Claude API directly):
// settings, connectivity test, chat.
export const aiApi = {
  getSettings: async (): Promise<AISettings> => {
    const { data } = await client.get<AISettings>('/ai/settings')
    return data
  },

  // Every field is optional and omitted ones keep their stored value — the
  // enable/disable toggle PUTs nothing but { enabled }. Empty api_key keeps
  // the stored key; clear_key removes it.
  saveSettings: async (input: {
    gateway_url?: string
    model?: string
    provider?: AIProvider
    enabled?: boolean
    api_key?: string
    clear_key?: boolean
  }): Promise<AISettings> => {
    const { data } = await client.put<AISettings>('/ai/settings', input)
    return data
  },

  test: async (): Promise<void> => {
    await client.post('/ai/test')
  },

  // The model catalogue exposed by the gateway. When the gateway has no
  // models endpoint the backend returns ok:false with an empty list — the UI
  // falls back to a free-text field. Never throws: any failure looks like an
  // empty, not-ok list so the caller can degrade gracefully.
  spend: async (): Promise<SpendReport> => {
    const { data } = await client.get<SpendReport>('/ai/spend')
    return data
  },

  addTopUp: async (input: { amount_usd: number; note?: string; occurred_on?: string }): Promise<void> => {
    await client.post('/ai/spend/topups', input)
  },

  deleteTopUp: async (id: number): Promise<void> => {
    await client.delete(`/ai/spend/topups/${id}`)
  },

  models: async (): Promise<{ models: string[]; ok: boolean }> => {
    try {
      const { data } = await client.get<{ models?: string[]; ok?: boolean }>('/ai/models')
      return { models: data?.models ?? [], ok: !!data?.ok }
    } catch {
      return { models: [], ok: false }
    }
  },

  // History lives server-side so the conversation follows the user across
  // phone and browser — only the new message travels up. An optional image
  // (receipt/screenshot) switches the request to multipart so the AI can read
  // it; message may be empty when a file is attached. Returns the reply plus
  // what the model spent on this answer (summed over tool rounds).
  chat: async (
    message: string,
    file?: File,
  ): Promise<{ reply: string; costUsd: number; costEstimated: boolean; inputTokens: number; outputTokens: number }> => {
    // The server streams whitespace heartbeats during the long agentic call
    // and commits a 200 up front, so failures arrive as {error} in the body
    // (not a status) — check for it. Leading heartbeat whitespace parses away.
    type ChatResponse = {
      reply?: string
      error?: string
      cost_usd?: number
      cost_estimated?: boolean
      input_tokens?: number
      output_tokens?: number
    }
    let data: ChatResponse
    if (file) {
      const form = new FormData()
      form.append('message', message)
      form.append('file', file)
      ;({ data } = await client.post<ChatResponse>('/ai/chat', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      }))
    } else {
      ;({ data } = await client.post<ChatResponse>('/ai/chat', { message }))
    }
    if (data?.error) throw new Error(data.error)
    return {
      reply: data?.reply ?? '',
      costUsd: data?.cost_usd ?? 0,
      costEstimated: data?.cost_estimated ?? false,
      inputTokens: data?.input_tokens ?? 0,
      outputTokens: data?.output_tokens ?? 0,
    }
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

  // Scan a photo (receipt/invoice/screenshot) into a draft transaction.
  // Suggestion only — the returned fields prefill the form, nothing is saved.
  scanTransaction: async (file: File): Promise<TransactionScan> => {
    const form = new FormData()
    form.append('file', file)
    const { data } = await client.post<TransactionScan>('/ai/scan-transaction', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
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
    // Raw source behind the row (a bank narrative) — context for the model,
    // never written anywhere.
    context?: string
  }): Promise<{ labels: string[]; comment: string; note?: string }> => {
    const { data } = await client.post('/ai/assist-transaction', input)
    if (data && (data as any).error) throw new Error((data as any).error)
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
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },

  // Writes the user-approved changes (fixed labels can never be removed).
  applyLabelSuggestions: async (items: { id: number; add: string[]; remove?: string[] }[]): Promise<number> => {
    const { data } = await client.post<{ applied: number }>('/ai/label-reindex/apply', { items })
    if (data && (data as any).error) throw new Error((data as any).error)
    return data.applied
  },

  // The user's CFO-context document — injected into every AI call and
  // exposed read-only via MCP (get_user_context).
  getContext: async (): Promise<{ content: string; updated_at: string }> => {
    const { data } = await client.get('/ai/context')
    return data
  },

  saveContext: async (content: string): Promise<{ content: string; updated_at: string }> => {
    const { data } = await client.put('/ai/context', { content })
    return data
  },

  // AI audit of the auto-labeling rule set: proposes new rules for recurring
  // uncovered merchants, updates for misfiring patterns and cleanup of dead
  // rules. Suggestion-only — every item carries live match counts.
  ruleReview: async (): Promise<{ suggestions: RuleSuggestion[]; rules_scanned: number }> => {
    const { data } = await client.post('/ai/rule-review', {})
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },

  // Writes the user-approved rule changes; adds/updates also label matching
  // history (add-only). Returns what was touched.
  applyRuleSuggestions: async (
    items: { action: string; rule_id?: number; label: string; comment_match?: string; category?: string }[],
  ): Promise<{ added: number; updated: number; deleted: number; relabeled: number }> => {
    const { data } = await client.post('/ai/rule-review/apply', { items })
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },

  // Short AI review of the view the user has open; server-cached 15 min per
  // view+period, so tab-hopping is free.
  viewSummary: async (
    view: string,
    range: { date_from?: string; date_to?: string },
    refresh = false,
  ): Promise<{ summary: string; costUsd: number; costEstimated: boolean; inputTokens: number; outputTokens: number }> => {
    const { data } = await client.get<{
      summary: string
      cost_usd?: number
      cost_estimated?: boolean
      input_tokens?: number
      output_tokens?: number
    }>('/ai/view-summary', {
      params: { view, ...range, ...(refresh ? { refresh: '1' } : {}) },
    })
    if (data && (data as any).error) throw new Error((data as any).error)
    return {
      summary: data.summary,
      costUsd: data.cost_usd ?? 0,
      costEstimated: data.cost_estimated ?? false,
      inputTokens: data.input_tokens ?? 0,
      outputTokens: data.output_tokens ?? 0,
    }
  },

  // Saved AI investment forecast: GET is free (persisted server-side),
  // generateForecast spends gateway tokens and replaces the saved one.
  getForecast: async (): Promise<ForecastResponse> => {
    const { data } = await client.get<ForecastResponse>('/ai/forecast')
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },

  generateForecast: async (): Promise<ForecastResponse> => {
    const { data } = await client.post<ForecastResponse>('/ai/forecast', {}, { timeout: 180_000 })
    if (data && (data as any).error) throw new Error((data as any).error)
    return data
  },
}

export interface ForecastScenario {
  name: string
  annual_return: number
  rationale: string
}

export interface ForecastAllocation {
  bucket: string
  current_pct: number
  target_pct: number
  action: string
}

export interface ForecastBucket {
  bucket: 'free_cash' | 'investments' | 'pensions' | 'crypto'
  annual_return: number
  monthly_flow: number
  note?: string
}

export interface ForecastDoc {
  generated_at: string
  current: {
    total_eur: number
    free_cash: number
    investments: number
    pensions: number
    crypto: number
    monthly_income_avg: number
    monthly_expense_avg: number
    monthly_invest_avg: number
  }
  ai: {
    narrative: string
    monthly_contribution: number
    scenarios: ForecastScenario[]
    bucket_projections?: ForecastBucket[]
    target_allocation: ForecastAllocation[]
    actions: string[]
  }
}

export interface ForecastResponse {
  exists: boolean
  forecast?: ForecastDoc
  model?: string
  cost_usd?: number
  cost_estimated?: boolean
  input_tokens?: number
  output_tokens?: number
  created_at?: string
}

export type TopUp = {
  id: number
  amount_usd: number
  note: string
  occurred_on: string
}

export type SpendReport = {
  this_month: number
  last_30_days: number
  all_time: number
  /** null until a top-up is recorded — there is nothing to count down from. */
  remaining: number | null
  topped_up: number
  since_date?: string
  by_kind: Record<string, number>
  top_ups: TopUp[]
}
