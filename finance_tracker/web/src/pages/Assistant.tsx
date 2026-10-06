import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import clsx from 'clsx'
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Line, LineChart, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../lib/api'
import { useRefresh } from '../lib/hooks'
import { eurk } from '../lib/format'
import { Empty, ErrorBox, PageHeader, Sheet, Spinner } from '../components/ui'
import { AIMemory } from '../components/AIMemory'
import { axisProps, gridProps, Legend, MoneyTooltip } from '../components/charts'
import { Icon } from '../components/Icon'

type Msg = { id?: number; role: 'user' | 'assistant'; content: string; cost?: number; tools?: string[]; image?: string }

const SUGGESTIONS = [
  'How am I doing this month versus my plan?',
  'Where did my money go in the last 3 months, and what changed?',
  'What is my real savings rate over the last 12 months, counting mortgage principal?',
  'Should I prepay the mortgage or invest more, given my rate reset in February?',
  'Which subscriptions and recurring costs could I cut?',
  'Review my investment positions against my satellite rules.',
]

/** Pick the model for chat, scans and assists; saved in AI settings. */
function ModelPicker() {
  const qc = useQueryClient()
  const { data: st } = useQuery({ queryKey: ['ai-settings'], queryFn: () => api.get<{ model: string; has_key: boolean }>('/ai/settings') })
  const { data: models } = useQuery({ queryKey: ['ai-models'], queryFn: () => api.get<string[]>('/ai/models'), enabled: !!st?.has_key, staleTime: 3_600_000, retry: false })
  const [saving, setSaving] = useState(false)
  if (!st?.has_key) return null
  // Newest families first; keep the current choice even if the list lacks it.
  const list = Array.from(new Set([st.model, ...(models ?? [])].filter(Boolean))).sort((a, b) => (a === st.model ? -1 : b === st.model ? 1 : a.localeCompare(b)))
  const pick = async (model: string) => {
    setSaving(true)
    try {
      await api.put('/ai/settings', { model })
      qc.setQueryData(['ai-settings'], { ...st, model })
    } finally { setSaving(false) }
  }
  return (
    <label className="relative inline-flex items-center gap-1.5 text-xs text-muted">
      <Icon name="spark" size={14} />
      <span className="sr-only">Model</span>
      <select className="input select-pad h-8 w-auto max-w-[11rem] truncate text-xs" value={st.model} disabled={saving} onChange={(e) => pick(e.target.value)} aria-label="Model">
        {list.map((m) => <option key={m} value={m}>{m}</option>)}
      </select>
    </label>
  )
}

export default function Assistant() {
  const qc = useQueryClient()
  const refresh = useRefresh()
  const { data: history, isLoading } = useQuery({ queryKey: ['chat'], queryFn: () => api.get<Msg[]>('/ai/chat') })
  const [local, setLocal] = useState<Msg[]>([])
  const [image, setImage] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const end = useRef<HTMLDivElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const msgs = useMemo(() => [...(history ?? []), ...local], [history, local])
  const [sp, setSp] = useSearchParams()
  // ?memory=1 (from AI settings) opens the memory sheet.
  const memoryOpen = sp.get('memory') === '1'
  const setMemoryOpen = (v: boolean) => { const n = new URLSearchParams(sp); v ? n.set('memory', '1') : n.delete('memory'); setSp(n, { replace: true }) }
  useEffect(() => {
    // Only follow a conversation; an empty chat stays at the top.
    if (msgs.length || busy) end.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [msgs.length, busy])

  const send = async (q: string) => {
    const content = q.trim()
    if ((!content && !image) || busy) return
    setError(null)
    const preview = image ? URL.createObjectURL(image) : undefined
    setLocal((l) => [...l, { role: 'user', content: content || '(image)', image: preview }])
    setBusy(true)
    try {
      let r: any
      if (image) {
        const fd = new FormData()
        fd.append('text', content)
        fd.append('image', image)
        r = await api.post('/ai/chat', fd)
      } else r = await api.post('/ai/chat', { text: content })
      setLocal((l) => [...l, { role: 'assistant', content: r.reply, cost: r.usage?.cost_usd, tools: r.tools }])
      if (r.changed) refresh()
    } catch (e) {
      setError(e)
    } finally {
      setImage(null)
      setBusy(false)
    }
  }
  // "Ask CFO about this" buttons elsewhere land here with ?q=.
  useEffect(() => {
    const q = sp.get('q')
    if (q && !isLoading && !busy) {
      setSp({}, { replace: true })
      send(q)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sp, isLoading])
  const clear = async () => {
    if (!confirm('Clear the conversation?')) return
    await api.del('/ai/chat')
    setLocal([])
    qc.invalidateQueries({ queryKey: ['chat'] })
  }

  return (
    <div className="flex min-h-[calc(100dvh-12rem)] flex-col">
      <PageHeader title="Ask your CFO" actions={<><ModelPicker /><button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => setMemoryOpen(true)}><Icon name="edit" size={14} />Memory</button>{msgs.length > 0 && <button className="btn-ghost h-8 text-xs" onClick={clear}>Clear</button>}</>} />
      <div className="flex-1 space-y-4">
        {isLoading ? null : msgs.length === 0 ? (
          <div>
            <Empty title="Ask anything about your money" icon="spark">Answers come from your live data — ledger, balances, plan, loans and investments.</Empty>
            <button onClick={() => setMemoryOpen(true)} className="mb-3 flex w-full items-center gap-2 rounded-xl border border-line px-3 py-2 text-left text-xs text-ink2 hover:bg-sunken/50">
              <Icon name="edit" size={14} className="text-accent" /><span className="flex-1">It also follows your brief and the decisions it remembers — <span className="text-accent">review its memory</span></span><Icon name="chevronR" size={14} className="text-muted" />
            </button>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {SUGGESTIONS.map((s) => <button key={s} onClick={() => send(s)} className="card p-3 text-left text-sm hover:bg-sunken/50">{s}</button>)}
            </div>
          </div>
        ) : msgs.map((m, i) => <Bubble key={m.id ?? `l${i}`} m={m} />)}
        {busy && <div className="flex items-center gap-2 text-sm text-muted"><Spinner className="h-4 w-4" />Looking at your data…</div>}
        <ErrorBox error={error} />
        <div ref={end} />
      </div>
      <div className="sticky bottom-16 mt-4 sm:bottom-4">
        {image && (
          <div className="mb-2 inline-flex items-center gap-2 rounded-xl border border-line bg-raised px-2 py-1 text-xs">
            <Icon name="image" size={14} />{image.name}<button onClick={() => setImage(null)} aria-label="Remove image"><Icon name="x" size={12} /></button>
          </div>
        )}
        <Composer busy={busy} hasImage={!!image} onAttach={() => fileRef.current?.click()} onSend={send} />
        <input ref={fileRef} type="file" accept="image/*" hidden onChange={(e) => { setImage(e.target.files?.[0] ?? null); e.target.value = '' }} />
      </div>
      {memoryOpen && <Sheet open wide onClose={() => setMemoryOpen(false)} title="What your CFO knows about you"><AIMemory /></Sheet>}
    </div>
  )
}

/** The input box keeps its own text, so typing never re-renders the
 *  conversation (and its charts) above it. */
function Composer({ busy, hasImage, onAttach, onSend }: { busy: boolean; hasImage: boolean; onAttach: () => void; onSend: (text: string) => void }) {
  const [text, setText] = useState('')
  const go = () => {
    if ((!text.trim() && !hasImage) || busy) return
    onSend(text)
    setText('')
  }
  return (
    <div className="flex items-end gap-2 rounded-2xl border border-line bg-raised p-2 shadow-sm">
      <button className="btn-ghost h-10 w-10 shrink-0 px-0" onClick={onAttach} aria-label="Attach image"><Icon name="camera" /></button>
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={1} placeholder="Ask about your money…"
        onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); go() } }}
        className="block max-h-40 min-h-10 min-w-0 flex-1 resize-none bg-transparent px-1 py-2.5 text-sm leading-5 outline-none" />
      <button className="btn-primary h-10 w-10 shrink-0 px-0" onClick={go} disabled={busy || (!text.trim() && !hasImage)} aria-label="Send"><Icon name="send" /></button>
    </div>
  )
}

// Created once: renderers made inside a component are new functions on every
// render, so React would rebuild each chart (and it would flicker).
const MARKDOWN: Components = {
  code({ className, children }) {
    if (className?.includes('language-chart')) return <ChatChart spec={String(children)} />
    return <code className={className}>{children}</code>
  },
  pre({ children }) { return <div className="overflow-x-auto">{children}</div> },
  table({ children }) { return <div className="overflow-x-auto"><table>{children}</table></div> },
}
const PLUGINS = [remarkGfm]

const Bubble = memo(function Bubble({ m }: { m: Msg }) {
  if (m.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-2xl rounded-br-md bg-accent px-3.5 py-2 text-sm text-white">
          {m.image && <img src={m.image} alt="" className="mb-2 max-h-48 rounded-lg" />}
          {m.content}
        </div>
      </div>
    )
  }
  return (
    <div className="card p-4">
      <div className="prose-sm text-sm leading-relaxed [&_table]:my-2 [&_table]:w-full [&_td]:border-t [&_td]:border-line [&_td]:px-2 [&_td]:py-1 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left [&_th]:text-xs [&_th]:text-muted [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-2 [&_strong]:font-semibold [&_h3]:mt-3 [&_h3]:font-semibold [&_code]:rounded [&_code]:bg-sunken [&_code]:px-1">
        <ReactMarkdown remarkPlugins={PLUGINS} components={MARKDOWN}>{m.content}</ReactMarkdown>
      </div>
      {(m.cost != null || m.tools?.length) && (
        <div className="mt-2 text-[11px] text-muted">{m.tools?.length ? `${[...new Set(m.tools)].join(' · ')} · ` : ''}{m.cost != null ? `$${m.cost.toFixed(3)}` : ''}</div>
      )}
    </div>
  )
})

const ChatChart = memo(function ChatChart({ spec }: { spec: string }) {
  let c: any
  try {
    c = JSON.parse(spec)
  } catch {
    return <pre className="text-xs">{spec}</pre>
  }
  const series = (c.series ?? []).slice(0, 4)
  const color = (i: number) => `var(--s${i + 1})`
  const common = { data: c.data ?? [], margin: { top: 8, right: 4, bottom: 0, left: 0 } }
  const axes = <><CartesianGrid {...gridProps} /><XAxis dataKey={c.x} {...axisProps} /><YAxis {...axisProps} tickFormatter={c.unit === '€' ? eurk : undefined} width={48} /><Tooltip content={<MoneyTooltip />} /></>
  return (
    <div className="my-3 rounded-xl border border-line p-3">
      {c.title && <div className="mb-2 text-sm font-semibold">{c.title}</div>}
      <div className="h-56">
        <ResponsiveContainer>
          {c.type === 'pie' ? (
            <PieChart>
              <Pie data={c.data} dataKey={series[0]?.key} nameKey={c.x} innerRadius="55%" outerRadius="85%" stroke="var(--chart-surface)" strokeWidth={2} isAnimationActive={false}>
                {(c.data ?? []).slice(0, 8).map((_: any, i: number) => <Cell key={i} fill={color(i)} />)}
              </Pie>
              <Tooltip content={<MoneyTooltip />} />
            </PieChart>
          ) : c.type === 'line' ? (
            <LineChart {...common}>{axes}{series.map((s: any, i: number) => <Line key={s.key} dataKey={s.key} name={s.name} stroke={color(i)} strokeWidth={2} dot={false} isAnimationActive={false} />)}</LineChart>
          ) : c.type === 'area' ? (
            <AreaChart {...common}>{axes}{series.map((s: any, i: number) => <Area key={s.key} dataKey={s.key} name={s.name} stroke={color(i)} fill={color(i)} fillOpacity={0.2} strokeWidth={2} isAnimationActive={false} />)}</AreaChart>
          ) : (
            <BarChart {...common}>{axes}{series.map((s: any, i: number) => <Bar key={s.key} dataKey={s.key} name={s.name} fill={color(i)} radius={[4, 4, 0, 0]} isAnimationActive={false} />)}</BarChart>
          )}
        </ResponsiveContainer>
      </div>
      <div className={clsx('mt-2', series.length < 2 && c.type !== 'pie' && 'hidden')}>
        <Legend items={c.type === 'pie' ? (c.data ?? []).slice(0, 8).map((d: any, i: number) => ({ color: color(i), label: d[c.x] })) : series.map((s: any, i: number) => ({ color: color(i), label: s.name }))} />
      </div>
    </div>
  )
})
