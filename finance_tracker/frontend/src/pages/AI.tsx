import { useEffect, useRef, useState } from 'react'
import { formatAICost, AI_COST_ESTIMATE_HINT } from '../utils/aiCost'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi, type ChatMessage } from '../api/insights'
import { useAIAvailable, useAIUnavailableReason } from '../hooks/useInsights'
import { invalidateTransactionQueries } from '../hooks/useTransactions'
import AINav from '../components/ui/AINav'
import Markdown from '../components/ui/Markdown'

// A rendered chat turn. Server history is plain {role, content}; the local
// optimistic echo may also carry an object-URL preview of an image the user
// attached (the image itself is never stored server-side). Assistant turns
// from the live send also carry what that answer cost — history from the
// server has none (not stored), so the badge is simply omitted there.
type ChatEntry = ChatMessage & {
  imageUrl?: string
  costUsd?: number
  costEstimated?: boolean
  inputTokens?: number
  outputTokens?: number
}

// Cost as `$` + up to 4 decimals with trailing zeros trimmed (e.g. `$0.045`,
// `$0.3`).
// Compact token count: 6553 → `6.6k`, >1M → `1.2M`.
function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${parseFloat((n / 1_000_000).toFixed(1))}M`
  if (n >= 1_000) return `${parseFloat((n / 1_000).toFixed(1))}k`
  return `${n}`
}

// Full-screen AI chat grounded in the full financial dataset. History is
// stored server-side (follows the user between phone and browser), and
// assistant answers render as markdown — tables, lists, bold figures.
// Analysis and gateway settings live on the Overview tab.
export default function AI() {
  const configured = useAIAvailable()
  const unavailable = useAIUnavailableReason()

  const qc = useQueryClient()
  const { data: serverHistory } = useQuery({
    queryKey: ['ai-chat-history'],
    queryFn: aiApi.chatHistory,
    // With AI off the endpoint is gone — don't fetch into a 404.
    enabled: configured,
  })
  // Local copy so an in-flight turn renders immediately; re-synced whenever
  // the server history lands (initial load or another device's turns).
  const [messages, setMessages] = useState<ChatEntry[]>([])
  useEffect(() => {
    if (!serverHistory) return
    // The authoritative history is text-only — free any preview URLs the
    // optimistic echoes were holding before they're replaced. Carry the
    // cost/token badge from this session's just-answered assistant turns onto
    // the matching history entries (history doesn't store cost), so the badge
    // survives the post-send resync instead of vanishing.
    setMessages((prev) => {
      prev.forEach((m) => m.imageUrl && URL.revokeObjectURL(m.imageUrl))
      return serverHistory.map((sm) => {
        if (sm.role !== 'assistant') return sm
        const local = prev.find(
          (p) => p.role === 'assistant' && p.content === sm.content &&
            ((p.costUsd ?? 0) > 0 || (p.inputTokens ?? 0) > 0 || (p.outputTokens ?? 0) > 0),
        )
        return local
          ? { ...sm, costUsd: local.costUsd, costEstimated: local.costEstimated, inputTokens: local.inputTokens, outputTokens: local.outputTokens }
          : sm
      })
    })
  }, [serverHistory])

  const [draft, setDraft] = useState('')
  const [thinking, setThinking] = useState(false)
  const [chatError, setChatError] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  // Pending image attachment: the File plus an object URL for its preview.
  const [image, setImage] = useState<File | null>(null)
  const [imageUrl, setImageUrl] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  // Release any live preview URL on unmount (covers a pending attachment or an
  // echo whose server resync hasn't landed yet).
  const imageUrlRef = useRef('')
  imageUrlRef.current = imageUrl
  useEffect(() => () => { if (imageUrlRef.current) URL.revokeObjectURL(imageUrlRef.current) }, [])

  const onPickImage = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = '' // let the same file be re-picked after removal
    if (!f) return
    if (imageUrl) URL.revokeObjectURL(imageUrl)
    setImage(f)
    setImageUrl(URL.createObjectURL(f))
  }

  const clearImage = () => {
    if (imageUrl) URL.revokeObjectURL(imageUrl)
    setImage(null)
    setImageUrl('')
  }

  useEffect(() => {
    // Don't yank the page down on mount — only follow along once the
    // conversation has content.
    if (messages.length === 0 && !thinking) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  const send = async () => {
    const text = draft.trim()
    // An attached image alone is enough — text may be empty in that case.
    if ((!text && !image) || thinking || !configured) return
    const attached = image
    const attachedUrl = imageUrl
    setMessages((m) => [...m, { role: 'user', content: text, imageUrl: attachedUrl || undefined }])
    setDraft('')
    // Detach without revoking — the echo above still renders this preview until
    // the server resync (or a failure restore) takes over.
    setImage(null)
    setImageUrl('')
    setChatError('')
    setThinking(true)
    try {
      const res = await aiApi.chat(text, attached ?? undefined)
      setMessages((m) => [...m, {
        role: 'assistant',
        content: res.reply,
        costUsd: res.costUsd,
        costEstimated: res.costEstimated,
        inputTokens: res.inputTokens,
        outputTokens: res.outputTokens,
      }])
      qc.invalidateQueries({ queryKey: ['ai-chat-history'] })
      // The chat has write tools (create_transaction and friends), so a turn
      // can have changed the ledger and the balances behind our back. Caches
      // live 5 minutes with no refocus refetch, so without this the user
      // books a receipt here and the dashboard still shows the old balance.
      invalidateTransactionQueries(qc)
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setChatError(e.response?.data?.error ?? e.message ?? 'Chat failed')
      // The failed turn was not persisted server-side — drop the local echo
      // so the view matches the stored history, and restore the draft + image.
      setMessages((m) => (m.length > 0 && m[m.length - 1].role === 'user' ? m.slice(0, -1) : m))
      setDraft(text)
      setImage(attached)
      setImageUrl(attachedUrl)
    } finally {
      setThinking(false)
    }
  }

  const clearChat = async () => {
    try {
      await aiApi.clearChat()
      setMessages([])
      setChatError('')
      qc.invalidateQueries({ queryKey: ['ai-chat-history'] })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setChatError(e.response?.data?.error ?? e.message ?? 'Clear failed')
    }
  }

  return (
    // Full-screen: the column fills the viewport under the app header
    // (mobile also reserves the bottom tab bar), the message list scrolls,
    // the composer stays pinned.
    <div className="mx-auto max-w-4xl flex flex-col h-[calc(100dvh-13.25rem)] md:h-[calc(100dvh-10.25rem)]">
      <div className="shrink-0 pb-3">
        <AINav />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 pb-3">
        <div className="min-w-0">
          <h1 className="text-xl font-bold text-gray-900">💬 Chat</h1>
          <p className="text-xs text-gray-400 truncate">
            Queries your data live while answering
          </p>
        </div>
        <span className="flex flex-wrap items-center justify-end gap-2">
          {messages.length > 0 && (
            <button onClick={clearChat} className="text-xs text-gray-400 hover:text-red-500">
              Clear chat
            </button>
          )}
        </span>
      </div>

      <div className="flex-1 min-h-0 bg-white rounded-2xl border border-gray-100 shadow-sm flex flex-col">
        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
          {messages.length === 0 && !thinking && (
            <div className="text-sm text-gray-400 py-10 text-center space-y-3">
              <p>
                {configured
                  ? 'Ask anything — the AI can search your transactions and query budgets, balances and live stock prices while answering.'
                  : unavailable === 'off'
                    ? 'AI features are switched off. Turn them back on with the ⚙️ button above.'
                    : 'Add your provider and API key with the ⚙️ button above to start chatting.'}
              </p>
              {configured && (
                <div className="flex flex-wrap justify-center gap-1.5">
                  {['Give me my daily status update', 'How am I tracking against my budgets this month?', 'How are my stock positions doing?', 'Where can I save €200/month?'].map((q) => (
                    <button
                      key={q}
                      onClick={() => setDraft(q)}
                      className="text-xs border border-gray-200 rounded-full px-2.5 py-1 text-gray-500 hover:bg-gray-50"
                    >
                      {q}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
          {messages.map((m, i) => {
            // Per-answer cost/token badge — live assistant turns only; combine
            // input+output tokens, and only show when there's something to show.
            const tokens = (m.inputTokens ?? 0) + (m.outputTokens ?? 0)
            const cost = m.costUsd ?? 0
            const badge = m.role === 'assistant'
              ? [formatAICost(cost, m.costEstimated), tokens > 0 ? `${fmtTokens(tokens)} tokens` : null]
                  .filter(Boolean)
                  .join(' · ')
              : ''
            return (
              <div key={i} className={`flex flex-col ${m.role === 'user' ? 'items-end' : 'items-start'}`}>
                <div
                  className={`max-w-[90%] sm:max-w-[85%] rounded-2xl px-3.5 py-2 min-w-0 ${
                    m.role === 'user'
                      ? 'bg-indigo-600 text-white rounded-br-sm'
                      : 'bg-gray-100 text-gray-800 rounded-bl-sm'
                  }`}
                >
                  {m.role === 'assistant' ? (
                    <Markdown>{m.content}</Markdown>
                  ) : (
                    <div className="space-y-1.5">
                      {m.imageUrl && (
                        <img
                          src={m.imageUrl}
                          alt="attachment"
                          className="max-h-40 rounded-lg"
                        />
                      )}
                      {m.content
                        ? <p className="text-sm whitespace-pre-wrap leading-relaxed">{m.content}</p>
                        : !m.imageUrl && <p className="text-sm text-indigo-100">📷 image</p>}
                    </div>
                  )}
                </div>
                {badge && (
                  <span
                    className="mt-1 px-1 text-[11px] text-gray-400"
                    title={m.costEstimated ? AI_COST_ESTIMATE_HINT : undefined}
                  >
                    {badge}
                  </span>
                )}
              </div>
            )
          })}
          {thinking && (
            <div className="flex justify-start">
              <div className="bg-gray-100 rounded-2xl rounded-bl-sm px-3.5 py-2.5 text-sm text-gray-400">
                <span className="animate-pulse">Thinking…</span>
              </div>
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        {chatError && <p className="px-4 pb-1 text-xs text-red-600">{chatError}</p>}
        <div className="border-t border-gray-100">
          {imageUrl && (
            <div className="px-3 pt-3">
              <div className="relative inline-block">
                <img
                  src={imageUrl}
                  alt="attachment preview"
                  className="h-16 w-16 object-cover rounded-lg border border-gray-200"
                />
                <button
                  type="button"
                  onClick={clearImage}
                  aria-label="Remove image"
                  className="absolute -top-1.5 -right-1.5 h-5 w-5 rounded-full bg-gray-700 text-white text-[11px] leading-none flex items-center justify-center shadow"
                >
                  ✕
                </button>
              </div>
            </div>
          )}
          <div className="p-3 flex items-end gap-2">
            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              capture="environment"
              onChange={onPickImage}
              className="hidden"
            />
            <button
              type="button"
              onClick={() => fileRef.current?.click()}
              disabled={!configured || thinking}
              aria-label="Attach image"
              className="shrink-0 px-3 py-2.5 rounded-xl border border-gray-200 text-lg leading-none text-gray-500 hover:bg-gray-50 disabled:opacity-40"
            >
              📎
            </button>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  send()
                }
              }}
              rows={draft.includes('\n') ? 3 : 1}
              placeholder={
                configured
                  ? 'Ask about your finances… (Enter to send)'
                  : unavailable === 'off' ? 'AI features are off' : 'Set up your AI provider first'
              }
              disabled={!configured || thinking}
              className="flex-1 min-w-0 resize-none text-sm border border-gray-200 rounded-xl px-3 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-200 disabled:bg-gray-50"
            />
            <button
              onClick={send}
              disabled={!configured || thinking || (!draft.trim() && !image)}
              className="px-4 py-2.5 rounded-xl bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-40"
            >
              Send
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
