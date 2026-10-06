import { useState, type CSSProperties } from 'react'
import clsx from 'clsx'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { CashAccount, CashPlan, Overview } from '../lib/types'
import { Link, useNavigate } from 'react-router-dom'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, ReferenceDot, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { useNetWorthHistory, useOverview, usePeriod, usePrefs } from '../lib/hooks'
import { RANGES, rangeFrom, rangeLabel, rangeStep, SHORT } from '../lib/periods'
import { eur, eurk, monthLabel, parseNum, pct, shortDate, signed, todayISO } from '../lib/format'
import { AskCFO, Card, Delta, ErrorBox, Loading, Meter, Segmented, Stat, Toggle, useToast } from '../components/ui'
import { axisProps, gridProps, TooltipBox } from '../components/charts'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { QuickActions } from '../components/QuickActions'
import { Icon, IconTile } from '../components/Icon'
import { catIcon, useCats, GROUPS, LIQUID_GROUPS } from '../lib/categories'

export default function Home() {
  const { data: o, isLoading, error } = useOverview()
  const editor = useTxEditor()
  const nav = useNavigate()
  const cats = useCats()
  const { prefs, set: setPrefs } = usePrefs()
  const liquid = prefs.liquid_only
  // The chart's period, remembered like every other chart's.
  const [range, setRange] = usePeriod('home', '1y', RANGES.map((r) => r.value))
  const { data: hist } = useNetWorthHistory(rangeFrom(range), rangeStep(range))
  if (isLoading) return <Loading />
  if (error || !o) return <ErrorBox error={error} />

  const avg = o.avg12
  const assets = GROUPS.filter((g) => g.id !== 'debt' && (!liquid || LIQUID_GROUPS.includes(g.id))).map((g) => ({ ...g, v: o.by_group[g.id] ?? 0 })).filter((g) => g.v > 0).sort((a, b) => b.v - a.v)
  const assetTotal = assets.reduce((a, g) => a + g.v, 0)
  // Lowest and highest points of the chart, marked with value and month.
  const key = liquid ? 'liquid' : 'value'
  const spark = hist?.length ? hist.map((h) => ({ date: h.date, value: h.net_worth, liquid: h.liquid })) : (o.spark ?? [])
  const periodChange = spark.length > 1 ? spark[spark.length - 1][key] - spark[0][key] : 0
  const pick = (better: (a: number, b: number) => boolean) => spark.reduce<{ i: number; p: (typeof spark)[number] } | null>((m, p, i) => (!m || better(p[key], m.p[key]) ? { i, p } : m), null)
  const hi = pick((a, b) => a > b)
  const lo = pick((a, b) => a < b)
  // Keep the label inside the chart near the edges.
  const anchor = (i: number) => (i < spark.length * 0.15 ? 'start' : i > spark.length * 0.85 ? 'end' : 'middle')
  const dotLabel = (m: { i: number; p: (typeof spark)[number] }, above: boolean) => (props: any) => {
    const vb = props.viewBox ?? {}
    const x = (vb.x ?? 0) + (vb.width ?? 0) / 2
    const y = (vb.y ?? 0) + (above ? -8 : (vb.height ?? 0) + 14)
    return (
      <text x={x} y={y} textAnchor={anchor(m.i)} fontSize={11} fill="var(--chart-text)">
        <tspan fontWeight={600} fill="rgb(var(--ink))">{eurk(m.p[key])}</tspan>
        <tspan dx={4}>{SHORT.includes(range) ? shortDate(m.p.date) : monthLabel(m.p.date.slice(0, 7))}</tspan>
      </text>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <QuickActions inboxOpen={o.inbox_open} />

      {/* Net worth */}
      <section className="card overflow-hidden">
        <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-end sm:justify-between sm:p-6">
          <div>
            <div className="flex items-center justify-between gap-4">
              <div className="text-sm text-ink2">{liquid ? 'Liquid assets' : 'Net worth'}</div>
              <span className="text-xs sm:hidden"><Toggle checked={liquid} onChange={(v) => setPrefs({ liquid_only: v })} label="Liquid only" /></span>
            </div>
            <div className="mt-1 text-4xl font-semibold tracking-tight sm:text-5xl">{eur(liquid ? o.liquid : o.net_worth)}</div>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
              <span className="text-muted">30 days <Delta value={liquid ? o.liquid_30d : o.net_worth_30d} /></span>
              <span className="text-muted">This year <Delta value={liquid ? o.liquid_ytd : o.net_worth_ytd} /></span>
              <span className="text-muted">12 months <Delta value={liquid ? o.liquid_12m : o.net_worth_12m} /></span>
            </div>
          </div>
          <div className="flex flex-col gap-3 sm:items-end">
          <span className="hidden sm:block"><Toggle checked={liquid} onChange={(v) => setPrefs({ liquid_only: v })} label="Liquid only" /></span>
          <div className="grid grid-cols-3 gap-4 text-sm sm:text-right">
            <div><div className="text-muted">Liquid</div><div className="font-semibold tnum">{eur(o.liquid)}</div></div>
            <div><div className="text-muted">Debt</div><div className="font-semibold tnum">{eur(o.debt)}</div></div>
            <div><div className="text-muted">Asset equity</div><div className="font-semibold tnum">{eur((o.by_group.real_assets ?? 0) - o.debt)}</div></div>
          </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 sm:px-6">
          {/* YTD and 1Y repeat a figure already in the headline row */}
          <span className="text-xs text-muted">{range !== 'ytd' && range !== '1y' && <>{rangeLabel(range)} <Delta value={periodChange} /></>}</span>
          <Segmented size="sm" value={range} onChange={setRange} options={RANGES} />
        </div>
        <div className="h-36 sm:h-44">
          <ResponsiveContainer>
            <AreaChart data={spark} margin={{ top: 22, right: 8, bottom: 20, left: 8 }}>
              <defs>
                <linearGradient id="nw" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--s1)" stopOpacity={0.22} />
                  <stop offset="100%" stopColor="var(--s1)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="date" hide />
              <YAxis hide domain={['dataMin - 5000', 'dataMax + 5000']} />
              <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) =>
                active && payload?.length ? <TooltipBox title={shortDate(payload[0].payload.date)} rows={[{ label: liquid ? 'Liquid' : 'Net worth', value: eur(payload[0].value as number), bold: true }]} /> : null} />
              <Area type="monotone" dataKey={liquid ? 'liquid' : 'value'} stroke="var(--s1)" strokeWidth={2} fill="url(#nw)" isAnimationActive={false} />
              {hi && lo && hi.i !== lo.i && <>
                <ReferenceDot x={hi.p.date} y={hi.p[key]} r={4} fill="var(--s6)" stroke="var(--chart-surface)" strokeWidth={2} label={{ content: dotLabel(hi, true) }} />
                <ReferenceDot x={lo.p.date} y={lo.p[key]} r={4} fill="var(--s8)" stroke="var(--chart-surface)" strokeWidth={2} label={{ content: dotLabel(lo, false) }} />
              </>}
            </AreaChart>
          </ResponsiveContainer>
        </div>
        {assetTotal > 0 && (
          <Link to="/wealth" className="block border-t border-line px-4 py-3 hover:bg-sunken/50 sm:px-6">
            {/* Each group is one column: its bar segment on top, its name and full
                amount right underneath, so labels line up with their segment. A column
                grows with its share but never narrower than its amount. */}
            <div className="flex w-full gap-1">
              {assets.map((g, i) => (
                <div key={g.id} className="min-w-0" style={{ flex: `${g.v / assetTotal} 1 0%`, minWidth: 'max-content' }}>
                  <div className={clsx('h-2', i === 0 && 'rounded-l-full', i === assets.length - 1 && 'rounded-r-full')} style={{ background: `var(--s${g.slot})` }} />
                  <div className="mt-1.5 flex flex-col pr-1 sm:flex-row sm:items-baseline sm:gap-1.5">
                    <span className="whitespace-nowrap text-[11px] text-ink2 sm:text-xs">{g.name}</span>
                    <span className="whitespace-nowrap text-xs font-semibold tnum sm:text-sm">{eur(g.v)}</span>
                  </div>
                </div>
              ))}
            </div>
            {!liquid && o.debt > 0 && (
              <div className="mt-2 flex items-baseline gap-1.5 text-xs sm:text-sm">
                <span className="h-2 w-2 shrink-0 self-center rounded-[3px] border border-axis" />
                <span className="text-ink2">Debt</span>
                <span className="font-semibold tnum text-bad">−{eur(o.debt)}</span>
              </div>
            )}
            <div className="mt-2 flex items-center justify-between text-xs text-muted">
              <span>{liquid ? `Liquid total ${eur(assetTotal)} · house and car excluded` : `Assets ${eur(assetTotal)} − debt ${eur(o.debt)} = ${eur(o.net_worth)}`}</span>
              <span className="inline-flex items-center gap-0.5 text-accent">Wealth <Icon name="chevronR" size={14} /></span>
            </div>
          </Link>
        )}
      </section>

      <LeftToSpend p={o.plan} progress={o.month_progress} month={monthLabel(o.date.slice(0, 7), true)} />

      {o.cash && <CashUntilPayday c={o.cash} />}

      {/* Progress: the slow numbers that say whether wealth is being built */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon="piggy" color="var(--s6)" label="Savings rate · 12 mo" value={pct(avg.savings_rate)} sub={`${eur(avg.saved)}/month kept`} tone={avg.savings_rate < 0 ? 'bad' : undefined} onClick={() => nav('/insights/cashflow')} />
        <Stat icon="umbrella" color="var(--s3)" label="Emergency fund" value={`${o.emergency.months.toFixed(1)} mo`} sub={`${eur(o.emergency.cash)} · target ${o.emergency.target_months} mo`}
          tone={o.emergency.months < o.emergency.target_months ? 'warn' : undefined} onClick={() => nav('/insights/fi')} />
        <Stat icon="trend" color="var(--s7)" label="Financial independence" value={pct(o.fi_progress, 1)} sub={o.years_to_fi >= 0 ? `${o.years_to_fi.toFixed(1)} years at this pace` : 'set a target in Plan'} onClick={() => nav('/insights/fi')} />
        <Stat icon="trend" color="var(--s2)" label="Invested · 12 mo" value={eurk(avg.invested * 12)} sub={`${eur(avg.invested)}/month incl. loan principal`} onClick={() => nav('/insights/cashflow')} />
      </div>

      <Card pad={false} icon="list" color="var(--s2)" title="Recent" action={
        <div className="flex gap-1">
          <button className="btn-ghost h-8 px-2.5 text-xs" onClick={editor.scan}><Icon name="camera" size={16} />Scan</button>
          <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => editor.open()}><Icon name="plus" size={16} />Add</button>
        </div>}>
        <div className="divide-y divide-line pb-1">
          {(o.recent ?? []).map((t) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
        </div>
        <Link to="/ledger" className="block border-t border-line px-4 py-3 text-center text-sm text-accent">All transactions</Link>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card icon="target" color="var(--s3)" title="Plan this month" action={<div className="flex items-center gap-1"><AskCFO q="Give me a short briefing on this month: spending vs plan, anything unusual, and what to watch before month end." label="Brief me" /><Link to="/plan" className="text-sm text-accent">Open plan</Link></div>}>
          <div className="mb-2 flex items-baseline justify-between text-sm">
            <span className="text-ink2">Spending budgets</span>
            <span className="tnum"><b>{eur(o.plan.spent)}</b> <span className="text-muted">of {eur(o.plan.budgeted)}</span></span>
          </div>
          <Meter value={o.plan.spent} max={o.plan.budgeted} pace={o.month_progress} />
          {(o.plan.over ?? []).length > 0 ? (
            <div className="mt-4 space-y-2">
              <div className="section-title">Over budget</div>
              {o.plan.over!.map((l) => (
                <div key={l.name} className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-1.5"><Icon name="alert" size={15} className="text-bad" />{l.name}</span>
                  <span className="tnum text-bad">{signed(-l.remaining).replace('+', '')} over</span>
                </div>
              ))}
            </div>
          ) : o.plan.projected_left >= 0 ? (
            <div className="mt-3 flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every budget is on track</div>
          ) : null}
          {(o.plan.lines ?? []).length > 0 && (
            <div className="mt-4 space-y-2.5">
              <div className="section-title">Busiest budgets</div>
              {o.plan.lines!.map((l) => (
                <div key={l.name}>
                  <div className="mb-1 flex items-baseline justify-between text-sm">
                    <span className="truncate">{l.name}</span>
                    <span className="shrink-0 tnum"><b>{eur(l.spent)}</b> <span className="text-muted">of {eur(l.budgeted)}</span></span>
                  </div>
                  <Meter value={l.spent} max={l.budgeted} pace={o.month_progress} />
                </div>
              ))}
            </div>
          )}
          {(o.anomalies ?? []).length > 0 && (
            <div className="mt-4 space-y-1.5">
              <div className="section-title">Running hot</div>
              {o.anomalies!.slice(0, 3).map((a) => (
                <div key={a.category} className="flex items-center justify-between text-sm">
                  <span>{cats.name(a.category)}</span>
                  <span className="text-muted tnum">on pace for {eur(a.projected)} · usually {eur(a.typical)}</span>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card icon="calendar" color="var(--s4)" title="Coming up" action={<Link to="/insights/recurring" className="text-sm text-accent">All recurring</Link>}>
          {(o.upcoming ?? []).length === 0 ? (
            <div className="text-sm text-muted">No other recurring charges in the next two weeks.</div>
          ) : (
            <div className="divide-y divide-line">
              {o.upcoming!.slice(0, 6).map((r) => (
                <div key={r.merchant} className="flex items-center gap-3 py-2 text-sm">
                  <IconTile name={catIcon(r.category)[0]} color={catIcon(r.category)[1]} size={32} round />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{r.merchant}</div>
                    <div className="truncate text-xs text-muted">{[shortDate(r.next), r.category && cats.path(r.category)].filter(Boolean).join(' · ')}</div>
                  </div>
                  <div className="tnum">{eur(r.amount)}</div>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>


      <button onClick={() => editor.open()} className="fixed bottom-20 right-4 z-20 grid h-14 w-14 place-items-center rounded-full bg-accent text-white shadow-lg sm:hidden" aria-label="Add transaction">
        <Icon name="plus" size={26} />
      </button>
    </div>
  )
}



/** The month day by day: free spending so far against an even pace through
 *  the free money, the typical month, and where this pace ends up; dated
 *  events (obligations due or paid, income) sit on the axis. */
function MonthTimeline({ p }: { p: Overview['plan'] }) {
  const days = p.days ?? []
  if (days.length < 28) return null
  const dim = days.length
  const today = days.filter((d) => d.cum != null).length
  const free = p.free_spent + p.safe_to_spend
  const cumToday = days[today - 1]?.cum ?? 0
  const projEnd = cumToday + p.expected_day * (dim - today)
  const over = projEnd > free
  const rows = days.map((d) => ({
    day: d.day, cum: d.cum, typical: d.typical, even: (free * d.day) / dim,
    proj: d.day >= today ? cumToday + p.expected_day * (d.day - today) : undefined,
  }))
  const events = p.events ?? []
  // The day this pace uses up the free money, if it does this month.
  const runOut = over && p.expected_day > 0 ? Math.max(today, Math.min(dim, today + Math.ceil((free - cumToday) / p.expected_day))) : null
  const evColor = (k: string) => (k === 'income' ? 'var(--s6)' : 'var(--s7)')
  const top = Math.max(free, projEnd, ...days.map((d) => d.typical)) * 1.08
  return (
    <div className="mt-3">
      <div className="h-40 sm:h-44">
        <ResponsiveContainer>
          <ComposedChart data={rows} margin={{ top: 14, right: 8, bottom: 0, left: 0 }}>
            <defs>
              <linearGradient id="mt-cum" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--s1)" stopOpacity={0.25} />
                <stop offset="100%" stopColor="var(--s1)" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="day" {...axisProps} ticks={[1, 8, 15, 22, dim]} type="number" domain={[1, dim]} />
            <YAxis {...axisProps} tickFormatter={eurk} width={44} domain={[0, top]} />
            <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) => {
              if (!active || !payload?.length) return null
              const r = payload[0].payload as (typeof rows)[number]
              const ev = events.filter((e) => e.day === r.day)
              return <TooltipBox title={`Day ${r.day}`} rows={[
                ...(r.cum != null ? [{ color: 'var(--s1)', label: 'Spent so far', value: eur(r.cum), bold: true }, { label: 'that day', value: eur(days[r.day - 1].spent) }]
                  : r.proj != null ? [{ color: over ? 'rgb(var(--warn))' : 'var(--s1)', label: 'At this pace', value: eur(r.proj) }] : []),
                { label: 'Even pace', value: eur(r.even) },
                { color: 'var(--s-other)', label: 'Typical month', value: eur(r.typical) },
                ...ev.map((e) => ({ color: evColor(e.kind), label: `${e.label}${e.done ? (e.kind === 'income' ? ' received' : ' paid') : ' due'}`, value: eur(e.amount) })),
              ]} />
            }} />
            <ReferenceLine y={free} stroke="var(--chart-axis)" strokeDasharray="4 3"
              label={{ value: `free money ${eurk(free)}`, position: 'insideTopRight', fontSize: 10, fill: 'var(--chart-text)' }} />
            <Line dataKey="even" stroke="rgb(var(--ink2))" strokeOpacity={0.55} strokeWidth={1.5} strokeDasharray="2 3" dot={false} isAnimationActive={false} />
            <Line dataKey="typical" stroke="var(--s-other)" strokeWidth={1.5} dot={false} isAnimationActive={false} />
            <Area dataKey="cum" stroke="var(--s1)" strokeWidth={2} fill="url(#mt-cum)" dot={false} isAnimationActive={false} connectNulls={false} />
            <Line dataKey="proj" stroke={over ? 'rgb(var(--warn))' : 'var(--s1)'} strokeWidth={2} strokeDasharray="5 4" dot={false} isAnimationActive={false} />
            <ReferenceLine x={today} stroke="rgb(var(--ink))" strokeOpacity={0.5} label={{ value: 'today', position: 'top', fontSize: 10, fill: 'var(--chart-text)' }} />
            {runOut != null && (
              <ReferenceDot x={runOut} y={free} r={4} fill="rgb(var(--warn))" stroke="var(--chart-surface)" strokeWidth={2} ifOverflow="visible"
                label={{ value: `runs out ~${runOut}`, position: 'top', fontSize: 10, fill: 'rgb(var(--warn))' }} />
            )}
            {events.map((e, i) => (
              <ReferenceDot key={i} x={e.day} y={0} r={5} fill={e.done ? evColor(e.kind) : 'rgb(var(--surface))'} stroke={evColor(e.kind)} strokeWidth={2} ifOverflow="visible" />
            ))}
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted">
        <span className="inline-flex items-center gap-1"><span className="h-0.5 w-3 rounded bg-[var(--s1)]" />you</span>
        <span className="inline-flex items-center gap-1"><span className="w-3 border-t-2 border-dashed" style={{ borderColor: over ? 'rgb(var(--warn))' : 'var(--s1)' }} />at this pace</span>
        <span className="inline-flex items-center gap-1"><span className="w-3 border-t-2 border-dotted border-ink2/60" />even pace</span>
        <span className="inline-flex items-center gap-1"><span className="h-0.5 w-3 rounded bg-[var(--s-other)]" />typical month</span>
        {events.length > 0 && <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full border-2" style={{ borderColor: 'var(--s7)' }} />due · <span className="h-2 w-2 rounded-full" style={{ background: 'var(--s7)' }} />paid</span>}
      </div>
      {events.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-ink2">
          {events.map((e, i) => (
            <span key={i} className="inline-flex items-center gap-1">
              <span className="h-2 w-2 rounded-full border-2" style={{ borderColor: evColor(e.kind), background: e.done ? evColor(e.kind) : 'transparent' }} />
              {e.day} — {e.label} <span className="tnum">{e.kind === 'income' ? '+' : ''}{eur(e.amount)}</span>{!e.done && <span className="text-muted">due</span>}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

/** The whole month's income as one bar: what is committed (fixed, saving,
 *  funds — solid once paid, striped while still to go), what was spent, and
 *  what is left. The tick marks where spending would be at an even pace. */
function MonthBar({ p, progress, warn }: { p: Overview['plan']; progress: number; warn: boolean }) {
  const fixedPaid = Math.min(p.fixed_spent, p.fixed_planned)
  const savedDone = Math.min(p.saved_actual, p.saving_planned)
  const segs = [
    { key: 'fixed-paid', label: 'Fixed paid', v: fixedPaid, color: 'var(--s7)', faded: false },
    { key: 'fixed-due', label: 'Fixed still to pay', v: p.fixed_planned - fixedPaid, color: 'var(--s7)', faded: true },
    { key: 'saved', label: 'Saved', v: savedDone, color: 'var(--s6)', faded: false },
    { key: 'saving-due', label: 'Saving still to do', v: p.saving_planned - savedDone, color: 'var(--s6)', faded: true },
    { key: 'funds', label: 'Set aside in funds', v: p.fund_set_aside, color: 'var(--s4)', faded: false },
    { key: 'spent', label: 'Spent', v: p.free_spent, color: p.safe_to_spend < 0 ? 'rgb(var(--bad))' : warn ? 'rgb(var(--warn))' : 'var(--s1)', faded: false },
  ].filter((x) => x.v > 0.005)
  const committed = p.fixed_planned + p.saving_planned + p.fund_set_aside
  const total = Math.max(p.income_base, committed + p.free_spent, 1)
  const pctOf = (v: number) => `${(v / total) * 100}%`
  // Free money = what was spent + what is left; an even pace spends it evenly.
  const free = p.free_spent + Math.max(p.safe_to_spend, 0)
  const tick = Math.min(1, (committed + free * progress) / total)
  return (
    <div className="mt-2.5">
      <div className="relative">
        <div className="flex h-2.5 w-full gap-px overflow-hidden rounded-full bg-sunken">
          {segs.map((x) => (
            <div key={x.key} title={`${x.label} ${eur(x.v)}`} className="h-full" style={{ width: pctOf(x.v), ...(x.faded ? hatch(x.color) : { background: x.color }) }} />
          ))}
        </div>
        <div className="absolute -top-1 h-[18px] w-0.5 rounded bg-ink/70" style={{ left: `${tick * 100}%` }} title="where spending would be today at an even pace" />
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted">
        <Swatch color="var(--s7)" label="Fixed" v={p.fixed_planned} />
        <Swatch color="var(--s6)" label="Saving" v={p.saving_planned} />
        {p.fund_set_aside > 0 && <Swatch color="var(--s4)" label="Funds" v={p.fund_set_aside} />}
        <Swatch color="var(--s1)" label="Spent" v={p.free_spent} />
        <Swatch color="rgb(var(--sunken))" label="Left" v={Math.max(p.safe_to_spend, 0)} border />
        <span className="inline-flex items-center gap-1"><span className="h-2 w-2.5 rounded-sm" style={hatch('rgb(var(--ink2))')} />striped = still to pay</span>
      </div>
    </div>
  )
}

// Committed but not yet paid: the series colour in 135° stripes. A small
// tile (Firefox draws long repeating gradients unevenly on wide bars).
const hatch = (c: string): CSSProperties => {
  const f = `color-mix(in oklab, ${c} 25%, transparent)`
  return { backgroundImage: `linear-gradient(135deg, ${c} 25%, ${f} 25%, ${f} 50%, ${c} 50%, ${c} 75%, ${f} 75%, ${f})`, backgroundSize: '6px 6px' }
}

function Swatch({ color, label, v, border = false }: { color: string; label: string; v: number; border?: boolean }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className={clsx('h-2 w-2 rounded-sm', border && 'border border-axis')} style={{ background: color }} />
      {label} <span className="tnum text-ink2">{eur(v)}</span>
    </span>
  )
}

/** The everyday question: how much free money is left this month, and what
 *  that means per day. Same money as Plan's "safe to spend" — with what is
 *  still to pay (alimony, loan) in view and the sum behind it one tap away. */
function LeftToSpend({ p, progress, month }: { p: Overview['plan']; progress: number; month: string }) {
  const [open, setOpen] = useState(() => { try { return localStorage.getItem('home-left-open') === '1' } catch { return false } })
  const toggle = () => { setOpen(!open); try { localStorage.setItem('home-left-open', open ? '0' : '1') } catch { /* private mode */ } }
  const left = p.safe_to_spend
  const total = Math.max(p.free_spent + Math.max(left, 0), 1)
  const used = Math.min(1, Math.max(0, p.free_spent / total))
  const vsTypical = p.typical_day > 0 ? p.avg_day / p.typical_day - 1 : 0
  const outlook = left < 0 ? 'text-bad' : p.projected_left < 0 ? 'text-warn' : 'text-good'
  const fixed = p.fixed ?? []
  const due = fixed.filter((f) => f.spent < f.budgeted).sort((a, b) => (a.due ?? '9').localeCompare(b.due ?? '9'))
  const paid = fixed.filter((f) => f.spent >= f.budgeted)
  const rows: { label: string; value: number; sub?: string; sign: '' | '−' | '=' }[] = [
    { label: 'Income this month', value: p.income_base, sub: p.income_actual < p.income_base ? `${eur(p.income_actual)} received so far` : 'received', sign: '' },
    { label: 'Fixed obligations', value: p.fixed_planned, sub: `${eur(p.fixed_spent)} paid`, sign: '−' },
    { label: 'Saving & investing', value: p.saving_planned, sub: `${eur(p.saved_actual)} done`, sign: '−' },
    ...(p.fund_set_aside > 0 ? [{ label: 'Set aside in funds', value: p.fund_set_aside, sub: 'trips, car and other lumpy costs', sign: '−' as const }] : []),
    { label: 'Spent so far', value: p.free_spent, sub: 'day-to-day, outside funds', sign: '−' },
  ]
  return (
    <section className="card">
      <Link to="/plan" className="block px-4 pt-3 hover:bg-sunken/30" title="Free money this month — same as Plan's safe to spend">
        <div className="flex items-center gap-3">
          <IconTile name="wallet" color="var(--s1)" size={32} />
          <div className="min-w-0 flex-1">
            <div className="text-xs text-ink2">Left to spend in {month}</div>
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className={clsx('text-xl font-semibold tracking-tight tnum', left < 0 ? 'text-bad' : 'text-good')}>{eur(left)}</span>
              <span className="text-xs text-muted">{left > 0 ? <>≈ <b className="tnum text-ink">{eur(p.per_day_left)}</b>/day · {p.days_left} days left</> : <span className="text-bad">over budget</span>}</span>
            </div>
          </div>
          <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
        </div>
        <MonthBar p={p} progress={progress} warn={used > progress + 0.1} />
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-0.5 pb-2.5 text-xs text-muted">
          <span>Avg day <b className="tnum text-ink">{eur(p.avg_day)}</b>{p.typical_day > 0 && <> vs <span className="tnum">{eur(p.typical_day)}</span> typical <span className={clsx('tnum', vsTypical > 0.1 ? 'text-bad' : vsTypical < -0.1 ? 'text-good' : '')}>({vsTypical >= 0 ? '+' : '−'}{pct(Math.abs(vsTypical), 0)})</span></>}</span>
          {left > 0 && <span title="This month's pace, blended with your typical day while the month is young">
            {p.projected_left >= 0 ? <>At this pace <b className={clsx('tnum', outlook)}>{eur(p.projected_left)}</b> left at month end</>
              : <>At your usual pace you'd overspend by <b className={clsx('tnum', outlook)}>{eur(-p.projected_left)}</b>{p.expected_day > 0 && p.safe_to_spend > 0 && <> — runs out around <b className="tnum text-ink">{shortDate(addDays(todayISO(), Math.ceil(p.safe_to_spend / p.expected_day)))}</b></>}</>}
          </span>}
        </div>
      </Link>
      <div className="px-4 pb-3"><MonthTimeline p={p} /></div>
      {fixed.length > 0 && (
        <div className="border-t border-line px-4 py-2.5">
          <div className="mb-1.5 flex items-baseline justify-between text-xs">
            <span className="font-medium text-ink2">{due.length ? 'Still to pay — already taken off' : 'Fixed obligations'}</span>
            <span className="text-muted tnum">{paid.length} of {fixed.length} paid</span>
          </div>
          <div className="space-y-1">
            {due.map((f) => (
              <div key={f.name} className="flex items-center gap-2 text-sm">
                <span className="h-2 w-2 shrink-0 rounded-full border border-warn" />
                <span className="min-w-0 flex-1 truncate">{f.name}{f.due && <span className="text-xs text-muted"> · due {shortDate(f.due)}</span>}</span>
                <span className="tnum">{eur(f.budgeted - f.spent)}</span>
              </div>
            ))}
            {paid.map((f) => (
              <div key={f.name} className="flex items-center gap-2 text-sm text-muted">
                <Icon name="check" size={12} className="shrink-0 text-good" />
                <span className="min-w-0 flex-1 truncate">{f.name} paid</span>
                <span className="tnum">{eur(f.spent)}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      <button type="button" onClick={toggle} aria-expanded={open} className="flex w-full items-center justify-between border-t border-line px-4 py-2 text-left text-xs text-accent hover:bg-sunken/30">
        <span>How {eur(left)} is worked out</span>
        <Icon name="chevronD" size={14} className={clsx('transition', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-1.5 px-4 pb-3 text-sm">
          {rows.map((r) => (
            <div key={r.label} className="flex items-baseline gap-2">
              <span className="w-3 shrink-0 text-muted">{r.sign}</span>
              <span className="min-w-0 flex-1">{r.label}{r.sub && <span className="block text-xs text-muted sm:inline sm:pl-1.5">{r.sub}</span>}</span>
              <span className="tnum">{eur(r.value)}</span>
            </div>
          ))}
          <div className="flex items-baseline gap-2 border-t border-line pt-1.5 font-semibold">
            <span className="w-3 shrink-0 text-muted">=</span>
            <span className="flex-1">Left to spend</span>
            <span className={clsx('tnum', left < 0 ? 'text-bad' : 'text-good')}>{eur(left)}</span>
          </div>
          <div className="pt-1 text-xs text-muted">Obligations and savings count in full from the 1st, paid or not, so what is left is yours to spend.</div>
        </div>
      )}
    </section>
  )
}

function addDays(iso: string, n: number) {
  const d = new Date(iso + 'T00:00:00Z')
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

/** Cash by the day it moves, per account, until the next salary: what each
 *  everyday account must pay, how low it gets, and the top-ups from savings —
 *  staged as late as possible so savings keep earning. Read-only advice. */
function CashUntilPayday({ c }: { c: CashPlan }) {
  const editor = useTxEditor()
  if (!c.payday || !c.accounts.length) return null
  // Recording a move is an ordinary transfer entry, opened for review — never saved without you.
  const record = (m: CashPlan['moves'][number], note: string) =>
    editor.open({ kind: 'transfer', date: todayISO(), amount: m.amount, account_id: m.from, to_account_id: m.to, category: 'transfer.internal', note })
  const days = Math.round((new Date(c.payday + 'T00:00:00').getTime() - new Date(todayISO() + 'T00:00:00').getTime()) / 86400000)
  const salary = c.accounts.find((a) => a.id === c.salary_account)?.name
  return (
    <section id="cash" className="card scroll-mt-16">
      <div className="flex items-start gap-3 px-4 pt-3">
        <IconTile name="swap" color="var(--s7)" size={32} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">Cash until payday</div>
          <div className="text-xs text-muted">Salary by <b className="text-ink2">{shortDate(c.payday)}</b>{salary && <> into {salary}</>} · {days} days · day-to-day at {eur(c.daily)}/day (your plan)</div>
        </div>
      </div>
      <div className="px-4 py-3">
        {c.moves.length > 0 ? (
          <>
            <div className="mb-1.5 text-xs text-ink2">Top up from savings in {c.moves.length} {c.moves.length === 1 ? 'step' : 'steps'}, each as late as is safe — every account stays above its buffer, and savings keep earning until then:</div>
            <div className="space-y-1.5">
              {c.moves.map((m, i) => (
                <div key={i} className="flex items-start gap-3 rounded-lg bg-sunken/50 px-3 py-2 text-sm">
                  <span className="w-14 shrink-0 text-xs text-muted">by<br /><b className="text-ink">{shortDate(m.by).replace(/ \d{2}$/, '')}</b></span>
                  <span className="min-w-0 flex-1">
                    <span className="block"><b className="tnum">{eur(m.amount)}</b> {m.from_name} → {m.to_name}</span>
                    {m.for && <span className="block truncate text-xs text-muted">for {m.for}</span>}
                  </span>
                  <button className="btn-ghost h-7 shrink-0 px-2 text-xs" onClick={() => record(m, `Top-up for ${m.for.charAt(0).toLowerCase()}${m.for.slice(1)}`)} title="Made the transfer? Record it">Record</button>
                </div>
              ))}
            </div>
          </>
        ) : (
          <div className="flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every account covers what's due until payday</div>
        )}
        {c.short > 0 && (
          <div className={clsx('mt-2 flex items-start gap-1.5 text-sm', c.cash >= c.short ? 'text-warn' : 'text-bad')}>
            <Icon name="alert" size={16} className="mt-0.5 shrink-0" />
            <span>Savings fall <b className="tnum">{eur(c.short)}</b> short — {c.cash >= c.short ? <>deposit that much of your {eur(c.cash)} cash, or lower a buffer below</> : 'lower a buffer below or spend less until payday'}.</span>
          </div>
        )}
        {(c.to_savings ?? []).map((m, i) => (
          <div key={i} className="mt-2 flex items-start gap-3 rounded-lg border border-good/30 bg-good/5 px-3 py-2 text-sm">
            <Icon name="piggy" size={16} className="mt-0.5 shrink-0 text-good" />
            <span className="min-w-0 flex-1"><b className="tnum">{eur(m.amount)}</b> could earn in {m.to_name} — it {m.for}</span>
            <button className="btn-ghost h-7 shrink-0 px-2 text-xs" onClick={() => record(m, 'Idle cash to savings')}>Record</button>
          </div>
        ))}
      </div>
      <div className="divide-y divide-line border-t border-line">
        {c.accounts.map((a) => <CashAccountRow key={a.id} a={a} payday={c.payday} />)}
      </div>
    </section>
  )
}

function CashAccountRow({ a, payday }: { a: CashAccount; payday: string }) {
  const [open, setOpen] = useState(false)
  const items = a.items ?? []
  return (
    <div>
      <button onClick={() => setOpen(!open)} aria-expanded={open} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40">
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">{a.name}</span>
          <span className="block text-xs text-muted">
            now <span className="tnum text-ink2">{eur(a.balance)}</span> · buffer <span className="tnum text-ink2">{eur(a.buffer)}</span> · lowest <span className={clsx('tnum', a.low < 0 ? 'text-bad' : 'text-ink2')}>{a.low < 0 ? `−${eur(-a.low)}` : eur(a.low)}</span> on {shortDate(a.low_date)}
            {a.needed > 0 && <> · needs <b className="tnum text-warn">{eur(a.needed)}</b></>}
          </span>
        </span>
        <span className="text-xs text-muted">{items.length} {items.length === 1 ? 'item' : 'items'}</span>
        <Icon name="chevronD" size={14} className={clsx('shrink-0 text-muted transition', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-1 px-4 pb-3 text-sm">
          {items.map((it, i) => (
            <div key={i} className={clsx('flex items-baseline gap-2', it.within && 'text-muted')}>
              <span className="w-14 shrink-0 text-xs text-muted">{it.kind === 'spending' ? `to ${shortDate(payday).replace(/ \d{2}$/, '')}` : shortDate(it.date).replace(/ \d{2}$/, '')}</span>
              <span className="min-w-0 flex-1 truncate">{it.label}{it.within && <span className="text-xs"> · in day-to-day</span>}</span>
              <span className={clsx('tnum', it.amount > 0 && 'text-good')}>{it.amount > 0 ? '+' : '−'}{eur(Math.abs(it.amount))}</span>
            </div>
          ))}
          <BufferEditor a={a} />
          <div className="flex items-baseline gap-2 border-t border-line pt-1 text-xs text-muted">
            <span className="flex-1">Before payday, without top-ups</span>
            <span className={clsx('tnum', a.end < 0 ? 'text-bad' : 'text-ink2')}>{a.end < 0 ? `−${eur(-a.end)}` : eur(a.end)}</span>
          </div>
        </div>
      )}
    </div>
  )
}

/** The floor an everyday account is kept at — automatic from its history,
 *  or your own figure (saved in plan settings). */
function BufferEditor({ a }: { a: CashAccount }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [v, setV] = useState<string | null>(null)
  const save = async (value: number | null) => {
    try {
      const cur = (await api.get<any>('/plan/settings')).settings
      const buffers = { ...(cur.buffers ?? {}) }
      if (value === null) delete buffers[a.id]
      else buffers[a.id] = value
      await api.put('/plan/settings', { ...cur, buffers })
      setV(null)
      qc.invalidateQueries({ queryKey: ['overview'] }); qc.invalidateQueries({ queryKey: ['plan-settings'] })
      toast(value === null ? 'Buffer back to automatic' : 'Buffer saved', 'good')
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  return (
    <div className="my-2 rounded-lg bg-sunken/50 px-3 py-2 text-xs text-ink2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex-1">Buffer <b className="tnum text-ink">{eur(a.buffer)}</b> — {a.buffer_why}</span>
        {v === null ? (
          <>
            <button className="text-accent" onClick={() => setV(String(a.buffer))}>Change</button>
            {!a.buffer_auto && <button className="text-accent" onClick={() => save(null)}>Automatic</button>}
          </>
        ) : (
          <span className="flex items-center gap-1.5">
            <input className="input h-7 w-24 text-xs tnum" inputMode="decimal" value={v} onChange={(e) => setV(e.target.value)} autoFocus />
            <button className="btn-primary h-7 px-2 text-xs" onClick={() => { const n = parseNum(v); if (n !== undefined && n >= 0) save(n) }}>Save</button>
            <button className="text-muted" onClick={() => setV(null)}>Cancel</button>
          </span>
        )}
      </div>
      <div className="mt-1 text-muted">Kept in the account through the month; anything above it until payday is better off earning in savings.</div>
    </div>
  )
}
