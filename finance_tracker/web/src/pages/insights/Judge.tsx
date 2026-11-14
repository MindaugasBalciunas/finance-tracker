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
type V = 'worth' | 'cut' | 'unsure'
const VERDICTS: { v: V; label: string; icon: string; on: string }[] = [
  { v: 'worth', label: 'Worth it', icon: 'check', on: 'bg-good/15 text-good border-good/40' },
  { v: 'cut', label: 'Cut', icon: 'x', on: 'bg-bad/15 text-bad border-bad/40' },
  { v: 'unsure', label: 'Not sure', icon: 'dots', on: 'bg-sunken text-ink border-line' },
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

/** Worth it or a leak? The owner's verdict on each regular place and big
 *  category, with what the data suggests, and the year's optional spending
 *  added up by verdict. */
export function JudgeCard({ r }: { r: OptionalReport }) {
  const j = r.judge
  const [all, setAll] = useState(false)
  if (!j.items.length) return null
  // Biggest first, in a stable order: a row must not jump away when you judge it.
  const items = j.items
  const shown = all ? items : items.slice(0, 8)
  const total = Math.max(j.worth + j.cut + j.unsure + j.open, 1)
  const parts = [
    { k: 'worth', label: 'Worth it', v: j.worth, color: 'rgb(var(--good))' },
    { k: 'cut', label: 'To cut', v: j.cut, color: 'rgb(var(--bad))' },
    { k: 'unsure', label: 'Not sure', v: j.unsure, color: 'var(--s-other)' },
  ]
  return (
    <Card pad={false} icon="check" color="var(--s7)" title="Worth it or a leak?">
      <div className="px-4 pb-3">
        <div className="text-xs text-muted">Your call on each regular place and big category. A payment takes its place's verdict, else its own category's. Last 12 months:</div>
        <div className="mt-2 flex h-2.5 w-full gap-px overflow-hidden rounded-full bg-line" role="img"
          aria-label={`Worth it ${eur0(j.worth)}, to cut ${eur0(j.cut)}, not sure ${eur0(j.unsure)}, not judged ${eur0(j.open)}`}>
          {parts.filter((p) => p.v > 0.005).map((p) => <div key={p.k} className="h-full" style={{ width: `${(p.v / total) * 100}%`, background: p.color }} />)}
        </div>
        <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted">
          {parts.map((p) => <span key={p.k} className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-sm" style={{ background: p.color }} />{p.label} <span className="tnum text-ink2">{eur0(p.v)}</span></span>)}
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-sm border border-axis" />Not judged <span className="tnum text-ink2">{eur0(j.open)}</span></span>
        </div>
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
    <div className="px-4 py-2.5">
      <div className="flex items-center gap-3">
        <span className="grid w-7 shrink-0 place-items-center">
          {it.kind === 'merchant' ? <MerchantMark merchant={it.name} size={28} /> : <Icon name="tag" size={16} className="text-muted" />}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm">{name}</span>
          <span className="block truncate text-xs text-muted">{eur0(it.year)} a year · {eur(it.usual)}/mo · {eur(it.month)} this month</span>
        </span>
      </div>
      {!it.verdict && it.suggest && (
        <div className={clsx('ml-10 mt-1 text-xs', it.suggest === 'cut' ? 'text-warn' : 'text-good')}>
          {it.suggest === 'cut' ? 'Looks like a leak' : 'Looks worth it'} — {it.why}
        </div>
      )}
      <div className="ml-10 mt-1.5 flex gap-1.5" role="group" aria-label={`Your verdict on ${name}`}>
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
      </div>
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
