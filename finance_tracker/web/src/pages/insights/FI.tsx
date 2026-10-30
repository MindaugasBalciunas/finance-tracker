import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurk, monthLabel, pct } from '../../lib/format'
import type { FIPlan, Milestone } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading, Meter } from '../../components/ui'
import { axisProps, gridProps, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'

// ── Independence ────────────────────────────────────────────────────
//
// Not one finish line but levels of freedom, when saving could stop, what the
// money could pay you each year, and the milestones on the way — all in
// today's euros at your expected return and withdrawal rate.

const LEVEL_HUE: Record<string, string> = { obligations: 'var(--hue-fixed)', essentials: 'var(--s1)', lifestyle: 'var(--hue-good)', comfortable: 'var(--s7)' }
const when = (date?: string, age?: number) => date ? `${monthLabel(date, true)}${age ? ` · age ${Math.floor(age)}` : ''}` : 'not at this pace'

export function FI() {
  const { data: f, isLoading, error } = useQuery({ queryKey: ['fi-plan'], queryFn: () => api.get<FIPlan>('/wealth/fi-plan') })
  const { data: ms } = useQuery({ queryKey: ['milestones'], queryFn: () => api.get<Milestone[]>('/wealth/milestones') })
  if (isLoading) return <Loading />
  if (error || !f) return <ErrorBox error={error} />
  const share = f.spending > 0 ? f.passive_now / f.spending : 0
  return (
    <div className="space-y-4">
      {/* Where you stand: what the money could pay you today. */}
      <section className="hero p-4 sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="text-sm text-ink2">Your investments could pay you, safely, every month</div>
            <div className="grad-text mt-1 text-4xl font-bold tracking-tight">{eur(Math.round(f.passive_now))}</div>
            <div className="mt-1 text-sm text-ink2">{pct(Math.min(share, 9.99))} of the {eur(f.spending)} you spend a month · at a {f.withdrawal_rate}% withdrawal rate</div>
          </div>
          <AskCFO q="Look at my independence plan: when can I stop adding money, what should I change to get there sooner, and how do my obligations and pensions shape it?" />
        </div>
        <div className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><div className="text-xs text-muted">Investing now</div><div className="font-semibold tnum">{eur(f.monthly_invested)}/mo</div></div>
          <div><div className="text-xs text-muted">Savings rate, 12 months</div><div className="font-semibold tnum">{pct(f.savings_rate)}</div></div>
          <div><div className="text-xs text-muted">Invested toward FI</div><div className="font-semibold tnum">{eur(f.investable)}</div></div>
          <div><div className="text-xs text-muted">In pensions (locked)</div><div className="font-semibold tnum">{eur(f.locked)}</div></div>
        </div>
      </section>

      {/* The levels of freedom, each a date. */}
      <Card title="Levels of freedom" icon="target" color="var(--hue-good)">
        <ul className="space-y-4">
          {f.levels.map((l) => (
            <li key={l.key} style={{ ['--lvl' as any]: LEVEL_HUE[l.key] }}>
              <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  <span className="grid h-6 w-6 place-items-center rounded-full text-[11px]" style={{ background: 'color-mix(in oklab, var(--lvl) 18%, transparent)', color: 'var(--lvl)' }}>
                    {l.done ? <Icon name="check" size={13} /> : <Icon name="target" size={13} />}
                  </span>
                  {l.title}
                </div>
                <div className={clsx('tnum text-sm', l.done ? 'text-good font-semibold' : 'text-ink2')}>{l.done ? 'reached' : when(l.date, l.age)}</div>
              </div>
              <div className="mt-1.5 h-2 rounded-full bg-sunken"><div className="h-full rounded-full" style={{ width: `${Math.max(2, l.progress * 100)}%`, background: 'var(--lvl)' }} /></div>
              <div className="mt-1 flex flex-wrap justify-between gap-x-3 text-xs text-muted">
                <span>{eur(l.monthly)}/mo needs {eur(l.capital)} invested · {pct(l.progress)} there</span>
                {!l.done && l.months > 0 && <span className="tnum">in {l.months >= 24 ? `${(l.months / 12).toFixed(1)} years` : `${l.months} months`}</span>}
              </div>
            </li>
          ))}
        </ul>
        <p className="mt-3 text-xs text-muted">Each level is the invested money whose {f.withdrawal_rate}% a year covers it forever — obligations from your plan's fixed lines, essentials from essential categories, independence from {f.spend_source}.</p>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
        <Card title="When you can stop adding money" icon="piggy" color="var(--s3)">
          <div className="text-2xl font-semibold tracking-tight">{f.coast.done ? 'Already' : f.coast.date ? monthLabel(f.coast.date, true) : 'Not yet in sight'}</div>
          <p className="mt-1 text-sm text-ink2">{f.coast.detail}</p>
          {f.coast.later && f.coast.later.length > 0 && (
            <ul className="mt-3 space-y-1 text-sm">
              {f.coast.later.map((l) => (
                <li key={l.age} className="flex justify-between gap-2">
                  <span className="text-ink2">To be independent by age {l.age}</span>
                  <span className="tnum font-medium">{l.done ? 'already' : l.date ? `stop adding ${monthLabel(l.date, true)}` : 'not at this pace'}</span>
                </li>
              ))}
            </ul>
          )}
          {f.coast.needed > 0 && <p className="mt-2 text-xs text-muted">"Coast": {eur(f.coast.needed)} invested today grows to {eur(f.target)} by age {f.coast.age} untouched, at {f.expected_return}% real a year.</p>}
        </Card>
        <Card title="What more saving buys" icon="trend" color="var(--s7)">
          <ul className="divide-y divide-line">
            {f.what_ifs.map((w) => (
              <li key={w.extra} className="flex items-baseline justify-between gap-3 py-2 text-sm">
                <span><b className="tnum">+{eur(w.extra)}</b> a month</span>
                <span className="tnum text-ink2">{w.date ? <>FI {monthLabel(w.date, true)} · <b className="text-good">{w.sooner > 0 ? `${w.sooner} years sooner` : 'about the same'}</b></> : 'not within 60 years'}</span>
              </li>
            ))}
          </ul>
          <p className="mt-1 text-xs text-muted">Today's pace: {f.years_to_fi >= 0 ? `FI ${f.fi_date ? monthLabel(f.fi_date, true) : ''} (${f.years_to_fi} years)` : 'not within 60 years'}{f.required_monthly ? ` · ${eur(f.required_monthly)}/mo reaches it by age ${f.target_age}` : ''}.</p>
        </Card>
      </div>

      {/* What the money could pay each year, against what life costs. */}
      <Card title="Projected payouts" icon="coins" color="var(--s4)">
        <p className="-mt-1 mb-2 text-xs text-muted">If you stopped working in a given year: the safe monthly payout, in today's euros, while saving continues until then.</p>
        <div className="h-64">
          <ResponsiveContainer>
            <ComposedChart data={f.payouts} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="year" {...axisProps} interval="preserveStartEnd" minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={eurk} width={52} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                <TooltipBox title={`${label}${payload[0].payload.age ? ` · age ${payload[0].payload.age}` : ''}`} rows={[
                  { color: 'var(--s4)', label: 'Safe payout', value: `${eur(payload[0].payload.monthly)}/mo`, bold: true },
                  { label: 'Invested', value: eur(payload[0].payload.investable) }]} />) : null} />
              {f.obligations > 0 && <ReferenceLine y={f.obligations} stroke="var(--hue-fixed)" strokeDasharray="4 3" label={{ value: 'obligations', fill: 'var(--chart-text)', fontSize: 10, position: 'insideTopLeft' }} />}
              {f.essentials > 0 && <ReferenceLine y={f.essentials} stroke="var(--s1)" strokeDasharray="4 3" label={{ value: 'essentials', fill: 'var(--chart-text)', fontSize: 10, position: 'insideTopLeft' }} />}
              <ReferenceLine y={f.spending} stroke="rgb(var(--good))" label={{ value: 'your spending', fill: 'var(--chart-text)', fontSize: 10, position: 'insideTopLeft' }} />
              <Line dataKey="monthly" stroke="var(--s4)" strokeWidth={2.5} dot={false} isAnimationActive={false} />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
        {ms && ms.length > 0 && (
          <Card title="Milestones" icon="calendar" color="var(--s5)">
            <ol className="relative space-y-3 border-l border-line pl-4">
              {ms.map((m) => (
                <li key={m.key} className="relative">
                  <span className={clsx('absolute -left-[21px] top-1 h-2.5 w-2.5 rounded-full ring-2 ring-[rgb(var(--surface))]', m.done ? 'bg-good' : m.date ? 'bg-accent' : 'bg-axis')} />
                  <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                    <span className="text-sm font-medium">{m.title}</span>
                    <span className={clsx('tnum text-xs', m.done ? 'text-good' : 'text-ink2')}>{m.done ? 'done' : m.date ? `${monthLabel(m.date, true)}${m.months > 0 ? ` · ${m.months >= 24 ? `${(m.months / 12).toFixed(1)} yrs` : `${m.months} mo`}` : ''}` : 'not at this pace'}</span>
                  </div>
                  <div className="text-xs text-muted">{m.detail}</div>
                </li>
              ))}
            </ol>
          </Card>
        )}
        <div className="space-y-4">
          <Card title="Emergency fund" icon="umbrella" color={f.emergency.months < f.emergency.target_months ? 'var(--hue-warn)' : 'var(--hue-good)'}>
            <div className="grid grid-cols-3 gap-3 text-sm">
              <div><div className="text-xs text-muted">Cash</div><div className="font-semibold tnum">{eur(f.emergency.cash)}</div></div>
              <div><div className="text-xs text-muted">Essentials</div><div className="font-semibold tnum">{eur(f.emergency.monthly_essential)}/mo</div></div>
              <div><div className="text-xs text-muted">Covers</div><div className={clsx('font-semibold', f.emergency.months < f.emergency.target_months ? 'text-warn' : 'text-good')}>{f.emergency.months} months</div><div className="text-xs text-muted">target {f.emergency.target_months}</div></div>
            </div>
            <Meter className="mt-3" value={f.emergency.cash} max={f.emergency.target || 1} goal />
          </Card>
          <Card title="Growth to independence" icon="chart" color="var(--s1)">
            <div className="h-48">
              <ResponsiveContainer>
                <AreaChart data={f.payouts} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                  <CartesianGrid {...gridProps} />
                  <XAxis dataKey="year" {...axisProps} interval="preserveStartEnd" minTickGap={24} />
                  <YAxis {...axisProps} tickFormatter={eurk} width={52} />
                  <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={String(label)} rows={[{ color: 'var(--s1)', label: 'Invested', value: eur(payload[0].payload.investable), bold: true }, { label: 'FI target', value: eur(f.target) }]} /> : null} />
                  <ReferenceLine y={f.target} stroke="var(--s2)" label={{ value: 'FI target', fill: 'var(--chart-text)', fontSize: 11, position: 'insideTopLeft' }} />
                  <Area dataKey="investable" stroke="var(--s1)" strokeWidth={2} fill="var(--s1)" fillOpacity={0.15} isAnimationActive={false} />
                </AreaChart>
              </ResponsiveContainer>
            </div>
            <p className="mt-2 text-xs text-muted">Target = {f.spend_source} ({eur(f.annual_spend)}/yr) ÷ {f.withdrawal_rate}%. Invested = brokers, pensions, crypto and cash above the emergency fund — the house isn't counted.</p>
          </Card>
        </div>
      </div>
      {f.notes.map((n) => <p key={n} className="text-xs text-muted">{n}</p>)}
    </div>
  )
}
