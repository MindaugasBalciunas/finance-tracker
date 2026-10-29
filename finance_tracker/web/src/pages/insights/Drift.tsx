import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur, monthLabel, shortDate } from '../../lib/format'
import { useCats } from '../../lib/categories'
import type { Creep, Prices } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading } from '../../components/ui'
import { Icon } from '../../components/Icon'

const TONE = { good: 'text-good', warn: 'text-warn', bad: 'text-bad' } as const
const sgn = (v: number) => `${v > 0 ? '+' : v < 0 ? '−' : ''}${Math.abs(v).toFixed(Math.abs(v) < 10 ? 1 : 0)}%`

/** Drift: the slow changes nobody notices month to month — a lifestyle
 *  outgrowing the pay, bills creeping up, and what inflation is for you. */
export function DriftView() {
  return (
    <div className="space-y-4">
      <CreepCard />
      <PricesCards />
    </div>
  )
}

function CreepCard() {
  const cats = useCats()
  const { data, isLoading, error } = useQuery({ queryKey: ['creep'], queryFn: () => api.get<Creep>('/insights/creep') })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  return (
    <Card title="Lifestyle creep" icon="trend" action={<AskCFO q="Is my lifestyle outgrowing my income? What grew, and what would you trim first?" />}>
      <div className={clsx('flex items-start gap-2 text-sm font-medium', TONE[data.tone])}>
        <Icon name={data.tone === 'good' ? 'check' : 'alert'} size={17} className="mt-0.5 shrink-0" /><span>{data.text}</span>
      </div>
      {data.years > 0 && <>
        <div className="mt-3 grid grid-cols-3 gap-2 text-center">
          {[{ l: 'Income', v: data.income_growth, good: true }, { l: 'Spending', v: data.spending_growth, good: false }, { l: 'Essentials', v: data.fixed_growth, good: false }].map((x) => (
            <div key={x.l} className="rounded-xl bg-sunken/60 p-2">
              <div className="text-xs text-ink2">{x.l}</div>
              <div className={clsx('tnum text-lg font-semibold', x.v === 0 ? '' : (x.v > 0) === x.good ? 'text-good' : 'text-bad')}>{sgn(x.v)}</div>
            </div>
          ))}
        </div>
        <p className="mt-2 text-xs text-muted">{monthLabel(data.then.from)}–{monthLabel(data.then.to)} against {monthLabel(data.now.from)}–{monthLabel(data.now.to)}: spending {eur(data.then.spending)} → {eur(data.now.spending)} a year.</p>
        {data.drivers.length > 0 && <>
          <div className="mt-3 text-xs font-medium text-ink2">What grew most, a year</div>
          <ul className="mt-1 space-y-1 text-sm">
            {data.drivers.map((d) => (
              <li key={d.category} className="flex justify-between gap-2"><span>{cats.name(d.category)}</span><span className="tnum text-ink2">{eur(d.then)} → {eur(d.now)} <b className="text-bad">+{eur(d.change)}</b></span></li>
            ))}
          </ul>
        </>}
        {data.new.length > 0 && <>
          <div className="mt-3 text-xs font-medium text-ink2">New regular bills since then · {eur(data.new_monthly)} a month</div>
          <ul className="mt-1 space-y-1 text-sm">
            {data.new.map((n) => <li key={n.merchant + n.category} className="flex justify-between gap-2"><span>{n.merchant} <span className="text-xs text-muted">since {monthLabel(n.since.slice(0, 7))}</span></span><span className="tnum">{eur(n.monthly)}/mo</span></li>)}
          </ul>
        </>}
      </>}
    </Card>
  )
}

function PricesCards() {
  const cats = useCats()
  const { data, isLoading, error } = useQuery({ queryKey: ['prices'], queryFn: () => api.get<Prices>('/insights/prices') })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  const off = data.official
  return <>
    <Card title="Your inflation" icon="percent">
      {data.personal == null ? <p className="text-sm text-ink2">Needs a year of regular bills to compare prices with.</p> : <>
        <div className="flex items-end gap-6">
          <div><div className="text-xs text-ink2">Yours</div><div className={clsx('tnum text-3xl font-semibold', off && data.personal > off.rate ? 'text-bad' : 'text-good')}>{sgn(data.personal)}</div></div>
          {off && <div><div className="text-xs text-ink2">Lithuania</div><div className="tnum text-3xl font-semibold text-ink2">{sgn(off.rate)}</div></div>}
        </div>
        <p className="mt-2 text-sm">
          {off ? (data.personal > off.rate ? 'Your regular bills are rising faster than prices in general.' : 'Your regular bills are rising slower than prices in general.')
            : 'The official rate couldn’t be reached just now.'}
        </p>
        <p className="mt-1 text-xs text-muted">
          Measured on {data.basket} regular bill{data.basket === 1 ? '' : 's'} costing {eur(data.weight)} a year, each weighted by what it costs — groceries and one-off shopping aren’t in it.
          {off && <> Official: {off.source}, {monthLabel(off.month, true)}.</>}
        </p>
      </>}
    </Card>
    <Card title="Price creep & renewals" icon="repeat" action={<AskCFO q="Which of my bills went up the most, and which renewals should I shop around for first?" />}>
      {data.rising > 0
        ? <p className="text-sm"><b>{data.rising}</b> bill{data.rising === 1 ? '' : 's'} cost more than a year ago — <b className="tnum text-bad">{eur(data.extra)}</b> a year extra together.</p>
        : <p className="text-sm text-ink2">No regular bill rose 5% or more in a year.</p>}
      <ul className="mt-2 divide-y divide-line">
        {data.items.map((it) => (
          <li key={it.merchant + it.category} className="py-2.5">
            <div className="flex items-baseline justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{it.merchant}{it.renews_soon && <span className="ml-2 rounded-full bg-warn/15 px-2 py-0.5 text-[11px] text-warn">renews {shortDate(it.next!)}</span>}</div>
                <div className="text-xs text-ink2">{cats.name(it.category)} · {it.cadence} · {eur(it.yearly)} a year</div>
              </div>
              <div className="text-right">
                <div className="tnum text-sm">{it.year_ago ? <>{eur(it.year_ago)} → <b>{eur(it.now)}</b></> : eur(it.now)}</div>
                {it.year_ago > 0 && <div className={clsx('tnum text-xs', it.change >= 5 ? 'text-bad' : it.change <= -5 ? 'text-good' : 'text-muted')}>{sgn(it.change)}</div>}
              </div>
            </div>
            {it.advice && <p className="mt-1 text-xs text-warn">{it.advice}</p>}
          </li>
        ))}
      </ul>
      <p className="mt-1 text-xs text-muted">Each bill’s latest charge against the one about a year earlier. <Link to="/plan/recurring" className="text-accent">All recurring</Link></p>
    </Card>
  </>
}
