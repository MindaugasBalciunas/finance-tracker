import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Route, Routes, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import { TxFilter, useOverview, useRefresh, useTransactions } from '../lib/hooks'
import { useCats } from '../lib/categories'
import { dayLabel, eur, eurc } from '../lib/format'
import { PERIODS, periodRange } from '../lib/periods'
import type { Tx } from '../lib/types'
import { Empty, ErrorBox, hue, Loading, PageHeader, Segmented, Sheet, Spinner, Tabs, useToast } from '../components/ui'
import { useTxEditor } from '../components/TxEditor'
import { NeedRow } from './ledger/NeedRow'
import { AccountSelect, CategoryPicker, TagInput } from '../components/pickers'
import { Icon } from '../components/Icon'
import { MerchantMark } from '../components/MerchantLogo'
import { DateInput } from '../components/DateInput'
import { Inbox } from './ledger/Inbox'

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


// Only the two values the server knows; anything else is no filter, so the
// page never claims a split the rows don't have.
const needParam = (v: string | null) => (v === 'essential' || v === 'optional' ? v : undefined)

function Transactions() {
  const [sp, setSp] = useSearchParams()
  const editor = useTxEditor()
  const cats = useCats()
  const period = sp.get('period') || (sp.get('from') || sp.get('to') ? 'custom' : 'month')
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
      choices: sp.get('choices') || undefined, commitments: sp.get('commitments') || undefined, salary: sp.get('salary') || undefined, need: needParam(sp.get('need')),
      min: sp.get('min') || undefined, max: sp.get('max') || undefined,
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
    ...(filter.choices ? [{ k: 'choices', label: 'fixed commitments left out' }] : []),
    ...(filter.commitments ? [{ k: 'commitments', label: 'fixed commitments only' }] : []),
    ...(filter.need ? [{ k: 'need', label: filter.need === 'optional' ? 'optional spending' : 'essential spending' }] : []),
    ...(filter.salary ? [{ k: 'salary', label: 'salary in the month it pays for' }] : []),
    ...(filter.min ? [{ k: 'min', label: `from ${eurc(Number(filter.min))}` }] : []),
    ...(filter.max ? [{ k: 'max', label: `up to ${eurc(Number(filter.max))}` }] : []),
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
        {period === 'custom' && <span className="chip-on">{filter.from ?? '…'} → {filter.to ?? '…'}</span>}
      </div>
      {activeFilters.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {activeFilters.map((f, i) => (
            <button key={i} className="chip-on" onClick={() => removeFilter(f)}>{f.label}<Icon name="x" size={12} /></button>
          ))}
        </div>
      )}

      <Owed />
      {data && filter.need ? (
        // Essential or optional spending: one total — the Free money card's
        // figure for the month — and a switch between the two.
        <div className="card mb-3 flex flex-wrap items-center justify-between gap-2 px-3 py-2" style={hue(filter.need === 'optional' ? 'var(--s7)' : 'var(--s1)')}>
          <div>
            <div className="text-xs text-muted">{filter.need === 'optional' ? 'Optional' : 'Essential'} spending · {data.total} payment{data.total === 1 ? '' : 's'}</div>
            <div className="tnum text-lg font-semibold">{eur(data.expense)}</div>
          </div>
          <Segmented size="sm" value={filter.need} onChange={(v) => set('need', v)}
            options={[{ value: 'essential', label: 'Essential' }, { value: 'optional', label: 'Optional' }]} />
        </div>
      ) : data && (
        <div className="mb-3 grid grid-cols-3 gap-2 text-center text-xs">
          {/* What these rows add up to — every expense, obligations included; Home's
              "spent" is only the day-to-day part of it. */}
          <div className="card px-2 py-2" style={hue('var(--hue-income)')}><div className="text-muted">Income</div><div className="tnum text-sm font-semibold text-good">{eur(data.income)}</div></div>
          <div className="card px-2 py-2" style={hue('var(--hue-spending)')} title="Every expense in these rows, fixed commitments and one-offs included — Home's 'spent' counts day-to-day spending only"><div className="text-muted">All spending</div><div className="tnum text-sm font-semibold">{eur(data.expense)}</div>{(data.refunds ?? 0) > 0.005 && <div className="tnum text-[11px] text-good">{eur(data.refunds!)} refunded</div>}</div>
          <div className="card px-2 py-2" style={hue('var(--hue-invest)')} title="Transfers between your accounts, investing and loan principal"><div className="text-muted">Transfers</div><div className="tnum text-sm font-semibold text-ink2">{eur(data.transfer)}</div></div>
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
                  <NeedRow key={t.id} t={t} showDate={!!filter.sort} selected={selected.has(t.id)}
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

function FilterSheet({ open, onClose, sp, setSp }: { open: boolean; onClose: () => void; sp: URLSearchParams; setSp: (s: URLSearchParams, o?: { replace?: boolean }) => void }) {
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
          <Segmented value={draft.get('kind') || ''} onChange={(v) => upd((n) => (v ? n.set('kind', v) : n.delete('kind')))}
            options={[{ value: '', label: 'All' }, { value: 'expense', label: 'Expenses' }, { value: 'income', label: 'Income' }, { value: 'transfer', label: 'Transfers' }]} />
        </div>
        <div>
          <div className="label">Spending</div>
          {/* The Free money card's split: free-spending expenses by your own mark on the row, else their category's essential flag (Settings → Categories); fixed lines left out. */}
          <Segmented value={draft.get('need') || ''} onChange={(v) => upd((n) => (v ? n.set('need', v) : n.delete('need')))}
            options={[{ value: '', label: 'All' }, { value: 'essential', label: 'Essential' }, { value: 'optional', label: 'Optional' }]} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <label><span className="label">From</span><DateInput clearable value={draft.get('from') || ''} onChange={(v) => upd((n) => { n.set('from', v); n.delete('period') })} /></label>
          <label><span className="label">To</span><DateInput clearable value={draft.get('to') || ''} onChange={(v) => upd((n) => { n.set('to', v); n.delete('period') })} /></label>
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
          <Segmented value={draft.get('sort') || ''} onChange={(v) => upd((n) => (v ? n.set('sort', v) : n.delete('sort')))}
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
              <MerchantMark merchant={r.merchant} category={r.category} transfer={r.kind === 'transfer'} to={r.to_account_id} size={32} />
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
