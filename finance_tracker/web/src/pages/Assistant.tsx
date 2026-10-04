import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import clsx from 'clsx'
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Line, LineChart, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../lib/api'
import { useRefresh } from '../lib/hooks'
import { eurk } from '../lib/format'
import { Empty, ErrorBox, Spinner } from '../components/ui'
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

export default function Assistant() {
  const qc = useQueryClient()
  const refresh = useRefresh()
  const { data: history, isLoading } = useQuery({ queryKey: ['chat'], queryFn: () => api.get<Msg[]>('/ai/chat') })
  const [local, setLocal] = useState<Msg[]>([])
  const [text, setText] = useState('')
  const [image, setImage] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const end = useRef<HTMLDivElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const msgs = [...(history ?? []), ...local]
  const [sp, setSp] = useSearchParams()
  useEffect(() => {
    end.current?.scrollIntoView({ behavior: 'smooth' })
  }, [msgs.length, busy])

  const send = async (q?: string) => {
    const content = (q ?? text).trim()
    if ((!content && !image) || busy) return
    setError(null)
    const preview = image ? URL.createObjectURL(image) : undefined
    setLocal((l) => [...l, { role: 'user', content: content || '(image)', image: preview }])
    setText('')
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
    <div className="flex min-h-[calc(100dvh-10rem)] flex-col">
      <div className="mb-3 flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Ask your CFO</h1>
        {msgs.length > 0 && <button className="btn-ghost h-8 text-xs" onClick={clear}>Clear</button>}
      </div>
      <div className="flex-1 space-y-4">
        {isLoading ? null : msgs.length === 0 ? (
          <div>
            <Empty title="Ask anything about your money" icon="spark">Answers come from your live data — ledger, balances, plan, loans and investments.</Empty>
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
        <div className="flex items-end gap-2 rounded-2xl border border-line bg-raised p-2 shadow-sm">
          <button className="btn-ghost h-10 w-10 shrink-0 px-0" onClick={() => fileRef.current?.click()} aria-label="Attach image"><Icon name="camera" /></button>
          <textarea value={text} onChange={(e) => setText(e.target.value)} rows={1} placeholder="Ask about spending, plan, investments…"
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }}
            className="max-h-40 min-h-10 flex-1 resize-none bg-transparent py-2 text-sm outline-none" />
          <button className="btn-primary h-10 w-10 shrink-0 px-0" onClick={() => send()} disabled={busy || (!text.trim() && !image)} aria-label="Send"><Icon name="send" /></button>
        </div>
        <input ref={fileRef} type="file" accept="image/*" hidden onChange={(e) => { setImage(e.target.files?.[0] ?? null); e.target.value = '' }} />
      </div>
    </div>
  )
}

function Bubble({ m }: { m: Msg }) {
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
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={{
          code({ className, children }) {
            if (className?.includes('language-chart')) return <ChatChart spec={String(children)} />
            return <code className={className}>{children}</code>
          },
          pre({ children }) { return <div className="overflow-x-auto">{children}</div> },
          table({ children }) { return <div className="overflow-x-auto"><table>{children}</table></div> },
        }}>{m.content}</ReactMarkdown>
      </div>
      {(m.cost != null || m.tools?.length) && (
        <div className="mt-2 text-[11px] text-muted">{m.tools?.length ? `${[...new Set(m.tools)].join(' · ')} · ` : ''}{m.cost != null ? `$${m.cost.toFixed(3)}` : ''}</div>
      )}
    </div>
  )
}

function ChatChart({ spec }: { spec: string }) {
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
}
