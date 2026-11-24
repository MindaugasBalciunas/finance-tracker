import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur, monthLabel, todayISO } from '../../lib/format'
import { useCats } from '../../lib/categories'
import type { JudgeItem, OptionalReport } from '../../lib/types'
import { Card, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { MerchantMark } from '../../components/MerchantLogo'

const eur0 = (v: number) => eur(Math.round(v))
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`

/** The twelve complete months every "last 12 months" figure covers, e.g. "Nov '25 – Oct '26". */
export function yearRange(r: OptionalReport): string {
  const last = r.months[r.months.length - 2]?.month
  return last ? `${monthLabel(r.months[0].month)} – ${monthLabel(last)}` : monthLabel(r.months[0].month)
}

/** How a place or category was paid over the twelve months: how often, and
 *  a monthly figure (the 12 months over 12) only when it really is
 *  monthly-ish, paid in six or more of the months — "€54/mo" beside four
 *  payments a year says nothing. */
export function paceLine(l: { usual: number; count: number; months: number; month: number }): string {
  const parts = [l.count === 0 ? 'only this month' : l.months >= 6 || l.count === 1 ? plural(l.count, 'payment') : `${plural(l.count, 'payment')} in ${plural(l.months, 'month')}`]
  if (l.months >= 6) parts.push(`${eur(l.usual)} a month on average`)
  if (l.month > 0) parts.push(`${eur(l.month)} this month`)
  return parts.join(' · ')
}
type V = 'worth' | 'cut'
// Keep is stored as "worth", cut back as "cut" (the verdicts table's words).
const VERDICTS: { v: V; label: string; icon: string; on: string }[] = [
  { v: 'worth', label: 'Keep', icon: 'check', on: 'bg-good/15 text-good border-good/40' },
  { v: 'cut', label: 'Cut back', icon: 'down', on: 'bg-warn/15 text-warn border-warn/40' },
]

function useVerdict() {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ it, v }: { it: JudgeItem; v: V | null }) => v
      ? api.put('/insights/optional/verdicts', { kind: it.kind, key: it.key, verdict: v })
      : api.del('/insights/optional/verdicts', { kind: it.kind, key: it.key }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['insights-optional'] }),
    onError: (e) => toast((e as Error).message, 'bad'),
  })
}

/** Your regular optional spending: the habits — places and categories paid
 *  in six or more of the last twelve months — with what each costs a month,
 *  and your call: keep it or cut back. One-off shops (a laptop, DIY runs)
 *  and guessed "leak" labels were dropped in 3.1.1: there is nothing to keep
 *  or cut about a single purchase, and buying late in a month doesn't make
 *  something a leak. */
export function JudgeCard({ r }: { r: OptionalReport }) {
  const j = r.judge
  const [all, setAll] = useState(false)
  if (!j.items.length) return null
  // Biggest first, in a stable order: a row must not jump away when you judge it.
  const items = j.items
  const shown = all ? items : items.slice(0, 8)
  const sum = (v?: V) => items.filter((it) => (v ? it.verdict === v : true)).reduce((a, it) => a + it.usual, 0)
  const keep = sum('worth'), cut = sum('cut')
  return (
    <Card pad={false} icon="repeat" color="var(--s7)" title="Your regular optional spending">
      <div className="px-4 pb-3 text-xs text-muted">
        Places and categories you paid in 6 or more of the last 12 months ({yearRange(r)}). About <b className="tnum text-ink2">{eur0(sum())}</b> a month in all. Keep it, or cut back?
        {(keep > 0 || cut > 0) && <div className="mt-1">Keeping <b className="tnum text-good">{eur0(keep)}</b> a month · cutting back <b className="tnum text-warn">{eur0(cut)}</b> a month</div>}
      </div>
      <div className="divide-y divide-line border-t border-line">
        {shown.map((it) => <JudgeRow key={it.kind + it.key} it={it} />)}
      </div>
      {items.length > 8 && (
        <button className="w-full border-t border-line px-4 py-2.5 text-center text-sm text-accent hover:bg-sunken/40" onClick={() => setAll(!all)}>
          {all ? 'Show fewer' : `Show all ${items.length}`}
        </button>
      )}
    </Card>
  )
}

function JudgeRow({ it }: { it: JudgeItem }) {
  const cats = useCats()
  const set = useVerdict()
  const name = it.kind === 'category' ? cats.path(it.key) : it.name || it.key
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-4 py-2.5">
      <span className="grid w-7 shrink-0 place-items-center">
        {it.kind === 'merchant' ? <MerchantMark merchant={it.name} size={28} /> : <Icon name="tag" size={16} className="text-muted" />}
      </span>
      {/* Phone: the name takes the row and the buttons go under it. */}
      <span className="min-w-0 flex-1 basis-[calc(100%-2.5rem)] sm:basis-auto">
        <span className="block text-sm"><span className="font-medium">{name}</span> <span className="tnum text-ink2">{eur0(it.usual)}/mo</span></span>
        <span className="block text-xs text-muted">{eur0(it.year)} in 12 months · {plural(it.months, 'month')} of 12{it.month > 0 && <> · {eur(it.month)} this month</>}</span>
      </span>
      <span className="ml-10 flex gap-1.5 sm:ml-0" role="group" aria-label={`Your call on ${name}`}>
        {VERDICTS.map((x) => {
          const on = it.verdict === x.v
          return (
            <button key={x.v} type="button" aria-pressed={on} disabled={set.isPending}
              className={clsx('inline-flex h-10 items-center gap-1 rounded-full border px-3 text-xs', on ? x.on : 'border-line text-ink2 hover:bg-sunken')}
              onClick={() => set.mutate({ it, v: on ? null : x.v })}>
              <Icon name={x.icon} size={14} />{x.label}
            </button>
          )
        })}
      </span>
    </div>
  )
}

/** What the owner said to cut, and how it has gone since. */
export function CuttingCard({ r }: { r: OptionalReport }) {
  const cats = useCats()
  const cut = r.judge.cutting
  if (!cut.length) return null
  return (
    <Card pad={false} icon="trend" color="rgb(var(--bad))" title="Cutting back">
      <div className="divide-y divide-line border-t border-line">
        {cut.map((f) => {
          const name = f.kind === 'category' ? cats.path(f.key) : f.name || f.key
          const last = f.last_month
          const tone = last == null ? '' : last <= f.usual * 0.75 ? 'text-good' : last >= f.usual ? 'text-bad' : 'text-warn'
          return (
            <div key={f.kind + f.key} className="px-4 py-2.5">
              <div className="flex items-baseline justify-between gap-2">
                <span className="min-w-0 truncate text-sm font-medium">{name}</span>
                <span className="shrink-0 text-xs text-muted">said cut {monthLabel(f.judged_on.slice(0, 7))} · was {eur(f.usual)}/mo</span>
              </div>
              <div className="mt-0.5 text-xs text-ink2">
                <span className="tnum">{eur(f.this_month)}</span> {f.judged_on.slice(0, 7) === todayISO().slice(0, 7) ? 'since you said cut' : 'so far this month'}
                {last != null && <> · last month <span className={clsx('tnum font-semibold', tone)}>{eur(last)}</span>{f.usual > 0 && <span className={tone}> ({last <= f.usual ? '−' : '+'}{Math.round(Math.abs(1 - last / f.usual) * 100)}%)</span>}</>}
              </div>
            </div>
          )
        })}
      </div>
    </Card>
  )
}
