import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur, monthLabel, shortDate } from '../../lib/format'
import { useCats } from '../../lib/categories'
import type { Creep, Prices } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading, Segmented } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { MerchantMark } from '../../components/MerchantLogo'

const TONE = { good: 'text-good', warn: 'text-warn', bad: 'text-bad' } as const
// Yearly totals in whole euros: €599.80 beside €9,100 read as two kinds of number.
const eur0 = (v: number) => eur(Math.round(v))
const sgn = (v: number) => `${v > 0 ? '+' : v < 0 ? '−' : ''}${Math.abs(v).toFixed(Math.abs(v) < 10 ? 1 : 0)}%`

/** Drift: the slow changes nobody notices month to month — a lifestyle
 *  outgrowing the pay, bills creeping up, and what inflation is for you. */
export function DriftView() {
  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
      <CreepCard />
      <div className="space-y-4"><PricesCards /></div>
    </div>
  )
}

function CreepCard() {
  const cats = useCats()
  const [years, setYears] = useState<'1' | '3'>('1')
  const { data, isLoading, error } = useQuery({ queryKey: ['creep', years], queryFn: () => api.get<Creep>(`/insights/creep?years=${years}`), placeholderData: (p) => p })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  return (
    <Card title="Lifestyle creep" icon="trend" color={`var(--hue-${data.tone})`} action={<div className="flex items-center gap-1"><Segmented size="sm" value={years} onChange={setYears} options={[{ value: '1', label: '1 year' }, { value: '3', label: '3 years' }]} /><AskCFO q="Is my lifestyle outgrowing my income? What grew, and what would you trim first?" label="" /></div>}>
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
        <p className="mt-2 text-xs text-muted">{monthLabel(data.then.from)}–{monthLabel(data.then.to)} against {monthLabel(data.now.from)}–{monthLabel(data.now.to)}: spending {eur0(data.then.spending)} → {eur0(data.now.spending)} a year.</p>
        {/* Both directions: a total that fell can hide one thing that grew a lot. */}
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {([['What grew most, a year', data.drivers, 'text-bad'], ['What fell most, a year', data.fallers ?? [], 'text-good']] as const).map(([title, rows, tone]) => rows.length > 0 && (
            <div key={title}>
              <div className="text-xs font-medium text-ink2">{title}</div>
              <ul className="mt-1 space-y-1 text-sm">
                {rows.map((d) => (
                  <li key={d.category} className="flex justify-between gap-2"><span className="truncate">{cats.name(d.category)}</span>
                    <span className="tnum shrink-0 text-ink2">{eur0(d.then)} → {eur0(d.now)} <b className={tone}>{d.change > 0 ? '+' : '−'}{eur0(Math.abs(d.change))}</b></span></li>
                ))}
              </ul>
            </div>
          ))}
        </div>
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
  const { data, isLoading, error } = useQuery({ queryKey: ['prices'], queryFn: () => api.get<Prices>('/insights/prices') })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  const off = data.official
  // A handful of bills is a hint, not a measure: say so, and don't shout it.
  const small = data.basket < 5 || data.weight < 1000
  const comparable = data.items.filter((it) => it.year_ago > 0 || it.renews_soon)
  const rest = data.items.filter((it) => !(it.year_ago > 0 || it.renews_soon))
  return <>
    <Card title="Your inflation" icon="percent" color={data.personal != null && off ? (data.personal > off.rate ? 'var(--hue-bad)' : 'var(--hue-good)') : undefined}>
      {data.personal == null ? <p className="text-sm text-ink2">Needs a year of regular bills to compare prices with.</p> : <>
        <div className="flex items-end gap-6">
          <div><div className="text-xs text-ink2">Yours</div><div className={clsx('tnum font-semibold', small ? 'text-xl' : 'text-3xl', off && data.personal > off.rate ? 'text-bad' : 'text-good')}>{sgn(data.personal)}</div></div>
          {off && <div><div className="text-xs text-ink2">Lithuania</div><div className={clsx('tnum font-semibold text-ink2', small ? 'text-xl' : 'text-3xl')}>{sgn(off.rate)}</div></div>}
        </div>
        <p className="mt-2 text-sm">
          {off ? (data.personal > off.rate ? 'Your regular bills are rising faster than prices in general.' : 'Your regular bills are rising slower than prices in general.')
            : 'The official rate couldn’t be reached just now.'}
        </p>
        <p className="mt-1 text-xs text-muted">
          {small && <b className="text-ink2">A small basket — read it as a hint. </b>}
          Measured on {data.basket} regular bill{data.basket === 1 ? '' : 's'} costing {eur(data.weight)} a year, each weighted by what it costs — groceries and one-off shopping aren’t in it.
          {data.excluded > 0 && <> {data.excluded} bill{data.excluded === 1 ? '' : 's'} that moved more than 30% (a plan change, not a price change) {data.excluded === 1 ? 'is' : 'are'} left out.</>}
          {off && <> Official: {off.source}, {monthLabel(off.month, true)}.</>}
        </p>
      </>}
    </Card>
    <Card title="Price creep & renewals" icon="repeat" color={data.rising > 0 ? 'var(--hue-warn)' : 'var(--hue-good)'} action={<AskCFO q="Which of my bills went up the most, and which renewals should I shop around for first?" />}>
      {data.rising > 0
        ? <p className="text-sm"><b>{data.rising}</b> bill{data.rising === 1 ? '' : 's'} cost more than a year ago — <b className="tnum text-bad">{eur(data.extra)}</b> a year extra together.</p>
        : <p className="text-sm text-ink2">No regular bill rose 5% or more in a year.</p>}
      <ul className="mt-2 divide-y divide-line">{comparable.map((it) => <PriceRow key={it.merchant + it.category} it={it} />)}</ul>
      {rest.length > 0 && (
        <details className="border-t border-line">
          <summary className="cursor-pointer py-2 text-xs text-ink2">{rest.length} more, no year to compare yet</summary>
          <ul className="divide-y divide-line">{rest.map((it) => <PriceRow key={it.merchant + it.category} it={it} />)}</ul>
        </details>
      )}
      <p className="mt-1 text-xs text-muted">Each bill’s latest charge against the one about a year earlier. <Link to="/plan/recurring" className="text-accent">All recurring</Link></p>
    </Card>
  </>
}

function PriceRow({ it }: { it: Prices['items'][number] }) {
  const cats = useCats()
  return (
    <li className="py-2.5">
      <div className="flex items-start justify-between gap-3">
        <MerchantMark merchant={it.merchant} category={it.category} size={32} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{it.merchant}{it.renews_soon && <span className="ml-2 rounded-full bg-warn/15 px-2 py-0.5 text-[11px] text-warn">renews {shortDate(it.next!)}</span>}</div>
          <div className="truncate text-xs text-ink2">{cats.name(it.category)} · {it.cadence} · {eur0(it.yearly)}/yr</div>
        </div>
        <div className="shrink-0 text-right">
          <div className="tnum whitespace-nowrap text-sm">{it.year_ago ? <><span className="text-ink2">{eur(it.year_ago)} →</span> <b>{eur(it.now)}</b></> : eur(it.now)}</div>
          {it.year_ago > 0 && (it.plan_change
            ? <div className="text-xs text-muted">plan changed</div>
            : <div className={clsx('tnum text-xs', it.change >= 5 ? 'text-bad' : it.change <= -5 ? 'text-good' : 'text-muted')}>{sgn(it.change)}</div>)}
        </div>
      </div>
      {it.advice && <p className="mt-1 text-xs text-warn">{it.advice}</p>}
    </li>
  )
}
