import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi, type ChatMessage } from '../api/insights'
import { useAISettings } from '../hooks/useInsights'
import AINav from '../components/ui/AINav'
import Markdown from '../components/ui/Markdown'

// Full-screen AI chat grounded in the full financial dataset. History is
// stored server-side (follows the user between phone and browser), and
// assistant answers render as markdown — tables, lists, bold figures.
// Analysis and gateway settings live on the Overview tab.
export default function AI() {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model

  const qc = useQueryClient()
  const { data: serverHistory } = useQuery({
    queryKey: ['ai-chat-history'],
    queryFn: aiApi.chatHistory,
  })
  // Local copy so an in-flight turn renders immediately; re-synced whenever
  // the server history lands (initial load or another device's turns).
  const [messages, setMessages] = useState<ChatMessage[]>([])
  useEffect(() => {
    if (serverHistory) setMessages(serverHistory)
  }, [serverHistory])

  const [draft, setDraft] = useState('')
  const [thinking, setThinking] = useState(false)
  const [chatError, setChatError] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // Don't yank the page down on mount — only follow along once the
    // conversation has content.
    if (messages.length === 0 && !thinking) return
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  const send = async () => {
    const text = draft.trim()
    if (!text || thinking || !configured) return
    setMessages((m) => [...m, { role: 'user', content: text }])
    setDraft('')
    setChatError('')
    setThinking(true)
    try {
      const reply = await aiApi.chat(text)
      setMessages((m) => [...m, { role: 'assistant', content: reply }])
      qc.invalidateQueries({ queryKey: ['ai-chat-history'] })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setChatError(e.response?.data?.error ?? e.message ?? 'Chat failed')
      // The failed turn was not persisted server-side — drop the local echo
      // so the view matches the stored history.
      setMessages((m) => (m.length > 0 && m[m.length - 1].role === 'user' ? m.slice(0, -1) : m))
      setDraft(text)
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
    <div className="mx-auto max-w-4xl flex flex-col h-[calc(100dvh-10.5rem)] md:h-[calc(100dvh-7.5rem)]">
      <div className="flex flex-wrap items-center justify-between gap-2 pb-3">
        <div className="min-w-0">
          <h1 className="text-xl font-bold text-gray-900">💬 Chat</h1>
          <p className="text-xs text-gray-400 truncate">
            Queries your data live while answering
            {configured && settings?.model && <> · <span className="text-indigo-500">{settings.model}</span></>}
          </p>
        </div>
        <span className="flex flex-wrap items-center justify-end gap-2">
          {messages.length > 0 && (
            <button onClick={clearChat} className="text-xs text-gray-400 hover:text-red-500">
              Clear chat
            </button>
          )}
          <AINav />
        </span>
      </div>

      <div className="flex-1 min-h-0 bg-white rounded-2xl border border-gray-100 shadow-sm flex flex-col">
        <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
          {messages.length === 0 && !thinking && (
            <div className="text-sm text-gray-400 py-10 text-center space-y-3">
              <p>
                {configured
                  ? 'Ask anything — the AI can search your transactions and query budgets, balances and live stock prices while answering.'
                  : <>Configure the nexos.ai gateway on the <Link to="/ai/overview" className="text-indigo-500 hover:underline">Overview tab</Link> to start chatting.</>}
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
          {messages.map((m, i) => (
            <div key={i} className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-[90%] sm:max-w-[85%] rounded-2xl px-3.5 py-2 min-w-0 ${
                  m.role === 'user'
                    ? 'bg-indigo-600 text-white rounded-br-sm'
                    : 'bg-gray-100 text-gray-800 rounded-bl-sm'
                }`}
              >
                {m.role === 'assistant'
                  ? <Markdown>{m.content}</Markdown>
                  : <p className="text-sm whitespace-pre-wrap leading-relaxed">{m.content}</p>}
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

        {chatError && <p className="px-4 pb-1 text-xs text-red-600">{chatError}</p>}
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
    </div>
  )
}
