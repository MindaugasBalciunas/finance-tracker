import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { ledgerLink } from '../../lib/links'
import { eur, shortDate, todayISO } from '../../lib/format'
import { useCats } from '../../lib/categories'
import type { Subscription, Subscriptions } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading, useToast } from '../../components/ui'
import { DateInput } from '../../components/DateInput'
import { MerchantMark } from '../../components/MerchantLogo'
import { Icon } from '../../components/Icon'

/** The subscription audit: what you pay for regularly that you could cancel,
 *  what each costs a year, and your verdict — keep it, or cancel by a date. */
export function SubscriptionsCard() {
  const { data, isLoading, error } = useQuery({ queryKey: ['subscriptions'], queryFn: () => api.get<Subscriptions>('/plan/subscriptions') })
  if (isLoading) return <Card id="subscriptions" title="Subscription review" icon="repeat"><Loading /></Card>
  if (error || !data) return <ErrorBox error={error} />
  if (!data.items.length) return null
  // Only what still needs a verdict stays open (a cancelled one charged
  // again needs one too); the decided ones fold away.
  const pending = data.items.filter((s) => !s.verdict || s.charged_after_cancel)
  const reviewed = data.items.filter((s) => s.verdict && !s.charged_after_cancel)
  const open = pending.length
  return (
    <Card id="subscriptions" title="Subscription review" icon="repeat" color={data.items.some((s) => s.charged_after_cancel) ? 'var(--hue-bad)' : open ? 'var(--hue-warn)' : 'var(--hue-good)'}
      action={<AskCFO q="Audit my subscriptions: which look worth cancelling, which overlap, and what would I save a year?" />}>
      <div className="grid grid-cols-3 gap-3 text-sm">
        <div><div className="text-xs text-muted">A month</div><div className="tnum text-xl font-semibold">{eur(data.monthly)}</div></div>
        <div><div className="text-xs text-muted">A year</div><div className="tnum text-xl font-semibold">{eur(data.yearly)}</div></div>
        <div><div className="text-xs text-muted">Cancelling saves</div><div className={clsx('tnum text-xl font-semibold', data.to_cancel > 0 && 'text-good')}>{data.to_cancel > 0 ? `${eur(data.to_cancel)}/yr` : '—'}</div></div>
      </div>
      {open > 0 && <p className="mt-2 text-xs text-ink2">{open} still to review — keep or cancel each; the bell reminds you on the day. Tap one to see its charges.</p>}
      {pending.length > 0 && <ul className="mt-2 divide-y divide-line">{pending.map((s) => <Row key={s.key} s={s} />)}</ul>}
      {reviewed.length > 0 && (
        <details className="mt-1 border-t border-line">
          <summary className="cursor-pointer py-2 text-xs text-ink2">Reviewed ({reviewed.length})</summary>
          <ul className="divide-y divide-line">{reviewed.map((s) => <Row key={s.key} s={s} />)}</ul>
        </details>
      )}
    </Card>
  )
}

function Row({ s }: { s: Subscription }) {
  const cats = useCats()
  const qc = useQueryClient()
  const toast = useToast()
  const [picking, setPicking] = useState(false)
  const [day, setDay] = useState(s.next && s.next > todayISO() ? s.next : todayISO())
  const set = useMutation({
    mutationFn: (v: { verdict: string; cancel_by?: string }) => api.put('/plan/subscriptions', { key: s.key, ...v }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['subscriptions'] }); qc.invalidateQueries({ queryKey: ['overview'] }); setPicking(false) },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  const v = s.verdict
  return (
    <li className="py-2.5">
      {/* The charges behind the verdict, before you make it. */}
      <Link to={ledgerLink({ merchant: s.merchant })} className="-mx-2 flex items-center gap-3 rounded-lg px-2 py-1 hover:bg-sunken/40" title="See its transactions">
        <MerchantMark merchant={s.merchant} category={s.category} size={32} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
            <span className="truncate">{s.merchant}</span>
            {s.new && <span className="rounded-full bg-accent/10 px-1.5 text-[10px] text-accent">new</span>}
            {s.trial_from && <span className="rounded-full bg-warn/15 px-1.5 text-[10px] text-warn">began as a trial {shortDate(s.trial_from)}</span>}
            {s.charged_after_cancel && <span className="rounded-full bg-bad/15 px-1.5 text-[10px] text-bad">charged {shortDate(s.charged_after_cancel)} after you said cancel</span>}
          </div>
          <div className="truncate text-xs text-muted">{cats.name(s.category)} · {s.cadence} · since {shortDate(s.since)}{s.next ? ` · next ${shortDate(s.next)}` : ''}</div>
        </div>
        <div className="text-right">
          <div className="tnum text-sm font-semibold">{eur(s.yearly)}<span className="text-xs font-normal text-muted">/yr</span></div>
          <div className="tnum text-xs text-muted">{eur(s.amount)} {s.cadence === 'monthly' ? '/mo' : s.cadence === 'yearly' ? '/yr' : '/qtr'}</div>
        </div>
        <Icon name="chevronR" size={14} className="shrink-0 text-muted" />
      </Link>
      <div className="mt-2 flex flex-wrap items-center gap-2 pl-11">
        {v?.verdict === 'keep' && <span className="text-xs text-good">Kept {v.decided_on ? shortDate(v.decided_on) : ''}</span>}
        {v?.verdict === 'cancel' && <span className="text-xs text-warn">Cancel by {shortDate(v.cancel_by!)}</span>}
        {!picking && v?.verdict !== 'keep' && <button className="btn-outline h-8 px-3 text-xs" disabled={set.isPending} onClick={() => set.mutate({ verdict: 'keep' })}>Keep</button>}
        {!picking && v?.verdict !== 'cancel' && <button className="btn-outline h-8 px-3 text-xs" onClick={() => setPicking(true)}>Cancel by…</button>}
        {picking && <>
          <div className="w-40"><DateInput value={day} onChange={setDay} min={todayISO()} className="input h-8 text-xs" aria-label="Cancel by" /></div>
          <button className="btn-primary h-8 px-3 text-xs" disabled={set.isPending} onClick={() => set.mutate({ verdict: 'cancel', cancel_by: day })}>Set</button>
          <button className="btn-ghost h-8 px-2 text-xs" onClick={() => setPicking(false)}>Back</button>
        </>}
        {v && !picking && <button className="btn-ghost h-8 px-2 text-xs text-muted" onClick={() => set.mutate({ verdict: '' })}>Undo</button>}
      </div>
    </li>
  )
}
