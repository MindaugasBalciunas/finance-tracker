import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { ledgerLink } from '../../lib/links'
import { addMonths, eur, shortDate, thisMonth } from '../../lib/format'
import type { TripSuggestion, Trips as TripsReport, Tx, TxList } from '../../lib/types'
import { Card, Empty, ErrorBox, Loading, Meter, Sheet, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { TxRow, useTxEditor } from '../../components/TxEditor'
import { DateInput } from '../../components/DateInput'

/** Plan → Trips: trips saved for on the wish list, trips costed from their
 *  trip:<name> tag, and untagged travel that looks like a trip. */
export function Trips() {
  const nav = useNavigate()
  const { data, isLoading } = useQuery({ queryKey: ['trips'], queryFn: () => api.get<TripsReport>('/trips') })
  const cats = useCats()
  const refresh = useRefresh()
  const toast = useToast()
  const [names, setNames] = useState<Record<number, string>>({})
  const [open, setOpen] = useState<number | null>(null)
  // Rows left out of a suggestion before tagging (unticked).
  const [skipped, setSkipped] = useState<Record<number, Set<number>>>({})
  const [showHistory, setShowHistory] = useState(false)
  const qc = useQueryClient()
  const setDone = async (tag: string, done: boolean) => {
    try {
      await api.put('/trips/history', { tag, done })
      toast(done ? 'Moved to history' : 'Back on the list', 'good')
      qc.invalidateQueries({ queryKey: ['trips'] })
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  if (isLoading) return <Loading />
  // Over and done with: no more dates or mapping prompts, kept apart below.
  const current = (data?.trips ?? []).filter((t) => !t.done)
  const past = (data?.trips ?? []).filter((t) => t.done)
  // Untagged travel from long ago isn't worth mapping any more either.
  const old = (s: TripSuggestion) => s.to < addMonths(thisMonth(), -2) + '-01'
  const suggestions = (data?.suggestions ?? []).map((s, i) => ({ s, i }))
  const recentSugg = suggestions.filter((x) => !old(x.s))
  const oldSugg = suggestions.filter((x) => old(x.s))
  const dates = Object.fromEntries((data?.dates ?? []).map((d) => [d.tag, d]))
  const tag = async (i: number, ids: number[]) => {
    const name = names[i]
    if (!name) return
    const keep = ids.filter((id) => !skipped[i]?.has(id))
    if (!keep.length) return
    await api.post('/trips/tag', { name, ids: keep })
    toast(`Tagged ${keep.length} transactions`, 'good')
    setSkipped({ ...skipped, [i]: new Set() })
    setOpen(null)
    refresh()
  }
  const toggleSkip = (i: number, id: number) => {
    const s = new Set(skipped[i] ?? [])
    s.has(id) ? s.delete(id) : s.add(id)
    setSkipped({ ...skipped, [i]: s })
  }
  const suggestionList = (list: { s: TripSuggestion; i: number }[]) => (
          <div className="divide-y divide-line">
            {list.map(({ s, i }) => {
              const kept = s.tx_ids.length - (skipped[i]?.size ?? 0)
              return (
                <div key={i}>
                  <div className="flex flex-wrap items-center gap-2 px-4 py-3">
                    <button className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={() => setOpen(open === i ? null : i)} aria-expanded={open === i}>
                      <Icon name="chevronD" size={16} className={clsx('shrink-0 text-muted transition', open === i && 'rotate-180')} />
                      <span className="min-w-0">
                        <span className="block text-sm font-medium">{shortDate(s.from)} – {shortDate(s.to)} · {eur(s.total)}</span>
                        <span className="block truncate text-xs text-muted">{open === i ? `${kept} of ${s.count} selected` : `${s.count} untagged travel rows · ${s.top?.join(', ')}`}</span>
                      </span>
                    </button>
                    <input className="input h-8 w-36 text-xs" placeholder="trip name" value={names[i] ?? ''} onChange={(e) => setNames({ ...names, [i]: e.target.value })} />
                    <button className="btn-primary h-8 text-xs" onClick={() => tag(i, s.tx_ids)} disabled={!names[i] || kept === 0}>Tag{open === i ? ` ${kept}` : ''}</button>
                  </div>
                  {open === i && <SuggestionRows s={s} skipped={skipped[i] ?? new Set()} onToggle={(id) => toggleSkip(i, id)} />}
                </div>
              )
            })}
          </div>
  )
  return (
    <div className="space-y-4">
      {/* Trips ahead are saved for on the wish list, funded with everything else. */}
      <Card title="Planned trips" pad={false} action={<button className="btn-ghost h-8 px-2.5 text-xs text-accent" onClick={() => nav('/plan/wishlist?trip=1')}><Icon name="plus" size={14} />Plan a trip</button>}>
        {!data?.planned?.length ? (
          <div className="border-t border-line px-4 py-3 text-sm text-muted">No trip planned. Plan one and save for it on the wish list — what's tagged to it later counts against what you saved.</div>
        ) : (
          <div className="divide-y divide-line border-t border-line">
            {data.planned.map((g) => (
              <div key={g.id} className="px-4 py-3">
              <button onClick={() => nav('/plan/wishlist')} className="block w-full text-left hover:opacity-80">
                <div className="flex items-baseline justify-between gap-2 text-sm">
                  <span className="truncate font-medium"><Icon name="plane" size={14} className="mr-1.5 inline text-muted" />{g.name}{g.target_date && <span className="font-normal text-muted"> · {shortDate(g.target_date)}</span>}</span>
                  <span className="tnum">{eur(g.saved)} <span className="text-muted">of {eur(g.target)}</span></span>
                </div>
                <Meter value={g.saved} max={g.target} goal className="mt-2" />
                {(g.spent ?? 0) > 0 && <div className="mt-1 text-xs text-muted tnum">{eur(g.spent)} already spent under {g.tag}</div>}
              </button>
              {g.tag && <TripDatesRow tag={g.tag} dates={dates[g.tag]} guess={{ start: g.target_date?.slice(0, 10) ?? '', end: g.target_date?.slice(0, 10) ?? '' }} />}
              </div>
            ))}
          </div>
        )}
      </Card>
      {recentSugg.length > 0 && <Card title="Looks like a trip" pad={false}>{suggestionList(recentSugg)}</Card>}
      {!current.length ? (past.length ? <div className="text-sm text-muted">No trip on the list — past ones are in History below.</div> : <Empty title="No trips yet" icon="plane">Tag travel transactions with trip:name to cost a trip as a whole.</Empty>) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {current.map((t) => (
            <div key={t.tag} className="card p-4">
              <a href={`#/ledger?period=all&tag=${encodeURIComponent(t.tag)}`} className="block hover:opacity-80">
                <div className="flex items-baseline justify-between">
                  <div className="font-semibold capitalize">{t.name.replace(/-/g, ' ')}</div>
                  <div className="tnum font-semibold">{eur(t.total)}</div>
                </div>
                <div className="text-xs text-muted">{t.count} rows{t.days <= 31 ? ` · ${eur(t.per_day)}/day` : ''}{t.booked_from && ` · booked from ${shortDate(t.booked_from)}`}</div>
              </a>
              {t.budget != null && <Meter className="mt-2" value={t.total} max={t.budget} />}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {t.by_category.slice(0, 4).map((c) => <span key={c.name} className="chip">{cats.name(c.name)} {eur(c.amount)}</span>)}
              </div>
              <TripDatesRow tag={t.tag} dates={dates[t.tag]} guess={{ start: t.from, end: t.to }} />
              <button className="btn-ghost mt-1 h-8 px-2 text-xs text-muted" onClick={() => setDone(t.tag, true)}><Icon name="check" size={14} />Trip's over — move to history</button>
            </div>
          ))}
        </div>
      )}
      {(past.length > 0 || oldSugg.length > 0) && (
        <Card pad={false} icon="calendar" color="var(--s-other)" title={past.length ? `History · ${past.length} ${past.length === 1 ? 'trip' : 'trips'}` : 'History'}
          action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowHistory(!showHistory)} aria-expanded={showHistory}>{showHistory ? 'Hide' : 'Show'}</button>}>
          <div className="-mt-1 px-4 pb-2 text-xs text-muted">Trips that are over — a month after they end, or moved here by hand. Their costs stay in the Ledger.</div>
          {showHistory && (
            <div className="divide-y divide-line border-t border-line">
              {past.map((t) => (
                <div key={t.tag} className="flex items-center gap-3 px-4 py-2.5">
                  <Link to={ledgerLink({ tag: t.tag })} className="min-w-0 flex-1 hover:text-accent">
                    <div className="truncate text-sm font-medium capitalize">{t.name.replace(/-/g, ' ')}</div>
                    <div className="text-xs text-muted">{shortDate(dates[t.tag]?.start ?? t.from)} – {shortDate(dates[t.tag]?.end ?? t.to)} · {t.count} rows</div>
                  </Link>
                  <span className="tnum text-sm font-semibold">{eur(t.total)}</span>
                  <button className="btn-ghost h-8 px-2 text-xs text-muted" onClick={() => setDone(t.tag, false)}>Bring back</button>
                </div>
              ))}
              {oldSugg.length > 0 && (
                <div>
                  <div className="px-4 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wide text-muted">Older untagged travel</div>
                  {suggestionList(oldSugg)}
                </div>
              )}
            </div>
          )}
        </Card>
      )}
    </div>
  )
}

/** A trip's real dates — set once, its spending tags itself from then on —
 *  and a sweep for what it missed: untagged spending inside the dates
 *  (ticked) and travel booked in the nine months before (to confirm). */
function TripDatesRow({ tag, dates, guess }: { tag: string; dates?: { start: string; end: string }; guess: { start: string; end: string } }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState(false)
  const [start, setStart] = useState(dates?.start ?? guess.start)
  const [end, setEnd] = useState(dates?.end ?? guess.end)
  const [sweep, setSweep] = useState(false)
  const save = async (clear = false) => {
    try {
      if (clear) await api.del('/trips/dates', { tag })
      else await api.put('/trips/dates', { tag, start, end })
      qc.invalidateQueries({ queryKey: ['trips'] }); qc.invalidateQueries({ queryKey: ['tag-suggest'] })
      toast(clear ? 'Dates cleared' : 'Dates saved — spending inside them tags itself', 'good')
      setEdit(false)
      if (!clear) setSweep(true)
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  return (
    <div className="mt-3 border-t border-line pt-2 text-xs">
      {dates ? (
        <div className="flex flex-wrap items-center gap-2">
          <Icon name="calendar" size={14} className="text-accent" />
          <span className="font-medium">{shortDate(dates.start)} – {shortDate(dates.end)}</span>
          <span className="text-muted">· spending inside tags itself</span>
          <span className="ml-auto flex gap-2">
            <button className="text-accent" onClick={() => setSweep(true)}>Find untagged</button>
            <button className="text-muted" onClick={() => setEdit(true)}>Edit</button>
          </span>
        </div>
      ) : (
        <button className="flex items-center gap-1.5 text-accent" onClick={() => setEdit(true)}><Icon name="calendar" size={14} />Set the trip's dates — then its spending tags itself</button>
      )}
      {edit && (
        <Sheet open onClose={() => setEdit(false)} title={`Dates · ${tag.slice(5).replace(/-/g, ' ')}`} footer={<>
          {dates && <button className="btn-ghost text-bad" onClick={() => save(true)}>Clear</button>}
          <button className="btn-primary" disabled={!start || !end || end < start} onClick={() => save()}>Save</button>
        </>}>
          <div className="grid grid-cols-2 gap-3">
            <label><span className="label">First day</span><DateInput value={start} onChange={(v) => setStart(v)} /></label>
            <label><span className="label">Last day</span><DateInput value={end} min={start} onChange={(v) => setEnd(v)} /></label>
          </div>
          <p className="mt-3 text-sm text-muted">New and bank-synced expenses dated inside get <b>{tag}</b> automatically — bills, obligations and transfers aside. Flights and hotels booked earlier are suggested, not added.</p>
        </Sheet>
      )}
      {sweep && <TripSweep tag={tag} onClose={() => setSweep(false)} />}
    </div>
  )
}

function TripSweep({ tag, onClose }: { tag: string; onClose: () => void }) {
  const qc = useQueryClient()
  const toast = useToast()
  const { data, isLoading, error } = useQuery({ queryKey: ['trip-untagged', tag], queryFn: () => api.get<{ in_dates: Tx[]; maybe: Tx[]; bookings: Tx[] }>('/trips/untagged', { tag }) })
  const [pick, setPick] = useState<Set<number> | null>(null)
  const chosen = pick ?? new Set((data?.in_dates ?? []).map((t) => t.id!))
  const toggle = (id: number) => { const n = new Set(chosen); n.has(id) ? n.delete(id) : n.add(id); setPick(n) }
  const apply = async () => {
    await api.post('/transactions/bulk', { ids: [...chosen], add_tags: [tag] })
    qc.invalidateQueries()
    toast(`Tagged ${chosen.size} with ${tag}`, 'good')
    onClose()
  }
  const rows = (title: string, list: Tx[], hint: string) => list.length > 0 && (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">{title} <span className="font-normal normal-case">· {hint}</span></div>
      <div className="divide-y divide-line rounded-xl border border-line">
        {list.map((t) => (
          <label key={t.id} className="flex items-center gap-3 px-3 py-2 text-sm">
            <input type="checkbox" className="h-4 w-4 accent-[rgb(var(--accent))]" checked={chosen.has(t.id!)} onChange={() => toggle(t.id!)} />
            <span className="min-w-0 flex-1"><span className="block truncate">{t.merchant || t.note || '—'}</span><span className="block text-xs text-muted">{shortDate(t.date)} · {t.category}</span></span>
            <span className="tnum">{eur(t.amount)}</span>
          </label>
        ))}
      </div>
    </div>
  )
  return (
    <Sheet open onClose={onClose} title={`Untagged · ${tag.slice(5).replace(/-/g, ' ')}`} footer={<button className="btn-primary" disabled={!chosen.size} onClick={apply}>Tag {chosen.size}</button>}>
      {isLoading ? <Loading /> : error ? <ErrorBox error={error} /> : !data?.in_dates.length && !data?.maybe.length && !data?.bookings.length ? (
        <Empty title="Nothing missed" icon="check">Every expense in the trip's dates already carries a trip tag.</Empty>
      ) : (
        <div className="space-y-4">
          {rows('In the trip\'s dates', data!.in_dates, 'places you don\'t pay at home — ticked')}
          {rows('Maybe', data!.maybe, 'in the dates too, but a place you pay at home, kids\' costs, or no merchant — tick the trip\'s')}
          {rows('Booked before', data!.bookings, 'flights and stays in the nine months before — tick the ones for this trip')}
        </div>
      )}
    </Sheet>
  )
}

/** The transactions a trip suggestion would tag, each with a tick box. */
function SuggestionRows({ s, skipped, onToggle }: { s: TripSuggestion; skipped: Set<number>; onToggle: (id: number) => void }) {
  const ids = new Set<number>(s.tx_ids)
  const { data, isLoading } = useQuery({
    queryKey: ['trip-suggestion', s.from, s.to],
    queryFn: () => api.get<TxList>('/transactions', { from: s.from, to: s.to, limit: 500, sort: '+date' }),
  })
  const editor = useTxEditor()
  if (isLoading) return <div className="px-4 pb-3"><Loading /></div>
  const rows = (data?.items ?? []).filter((t) => ids.has(t.id))
  return (
    <div className="border-t border-line bg-sunken/30">
      <div className="divide-y divide-line">
        {rows.map((t) => <TxRow key={t.id} t={t} showDate selected={!skipped.has(t.id)} onSelect={() => onToggle(t.id)} onClick={() => editor.open(t)} />)}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2 text-xs text-muted">
        <span>Untick anything that wasn't part of the trip; tap a row to edit it.</span>
        <a className="text-accent" href={`#/ledger?from=${s.from}&to=${s.to}`}>Everything in these dates in the Ledger →</a>
      </div>
    </div>
  )
}
