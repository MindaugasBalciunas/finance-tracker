import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { eur, monthLabel, pct, shortDate } from '../../lib/format'
import type { TagReport, TagStat } from '../../lib/types'
import { Card, Field, Loading, Segmented, Sheet, Stat, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

// ── tags ────────────────────────────────────────────────────────────

const SORTS = [{ value: 'net', label: 'Spend' }, { value: 'year', label: '12 months' }, { value: 'count', label: 'Uses' }, { value: 'last', label: 'Recent' }]

/** Tags say who or why; this shows what each one has cost, when, and on what. */
export function Tags() {
  const { data, isLoading } = useQuery({ queryKey: ['tag-stats'], queryFn: () => api.get<TagReport>('/tags/stats') })
  const [sort, setSort] = useState('net')
  const [q, setQ] = useState('')
  const [open, setOpen] = useState<TagStat | null>(null)
  const [rename, setRename] = useState<{ from: string; to: string } | null>(null)
  if (isLoading || !data) return <Loading />
  const key = (t: TagStat) => (sort === 'last' ? t.last : sort === 'count' ? t.count : sort === 'year' ? t.year : t.net)
  const list = data.tags.filter((t) => !q || t.tag.includes(q.toLowerCase())).sort((a, b) => (key(b) > key(a) ? 1 : key(b) < key(a) ? -1 : 0))
  const trips = list.filter((t) => t.trip)
  const other = list.filter((t) => !t.trip)
  const tripsYear = data.tags.filter((t) => t.trip).reduce((a, t) => a + t.year, 0)
  const active = data.tags.filter((t) => t.year !== 0).length
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon="tag" label="Tags" value={data.tags.length} sub={`${active} used in 12 months`} />
        <Stat icon="bag" color="var(--s2)" label="Spending tagged" value={pct(data.tagged_share)} sub={`of ${eur(data.year_spent)} · 12 months`} />
        <Stat icon="plane" color="var(--s4)" label="Trips · 12 months" value={eur(tripsYear)} sub={`${data.tags.filter((t) => t.trip).length} trips in total`} />
        <Stat icon="rule" color="var(--s5)" label="Added by rules" value={data.tags.filter((t) => t.rules > 0).length} sub="tags some rule adds" />
      </div>
      <TagSuggestions onPick={(from, to) => setRename({ from, to })} />
      <div className="flex flex-wrap items-center gap-2">
        <input className="input w-full sm:max-w-xs sm:flex-1" placeholder="Find a tag" value={q} onChange={(e) => setQ(e.target.value)} />
        <Segmented size="sm" value={sort} onChange={setSort} options={SORTS} />
      </div>
      {other.length > 0 && <TagList title="Tags" tags={other} months={data.months} onOpen={setOpen} />}
      {trips.length > 0 && <TagList title="Trips" tags={trips} months={data.months} onOpen={setOpen} />}
      <div className="text-xs text-muted">Net = spending minus refunds and income carrying the tag. Rename a tag to merge it into another; remove it to take it off every transaction and rule.</div>
      {open && <TagSheet t={open} months={data.months} onClose={() => setOpen(null)} onRename={() => { setRename({ from: open.tag, to: open.tag }); setOpen(null) }} />}
      {rename && <RenameSheet edit={rename} onChange={setRename} onClose={() => setRename(null)} />}
    </div>
  )
}

function TagList({ title, tags, months, onOpen }: { title: string; tags: TagStat[]; months: string[]; onOpen: (t: TagStat) => void }) {
  const cats = useCats()
  return (
    <Card pad={false} title={<span>{title} <span className="font-normal text-muted">{tags.length}</span></span>}>
      <div className="divide-y divide-line border-t border-line">
        {tags.map((t) => (
          <button key={t.tag} onClick={() => onOpen(t)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{t.trip ? t.tag.slice(5) : t.tag}{t.rules > 0 && <span className="ml-1.5 rounded-full bg-sunken px-1.5 py-0.5 text-[10px] font-normal text-ink2">rule</span>}</div>
              <div className="truncate text-xs text-muted">
                {t.count}× · {t.first.slice(0, 7) === t.last.slice(0, 7) ? monthLabel(t.first.slice(0, 7)) : `${monthLabel(t.first.slice(0, 7))} – ${monthLabel(t.last.slice(0, 7))}`}
                {t.categories[0] && ` · mostly ${cats.name(t.categories[0].name)}`}
              </div>
            </div>
            <Spark values={t.months} months={months} />
            <div className="w-24 shrink-0 text-right">
              <div className="tnum text-sm font-semibold">{eur(t.net)}</div>
              <div className="tnum text-[11px] text-muted">{t.year !== 0 ? `${eur(t.year)} · 12 mo` : `last ${shortDate(t.last)}`}</div>
            </div>
          </button>
        ))}
      </div>
    </Card>
  )
}

/** Twelve tiny monthly bars, oldest first; empty months stay as a dot. */
function Spark({ values, months }: { values: number[]; months: string[] }) {
  const max = Math.max(...values.map((v) => Math.max(v, 0)), 1)
  if (values.every((v) => v <= 0)) return <div className="hidden w-20 sm:block" />
  return (
    <div className="hidden h-7 w-20 shrink-0 items-end gap-px sm:flex" aria-hidden>
      {values.map((v, i) => <div key={months[i]} title={`${monthLabel(months[i])}: ${eur(v)}`} className={clsx('flex-1 rounded-sm', v > 0 ? 'bg-accent/70' : 'bg-line')} style={{ height: v > 0 ? `${Math.max(8, (v / max) * 100)}%` : 2 }} />)}
    </div>
  )
}

function TagSheet({ t, months, onClose, onRename }: { t: TagStat; months: string[]; onClose: () => void; onRename: () => void }) {
  const cats = useCats()
  const max = Math.max(...t.months, 1)
  return (
    <Sheet open onClose={onClose} title={t.trip ? `Trip ${t.tag.slice(5)}` : `#${t.tag}`} wide footer={<>
      <a className="btn-outline mr-auto" href={`#/ledger?period=all&tag=${encodeURIComponent(t.tag)}`} onClick={onClose}><Icon name="list" size={16} />Transactions</a>
      <button className="btn-primary" onClick={onRename}><Icon name="edit" size={16} />Rename or remove</button>
    </>}>
      <div className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-4">
        <div><div className="text-xs text-muted">Net spend</div><div className="text-lg font-semibold tnum">{eur(t.net)}</div></div>
        <div><div className="text-xs text-muted">Last 12 months</div><div className="text-lg font-semibold tnum">{eur(t.year)}</div></div>
        <div><div className="text-xs text-muted">Per active month</div><div className="text-lg font-semibold tnum">{eur(t.per_month)}</div></div>
        <div><div className="text-xs text-muted">Transactions</div><div className="text-lg font-semibold tnum">{t.count}</div></div>
      </div>
      <div className="mt-2 text-xs text-muted">{shortDate(t.first)} – {shortDate(t.last)} · spent {eur(t.spent)}{t.income > 0 && ` · ${eur(t.income)} back (refunds, income)`}{t.rules > 0 && ` · added by ${t.rules} ${t.rules === 1 ? 'rule' : 'rules'}`}</div>
      <div className="mt-4 section-title">Last 12 months</div>
      <div className="mt-2 flex h-28 items-end gap-1">
        {t.months.map((v, i) => (
          <div key={months[i]} className="flex h-full flex-1 flex-col justify-end" title={`${monthLabel(months[i], true)}: ${eur(v)}`}>
            <div className={clsx('rounded-t', v > 0 ? 'bg-accent' : 'bg-line')} style={{ height: v > 0 ? `${Math.max(4, (v / max) * 100)}%` : 2 }} />
          </div>
        ))}
      </div>
      <div className="mt-1 flex justify-between text-[10px] text-muted"><span>{monthLabel(months[0])}</span><span>{monthLabel(months[months.length - 1])}</span></div>
      <div className="mt-5 grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Top title="Categories" rows={t.categories.map((c) => ({ ...c, name: cats.path(c.name) }))} total={t.spent} />
        <Top title="Merchants" rows={t.merchants} total={t.spent} />
      </div>
    </Sheet>
  )
}

function Top({ title, rows, total }: { title: string; rows: { name: string; amount: number; count: number }[]; total: number }) {
  if (!rows.length) return null
  return (
    <div>
      <div className="section-title mb-2">{title}</div>
      <div className="space-y-2">
        {rows.map((r) => (
          <div key={r.name}>
            <div className="flex justify-between gap-2 text-sm"><span className="truncate">{r.name} <span className="text-xs text-muted">{r.count}×</span></span><span className="tnum">{eur(r.amount)}</span></div>
            <div className="mt-1 h-1 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent/70" style={{ width: `${total > 0 ? (r.amount / total) * 100 : 0}%` }} /></div>
          </div>
        ))}
      </div>
    </div>
  )
}

function RenameSheet({ edit, onChange, onClose }: { edit: { from: string; to: string }; onChange: (e: { from: string; to: string }) => void; onClose: () => void }) {
  const refresh = useRefresh()
  const toast = useToast()
  const run = async () => {
    const r = await api.post<any>('/tags/rename', edit)
    toast(`${r.changed} transactions updated`, 'good')
    refresh()
    onClose()
  }
  return (
    <Sheet open onClose={onClose} title={`Tag “${edit.from}”`} footer={<>
      <button className="btn-danger mr-auto" onClick={() => onChange({ ...edit, to: '' })}>Remove everywhere</button>
      <button className="btn-primary" onClick={run}>{edit.to ? (edit.to === edit.from ? 'Keep' : 'Rename') : 'Remove'}</button>
    </>}>
      <Field label="New name" hint="An existing tag's name merges the two."><input className="input" value={edit.to} onChange={(e) => onChange({ ...edit, to: e.target.value })} /></Field>
    </Sheet>
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
