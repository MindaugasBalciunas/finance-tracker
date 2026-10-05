import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Route, Routes, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import clsx from 'clsx'
import { api } from '../lib/api'
import { TxFilter, useInbox, useOverview, useRefresh, useTransactions } from '../lib/hooks'
import { useCats } from '../lib/categories'
import { dayLabel, daysAgo, eur, eurc, todayISO } from '../lib/format'
import type { InboxRow, Tx } from '../lib/types'
import { Empty, ErrorBox, Loading, NumberInput, PageHeader, Segmented, Sheet, Spinner, Tabs, useToast } from '../components/ui'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { AccountSelect, CategoryPicker, TagInput, TRANSFER_FROM_KINDS, TRANSFER_TO_KINDS } from '../components/pickers'
import { Icon } from '../components/Icon'

export default function Ledger() {
  const loc = useLocation()
  const nav = useNavigate()
  const { data: o } = useOverview()
  const tab = loc.pathname.includes('/inbox') ? 'inbox' : loc.pathname.includes('/tidy') ? 'tidy' : 'tx'
  return (
    <div>
      <PageHeader title="Ledger" />
      <Tabs value={tab} onChange={(v) => nav(v === 'tx' ? '/ledger' : `/ledger/${v}`)}
        tabs={[{ value: 'tx', label: 'Transactions' }, { value: 'inbox', label: 'Bank inbox', badge: o?.inbox_open }, { value: 'tidy', label: 'Tidy up' }]} />
      <Routes>
        <Route path="/" element={<Transactions />} />
        <Route path="/inbox" element={<Inbox />} />
        <Route path="/tidy" element={<Tidy />} />
      </Routes>
    </div>
  )
}

const PERIODS = [
  { value: '30', label: '30 days' }, { value: 'month', label: 'This month' }, { value: '90', label: '3 months' },
  { value: 'year', label: 'This year' }, { value: '365', label: '12 months' }, { value: 'all', label: 'All time' },
]

function periodRange(p: string): { from?: string; to?: string } {
  const t = todayISO()
  switch (p) {
    case 'month': return { from: t.slice(0, 8) + '01', to: t }
    case 'year': return { from: t.slice(0, 5) + '01-01', to: t }
    case 'all': return {}
    default: return { from: daysAgo(Number(p)), to: t }
  }
}

function Transactions() {
  const [sp, setSp] = useSearchParams()
  const editor = useTxEditor()
  const cats = useCats()
  const period = sp.get('period') || (sp.get('from') ? 'custom' : '30')
  const [q, setQ] = useState(sp.get('q') || '')
  const [limit, setLimit] = useState(100)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [bulkOpen, setBulkOpen] = useState(false)

  const filter: TxFilter = useMemo(() => {
    const r = period === 'custom' ? { from: sp.get('from') || undefined, to: sp.get('to') || undefined } : periodRange(period)
    return {
      ...r, q: sp.get('q') || undefined, kind: sp.get('kind') || undefined, category: sp.getAll('category'), account: sp.getAll('account'),
      tag: sp.getAll('tag'), merchant: sp.get('merchant') || undefined, sort: sp.get('sort') || undefined, limit,
    }
  }, [sp, period, limit])
  const { data, isLoading, isFetching, error } = useTransactions(filter)

  useEffect(() => {
    const h = setTimeout(() => {
      const next = new URLSearchParams(sp)
      if (q) next.set('q', q)
      else next.delete('q')
      if (next.toString() !== sp.toString()) setSp(next, { replace: true })
    }, 300)
    return () => clearTimeout(h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])
  useEffect(() => {
    setLimit(100)
  }, [sp])

  const set = (k: string, v: string | string[] | null) => {
    const next = new URLSearchParams(sp)
    next.delete(k)
    if (Array.isArray(v)) v.forEach((x) => next.append(k, x))
    else if (v) next.set(k, v)
    setSp(next, { replace: true })
  }
  const groups = useMemo(() => {
    const out: { date: string; items: Tx[]; net: number }[] = []
    for (const t of data?.items ?? []) {
      let g = out[out.length - 1]
      if (!g || g.date !== t.date || filter.sort) {
        g = { date: t.date, items: [], net: 0 }
        out.push(g)
      }
      g.items.push(t)
      g.net += t.kind === 'income' ? t.amount : t.kind === 'expense' ? -t.amount : 0
    }
    return out
  }, [data, filter.sort])
  const toggle = (id: number) => setSelected((s) => {
    const n = new Set(s)
    n.has(id) ? n.delete(id) : n.add(id)
    return n
  })
  const activeFilters = [
    ...(filter.kind ? [{ k: 'kind', label: filter.kind }] : []),
    ...(filter.category ?? []).map((c) => ({ k: 'category', v: c, label: cats.path(c) })),
    ...(filter.account ?? []).map((a) => ({ k: 'account', v: a, label: a })),
    ...(filter.tag ?? []).map((t) => ({ k: 'tag', v: t, label: '#' + t })),
    ...(filter.merchant ? [{ k: 'merchant', label: filter.merchant }] : []),
  ]
  const removeFilter = (f: { k: string; v?: string }) => {
    if (!f.v) return set(f.k, null)
    set(f.k, sp.getAll(f.k).filter((x) => x !== f.v))
  }

  return (
    <div>
      <div className="mb-3 flex gap-2">
        <div className="relative flex-1">
          <Icon name="search" size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
          <input className="input pl-9" placeholder="Search merchant, note, tag" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <button className="btn-outline" onClick={() => setFiltersOpen(true)}><Icon name="filter" size={16} /><span className="hidden sm:inline">Filters</span></button>
        <button className="btn-outline hidden sm:inline-flex" onClick={editor.scan}><Icon name="camera" size={16} />Scan</button>
        <button className="btn-primary" onClick={() => editor.open()}><Icon name="plus" size={16} /><span className="hidden sm:inline">Add</span></button>
      </div>
      <div className="no-scrollbar overflow-y-hidden -mx-4 mb-3 flex gap-1.5 overflow-x-auto px-4 sm:mx-0 sm:px-0">
        {PERIODS.map((p) => (
          <button key={p.value} className={period === p.value ? 'chip-on' : 'chip'} onClick={() => { const n = new URLSearchParams(sp); n.delete('from'); n.delete('to'); n.set('period', p.value); setSp(n, { replace: true }) }}>{p.label}</button>
        ))}
        {period === 'custom' && <span className="chip-on">{filter.from} → {filter.to}</span>}
      </div>
      {activeFilters.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {activeFilters.map((f, i) => (
            <button key={i} className="chip-on" onClick={() => removeFilter(f)}>{f.label}<Icon name="x" size={12} /></button>
          ))}
        </div>
      )}

      <Owed />
      {data && (
        <div className="mb-3 grid grid-cols-3 gap-2 text-center text-xs">
          <div className="card px-2 py-2"><div className="text-muted">In</div><div className="tnum text-sm font-semibold text-good">{eur(data.income)}</div></div>
          <div className="card px-2 py-2"><div className="text-muted">Out</div><div className="tnum text-sm font-semibold">{eur(data.expense)}</div></div>
          <div className="card px-2 py-2"><div className="text-muted">Moved</div><div className="tnum text-sm font-semibold text-ink2">{eur(data.transfer)}</div></div>
        </div>
      )}

      <ErrorBox error={error} />
      {isLoading ? <Loading /> : !data?.items.length ? (
        <Empty title="No transactions" icon="search">Try a wider period or clear the filters.</Empty>
      ) : (
        <div className="card">
          {groups.map((g, gi) => (
            <div key={g.date + gi}>
              {!filter.sort && (
                <div className="sticky top-12 z-10 flex justify-between border-b border-line bg-sunken first:rounded-t-2xl px-4 py-1.5 text-xs font-medium text-ink2 backdrop-blur sm:top-0">
                  <span>{dayLabel(g.date)}</span>
                  <span className="tnum">{g.net !== 0 && (g.net > 0 ? '+' : '−') + eurc(Math.abs(g.net))}</span>
                </div>
              )}
              <div className="divide-y divide-line">
                {g.items.map((t) => (
                  <TxRow key={t.id} t={t} showDate={!!filter.sort} selected={selected.has(t.id)}
                    onSelect={selected.size ? () => toggle(t.id) : undefined}
                    onClick={() => (selected.size ? toggle(t.id) : editor.open(t))} />
                ))}
              </div>
            </div>
          ))}
          <div className="flex items-center justify-between border-t border-line px-4 py-3 text-sm">
            <span className="text-muted">{data.items.length} of {data.total}</span>
            <div className="flex gap-2">
              {!selected.size && <button className="btn-ghost h-8 text-xs" onClick={() => setSelected(new Set(data.items.map((t) => t.id)))}>Select</button>}
              {data.items.length < data.total && (
                <button className="btn-outline h-8 text-xs" onClick={() => setLimit(limit + 200)} disabled={isFetching}>{isFetching ? <Spinner className="h-4 w-4" /> : 'Load more'}</button>
              )}
            </div>
          </div>
        </div>
      )}

      {selected.size > 0 && (
        <div className="fixed inset-x-0 bottom-16 z-30 mx-auto flex max-w-lg items-center gap-2 rounded-2xl border border-line bg-raised px-3 py-2 shadow-xl sm:bottom-6">
          <span className="flex-1 text-sm"><b>{selected.size}</b> selected</span>
          <button className="btn-ghost h-9" onClick={() => setSelected(new Set())}>Clear</button>
          <button className="btn-primary h-9" onClick={() => setBulkOpen(true)}>Edit</button>
        </div>
      )}

      <FilterSheet open={filtersOpen} onClose={() => setFiltersOpen(false)} sp={sp} setSp={setSp} />
      <BulkSheet open={bulkOpen} ids={[...selected]} onClose={() => setBulkOpen(false)} onDone={() => { setBulkOpen(false); setSelected(new Set()) }} />
    </div>
  )
}

function Owed() {
  const { data } = useQuery({ queryKey: ['owed'], queryFn: () => api.get<Record<string, number>>('/owed') })
  const editor = useTxEditor()
  const open = Object.entries(data ?? {}).filter(([, v]) => Math.abs(v) >= 0.01)
  if (!open.length) return null
  return (
    <div className="card mb-3 p-3">
      <div className="section-title mb-1.5">Owed to you</div>
      <div className="flex flex-wrap gap-2">
        {open.map(([who, v]) => (
          <span key={who} className="chip">
            <span className="capitalize text-ink">{who}</span> <span className="tnum">{eurc(v)}</span>
            <button className="text-accent" onClick={() => editor.open({ kind: 'income', category: 'refunds', amount: Math.abs(v), merchant: who, tags: [`owed:${who}`], note: 'Repayment' })}>record repayment</button>
          </span>
        ))}
      </div>
    </div>
  )
}

function FilterSheet({ open, onClose, sp, setSp }: { open: boolean; onClose: () => void; sp: URLSearchParams; setSp: (s: URLSearchParams, o?: any) => void }) {
  const [draft, setDraft] = useState(() => new URLSearchParams(sp))
  useEffect(() => {
    setDraft(new URLSearchParams(sp))
  }, [sp, open])
  const upd = (fn: (n: URLSearchParams) => void) => setDraft((d) => { const n = new URLSearchParams(d); fn(n); return n })
  return (
    <Sheet open={open} onClose={onClose} title="Filters" footer={<>
      <button className="btn-ghost mr-auto" onClick={() => { setSp(new URLSearchParams(), { replace: true }); onClose() }}>Reset</button>
      <button className="btn-primary" onClick={() => { setSp(draft, { replace: true }); onClose() }}>Apply</button>
    </>}>
      <div className="space-y-4">
        <div>
          <div className="label">Type</div>
          <Segmented value={(draft.get('kind') || '') as any} onChange={(v) => upd((n) => (v ? n.set('kind', v) : n.delete('kind')))}
            options={[{ value: '', label: 'All' }, { value: 'expense', label: 'Expenses' }, { value: 'income', label: 'Income' }, { value: 'transfer', label: 'Transfers' }]} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <label><span className="label">From</span><input type="date" className="input" value={draft.get('from') || ''} onChange={(e) => upd((n) => { n.set('from', e.target.value); n.delete('period') })} /></label>
          <label><span className="label">To</span><input type="date" className="input" value={draft.get('to') || ''} onChange={(e) => upd((n) => { n.set('to', e.target.value); n.delete('period') })} /></label>
        </div>
        <div>
          <div className="label">Category</div>
          <CategoryPicker value={draft.get('category') || ''} onChange={(id) => upd((n) => n.set('category', id))} placeholder="Any category" />
        </div>
        <div>
          <div className="label">Account</div>
          <AccountSelect value={draft.get('account') || ''} onChange={(v) => upd((n) => (v ? n.set('account', v) : n.delete('account')))} placeholder="Any account" />
        </div>
        <div>
          <div className="label">Tags (any)</div>
          <TagInput value={draft.getAll('tag')} onChange={(v) => upd((n) => { n.delete('tag'); v.forEach((t) => n.append('tag', t)) })} />
        </div>
        <div>
          <div className="label">Sort</div>
          <Segmented value={(draft.get('sort') || '') as any} onChange={(v) => upd((n) => (v ? n.set('sort', v) : n.delete('sort')))}
            options={[{ value: '', label: 'Newest' }, { value: 'amount', label: 'Largest' }, { value: '+date', label: 'Oldest' }]} />
        </div>
      </div>
    </Sheet>
  )
}

function BulkSheet({ open, ids, onClose, onDone }: { open: boolean; ids: number[]; onClose: () => void; onDone: () => void }) {
  const [cat, setCat] = useState('')
  const [add, setAdd] = useState<string[]>([])
  const [remove, setRemove] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const refresh = useRefresh()
  const toast = useToast()
  const run = async (del = false) => {
    if (del && !confirm(`Delete ${ids.length} transactions?`)) return
    setBusy(true)
    try {
      const r = await api.post<{ changed: number }>('/transactions/bulk', { ids, set_category: cat, add_tags: add, remove_tags: remove, delete: del })
      toast(`${r.changed} ${del ? 'deleted' : 'updated'}`, 'good')
      refresh()
      onDone()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Sheet open={open} onClose={onClose} title={`Edit ${ids.length} transactions`} footer={<>
      <button className="btn-danger mr-auto" onClick={() => run(true)} disabled={busy}>Delete</button>
      <button className="btn-primary" onClick={() => run()} disabled={busy || (!cat && !add.length && !remove.length)}>Apply</button>
    </>}>
      <div className="space-y-4">
        <div><div className="label">Set category</div><CategoryPicker value={cat} onChange={(id) => setCat(id)} placeholder="Keep each row's category" /></div>
        <div><div className="label">Add tags</div><TagInput value={add} onChange={setAdd} /></div>
        <div><div className="label">Remove tags</div><TagInput value={remove} onChange={setRemove} placeholder="Tag to remove" /></div>
      </div>
    </Sheet>
  )
}

// ── bank inbox ──────────────────────────────────────────────────────

function Inbox() {
  const [view, setView] = useState<'open' | 'dismissed' | 'imported'>('open')
  const { data, isLoading, error } = useInbox(view)
  const refresh = useRefresh()
  const toast = useToast()
  const [syncing, setSyncing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [edit, setEdit] = useState<InboxRow | null>(null)
  const { data: conns } = useConnections()

  const rows = data ?? []
  const ready = rows.filter((r) => !r.pending && r.verdict !== 'needs_review')
  const pending = rows.filter((r) => r.pending)
  const review = rows.filter((r) => !r.pending && r.verdict === 'needs_review')
  const [picked, setPicked] = useState<Set<number>>(new Set())
  useEffect(() => {
    setPicked(new Set(ready.filter((r) => r.verdict === 'new' && !r.match && !r.guessed).map((r) => r.id)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const sync = async (days?: number) => {
    setSyncing(true)
    try {
      const r = await api.post<any>('/bank/sync', {}, days ? { days } : undefined)
      const failed = r.accounts.filter((a: any) => a.error || a.skipped)
      toast(`${r.new} new · ${r.auto_linked} linked · ${r.fetched} fetched${failed.length ? ` · ${failed.length} account(s) need attention` : ''}`, failed.length ? 'bad' : 'good')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setSyncing(false)
    }
  }
  const commit = async (ids: number[]) => {
    setBusy(true)
    try {
      const r = await api.post<any>('/bank/inbox/commit', { ids })
      toast(`${r.imported.length} added${r.skipped ? ` · ${r.skipped} skipped` : ''}`, 'good')
      if (r.notes?.length) setTimeout(() => toast(r.notes.join(' · ')), 400)
      refresh()
    } finally {
      setBusy(false)
    }
  }
  const state = async (ids: number[], action: 'dismiss' | 'restore') => {
    await api.post(`/bank/inbox/${action}`, { ids })
    refresh()
  }
  const link = async (r: InboxRow) => {
    await api.post(`/bank/inbox/${r.id}/link`, { tx_id: r.match!.id })
    toast('Linked to your entry', 'good')
    refresh()
  }
  const live = (conns ?? []).filter((c: any) => c.status === 'authorized')

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Segmented value={view} onChange={setView} size="sm" options={[{ value: 'open', label: 'To review' }, { value: 'imported', label: 'Added' }, { value: 'dismissed', label: 'Dismissed' }]} />
        <div className="ml-auto flex gap-2">
          <button className="btn-outline h-9" onClick={() => sync(7)} disabled={syncing}>Last 7 days</button>
          <button className="btn-primary h-9" onClick={() => sync()} disabled={syncing}>{syncing ? <Spinner className="h-4 w-4" /> : <Icon name="refresh" size={16} />}Sync banks</button>
        </div>
      </div>
      {live.length === 0 && (
        <div className="mb-3 rounded-xl border border-line bg-sunken px-3 py-2 text-sm text-ink2">No live bank connection. Connect Swedbank / SEB under <a className="text-accent" href="#/settings/banks">Settings → Banks</a>.</div>
      )}
      <ErrorBox error={error} />
      {isLoading ? <Loading /> : rows.length === 0 ? (
        <Empty title={view === 'open' ? 'Inbox zero' : 'Nothing here'}>{view === 'open' ? 'Every bank transaction is in your ledger.' : ''}</Empty>
      ) : view !== 'open' ? (
        <div className="card divide-y divide-line">
          {rows.map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-4 py-2.5">
              <InboxSummary r={r} />
              {view === 'dismissed' && <button className="btn-outline h-8 text-xs" onClick={() => state([r.id], 'restore')}>Restore</button>}
            </div>
          ))}
        </div>
      ) : (
        <div className="space-y-4">
          {review.length > 0 && <InboxGroup title="Needs your input" rows={review} picked={picked} setPicked={setPicked} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')} />}
          <InboxGroup title="Ready" rows={ready} picked={picked} setPicked={setPicked} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')}
            action={picked.size > 0 && <button className="btn-primary h-8 text-xs" disabled={busy} onClick={() => commit([...picked])}>Add {picked.size} to ledger</button>} />
          {pending.length > 0 && <InboxGroup title="Reserved by the bank (not booked yet)" rows={pending} picked={new Set()} setPicked={() => {}} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')} muted
            action={<button className="btn-outline h-8 text-xs" disabled={busy} onClick={() => commit(pending.filter((r) => r.category).map((r) => r.id))}>Add as pending</button>} />}
        </div>
      )}
      {edit && <InboxEditor row={edit} onClose={() => setEdit(null)} />}
    </div>
  )
}

function useConnections() {
  return useQuery({ queryKey: ['bank-connections'], queryFn: () => api.get<any[]>('/bank/connections'), staleTime: 60_000 })
}

function InboxSummary({ r }: { r: InboxRow }) {
  const cats = useCats()
  return (
    <div className="min-w-0 flex-1">
      <div className="flex items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium">{r.merchant || r.raw_payee || r.note}</span>
        <span className={clsx('tnum text-sm font-semibold', r.kind === 'income' && 'text-good')}>{r.kind === 'income' ? '+' : r.kind === 'expense' ? '−' : ''}{eurc(r.amount)}</span>
      </div>
      <div className="truncate text-xs text-muted">{dayLabel(r.date)} · {r.category ? cats.path(r.category) : 'no category'}{r.tags.length ? ' · ' + r.tags.join(', ') : ''}</div>
    </div>
  )
}

function InboxGroup({ title, rows, picked, setPicked, onEdit, onLink, onDismiss, action, muted }: {
  title: string; rows: InboxRow[]; picked: Set<number>; setPicked: (s: Set<number>) => void; onEdit: (r: InboxRow) => void
  onLink: (r: InboxRow) => void; onDismiss: (id: number) => void; action?: React.ReactNode; muted?: boolean
}) {
  if (!rows.length) return null
  return (
    <section className="card overflow-hidden">
      <div className="flex items-center justify-between gap-2 border-b border-line px-4 py-2.5">
        <div className="text-sm font-semibold">{title} <span className="text-muted font-normal">{rows.length}</span></div>
        {action}
      </div>
      <div className="divide-y divide-line">
        {rows.map((r) => {
          const on = picked.has(r.id)
          return (
            <div key={r.id} className={clsx('px-4 py-2.5', muted && 'opacity-70')}>
              <div className="flex items-center gap-3">
                {!muted && (
                  <button onClick={() => { const n = new Set(picked); on ? n.delete(r.id) : n.add(r.id); setPicked(n) }}
                    className={clsx('grid h-6 w-6 shrink-0 place-items-center rounded-md border', on ? 'border-accent bg-accent text-white' : 'border-axis text-transparent')} aria-label="Pick">
                    <Icon name="check" size={14} />
                  </button>
                )}
                <button className="min-w-0 flex-1 text-left" onClick={() => onEdit(r)}><InboxSummary r={r} /></button>
              </div>
              {(r.match || r.guessed || r.verdict_note) && (
                <div className="mt-1.5 flex flex-wrap items-center gap-2 pl-9 text-xs">
                  {r.match ? (
                    <>
                      <span className="text-ink2">Looks like your entry “{r.match.merchant || r.match.note}” on {dayLabel(r.match.date)}</span>
                      <button className="chip-on" onClick={() => onLink(r)}><Icon name="link" size={12} />Link to it</button>
                    </>
                  ) : r.guessed ? (
                    <span className="text-warn">Category is a guess — check it</span>
                  ) : (
                    <span className="text-muted">{r.verdict_note}</span>
                  )}
                  <button className="ml-auto text-muted hover:text-bad" onClick={() => onDismiss(r.id)}>Dismiss</button>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}

function InboxEditor({ row, onClose }: { row: InboxRow; onClose: () => void }) {
  const [r, setR] = useState(row)
  const [raw, setRaw] = useState('')
  const refresh = useRefresh()
  const toast = useToast()
  const save = async (andAdd: boolean) => {
    try {
      await api.put(`/bank/inbox/${r.id}`, { date: r.date, amount: r.amount, category: r.category, merchant: r.merchant, note: r.note, tags: r.tags, account_id: r.account_id, to_account_id: r.to_account_id })
      if (andAdd) await api.post('/bank/inbox/commit', { ids: [r.id] })
      toast(andAdd ? 'Added to ledger' : 'Saved', 'good')
      refresh()
      onClose()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  return (
    <Sheet open onClose={onClose} title="Bank transaction" footer={<>
      <button className="btn-ghost" onClick={() => save(false)}>Save</button>
      <button className="btn-primary" onClick={() => save(true)}>{r.pending ? 'Save & add as pending' : 'Save & add'}</button>
    </>}>
      <div className="space-y-4">
        <div className="rounded-xl bg-sunken px-3 py-2 text-xs text-ink2">
          <div className="font-medium text-ink">{r.raw_payee || '—'}</div>
          <div className="break-words">{r.raw_details}</div>
          <div className="mt-1 flex gap-3 text-muted"><span>{r.date}</span><span>{r.raw_currency}</span>{r.pending && <span className="text-warn">reserved</span>}</div>
          {!raw ? <button className="mt-1 text-accent" onClick={async () => setRaw((await api.get<any>(`/bank/inbox/${r.id}/raw`)).raw)}>Show raw bank data</button>
            : <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap text-[10px]">{JSON.stringify(JSON.parse(raw || '{}'), null, 2)}</pre>}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <label><span className="label">Amount €</span><NumberInput value={r.amount} onChange={(v) => setR({ ...r, amount: v as number })} /></label>
          <label><span className="label">Date</span><input type="date" className="input" value={r.date} onChange={(e) => setR({ ...r, date: e.target.value })} /></label>
        </div>
        <label className="block"><span className="label">Merchant</span><input className="input" value={r.merchant} onChange={(e) => setR({ ...r, merchant: e.target.value })} /></label>
        <div><span className="label">Category</span><CategoryPicker value={r.category} onChange={(id, kind) => setR({ ...r, category: id, kind })} /></div>
        {r.kind === 'transfer' && (
          <div className="grid grid-cols-2 gap-3">
            <div><span className="label">From</span><AccountSelect value={r.account_id} onChange={(v) => setR({ ...r, account_id: v })} kinds={TRANSFER_FROM_KINDS} placeholder="Outside" /></div>
            <div><span className="label">To</span><AccountSelect value={r.to_account_id} onChange={(v) => setR({ ...r, to_account_id: v })} kinds={TRANSFER_TO_KINDS} placeholder="Outside" /></div>
          </div>
        )}
        <label className="block"><span className="label">Note</span><input className="input" value={r.note} onChange={(e) => setR({ ...r, note: e.target.value })} /></label>
        <div><span className="label">Tags</span><TagInput value={r.tags} onChange={(v) => setR({ ...r, tags: v })} /></div>
      </div>
    </Sheet>
  )
}

// ── tidy up ─────────────────────────────────────────────────────────

/** Expenses that only carry a broad category, with sharper proposals from
 *  history and rules (free) or the AI (on request). Nothing changes until
 *  the owner applies a proposal. */
function Tidy() {
  const { data, isLoading } = useQuery({ queryKey: ['tidy'], queryFn: () => api.get<(Tx & { suggested?: string; why?: string })[]>('/tidy') })
  const [ai, setAi] = useState<Record<number, { category: string; merchant?: string; reason?: string }>>({})
  const [asking, setAsking] = useState(false)
  const [done, setDone] = useState<Set<number>>(new Set())
  const cats = useCats()
  const refresh = useRefresh()
  const toast = useToast()
  const editor = useTxEditor()
  const rows = (data ?? []).filter((r) => !done.has(r.id))
  const proposal = (r: Tx & { suggested?: string }) => ai[r.id]?.category || r.suggested
  const ask = async () => {
    setAsking(true)
    try {
      const props = await api.post<{ id: number; category: string; merchant?: string; reason?: string }[]>('/ai/tidy', { ids: rows.filter((r) => !r.suggested).slice(0, 80).map((r) => r.id) })
      const m: typeof ai = { ...ai }
      props.forEach((p) => (m[p.id] = p))
      setAi(m)
      toast(`${props.length} suggestions`, 'good')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setAsking(false)
    }
  }
  const apply = async (list: (Tx & { suggested?: string })[]) => {
    for (const r of list) {
      const c = proposal(r)
      if (!c) continue
      await api.put(`/transactions/${r.id}`, { ...r, category: c, kind: undefined, merchant: r.merchant || ai[r.id]?.merchant || '' })
    }
    setDone(new Set([...done, ...list.map((r) => r.id)]))
    toast(`${list.length} updated`, 'good')
    refresh()
  }
  if (isLoading) return <Loading />
  if (!rows.length) return <Empty title="Nothing to tidy" icon="check">Every expense from the last six months has a specific category.</Empty>
  const ready = rows.filter((r) => proposal(r))
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted">{rows.length} recent expenses only have a broad category.</span>
        <div className="ml-auto flex gap-2">
          <button className="btn-outline h-9" onClick={ask} disabled={asking}>{asking ? <Spinner className="h-4 w-4" /> : <Icon name="spark" size={16} />}Ask AI</button>
          {ready.length > 0 && <button className="btn-primary h-9" onClick={() => apply(ready)}>Apply {ready.length}</button>}
        </div>
      </div>
      <div className="card divide-y divide-line">
        {rows.map((r) => {
          const p = proposal(r)
          return (
            <div key={r.id} className="flex items-center gap-3 px-4 py-2.5">
              <button className="min-w-0 flex-1 text-left" onClick={() => editor.open(r)}>
                <div className="truncate text-sm font-medium">{r.merchant || r.note || '—'} <span className="font-normal text-muted tnum">· {eurc(r.amount)}</span></div>
                <div className="truncate text-xs text-muted">{dayLabel(r.date)} · now {cats.path(r.category)}{p ? <> → <b className="text-ink">{cats.path(p)}</b> <span>({ai[r.id]?.reason || r.why})</span></> : ''}</div>
              </button>
              {p && <button className="btn-ghost h-8 text-xs text-accent" onClick={() => apply([r])}>Apply</button>}
            </div>
          )
        })}
      </div>
    </div>
  )
}
