import { useEffect, useRef, useState } from 'react'
import { aiApi, type ChatMessage } from '../api/insights'
import { useAISettings, useSaveAISettings } from '../hooks/useInsights'
import AIInsightCard from '../components/ui/AIInsightCard'

const CHAT_STORE = 'ai-chat-history'

// Only well-formed turns survive a reload — a null/legacy entry would crash
// the render, and a blank or foreign-role entry would 400 every later send
// (the backend replays the whole history each turn).
function loadChat(): ChatMessage[] {
  try {
    const raw = localStorage.getItem(CHAT_STORE)
    const parsed = raw ? JSON.parse(raw) : []
    if (!Array.isArray(parsed)) return []
    return parsed.filter(
      (m): m is ChatMessage =>
        m && (m.role === 'user' || m.role === 'assistant') &&
        typeof m.content === 'string' && m.content.trim() !== '',
    )
  } catch {
    return []
  }
}

// AI page: chat grounded in the full financial dataset, the one-shot
// analysis card, and the nexos.ai gateway configuration.
export default function AI() {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [autoOpened, setAutoOpened] = useState(false)

  // Open the settings panel once on first load when nothing is configured,
  // then leave it under the user's control (so the ✕ actually closes it).
  useEffect(() => {
    if (settings && !configured && !autoOpened) {
      setSettingsOpen(true)
      setAutoOpened(true)
    }
  }, [settings, configured, autoOpened])

  const [messages, setMessages] = useState<ChatMessage[]>(loadChat)
  const [draft, setDraft] = useState('')
  const [thinking, setThinking] = useState(false)
  const [chatError, setChatError] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    localStorage.setItem(CHAT_STORE, JSON.stringify(messages.slice(-60)))
  }, [messages])

  useEffect(() => {
    // Don't yank the page down on mount — only follow along once the
    // conversation has content.
    if (messages.length === 0 && !thinking) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  const send = async () => {
    const text = draft.trim()
    if (!text || thinking || !configured) return
    const next: ChatMessage[] = [...messages, { role: 'user', content: text }]
    setMessages(next)
    setDraft('')
    setChatError('')
    setThinking(true)
    try {
      const reply = await aiApi.chat(next)
      setMessages([...next, { role: 'assistant', content: reply }])
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setChatError(e.response?.data?.error ?? e.message ?? 'Chat failed')
    } finally {
      setThinking(false)
    }
  }

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">✦ AI</h1>
          <p className="text-xs text-gray-400">
            Chat and analysis grounded in your full financial history
            {configured && settings?.model && <> · <span className="text-indigo-500">{settings.model}</span></>}
          </p>
        </div>
        <button
          onClick={() => setSettingsOpen((o) => !o)}
          className={`text-sm px-3 py-1.5 rounded-lg border ${settingsOpen ? 'bg-gray-100 border-gray-300 text-gray-800' : 'border-gray-200 text-gray-600 hover:bg-gray-50'}`}
        >
          ⚙️ Gateway settings
        </button>
      </div>

      {settingsOpen && <GatewaySettings onClose={() => setSettingsOpen(false)} />}

      {/* ── Chat ── */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm flex flex-col" style={{ minHeight: '24rem' }}>
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
          <div>
            <h2 className="font-semibold text-gray-900">💬 Chat with your finances</h2>
            <p className="text-xs text-gray-400">
              The AI sees your balances, this month's spending &amp; budget status, labels and live stock positions — ask anything.
            </p>
          </div>
          {messages.length > 0 && (
            <button
              onClick={() => { setMessages([]); setChatError('') }}
              className="text-xs text-gray-400 hover:text-red-500"
            >
              Clear chat
            </button>
          )}
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3" style={{ maxHeight: '55vh' }}>
          {messages.length === 0 && !thinking && (
            <div className="text-sm text-gray-400 py-6 text-center space-y-2">
              <p>{configured ? 'Ask a question about your money.' : 'Configure the nexos.ai gateway above to start chatting.'}</p>
              {configured && (
                <div className="flex flex-wrap justify-center gap-1.5">
                  {['Give me my daily status update', 'How am I tracking against my budgets this month?', "How are my stock positions doing?", 'Where can I save €200/month?'].map((q) => (
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
          {messages.map((m, i) => (
            <div key={i} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-[85%] rounded-2xl px-3.5 py-2 text-sm whitespace-pre-wrap leading-relaxed ${
                  m.role === 'user'
                    ? 'bg-indigo-600 text-white rounded-br-sm'
                    : 'bg-gray-100 text-gray-800 rounded-bl-sm'
                }`}
              >
                {m.content}
              </div>
            </div>
          ))}
          {thinking && (
            <div className="flex justify-start">
              <div className="bg-gray-100 rounded-2xl rounded-bl-sm px-3.5 py-2.5 text-sm text-gray-400">
                <span className="animate-pulse">Thinking…</span>
              </div>
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        {chatError && (
          <p className="px-4 pb-1 text-xs text-red-600">{chatError}</p>
        )}
        <div className="p-3 border-t border-gray-100 flex items-end gap-2">
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
            placeholder={configured ? 'Ask about your finances… (Enter to send)' : 'Set up the gateway first'}
            disabled={!configured || thinking}
            className="flex-1 resize-none text-sm border border-gray-200 rounded-xl px-3 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-200 disabled:bg-gray-50"
          />
          <button
            onClick={send}
            disabled={!configured || thinking || !draft.trim()}
            className="px-4 py-2.5 rounded-xl bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-40"
          >
            Send
          </button>
        </div>
      </div>

      {/* ── One-shot analysis (moved from the dashboard) ── */}
      <AIInsightCard />

      <p className="text-[11px] text-gray-400">
        Conversations are not stored on the server — history lives in this browser only. Each question
        sends your aggregated financial summary to the configured gateway.
      </p>
    </div>
  )
}

function GatewaySettings({ onClose }: { onClose: () => void }) {
  const { data: settings } = useAISettings()
  const save = useSaveAISettings()
  const [form, setForm] = useState({ gateway_url: '', model: '', api_key: '' })
  const [loaded, setLoaded] = useState(false)
  const [status, setStatus] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)
  const [testing, setTesting] = useState(false)

  useEffect(() => {
    if (settings && !loaded) {
      setForm({ gateway_url: settings.gateway_url, model: settings.model, api_key: '' })
      setLoaded(true)
    }
  }, [settings, loaded])

  const doSave = async () => {
    try {
      await save.mutateAsync({
        gateway_url: form.gateway_url.trim(),
        model: form.model.trim(),
        ...(form.api_key.trim() ? { api_key: form.api_key.trim() } : {}),
      })
      setForm((f) => ({ ...f, api_key: '' }))
      setStatus({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Save failed' })
    }
  }

  const doTest = async () => {
    setTesting(true)
    setStatus(null)
    try {
      await aiApi.test()
      setStatus({ kind: 'ok', text: 'Gateway reachable — model replied.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Test failed' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex items-center justify-between mb-1">
        <h2 className="font-semibold text-gray-900">⚙️ nexos.ai gateway</h2>
        <button onClick={onClose} className="text-gray-300 hover:text-gray-600">✕</button>
      </div>
      <p className="text-xs text-gray-400 mb-3">
        Any OpenAI-compatible endpoint works. Get a key and model list from your nexos.ai workspace.
      </p>
      <div className="grid sm:grid-cols-2 gap-3">
        <label className="text-xs text-gray-500 sm:col-span-2">
          Gateway URL
          <input
            value={form.gateway_url}
            onChange={(e) => setForm({ ...form, gateway_url: e.target.value })}
            placeholder="https://api.nexos.ai/v1"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
        <label className="text-xs text-gray-500">
          Model
          <input
            value={form.model}
            onChange={(e) => setForm({ ...form, model: e.target.value })}
            placeholder="e.g. gpt-5 or a nexos model id"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
        <label className="text-xs text-gray-500">
          API key {settings?.has_key && <span className="text-emerald-600">(saved — leave blank to keep)</span>}
          <input
            type="password"
            value={form.api_key}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
            placeholder={settings?.has_key ? '••••••••' : 'nxs-…'}
            autoComplete="new-password"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-2 mt-3">
        <button
          onClick={doSave}
          disabled={save.isPending || !form.model.trim()}
          className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          {save.isPending ? 'Saving…' : 'Save'}
        </button>
        <button
          onClick={doTest}
          disabled={testing || !settings?.has_key}
          className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
        >
          {testing ? 'Testing…' : 'Test connection'}
        </button>
        {settings?.has_key && (
          <button
            onClick={async () => {
              // Only clear the key — keep the saved URL/model, not whatever
              // half-typed values happen to be in the form right now.
              try {
                await save.mutateAsync({
                  gateway_url: settings.gateway_url, model: settings.model, clear_key: true,
                })
                setForm((f) => ({ ...f, api_key: '' }))
                setStatus({ kind: 'ok', text: 'Key removed.' })
              } catch (err) {
                const e = err as { response?: { data?: { error?: string } }; message?: string }
                setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Failed' })
              }
            }}
            className="px-3 py-1.5 text-sm text-red-500 hover:text-red-700"
          >
            Remove key
          </button>
        )}
        {status && (
          <span className={`text-xs ${status.kind === 'ok' ? 'text-emerald-600' : 'text-red-600'}`}>
            {status.text}
          </span>
        )}
      </div>
    </div>
  )
}