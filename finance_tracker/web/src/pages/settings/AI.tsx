import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { parseNum } from '../../lib/format'
import { Card, Field, Loading, Segmented, Spinner, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

// ── AI ──────────────────────────────────────────────────────────────

export function AISettings() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data: s, isLoading } = useQuery({ queryKey: ['ai-settings'], queryFn: () => api.get<any>('/ai/settings') })
  const { data: ctx } = useQuery({ queryKey: ['ai-context'], queryFn: () => api.get<any>('/ai/context') })
  const [form, setForm] = useState<any>(null)
  const [context, setContext] = useState<string | null>(null)
  const [models, setModels] = useState<string[]>([])
  const [testing, setTesting] = useState(false)
  if (isLoading || !s) return <Loading />
  const f = form ?? { gateway_url: s.gateway_url, model: s.model, provider: s.provider, enabled: s.enabled, api_key: '' }
  const save = async () => {
    await api.put('/ai/settings', f)
    toast('Saved', 'good')
    setForm(null)
    qc.invalidateQueries({ queryKey: ['ai-settings'] })
  }
  const test = async () => {
    setTesting(true)
    try {
      await api.post('/ai/test')
      toast('The AI answered', 'good')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setTesting(false)
    }
  }
  return (
    <div className="max-w-2xl space-y-4">
      <Card title="Provider">
        <div className="space-y-3">
          <Toggle checked={f.enabled} onChange={(v) => setForm({ ...f, enabled: v })} label="AI features on" />
          <Segmented value={f.provider} onChange={(p) => setForm({ ...f, provider: p })} options={[{ value: 'anthropic', label: 'Claude API' }, { value: 'gateway', label: 'Gateway (nexos.ai…)' }]} />
          <Field label="API key" hint={s.has_key ? `Stored: ${s.key_hint}. Leave empty to keep it.` : 'Not set'}><input type="password" className="input font-mono text-xs" value={f.api_key} onChange={(e) => setForm({ ...f, api_key: e.target.value })} autoComplete="off" /></Field>
          <Field label="Base URL" hint="Empty = the provider's default"><input className="input" value={f.gateway_url} onChange={(e) => setForm({ ...f, gateway_url: e.target.value })} /></Field>
          <Field label="Model">
            <div className="flex gap-2">
              <input className="input font-mono text-xs" list="ai-models" value={f.model} onChange={(e) => setForm({ ...f, model: e.target.value })} />
              <datalist id="ai-models">{models.map((m) => <option key={m} value={m} />)}</datalist>
              <button className="btn-outline" onClick={async () => { try { setModels(await api.get('/ai/models')) } catch (e) { toast((e as Error).message, 'bad') } }} aria-label="Load models"><Icon name="refresh" size={16} /></button>
            </div>
          </Field>
          <div className="flex gap-2"><button className="btn-primary" onClick={save}>Save</button><button className="btn-outline" onClick={test} disabled={testing}>{testing ? <Spinner className="h-4 w-4" /> : 'Test'}</button></div>
        </div>
      </Card>
      <TopUps />
      <Card title="Usage">
        <div className="grid grid-cols-3 gap-3 text-sm">
          <div><div className="text-xs text-muted">This month</div><div className="font-semibold">${s.spend.month_usd.toFixed(2)}</div></div>
          <div><div className="text-xs text-muted">All time</div><div className="font-semibold">${s.spend.total_usd.toFixed(2)}</div></div>
          <div><div className="text-xs text-muted">Calls</div><div className="font-semibold">{s.spend.calls}</div></div>
        </div>
      </Card>
      <Notes />
      <Card title="Your brief for the assistant">
        <div className="mb-2 text-xs text-muted">Who you are, your rules and how you want to be advised. Numbers belong in the data, not here.</div>
        <textarea className="input h-80 py-2 text-xs leading-relaxed" value={context ?? ctx?.content ?? ''} onChange={(e) => setContext(e.target.value)} />
        <button className="btn-primary mt-2" disabled={context == null} onClick={async () => { await api.put('/ai/context', { content: context }); toast('Saved', 'good'); setContext(null); qc.invalidateQueries({ queryKey: ['ai-context'] }) }}>Save brief</button>
      </Card>
    </div>
  )
}

function TopUps() {
  const qc = useQueryClient()
  const { data } = useQuery({ queryKey: ['topups'], queryFn: () => api.get<any[]>('/ai/topups') })
  const { data: s } = useQuery({ queryKey: ['ai-settings'], queryFn: () => api.get<any>('/ai/settings') })
  const [amt, setAmt] = useState('')
  const [note, setNote] = useState('')
  const reload = () => { qc.invalidateQueries({ queryKey: ['topups'] }); qc.invalidateQueries({ queryKey: ['ai-settings'] }) }
  return (
    <Card title="Credit" action={s?.spend && <span className={clsx('text-sm tnum', s.spend.balance_usd < 2 ? 'text-bad' : 'text-ink2')}>balance ${s.spend.balance_usd.toFixed(2)}</span>}>
      <div className="mb-2 text-xs text-muted">Record what you pay the provider; the balance is top-ups minus measured spend.</div>
      <div className="flex gap-2">
        <input className="input w-28 tnum" inputMode="decimal" placeholder="$ amount" value={amt} onChange={(e) => setAmt(e.target.value)} />
        <input className="input" placeholder="Note" value={note} onChange={(e) => setNote(e.target.value)} />
        <button className="btn-outline" disabled={!((parseNum(amt) ?? 0) > 0)} onClick={async () => { await api.post('/ai/topups', { amount_usd: parseNum(amt), note }); setAmt(''); setNote(''); reload() }}>Add</button>
      </div>
      <div className="mt-2 divide-y divide-line text-sm">
        {(data ?? []).map((t) => (
          <div key={t.id} className="flex items-center justify-between py-1.5">
            <span>{t.occurred_on} <span className="text-muted">{t.note}</span></span>
            <span className="flex items-center gap-2 tnum">${t.amount_usd.toFixed(2)}<button className="text-muted hover:text-bad" onClick={async () => { await api.del(`/ai/topups/${t.id}`); reload() }} aria-label="Delete top-up"><Icon name="trash" size={14} /></button></span>
          </div>
        ))}
      </div>
    </Card>
  )
}

function Notes() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data } = useQuery({ queryKey: ['ai-notes'], queryFn: () => api.get<any>('/ai/notes') })
  const [v, setV] = useState<string | null>(null)
  return (
    <Card title="Remembered decisions">
      <div className="mb-2 text-xs text-muted">Things you told the assistant to remember. It adds to this list itself; edit or prune freely.</div>
      <textarea className="input h-32 py-2 text-xs" value={v ?? data?.content ?? ''} onChange={(e) => setV(e.target.value)} placeholder="Nothing yet" />
      <button className="btn-outline mt-2" disabled={v == null} onClick={async () => { await api.put('/ai/notes', { content: v }); toast('Saved', 'good'); setV(null); qc.invalidateQueries({ queryKey: ['ai-notes'] }) }}>Save</button>
    </Card>
  )
}
