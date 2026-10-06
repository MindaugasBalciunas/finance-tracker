import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { monthLabel, parseNum, shortDate } from '../../lib/format'
import { Card, Field, Loading, Spinner, Toggle, useToast } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'

// ── AI ──────────────────────────────────────────────────────────────

type Spend = {
  month_usd: number; total_usd: number; topups_usd: number; balance_usd: number; calls: number
  by_kind: Record<string, number>; last30_usd: number; last_call: string; months: { month: string; usd: number; calls: number }[]
}
type AISettingsData = { enabled: boolean; gateway_url: string; has_key: boolean; key_hint: string; model: string; provider: 'anthropic' | 'gateway'; spend: Spend }
type Form = { enabled: boolean; provider: 'anthropic' | 'gateway'; api_key: string; gateway_url: string; model: string }

const CLAUDE_MODELS = [
  { id: 'claude-opus-5-5', name: 'Opus 5.5', note: 'most capable' },
  { id: 'claude-sonnet-5-5', name: 'Sonnet 5.5', note: 'balanced' },
  { id: 'claude-haiku-4-5-20251001', name: 'Haiku 4.5', note: 'fastest, cheapest' },
]
const modelName = (id: string) => CLAUDE_MODELS.find((m) => m.id === id)?.name ?? id
const PROVIDERS = {
  anthropic: { name: 'Claude API', hint: 'Straight from Anthropic. Keys start with sk-ant-.', url: 'https://api.anthropic.com/v1' },
  gateway: { name: 'Gateway', hint: 'An OpenAI-compatible gateway such as nexos.ai.', url: '' },
}
// What each kind of call is, in the words the app uses.
const KINDS: Record<string, string> = { chat: 'Ask CFO', scan: 'Receipt scans', assist: 'Fill-in suggestions', tidy: 'Tidy-up suggestions', tagging: 'Tagging', forecast: 'Forecasts', view_summary: 'Page summaries', test: 'Connection tests', other: 'Other' }
const usd = (v: number) => `$${v.toFixed(2)}`

export function AISettings() {
  const { data: s, isLoading } = useQuery({ queryKey: ['ai-settings'], queryFn: () => api.get<AISettingsData>('/ai/settings') })
  if (isLoading || !s) return <Loading />
  return (
    <div className="max-w-3xl space-y-4">
      <Status s={s} />
      <Connection s={s} />
      <Spending s={s.spend} />
      <a href="#/ai?memory=1" className="card flex items-center gap-3 p-4 hover:bg-sunken/50">
        <IconTile name="edit" color="var(--s4)" size={36} />
        <span className="min-w-0 flex-1"><span className="block text-sm font-medium">Memory</span><span className="block text-xs text-muted">Your brief and the decisions the assistant remembers now live next to the chat, in Ask CFO.</span></span>
        <Icon name="chevronR" className="text-muted" />
      </a>
      <p className="text-xs text-muted">The assistant reads your ledger through the app's own tools when you ask it something; nothing is sent until you use an AI feature. Keys stay on this server and never appear in backups without secrets.</p>
    </div>
  )
}

/** One line that answers "is the AI working?", plus the master switch and a live test. */
function Status({ s }: { s: AISettingsData }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [test, setTest] = useState<{ ok: boolean; text: string } | 'running' | null>(null)
  const setEnabled = async (v: boolean) => {
    await api.put('/ai/settings', { enabled: v })
    qc.invalidateQueries({ queryKey: ['ai-settings'] })
    toast(v ? 'AI features on' : 'AI features off', 'good')
  }
  const run = async () => {
    setTest('running')
    const t0 = performance.now()
    try {
      await api.post('/ai/test')
      setTest({ ok: true, text: `Answered in ${((performance.now() - t0) / 1000).toFixed(1)} s` })
    } catch (e) {
      setTest({ ok: false, text: (e as Error).message })
    }
  }
  const state = !s.enabled ? 'off' : !s.has_key ? 'nokey' : 'on'
  return (
    <section className="card p-4">
      <div className="flex items-start gap-3">
        <IconTile name="spark" color={state === 'on' ? 'var(--s4)' : 'var(--s-other)'} size={40} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2">
            <span className="text-base font-semibold">AI assistant</span>
            <span className={clsx('rounded-full px-2 py-0.5 text-[11px] font-medium', state === 'on' ? 'bg-good/15 text-good' : state === 'nokey' ? 'bg-warn/15 text-warn' : 'bg-sunken text-muted')}>
              {state === 'on' ? 'On' : state === 'nokey' ? 'Needs a key' : 'Off'}
            </span>
          </div>
          <div className="mt-0.5 text-sm text-ink2">
            {state === 'off' && 'Ask CFO, receipt scanning and suggestions are hidden.'}
            {state === 'nokey' && 'Add an API key below to start using Ask CFO and receipt scanning.'}
            {state === 'on' && <>{PROVIDERS[s.provider].name} · {modelName(s.model)} · key {s.key_hint || 'stored'}</>}
          </div>
          <div className="mt-1 text-xs text-muted">Used by Ask CFO, receipt scans, fill-in suggestions and month summaries{s.spend.last_call && ` · last call ${shortDate(s.spend.last_call.slice(0, 10))}`}</div>
        </div>
        <Toggle checked={s.enabled} onChange={setEnabled} />
      </div>
      {state === 'on' && (
        <div className="mt-3 flex flex-wrap items-center gap-3 border-t border-line pt-3">
          <button className="btn-outline h-9" onClick={run} disabled={test === 'running'}>{test === 'running' ? <Spinner className="h-4 w-4" /> : <Icon name="send" size={16} />}Test connection</button>
          {test && test !== 'running' && <span className={clsx('flex items-center gap-1 text-sm', test.ok ? 'text-good' : 'text-bad')}><Icon name={test.ok ? 'check' : 'alert'} size={16} />{test.text}</span>}
        </div>
      )}
    </section>
  )
}

function Connection({ s }: { s: AISettingsData }) {
  const qc = useQueryClient()
  const toast = useToast()
  const initial: Form = { enabled: s.enabled, provider: s.provider, api_key: '', gateway_url: s.gateway_url, model: s.model }
  const [f, setF] = useState<Form>(initial)
  const [showKey, setShowKey] = useState(false)
  const [advanced, setAdvanced] = useState(!!s.gateway_url && s.gateway_url !== PROVIDERS[s.provider].url)
  const [models, setModels] = useState<string[] | null>(null)
  const [loadingModels, setLoadingModels] = useState(false)
  const [saving, setSaving] = useState(false)
  const dirty = f.provider !== initial.provider || f.api_key !== '' || f.gateway_url !== initial.gateway_url || f.model !== initial.model
  const set = (p: Partial<Form>) => setF({ ...f, ...p })
  const save = async () => {
    setSaving(true)
    try {
      await api.put('/ai/settings', { provider: f.provider, api_key: f.api_key, gateway_url: f.gateway_url, model: f.model })
      qc.invalidateQueries({ queryKey: ['ai-settings'] })
      setF({ ...f, api_key: '' })
      try {
        await api.post('/ai/test')
        toast('Saved — the AI answered', 'good')
      } catch (e) {
        toast(`Saved, but the test failed: ${(e as Error).message}`, 'bad')
      }
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setSaving(false)
    }
  }
  const loadModels = async () => {
    setLoadingModels(true)
    try { setModels(await api.get<string[]>('/ai/models')) } catch (e) { toast((e as Error).message, 'bad') } finally { setLoadingModels(false) }
  }
  const suggested = f.provider === 'anthropic' ? CLAUDE_MODELS.map((m) => m.id) : []
  const extra = (models ?? []).filter((m) => !suggested.includes(m))
  return (
    <Card title="Connection">
      <div className="space-y-5">
        <div>
          <div className="label">Provider</div>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {(Object.keys(PROVIDERS) as (keyof typeof PROVIDERS)[]).map((p) => (
              <button key={p} type="button" onClick={() => set({ provider: p, gateway_url: p === s.provider ? s.gateway_url : PROVIDERS[p].url })}
                className={clsx('rounded-xl border p-3 text-left transition', f.provider === p ? 'border-accent bg-accent/5' : 'border-line hover:bg-sunken/50')}>
                <div className="flex items-center justify-between text-sm font-medium">{PROVIDERS[p].name}{f.provider === p && <Icon name="check" size={16} className="text-accent" />}</div>
                <div className="mt-0.5 text-xs text-muted">{PROVIDERS[p].hint}</div>
              </button>
            ))}
          </div>
        </div>

        <Field label="API key" hint={s.has_key ? <>A key is stored (<span className="font-mono">{s.key_hint}</span>). Paste a new one only to replace it.</> : 'Not set yet.'}>
          <div className="flex gap-2">
            <input type={showKey ? 'text' : 'password'} className="input font-mono text-xs" placeholder={s.has_key ? '•••••••• stored' : f.provider === 'anthropic' ? 'sk-ant-…' : 'Gateway key'} value={f.api_key} onChange={(e) => set({ api_key: e.target.value })} autoComplete="off" spellCheck={false} />
            <button type="button" className="btn-outline px-3 text-xs" onClick={() => setShowKey(!showKey)}>{showKey ? 'Hide' : 'Show'}</button>
          </div>
        </Field>

        <div>
          <div className="label">Model</div>
          <div className="flex flex-wrap gap-1.5">
            {f.provider === 'anthropic' && CLAUDE_MODELS.map((m) => (
              <button key={m.id} type="button" className={f.model === m.id ? 'chip-on' : 'chip hover:bg-sunken'} onClick={() => set({ model: m.id })}>
                {m.name} <span className="text-muted">{m.note}</span>
              </button>
            ))}
            {extra.map((m) => <button key={m} type="button" className={f.model === m ? 'chip-on' : 'chip hover:bg-sunken'} onClick={() => set({ model: m })}><span className="font-mono text-[11px]">{m}</span></button>)}
          </div>
          <div className="mt-2 flex gap-2">
            <input className="input font-mono text-xs" value={f.model} onChange={(e) => set({ model: e.target.value })} placeholder="Model id" spellCheck={false} />
            <button type="button" className="btn-outline shrink-0 text-xs" onClick={loadModels} disabled={loadingModels || !s.has_key}>{loadingModels ? <Spinner className="h-4 w-4" /> : <Icon name="refresh" size={14} />}Models from provider</button>
          </div>
          {models && <div className="mt-1 text-xs text-muted">{models.length} models available from the provider{extra.length ? ' — listed above' : ''}.</div>}
        </div>

        <div>
          <button type="button" className="flex items-center gap-1 text-xs text-muted hover:text-ink" onClick={() => setAdvanced(!advanced)}>
            <Icon name="chevronD" size={14} className={clsx('transition', advanced && 'rotate-180')} />Advanced
          </button>
          {advanced && (
            <div className="mt-2">
              <Field label="Base URL" hint={`Leave empty for the default${PROVIDERS[f.provider].url ? ` (${PROVIDERS[f.provider].url})` : ''}.`}>
                <input className="input font-mono text-xs" value={f.gateway_url} onChange={(e) => set({ gateway_url: e.target.value })} placeholder={PROVIDERS[f.provider].url || 'https://…'} spellCheck={false} />
              </Field>
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line pt-3">
          {dirty && <span className="mr-auto text-xs text-warn">Unsaved changes</span>}
          {dirty && <button className="btn-ghost" onClick={() => setF(initial)}>Discard</button>}
          <button className="btn-primary" onClick={save} disabled={!dirty || saving}>{saving ? <Spinner className="h-4 w-4" /> : null}Save &amp; test</button>
        </div>
      </div>
    </Card>
  )
}

/** Balance first (it is what runs out), then where the money went. */
function Spending({ s }: { s: Spend }) {
  const qc = useQueryClient()
  const { data: topups } = useQuery({ queryKey: ['topups'], queryFn: () => api.get<{ id: number; amount_usd: number; note: string; occurred_on: string }[]>('/ai/topups') })
  const [adding, setAdding] = useState(false)
  const [amt, setAmt] = useState('')
  const [note, setNote] = useState('')
  const [showAll, setShowAll] = useState(false)
  const reload = () => { qc.invalidateQueries({ queryKey: ['topups'] }); qc.invalidateQueries({ queryKey: ['ai-settings'] }) }
  const perDay = s.last30_usd / 30
  const days = perDay > 0 ? Math.floor(s.balance_usd / perDay) : null
  const kinds = Object.entries(s.by_kind).sort((a, b) => b[1] - a[1])
  const maxMonth = Math.max(...s.months.map((m) => m.usd), 0.01)
  const add = async () => {
    await api.post('/ai/topups', { amount_usd: parseNum(amt), note })
    setAmt(''); setNote(''); setAdding(false); reload()
  }
  return (
    <Card title="Spending & credit">
      <div className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4">
        <div>
          <div className="text-xs text-muted">Credit left</div>
          <div className={clsx('text-2xl font-semibold tnum', s.balance_usd < 2 ? 'text-bad' : s.balance_usd < 5 ? 'text-warn' : '')}>{usd(s.balance_usd)}</div>
          <div className="text-xs text-muted">{s.topups_usd > 0 ? (days != null ? `≈ ${days} days at the last 30 days' pace` : 'no recent use') : 'record a top-up to track it'}</div>
        </div>
        <div><div className="text-xs text-muted">This month</div><div className="text-lg font-semibold tnum">{usd(s.month_usd)}</div></div>
        <div><div className="text-xs text-muted">All time</div><div className="text-lg font-semibold tnum">{usd(s.total_usd)}</div></div>
        <div><div className="text-xs text-muted">Calls</div><div className="text-lg font-semibold tnum">{s.calls}</div><div className="text-xs text-muted">{s.calls ? `${usd(s.total_usd / s.calls)} each on average` : ''}</div></div>
      </div>

      <div className="mt-5 grid grid-cols-1 gap-5 sm:grid-cols-2">
        <div>
          <div className="section-title mb-2">Last 6 months</div>
          <div className="flex h-24 items-end gap-1.5">
            {s.months.map((m) => (
              <div key={m.month} className="flex h-full flex-1 flex-col items-center justify-end gap-1" title={`${monthLabel(m.month, true)}: ${usd(m.usd)} · ${m.calls} calls`}>
                <div className={clsx('w-full rounded-t', m.usd > 0 ? 'bg-accent' : 'bg-line')} style={{ height: m.usd > 0 ? `${Math.max(6, (m.usd / maxMonth) * 100)}%` : 2 }} />
              </div>
            ))}
          </div>
          <div className="mt-1 flex gap-1.5">{s.months.map((m) => <span key={m.month} className="flex-1 text-center text-[10px] text-muted">{monthLabel(m.month).slice(0, 3)}</span>)}</div>
        </div>
        <div>
          <div className="section-title mb-2">By feature</div>
          {kinds.length === 0 ? <div className="text-sm text-muted">Nothing yet.</div> : (
            <div className="space-y-2">
              {kinds.map(([k, v]) => (
                <div key={k}>
                  <div className="flex justify-between text-sm"><span>{KINDS[k] ?? k}</span><span className="tnum">{usd(v)}</span></div>
                  <div className="mt-1 h-1 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent/70" style={{ width: `${s.total_usd > 0 ? (v / s.total_usd) * 100 : 0}%` }} /></div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="mt-5 border-t border-line pt-3">
        <div className="flex items-center justify-between">
          <div className="section-title">Top-ups <span className="font-normal normal-case text-muted">· {usd(s.topups_usd)} in total</span></div>
          {!adding && <button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => setAdding(true)}><Icon name="plus" size={14} />Record a top-up</button>}
        </div>
        {adding && (
          <div className="mt-2 flex flex-wrap gap-2">
            <input className="input w-28 tnum" inputMode="decimal" placeholder="$ amount" value={amt} onChange={(e) => setAmt(e.target.value)} autoFocus />
            <input className="input min-w-0 flex-1" placeholder="Note (optional)" value={note} onChange={(e) => setNote(e.target.value)} />
            <button className="btn-ghost" onClick={() => setAdding(false)}>Cancel</button>
            <button className="btn-primary" disabled={!((parseNum(amt) ?? 0) > 0)} onClick={add}>Add</button>
          </div>
        )}
        <div className="mt-1 divide-y divide-line text-sm">
          {(showAll ? topups ?? [] : (topups ?? []).slice(0, 3)).map((t) => (
            <div key={t.id} className="flex items-center justify-between py-1.5">
              <span>{shortDate(t.occurred_on)} <span className="text-muted">{t.note}</span></span>
              <span className="flex items-center gap-2 tnum">{usd(t.amount_usd)}<button className="text-muted hover:text-bad" onClick={async () => { if (confirm('Delete this top-up?')) { await api.del(`/ai/topups/${t.id}`); reload() } }} aria-label="Delete top-up"><Icon name="trash" size={14} /></button></span>
            </div>
          ))}
        </div>
        {(topups ?? []).length > 3 && <button className="mt-1 text-xs text-accent" onClick={() => setShowAll(!showAll)}>{showAll ? 'Fewer' : `All ${topups!.length}`}</button>}
        <div className="mt-2 text-xs text-muted">The provider bills you; record what you pay here and the balance is top-ups minus measured spend.</div>
      </div>
    </Card>
  )
}
