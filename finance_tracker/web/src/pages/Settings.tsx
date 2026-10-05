import { useEffect, useState } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { useAccounts, useDemo, usePrefs, useRefresh, useRules, useTags } from '../lib/hooks'
import { catColor, GROUPS, useCats } from '../lib/categories'
import { eurc, parseNum, shortDate } from '../lib/format'
import type { Account, Rule } from '../lib/types'
import { applyTheme } from '../App'
import { Card, ErrorBox, Field, Loading, NumberInput, PageHeader, Segmented, Sheet, Spinner, Tabs, Toggle, useToast } from '../components/ui'
import { AccountSelect, CategoryPicker, SPEND_KINDS, TagInput } from '../components/pickers'
import { TxRow } from '../components/TxEditor'
import { Icon, IconTile } from '../components/Icon'
import { accountIcon, brandColor } from '../lib/brand'
import { passkeyRegister } from '../lib/passkey'

const SECTIONS = [
  { to: 'categories', label: 'Categories' }, { to: 'accounts', label: 'Accounts' }, { to: 'rules', label: 'Rules' }, { to: 'tags', label: 'Tags' }, { to: 'banks', label: 'Banks' },
  { to: 'ai', label: 'AI' }, { to: 'security', label: 'Security' }, { to: 'data', label: 'Data & backup' }, { to: 'usage', label: 'Usage' }, { to: 'appearance', label: 'Appearance' },
]

// A stray deep path (e.g. an old link that stacked sections) lands on its
// last known section instead of a blank page.
function UnknownSection() {
  const rest = useParams()['*'] || ''
  const last = rest.split('/').reverse().find((seg) => SECTIONS.some((s) => s.to === seg))
  return <Navigate to={`/settings/${last || 'categories'}`} replace />
}

const AI_SHARE_TEXT = 'The attached zip is my complete finances. Open PROMPT.md and follow it, using README.md for the file formats.'

/** The everyday hand-off: the whole dataset to any AI assistant in one tap. */
function ShareForAI() {
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const share = async () => {
    setBusy(true)
    try {
      const res = await fetch('api/export/ai.zip', { credentials: 'same-origin' })
      if (!res.ok) throw new Error(`Export failed (${res.status})`)
      const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'finance-for-ai.zip'
      const file = new File([await res.blob()], name, { type: 'application/zip' })
      // Phones: the share sheet goes straight to an AI app. Elsewhere: download.
      if (navigator.canShare?.({ files: [file] })) {
        try { await navigator.share({ files: [file], title: 'Finances for AI', text: AI_SHARE_TEXT }) } catch (e: any) { if (e?.name !== 'AbortError') throw e }
      } else {
        const url = URL.createObjectURL(file)
        const a = Object.assign(document.createElement('a'), { href: url, download: name })
        document.body.append(a); a.click(); a.remove()
        setTimeout(() => URL.revokeObjectURL(url), 10_000)
        toast('Saved — attach it in your AI assistant', 'good')
      }
    } catch (e: any) {
      toast(e?.message ?? 'Export failed', 'bad')
    } finally { setBusy(false) }
  }
  return (
    <section className="card mb-4 flex flex-wrap items-center gap-3 p-4">
      <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="spark" /></span>
      <div className="min-w-0 flex-1 basis-48">
        <div className="text-sm font-medium">Export for AI</div>
        <div className="text-xs text-muted">Every transaction, balance, loan and plan as one zip with a start-here PROMPT.md — for any AI assistant.</div>
      </div>
      <div className="flex w-full sm:w-auto">
        <button className="btn-primary flex-1 sm:flex-none" onClick={share} disabled={busy}>{busy ? <Spinner /> : <Icon name="share" size={16} />}Export zip</button>
      </div>
    </section>
  )
}

export default function Settings() {
  const nav = useNavigate()
  const seg = useLocation().pathname.split('/')[2] || 'categories'
  const tab = SECTIONS.some((s) => s.to === seg) ? seg : 'categories'
  return (
    <div>
      <PageHeader title="Settings" />
      <ShareForAI />
      <Tabs value={tab} onChange={(v) => nav(`/settings/${v}`)} tabs={SECTIONS.map((s) => ({ value: s.to, label: s.label }))} />
      <Routes>
        <Route path="/" element={<Categories />} />
        <Route path="categories" element={<Categories />} />
        <Route path="accounts" element={<AccountsVisibility />} />
        <Route path="rules" element={<Rules />} />
        <Route path="tags" element={<Tags />} />
        <Route path="banks" element={<Banks />} />
        <Route path="ai" element={<AISettings />} />
        <Route path="security" element={<Security />} />
        <Route path="data" element={<Data />} />
        <Route path="usage" element={<UsageView />} />
        <Route path="appearance" element={<Appearance />} />
        <Route path="*" element={<UnknownSection />} />
      </Routes>
    </div>
  )
}

// ── accounts ────────────────────────────────────────────────────────

/** Hide closed or unused accounts (Luminor…): they leave Update balances,
 *  the "paid from" pickers and the Wealth lists; history stays intact. */
function AccountsVisibility() {
  const { data: accounts, isLoading } = useAccounts()
  const refresh = useRefresh()
  const toast = useToast()
  const [busy, setBusy] = useState<string | null>(null)
  if (isLoading || !accounts) return <Loading />
  const set = async (a: Account, shown: boolean) => {
    setBusy(a.id)
    try {
      await api.put(`/accounts/${a.id}`, { ...a, archived: !shown })
      refresh()
      toast(shown ? `${a.name} is back` : `${a.name} hidden`, 'good')
    } catch (e: any) { toast(e?.message ?? 'Failed', 'bad') } finally { setBusy(null) }
  }
  const groups = GROUPS.map((g) => ({ ...g, items: accounts.filter((a) => a.group === g.id) })).filter((g) => g.items.length)
  const other = accounts.filter((a) => !GROUPS.some((g) => g.id === a.group))
  if (other.length) groups.push({ id: 'other', name: 'Other', slot: 0, items: other })
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted">Hidden accounts stop appearing when you update balances or log a transaction, and drop out of the Wealth lists. Their past balances and transactions stay in every total and chart.</p>
      {groups.map((g) => (
        <Card key={g.id} pad={false} title={g.name}>
          <div className="divide-y divide-line border-t border-line">
            {g.items.map((a) => (
              <div key={a.id} className={clsx('flex items-center gap-3 px-4 py-2.5', a.archived && 'opacity-60')}>
                <IconTile name={accountIcon(a)} color={brandColor(a) ?? `var(--s${g.slot || 1})`} size={32} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{a.name}</div>
                  <div className="text-xs text-muted">{a.balance != null ? eurc(a.balance) : 'no balance'}{a.balance_date ? ` · ${shortDate(a.balance_date)}` : ''}</div>
                </div>
                <Toggle checked={!a.archived} onChange={(v) => busy !== a.id && set(a, v)} label={<span className="w-12 text-xs text-muted">{a.archived ? 'Hidden' : 'Shown'}</span>} />
              </div>
            ))}
          </div>
        </Card>
      ))}
    </div>
  )
}

// ── categories ──────────────────────────────────────────────────────

function Categories() {
  const cats = useCats()
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState<any>(null)
  const save = useMutation({
    mutationFn: (c: any) => (c.id && cats.byId[c.id] ? api.put(`/categories/${c.id}`, c) : api.post('/categories', c)),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['categories'] }); toast('Saved', 'good'); setEdit(null) },
  })
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Categories say what the money was for. Use tags for who and why (kids, trip:rome), and the merchant field for who was paid.</div>
      {cats.tree.map((p) => (
        <Card key={p.id} pad={false} title={<span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(p.id) }} />{p.name}<span className="text-[10px] uppercase tracking-wide text-muted">{p.kind}{p.essential ? ' · essential' : ''}</span></span>}
          action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setEdit({ parent: p.id, name: '' })}><Icon name="plus" size={14} />Sub</button>}>
          <div className="flex flex-wrap gap-1.5 px-4 pb-4">
            <button className="chip hover:bg-sunken" onClick={() => setEdit(p)}><Icon name="edit" size={12} />Edit</button>
            {p.children.map((c) => <button key={c.id} className={clsx('chip hover:bg-sunken', c.archived && 'line-through opacity-50')} onClick={() => setEdit(c)}>{c.name}{c.essential && ' ·e'}</button>)}
          </div>
        </Card>
      ))}
      <button className="btn-outline" onClick={() => setEdit({ name: '', kind: 'expense' })}><Icon name="plus" size={16} />Top-level category</button>
      {edit && (
        <Sheet open onClose={() => setEdit(null)} title={edit.id ? `Edit ${edit.name}` : 'New category'} footer={<button className="btn-primary" onClick={() => save.mutate(edit)}>Save</button>}>
          <div className="space-y-4">
            <Field label="Name"><input className="input" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} /></Field>
            {!edit.id && !edit.parent && (
              <Segmented value={edit.kind} onChange={(k) => setEdit({ ...edit, kind: k })} options={[{ value: 'expense', label: 'Expense' }, { value: 'income', label: 'Income' }, { value: 'transfer', label: 'Transfer' }]} />
            )}
            <Toggle checked={!!edit.essential} onChange={(v) => setEdit({ ...edit, essential: v })} label="Essential (counts toward the emergency fund and needs-vs-wants)" />
            {edit.id && <Toggle checked={!!edit.archived} onChange={(v) => setEdit({ ...edit, archived: v })} label="Archived (hidden from pickers)" />}
            <ErrorBox error={save.error} />
          </div>
        </Sheet>
      )}
    </div>
  )
}

// ── rules ───────────────────────────────────────────────────────────

function Rules() {
  const { data: rules, isLoading } = useRules()
  const cats = useCats()
  const [edit, setEdit] = useState<Partial<Rule> | null>(null)
  const [q, setQ] = useState('')
  if (isLoading) return <Loading />
  const list = (rules ?? []).filter((r) => !q || [r.pattern, r.set_category, r.set_merchant, r.add_tags.join(' ')].join(' ').toLowerCase().includes(q.toLowerCase()))
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Rules fill in merchant, category and tags for new transactions — typed, scanned or from the bank. History (what you filed similar merchants under) fills in the rest.</div>
      <div className="flex gap-2">
        <input className="input" placeholder="Search rules" value={q} onChange={(e) => setQ(e.target.value)} />
        <button className="btn-primary" onClick={() => setEdit({ pattern: '', enabled: true, add_tags: [], priority: 100 })}><Icon name="plus" size={16} />Rule</button>
      </div>
      <Card pad={false}>
        <div className="divide-y divide-line">
          {list.map((r) => (
            <button key={r.id} onClick={() => setEdit(r)} className={clsx('flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40', !r.enabled && 'opacity-50')}>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm"><span className="font-mono text-xs">{r.pattern || '∗'}</span>{r.when_category && <span className="text-muted"> in {cats.name(r.when_category)}</span>}</div>
                <div className="truncate text-xs text-muted">→ {[r.set_category && cats.path(r.set_category), r.set_merchant && `merchant ${r.set_merchant}`, r.add_tags.length && `tags ${r.add_tags.join(', ')}`].filter(Boolean).join(' · ')}</div>
              </div>
            </button>
          ))}
        </div>
      </Card>
      {edit && <RuleEditor rule={edit} onClose={() => setEdit(null)} />}
    </div>
  )
}

function RuleEditor({ rule, onClose }: { rule: Partial<Rule>; onClose: () => void }) {
  const [r, setR] = useState(rule)
  const qc = useQueryClient()
  const refresh = useRefresh()
  const toast = useToast()
  const preview = useQuery({ queryKey: ['rule-preview', r.pattern, r.when_category], queryFn: () => api.post<any>('/rules/preview', r), enabled: !!(r.pattern || r.when_category) })
  const save = async (apply: boolean) => {
    try {
      const saved = r.id ? await api.put<Rule>(`/rules/${r.id}`, r) : await api.post<Rule>('/rules', r)
      if (apply) {
        const res = await api.post<any>(`/rules/${saved.id}/apply`)
        toast(`Rule saved · ${res.changed} transactions updated`, 'good')
        refresh()
      } else toast('Rule saved', 'good')
      qc.invalidateQueries({ queryKey: ['rules'] })
      onClose()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const del = async () => {
    await api.del(`/rules/${r.id}`)
    qc.invalidateQueries({ queryKey: ['rules'] })
    onClose()
  }
  return (
    <Sheet open onClose={onClose} title={r.id ? 'Edit rule' : 'New rule'} wide footer={<>
      {r.id && <button className="btn-danger mr-auto" onClick={del}>Delete</button>}
      <button className="btn-ghost" onClick={() => save(false)}>Save</button>
      <button className="btn-primary" onClick={() => save(true)}>Save & apply to history</button>
    </>}>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="space-y-4">
          <Field label="When merchant or note contains" hint="Case and accents ignored. Start with ^ to match only the beginning."><input className="input font-mono" value={r.pattern ?? ''} onChange={(e) => setR({ ...r, pattern: e.target.value })} /></Field>
          <div><div className="label">…and the row is in (optional)</div><CategoryPicker value={r.when_category ?? ''} onChange={(id) => setR({ ...r, when_category: id })} placeholder="Any category" /></div>
          <div><div className="label">Set category</div><CategoryPicker value={r.set_category ?? ''} onChange={(id) => setR({ ...r, set_category: id })} placeholder="Leave as is" /></div>
          <Field label="Set merchant"><input className="input" value={r.set_merchant ?? ''} onChange={(e) => setR({ ...r, set_merchant: e.target.value })} /></Field>
          <div><div className="label">Add tags</div><TagInput value={r.add_tags ?? []} onChange={(v) => setR({ ...r, add_tags: v })} /></div>
          <div className="flex items-center gap-4">
            <Toggle checked={r.enabled ?? true} onChange={(v) => setR({ ...r, enabled: v })} label="Enabled" />
            <Field label="Priority"><NumberInput integer className="input h-8 w-20 tnum" value={r.priority ?? 100} onChange={(v) => setR({ ...r, priority: v ?? 100 })} /></Field>
          </div>
        </div>
        <div>
          <div className="label">Matches in your ledger {preview.data && <b className="text-ink">{preview.data.matches}</b>}</div>
          <div className="max-h-96 divide-y divide-line overflow-y-auto rounded-xl border border-line">
            {preview.isFetching ? <div className="p-4"><Spinner /></div> : (preview.data?.sample ?? []).map((t: any) => <TxRow key={t.id} t={t} showDate />)}
          </div>
        </div>
      </div>
    </Sheet>
  )
}

// ── tags ────────────────────────────────────────────────────────────

function Tags() {
  const { data, isLoading } = useTags()
  const refresh = useRefresh()
  const toast = useToast()
  const [edit, setEdit] = useState<{ from: string; to: string } | null>(null)
  if (isLoading) return <Loading />
  const run = async () => {
    const r = await api.post<any>('/tags/rename', edit)
    toast(`${r.changed} transactions updated`, 'good')
    refresh()
    setEdit(null)
  }
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Rename a tag to merge it into another; rename to nothing to remove it everywhere (rules included).</div>
      <TagSuggestions onPick={(from, to) => setEdit({ from, to })} />
      <div className="flex flex-wrap gap-1.5">
        {[...(data ?? [])].sort((a, b) => b.count - a.count).map((t) => (
          <button key={t.tag} className="chip hover:bg-sunken" onClick={() => setEdit({ from: t.tag, to: t.tag })}>{t.tag} <span className="text-muted">{t.count}</span></button>
        ))}
      </div>
      {edit && (
        <Sheet open onClose={() => setEdit(null)} title={`Tag “${edit.from}”`} footer={<>
          <button className="btn-danger mr-auto" onClick={() => setEdit({ ...edit, to: '' })}>Remove everywhere</button>
          <button className="btn-primary" onClick={run}>{edit.to ? 'Rename' : 'Remove'}</button>
        </>}>
          <Field label="New name"><input className="input" value={edit.to} onChange={(e) => setEdit({ ...edit, to: e.target.value })} /></Field>
          <a className="mt-3 inline-block text-sm text-accent" href={`#/ledger?period=all&tag=${encodeURIComponent(edit.from)}`}>See transactions</a>
        </Sheet>
      )}
    </div>
  )
}

function TagSuggestions({ onPick }: { onPick: (from: string, to: string) => void }) {
  const { data } = useQuery({ queryKey: ['tag-suggestions'], queryFn: () => api.get<{ from: string; to: string; reason: string }[]>('/tags/suggestions') })
  if (!data?.length) return null
  return (
    <Card title="Probably the same tag">
      <div className="space-y-1.5">
        {data.map((s) => (
          <div key={s.from + s.to} className="flex items-center justify-between gap-2 text-sm">
            <span><b>{s.from}</b> → <b>{s.to}</b> <span className="text-xs text-muted">{s.reason}</span></span>
            <button className="btn-outline h-8 text-xs" onClick={() => onPick(s.from, s.to)}>Merge…</button>
          </div>
        ))}
      </div>
    </Card>
  )
}

// ── banks ───────────────────────────────────────────────────────────

function Banks() {
  const [sp, setSp] = useSearchParams()
  const qc = useQueryClient()
  const toast = useToast()
  const { data: st } = useQuery({ queryKey: ['bank-settings'], queryFn: () => api.get<any>('/bank/settings') })
  const { data: conns, isLoading } = useQuery({ queryKey: ['bank-connections'], queryFn: () => api.get<any[]>('/bank/connections') })
  const [form, setForm] = useState<any>(null)
  const [bank, setBank] = useState('Swedbank')
  const [pasted, setPasted] = useState('')
  const [busy, setBusy] = useState(false)
  const reload = () => { qc.invalidateQueries({ queryKey: ['bank-connections'] }); qc.invalidateQueries({ queryKey: ['bank-settings'] }) }

  // The bank redirects back with ?code&state; finish the consent here.
  useEffect(() => {
    const code = sp.get('code'), state = sp.get('state'), err = sp.get('error_description') || sp.get('error')
    if (err) { toast(`The bank refused: ${err}`, 'bad'); setSp({}, { replace: true }) }
    if (code && state) {
      setSp({}, { replace: true })
      api.post('/bank/callback', { code, state }).then(() => { toast('Bank connected — map its accounts below', 'good'); reload() }).catch((e) => toast(e.message, 'bad'))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const connect = async () => {
    setBusy(true)
    try {
      const r = await api.post<any>('/bank/connect', { bank, country: 'LT' })
      window.location.href = r.url
    } catch (e) {
      toast((e as Error).message, 'bad')
      setBusy(false)
    }
  }
  const finishPasted = async () => {
    try {
      await api.post('/bank/callback', { url: pasted })
      toast('Bank connected', 'good')
      setPasted('')
      reload()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const mapAccount = async (id: number, account_id: string) => {
    await api.put(`/bank/accounts/${id}`, { account_id })
    reload()
  }
  const disconnect = async (id: number) => {
    if (!confirm('Disconnect this bank? The consent is revoked at the bank too.')) return
    await api.del(`/bank/connections/${id}`)
    reload()
  }
  const saveSettings = async () => {
    await api.put('/bank/settings', { ...form, owner_names: typeof form.owner_names === 'string' ? form.owner_names.split('\n').map((s: string) => s.trim()).filter(Boolean) : form.owner_names })
    toast('Saved', 'good')
    setForm(null)
    reload()
  }
  return (
    <div className="space-y-4">
      <Card title="Connections" action={st?.configured && (
        <div className="flex items-center gap-2">
          <select className="input select-pad h-8 w-auto text-xs" value={bank} onChange={(e) => setBank(e.target.value)}>{['Swedbank', 'SEB', 'Luminor', 'Revolut', 'Šiaulių bankas'].map((b) => <option key={b}>{b}</option>)}</select>
          <button className="btn-primary h-8 text-xs" onClick={connect} disabled={busy}>Connect</button>
        </div>)}>
        {!st?.configured ? <BankGuide /> : isLoading ? <Loading /> : (
          <div className="space-y-3">
            {(conns ?? []).filter((c) => c.accounts.length || c.status === 'authorized').map((c) => (
              <div key={c.id} className="rounded-xl border border-line p-3">
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold">{c.aspsp_name}</div>
                    <div className={clsx('text-xs', c.status === 'authorized' ? (c.days_left < 14 ? 'text-warn' : 'text-good') : 'text-bad')}>
                      {c.status === 'authorized' ? `connected · consent ends in ${c.days_left} days` : c.status}{c.last_error ? ` · ${c.last_error}` : ''}
                    </div>
                  </div>
                  <button className="btn-ghost h-8 text-xs text-bad" onClick={() => disconnect(c.id)}>Disconnect</button>
                </div>
                {c.accounts.map((a: any) => (
                  <div key={a.id} className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2 text-sm">
                    <div className="min-w-0 flex-1">
                      <div className="truncate">{a.display_name} <span className="text-xs text-muted">{a.iban}</span></div>
                      <div className="text-xs text-muted">{a.last_synced_at ? `synced ${shortDate(a.last_synced_at.slice(0, 10))}` : 'never synced'}{a.bank_balance != null ? ` · bank says €${a.bank_balance}` : ''}{a.open ? ` · ${a.open} in inbox` : ''}</div>
                    </div>
                    <div className="w-48"><AccountSelect value={a.account_id} onChange={(v) => mapAccount(a.id, v)} placeholder="Don't sync" kinds={SPEND_KINDS} /></div>
                  </div>
                ))}
              </div>
            ))}
            {(() => {
              const dead = (conns ?? []).filter((c) => !c.accounts.length && c.status !== 'authorized')
              if (!dead.length) return null
              return (
                <details className="rounded-xl border border-line px-3 py-2 text-sm">
                  <summary className="cursor-pointer text-muted">{dead.length} unfinished or revoked connection attempts</summary>
                  <div className="mt-2 space-y-1">
                    {dead.map((c) => <div key={c.id} className="flex justify-between text-xs"><span>{c.aspsp_name} · {c.status}{c.last_error ? ` · ${c.last_error}` : ''}</span><button className="text-bad" onClick={() => disconnect(c.id)}>Remove</button></div>)}
                  </div>
                </details>
              )
            })()}
            {!(conns ?? []).some((c) => c.status === 'authorized') && <div className="text-sm text-muted">No live connection — press Connect and approve access in your bank.</div>}
            <div className="text-xs text-muted">If the bank's redirect cannot reach this app (LAN or Tailscale), paste the address you landed on:</div>
            <div className="flex gap-2"><input className="input" placeholder="https://…?code=…&state=…" value={pasted} onChange={(e) => setPasted(e.target.value)} /><button className="btn-outline" onClick={finishPasted} disabled={!pasted}>Finish</button></div>
          </div>
        )}
      </Card>
      <Card title="Card reservations">
        <Toggle checked={!st?.keep_reserved_in_inbox} onChange={async (v) => { await api.put('/bank/settings', { keep_reserved_in_inbox: !v }); qc.invalidateQueries({ queryKey: ['bank-settings'] }) }}
          label="Add to the ledger right away" />
        <div className="mt-2 text-xs text-muted">Card payments the bank has only reserved go straight into the ledger, marked <span className="text-warn">pending</span>, so today's spending counts today. When the bank books one, the same transaction gets the final amount and date — your category and tags stay. A reservation the bank releases is removed. Off: they wait in the inbox until they book.</div>
      </Card>
      <Card title="Enable Banking application" action={!form && <button className="btn-ghost h-8 text-xs" onClick={() => setForm({ ...st, owner_names: (st?.owner_names ?? []).join('\n'), private_key_pem: '' })}>Edit</button>}>
        {!form ? (
          <div className="space-y-1 text-sm">
            <div><span className="text-muted">Application:</span> {st?.application_id || '—'}</div>
            <div><span className="text-muted">Private key:</span> {st?.has_key ? 'stored (never shown)' : 'missing'}</div>
            <div><span className="text-muted">Redirect URL:</span> {st?.redirect_url || '—'}</div>
          </div>
        ) : (
          <div className="space-y-3">
            <Field label="Application ID"><input className="input font-mono text-xs" value={form.application_id} onChange={(e) => setForm({ ...form, application_id: e.target.value })} /></Field>
            <Field label="Private key (PEM)" hint="Leave empty to keep the stored key."><textarea className="input h-24 py-2 font-mono text-xs" value={form.private_key_pem} onChange={(e) => setForm({ ...form, private_key_pem: e.target.value })} /></Field>
            <Field label="Redirect URL" hint="Must match the one registered with Enable Banking exactly."><input className="input" value={form.redirect_url} onChange={(e) => setForm({ ...form, redirect_url: e.target.value })} /></Field>
            <Field label="Your names as banks write them" hint="One per line — transfers to these are your own accounts."><textarea className="input h-20 py-2 text-xs" value={form.owner_names} onChange={(e) => setForm({ ...form, owner_names: e.target.value })} /></Field>
            <div className="flex gap-2"><button className="btn-primary" onClick={saveSettings}>Save</button><button className="btn-ghost" onClick={() => setForm(null)}>Cancel</button></div>
          </div>
        )}
      </Card>
    </div>
  )
}

function BankGuide() {
  return (
    <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink2">
      <li>Create a free account at <b>enablebanking.com</b> and register an application (environment: production, personal use).</li>
      <li>Set its redirect URL to this app's address, e.g. <code className="rounded bg-sunken px-1">{window.location.origin}/</code> — it must match exactly.</li>
      <li>Download the application's private key (PEM) and copy the application ID.</li>
      <li>Paste both below with the same redirect URL, save, then press <b>Connect</b> and approve access in your bank.</li>
      <li>Map each bank account to a ledger account. Sync from <b>Ledger → Bank inbox</b>; consent lasts up to 180 days.</li>
    </ol>
  )
}

// ── AI ──────────────────────────────────────────────────────────────

function AISettings() {
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

// ── security ────────────────────────────────────────────────────────

function Security() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data: st } = useQuery({ queryKey: ['auth-status'], queryFn: () => api.get<any>('/auth/status') })
  const { data: keys } = useQuery({ queryKey: ['passkeys'], queryFn: () => api.get<any[]>('/auth/passkeys'), enabled: !!st })
  const [pin, setPin] = useState({ current_pin: '', pin: '' })
  const [token, setToken] = useState('')
  const reload = () => { qc.invalidateQueries({ queryKey: ['auth-status'] }); qc.invalidateQueries({ queryKey: ['passkeys'] }) }
  const run = async (fn: () => Promise<any>, ok: string) => {
    try {
      await fn()
      toast(ok, 'good')
      reload()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  if (!st) return <Loading />
  return (
    <div className="max-w-xl space-y-4">
      <Card title="App lock">
        <div className="mb-3 text-sm">{st.enabled ? <span className="text-good">PIN lock is on.</span> : <span className="text-warn">PIN lock is off — anyone past the web login can see everything.</span>}</div>
        <div className="grid grid-cols-2 gap-3">
          {st.enabled && <Field label="Current PIN"><input type="password" inputMode="numeric" className="input" value={pin.current_pin} onChange={(e) => setPin({ ...pin, current_pin: e.target.value })} /></Field>}
          <Field label={st.enabled ? 'New PIN' : 'PIN (4–8 digits)'}><input type="password" inputMode="numeric" className="input" value={pin.pin} onChange={(e) => setPin({ ...pin, pin: e.target.value })} /></Field>
        </div>
        <div className="mt-3 flex gap-2">
          <button className="btn-primary" onClick={() => run(() => api.post('/auth/pin/setup', pin), st.enabled ? 'PIN changed' : 'Lock enabled')}>{st.enabled ? 'Change PIN' : 'Enable lock'}</button>
          {st.enabled && <button className="btn-danger" onClick={() => run(() => api.post('/auth/pin/disable', { pin: pin.current_pin }), 'Lock disabled')}>Disable</button>}
          {st.enabled && <button className="btn-ghost ml-auto" onClick={() => api.post('/auth/logout').then(() => window.location.reload())}>Lock now</button>}
        </div>
      </Card>
      <Card title="Passkeys (Face ID / fingerprint)">
        <div className="space-y-2">
          {(keys ?? []).map((k) => (
            <div key={k.id} className="flex items-center justify-between text-sm">
              <span>{k.name} <span className="text-xs text-muted">{shortDate(k.created_at.slice(0, 10))}</span></span>
              <button className="text-muted hover:text-bad" onClick={() => run(() => api.del(`/auth/passkeys/${k.id}`), 'Removed')}><Icon name="trash" size={16} /></button>
            </div>
          ))}
          <button className="btn-outline" disabled={!st.enabled} onClick={() => run(() => passkeyRegister(navigator.platform || 'Device'), 'Passkey added')}><Icon name="fingerprint" size={16} />Add this device</button>
          {!st.enabled && <div className="text-xs text-muted">Enable the PIN lock first.</div>}
        </div>
      </Card>
      <Card title="API tokens (MCP server)">
        <div className="mb-2 text-xs text-muted">Read-only tokens see data routes only; read-write tokens may also edit transactions, rules, tags and inbox proposals — never settings, backups or accepting bank rows.</div>
        {token && <div className="mb-2 break-all rounded-xl bg-sunken p-2 font-mono text-xs">{token}<div className="mt-1 font-sans text-warn">Copy it now — it is shown only once.</div></div>}
        <div className="flex flex-wrap gap-2 text-sm">
          <button className="btn-outline" onClick={async () => setToken((await api.post<any>('/auth/token', { rw: false })).token)}>{st.has_token ? 'Replace' : 'Create'} read-only</button>
          {st.has_token && <button className="btn-danger" onClick={() => run(() => api.del('/auth/token'), 'Revoked')}>Revoke</button>}
          <button className="btn-outline" onClick={async () => setToken((await api.post<any>('/auth/token', { rw: true })).token)}>{st.has_token_rw ? 'Replace' : 'Create'} read-write</button>
          {st.has_token_rw && <button className="btn-danger" onClick={() => run(() => api.del('/auth/token', { rw: '1' }), 'Revoked')}>Revoke</button>}
        </div>
      </Card>
    </div>
  )
}

// ── data ────────────────────────────────────────────────────────────

function Data() {
  const toast = useToast()
  const qc = useQueryClient()
  const { data: snaps } = useQuery({ queryKey: ['backups'], queryFn: () => api.get<any>('/backups') })
  const [busy, setBusy] = useState(false)
  const upload = async (path: string, file: File) => {
    if (!confirm('This REPLACES all data in the app with the file’s contents. A snapshot is taken first. Continue?')) return
    setBusy(true)
    try {
      const r = await fetch(`api${path}?confirm=replace`, { method: 'POST', body: await file.text(), headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin' })
      const d = await r.json()
      if (!r.ok) throw new Error(d.error)
      toast('Restored', 'good')
      qc.invalidateQueries()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }
  const pick = (path: string) => {
    const i = document.createElement('input')
    i.type = 'file'
    i.accept = 'application/json,.json'
    i.onchange = () => i.files?.[0] && upload(path, i.files[0])
    i.click()
  }
  return (
    <div className="max-w-2xl space-y-4">
      <DemoCard />
      <Card title="Backup">
        <div className="mb-3 text-sm text-muted">A full copy of every table. Without secrets it is safe to keep anywhere; a restore then keeps this instance's keys and PIN.{snaps?.last_download ? ` Last downloaded ${shortDate(snaps.last_download.slice(0, 10))}.` : ''}</div>
        <div className="flex flex-wrap gap-2">
          <a className="btn-primary" href="api/export/backup.json"><Icon name="download" size={16} />Download backup</a>
          <a className="btn-outline" href="api/export/backup.json?secrets=1" onClick={(e) => !confirm('This file will contain your AI key, bank private key and PIN hash. Store it safely.') && e.preventDefault()}>With secrets</a>
          <button className="btn-outline" onClick={() => pick('/import/backup')} disabled={busy}><Icon name="upload" size={16} />Restore…</button>
        </div>
      </Card>
      <Card title="Exports">
        <div className="flex flex-wrap gap-2">
          <a className="btn-outline" href="api/export/transactions.csv">Transactions CSV</a>
          <a className="btn-outline" href="api/export/ai.zip">Export for AI (zip)</a>
        </div>
        <div className="mt-2 text-xs text-muted">The AI export bundles every transaction, monthly cash flow and net worth, balances, loans and your brief with a README and a start-here PROMPT.md — for any AI assistant.</div>
      </Card>
      <Card title="Snapshots on the server" action={<button className="btn-ghost h-8 text-xs" onClick={async () => { await api.post('/backups'); qc.invalidateQueries({ queryKey: ['backups'] }); toast('Snapshot taken', 'good') }}>Take one now</button>}>
        <div className="max-h-64 divide-y divide-line overflow-y-auto text-sm">
          {(snaps?.snapshots ?? []).map((b: any) => (
            <a key={b.name} href={`api/backups/${b.name}`} className="flex justify-between py-1.5 hover:text-accent"><span className="font-mono text-xs">{b.name}</span><span className="text-xs text-muted">{(b.size / 1e6).toFixed(1)} MB</span></a>
          ))}
        </div>
      </Card>
      <Card title="Import from the old app (v1)">
        <div className="mb-2 text-sm text-muted">Converts a v1 finances.json backup — replaces everything. On first start the app already converted your v1 database automatically.</div>
        <button className="btn-outline" onClick={() => pick('/import/v1')} disabled={busy}>Import v1 backup…</button>
      </Card>
    </div>
  )
}

function Appearance() {
  const [t, setT] = useState(() => { try { return localStorage.getItem('theme') || 'system' } catch { return 'system' } })
  const set = (v: string) => {
    setT(v)
    try { v === 'system' ? localStorage.removeItem('theme') : localStorage.setItem('theme', v) } catch {}
    applyTheme()
  }
  return (
    <Card title="Theme">
      <Segmented value={t} onChange={set} options={[{ value: 'system', label: 'System' }, { value: 'light', label: 'Light' }, { value: 'dark', label: 'Dark' }]} />
    </Card>
  )
}

// ── usage analytics ─────────────────────────────────────────────────

/** What you open, how long you stay, and which pages you have to hunt for. */
function UsageView() {
  const [days, setDays] = useState('30')
  const { prefs, set } = usePrefs()
  const { data: u, isLoading, refetch } = useQuery({ queryKey: ['usage', days], queryFn: () => api.get<any>('/usage/summary', { days }) })
  const toast = useToast()
  if (isLoading || !u) return <Loading />
  for (const k of ['days', 'pages', 'actions', 'hunts', 'unused', 'suggestions']) u[k] ??= []
  // Page names come from the server (one source of truth).
  const pageName = (p: string) => u.names?.[p] ?? p
  const maxDay = Math.max(1, ...u.days.map((d: any) => d.views))
  const maxViews = Math.max(1, ...u.pages.map((p: any) => p.views))
  return (
    <div className="space-y-4">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Toggle checked={!prefs.usage_off} onChange={(v) => set({ usage_off: !v })} label="Record how I use the app" />
          <Segmented size="sm" value={days} onChange={setDays} options={[{ value: '7', label: '7 days' }, { value: '30', label: '30 days' }, { value: '90', label: '90 days' }]} />
        </div>
        <div className="mt-2 text-xs text-muted">Pages, time on each, clicks and where they land — kept only in this app's own database (180 days), never sent anywhere. Search terms and amounts are not recorded. Raw events for analysis: <code className="rounded bg-sunken px-1">GET /api/usage/events?days=90</code></div>
        <div className="mt-3 grid grid-cols-3 gap-3 text-sm">
          <div><div className="text-xs text-muted">Visits</div><div className="text-lg font-semibold tnum">{u.sessions}</div></div>
          <div><div className="text-xs text-muted">Page views</div><div className="text-lg font-semibold tnum">{u.views}</div></div>
          <div><div className="text-xs text-muted">Hard-to-find pages</div><div className={clsx('text-lg font-semibold tnum', u.hunts.length ? 'text-warn' : '')}>{u.hunts.length}</div></div>
        </div>
        <div className="mt-3 flex h-12 items-end gap-0.5" aria-label="Views per day">
          {u.days.map((d: any) => <div key={d.day} title={`${d.day}: ${d.views} views`} className="flex-1 rounded-t bg-accent/70" style={{ height: `${Math.max(d.views ? 6 : 2, (d.views / maxDay) * 100)}%`, opacity: d.views ? 1 : 0.25 }} />)}
        </div>
      </Card>

      {u.suggestions.length > 0 && (
        <Card icon="spark" color="var(--s4)" title="Suggestions">
          <ul className="space-y-1.5 text-sm">{u.suggestions.map((s: string) => <li key={s} className="flex gap-2"><Icon name="chevronR" size={14} className="mt-1 shrink-0 text-muted" />{s}</li>)}</ul>
        </Card>
      )}

      {u.hunts.length > 0 && (
        <Card pad={false} icon="search" color="var(--s2)" title="Pages you had to look for">
          <div className="divide-y divide-line border-t border-line">
            {u.hunts.map((h: any) => (
              <div key={h.target} className="px-4 py-2.5 text-sm">
                <div className="flex items-baseline justify-between gap-2"><b>{pageName(h.target)}</b><span className="text-xs text-muted tnum">{h.count}× · ≈{h.avg_hops} hops · {h.avg_sec}s</span></div>
                <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted">
                  {[...h.typical_path, h.target].map((p: string, i: number) => <span key={i} className="inline-flex items-center gap-1">{i > 0 && <Icon name="chevronR" size={11} />}<span className={clsx('chip h-6', p === h.target && 'chip-on')}>{pageName(p)}</span></span>)}
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card pad={false} icon="chart" color="var(--s1)" title="Most used pages">
          <div className="divide-y divide-line border-t border-line">
            {u.pages.slice(0, 15).map((p: any) => (
              <div key={p.path} className="px-4 py-2 text-sm">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="truncate">{pageName(p.path)}</span>
                  <span className="shrink-0 text-xs text-muted tnum">{p.views} views · {p.avg_sec}s avg{p.bounces ? ` · ${p.bounces} bounced` : ''}</span>
                </div>
                <div className="mt-1 h-1 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent" style={{ width: `${(p.views / maxViews) * 100}%` }} /></div>
              </div>
            ))}
            {!u.pages.length && <div className="px-4 py-6 text-center text-sm text-muted">Nothing recorded yet — use the app for a while.</div>}
          </div>
        </Card>
        <div className="space-y-4">
          <Card pad={false} icon="target" color="var(--s3)" title="Most clicked">
            <div className="divide-y divide-line border-t border-line">
              {u.actions.slice(0, 12).map((a: any) => (
                <div key={a.path + a.label} className="flex items-center justify-between gap-2 px-4 py-2 text-sm">
                  <span className="min-w-0 truncate">{a.label || '(icon)'} <span className="text-xs text-muted">on {pageName(a.path)}</span></span>
                  <span className="tnum text-xs text-muted">{a.count}×</span>
                </div>
              ))}
              {!u.actions.length && <div className="px-4 py-6 text-center text-sm text-muted">No clicks yet.</div>}
            </div>
          </Card>
          {u.unused.length > 0 && (
            <Card title="Not opened in this period">
              <div className="flex flex-wrap gap-1.5">{u.unused.map((p: string) => <a key={p} href={`#${p}`} className="chip">{pageName(p)}</a>)}</div>
            </Card>
          )}
        </div>
      </div>
      <button className="btn-danger" onClick={async () => { if (!confirm('Delete all recorded usage?')) return; await api.del('/usage'); toast('Usage data deleted', 'good'); refetch() }}>
        <Icon name="trash" size={16} />Delete recorded usage
      </button>
    </div>
  )
}

/** Show the app with fictional data — for demos and screenshots. */
function DemoCard() {
  const { on, switchTo, reset } = useDemo()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const run = async (f: () => Promise<void>, msg: string) => {
    setBusy(true)
    try { await f(); toast(msg, 'good') } catch (e: any) { toast(e?.message ?? 'Failed', 'bad') } finally { setBusy(false) }
  }
  return (
    <Card icon="spark" color="var(--s5)" title="Demo mode">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Toggle checked={on} onChange={(v) => !busy && run(() => switchTo(v), v ? 'Demo data on — your real data is untouched' : 'Back to your data')} label="Show demo data" />
        {on && <button className="btn-outline h-8 text-xs" disabled={busy} onClick={() => confirm('Throw away changes made in the demo and generate fresh demo data?') && run(reset, 'Fresh demo data')}>{busy ? <Spinner /> : <Icon name="refresh" size={14} />}Reset demo data</button>}
      </div>
      <div className="mt-2 text-xs text-muted">Swaps every page to a separate database of a fictional household — two years of transactions, accounts, budgets, a mortgage and an ETF portfolio — to show the app without showing your money. Your login stays the same; backups, imports, bank sync and AI settings are switched off while it is on, and nothing you do in the demo touches your real data.</div>
    </Card>
  )
}
