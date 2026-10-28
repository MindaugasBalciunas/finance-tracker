import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import clsx from 'clsx'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import type { CashAccount, CashItem, CashPlan, Overview } from '../lib/types'
import { Link, useNavigate } from 'react-router-dom'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, ReferenceDot, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { useNetWorthHistory, useOverview, usePeriod, usePrefs } from '../lib/hooks'
import { RANGES, rangeFrom, rangeLabel, rangeStep, SHORT } from '../lib/periods'
import { eur, eurk, monthLabel, parseNum, pct, shortDate, todayISO } from '../lib/format'
import { AskCFO, Card, Delta, ErrorBox, Loading, Segmented, Stat, Toggle, useToast } from '../components/ui'
import { axisProps, GroupBar, gridProps, TooltipBox } from '../components/charts'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { QuickActions } from '../components/QuickActions'
import { FunTeaser } from './insights/Fun'
import { Icon, IconTile } from '../components/Icon'
import { GROUPS, LIQUID_GROUPS } from '../lib/categories'

export default function Home() {
  const { data: o, isLoading, error } = useOverview()
  const editor = useTxEditor()
  const nav = useNavigate()
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

      {/* Net worth: the page's hero */}
      <section className="hero overflow-hidden">
        <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-end sm:justify-between sm:p-6">
          <div>
            <div className="flex items-center justify-between gap-4">
              <div className="text-sm text-ink2">{liquid ? 'Liquid assets' : 'Net worth'}</div>
              <span className="text-xs sm:hidden"><Toggle checked={liquid} onChange={(v) => setPrefs({ liquid_only: v })} label="Liquid only" /></span>
            </div>
            <div className="grad-text mt-1 text-4xl font-bold tracking-tight sm:text-5xl">{eur(liquid ? o.liquid : o.net_worth)}</div>
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
              {/* A low that is just the series starting near zero (an account opened) says nothing. */}
              {hi && lo && hi.i !== lo.i && Math.abs(lo.p[key]) > Math.abs(hi.p[key]) * 0.05 && <>
                <ReferenceDot x={hi.p.date} y={hi.p[key]} r={4} fill="var(--s6)" stroke="var(--chart-surface)" strokeWidth={2} label={{ content: dotLabel(hi, true) }} />
                <ReferenceDot x={lo.p.date} y={lo.p[key]} r={4} fill="var(--s8)" stroke="var(--chart-surface)" strokeWidth={2} label={{ content: dotLabel(lo, false) }} />
              </>}
            </AreaChart>
          </ResponsiveContainer>
        </div>
        {assetTotal > 0 && (
          <Link to="/wealth" className="block border-t border-line px-4 py-3 hover:bg-sunken/50 sm:px-6">
            <GroupBar parts={assets} debt={liquid ? 0 : o.debt} />
            <div className="mt-2 flex items-center justify-between text-xs text-muted">
              <span>{liquid ? `Liquid total ${eur(assetTotal)} · house and car excluded` : `Assets ${eur(assetTotal)} − debt ${eur(o.debt)} = ${eur(o.net_worth)}`}</span>
              <span className="inline-flex items-center gap-0.5 text-accent">Wealth <Icon name="chevronR" size={14} /></span>
            </div>
          </Link>
        )}
      </section>

      <LeftToSpend p={o.plan} progress={o.month_progress} month={monthLabel(o.date.slice(0, 7), true)} />

      {o.cash && <UntilPayday c={o.cash} />}

      {/* Progress: the slow numbers that say whether wealth is being built */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon="piggy" color="var(--s6)" label="Savings rate · usual month" value={pct(o.usual_rate ?? avg.savings_rate)}
          sub={`${eur(Math.round(o.usual_saved ?? avg.saved))}/mo · 12-mo avg ${pct(avg.savings_rate)}`} tone={(o.usual_rate ?? avg.savings_rate) < 0 ? 'bad' : undefined} onClick={() => nav('/insights/cashflow')} />
        <Stat icon="umbrella" color="var(--s3)" label="Emergency fund" value={`${o.emergency.months.toFixed(1)} mo`} sub={`${eur(o.emergency.cash)} · target ${o.emergency.target_months} mo`}
          tone={o.emergency.months < o.emergency.target_months ? 'warn' : undefined} onClick={() => nav('/insights/fi')} />
        <Stat icon="trend" color="var(--s7)" label="Financial independence" value={pct(o.fi_progress, 1)} sub={o.years_to_fi >= 0 ? `${o.years_to_fi.toFixed(1)} years at this pace` : 'set a target in Plan'} onClick={() => nav('/insights/fi')} />
        <Stat icon="trend" color="var(--s2)" label="Invested · 12 mo" value={eurk(avg.invested * 12)} sub={`${eur(avg.invested)}/month incl. loan principal`} onClick={() => nav('/insights/cashflow')} />
      </div>

      <FunTeaser />

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
    </div>
  )
}



/** Wide screen (≥ 1024 px), following resizes. */
function useWide() {
  const q = '(min-width: 1024px)'
  const [wide, setWide] = useState(() => (typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(q).matches : false))
  useEffect(() => {
    if (!window.matchMedia) return
    const m = window.matchMedia(q)
    const on = () => setWide(m.matches)
    m.addEventListener?.('change', on)
    return () => m.removeEventListener?.('change', on)
  }, [])
  return wide
}

/** How much of a month's free spending a typical month has done by each day
 *  (0–1), from the typical-month curve — so pace follows your usual rhythm
 *  (rent-week, weekends, payday) instead of a straight line. Falls back to
 *  an even split when there is no history. */
function typicalShare(p: Overview['plan'], day: number) {
  const days = p.days ?? []
  const end = days[days.length - 1]?.typical ?? 0
  if (!days.length || end <= 0) return days.length ? day / days.length : 0
  return Math.min(1, Math.max(0, (days[Math.min(day, days.length) - 1]?.typical ?? 0) / end))
}

/** The month day by day: free spending so far against your typical month's
 *  rhythm through this month's free money, and where this pace ends up; dated
 *  events (obligations due or paid, income) sit on the axis. */
function MonthTimeline({ p }: { p: Overview['plan'] }) {
  const wide = useWide()
  const days = p.days ?? []
  if (days.length < 28) return null
  const dim = days.length
  const today = days.filter((d) => d.cum != null).length
  // The month's free money as planned: the day line also carries what was
  // paid beyond what funds held (on the day it happened), so the gap at
  // today is still exactly what is left.
  const free = p.free_spent + (p.fund_overdrawn ?? 0) + p.safe_to_spend
  const cumToday = days[today - 1]?.cum ?? 0
  // The forecast comes from the server: your typical month's rhythm from
  // today on, plus its usual share of one-offs.
  const projEnd = days[dim - 1]?.proj ?? cumToday
  const over = projEnd > free
  const rows = days.map((d) => ({ day: d.day, cum: d.cum, pace: free * typicalShare(p, d.day), proj: d.proj }))
  const events = p.events ?? []
  const runOut = p.run_out_day || null
  const evColor = (k: string) => (k === 'income' ? 'var(--s6)' : 'var(--s7)')
  const top = Math.max(free, projEnd) * 1.08
  return (
    <div className="mt-3 flex flex-col lg:mt-0 lg:h-full">
      <div className="h-52 sm:h-60 lg:h-auto lg:min-h-72 lg:flex-1">
        <ResponsiveContainer>
          <ComposedChart data={rows} margin={{ top: 14, right: 8, bottom: 0, left: 0 }}>
            <defs>
              <linearGradient id="mt-cum" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--s1)" stopOpacity={0.25} />
                <stop offset="100%" stopColor="var(--s1)" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid {...gridProps} />
            {/* Every day of this month (28–31), labelled; phones label every other day. */}
            <XAxis dataKey="day" {...axisProps} type="number" domain={[1, dim]} interval={0}
              ticks={Array.from({ length: dim }, (_, i) => i + 1).filter((d) => wide || d % 2 === 1 || d === dim)}
              tick={{ ...axisProps.tick, fontSize: wide ? 9 : 10 }} />
            <YAxis {...axisProps} tickFormatter={eurk} width={44} domain={[0, top]} />
            <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) => {
              if (!active || !payload?.length) return null
              const r = payload[0].payload as (typeof rows)[number]
              const ev = events.filter((e) => e.day === r.day)
              return <TooltipBox title={`Day ${r.day}`} rows={[
                ...(r.cum != null ? [{ color: 'var(--s1)', label: 'Spent so far', value: eur(r.cum), bold: true }, { label: 'that day', value: eur(days[r.day - 1].spent) }]
                  : r.proj != null ? [{ color: over ? 'rgb(var(--warn))' : 'var(--s1)', label: 'Expected', value: eur(r.proj) }] : []),
                { color: 'var(--s-other)', label: 'On plan by now', value: eur(r.pace) },
                ...ev.map((e) => ({ color: evColor(e.kind), label: `${e.label}${e.done ? (e.kind === 'income' ? ' received' : ' paid') : ' due'}`, value: eur(e.amount) })),
              ]} />
            }} />
            <ReferenceLine y={free} stroke="var(--chart-axis)" strokeDasharray="4 3"
              label={{ value: `spending money ${eurk(free)}`, position: 'insideBottomLeft', fontSize: 10, fill: 'var(--chart-text)' }} />
            {/* Your usual rhythm squeezed into this month's free money: where
                spending should be by each day to land on plan. */}
            <Line dataKey="pace" stroke="var(--s-other)" strokeWidth={1.5} dot={false} isAnimationActive={false} />
            <Area dataKey="cum" stroke="var(--s1)" strokeWidth={2} fill="url(#mt-cum)" dot={false} isAnimationActive={false} connectNulls={false} />
            <Line dataKey="proj" stroke={over ? 'rgb(var(--warn))' : 'var(--s1)'} strokeWidth={2} strokeDasharray="5 4" dot={false} isAnimationActive={false} />
            <ReferenceLine x={today} stroke="rgb(var(--ink))" strokeOpacity={0.5} label={{ value: 'today', position: 'top', fontSize: 10, fill: 'var(--chart-text)' }} />
            {/* What is left today: the gap from your spending up to the free money. */}
            <ReferenceLine segment={[{ x: today, y: cumToday }, { x: today, y: free }]} stroke={p.safe_to_spend < 0 ? 'rgb(var(--bad))' : 'rgb(var(--good))'} strokeWidth={3} ifOverflow="extendDomain"
              label={{ value: p.safe_to_spend < 0 ? `${eur(-p.safe_to_spend)} over` : `${eur(p.safe_to_spend)} left`, position: 'left', fontSize: 11, fontWeight: 600, fill: p.safe_to_spend < 0 ? 'rgb(var(--bad))' : 'rgb(var(--good))' }} />
            {runOut != null && (
              <ReferenceDot x={runOut} y={free} r={4} fill="rgb(var(--warn))" stroke="var(--chart-surface)" strokeWidth={2} ifOverflow="visible"
                label={wide ? { value: `runs out ~${runOut}`, position: 'top', fontSize: 10, fill: 'rgb(var(--warn))' } : undefined} />
            )}
            {events.map((e, i) => (
              <ReferenceDot key={i} x={e.day} y={0} r={5} fill={e.done ? evColor(e.kind) : 'rgb(var(--surface))'} stroke={evColor(e.kind)} strokeWidth={2} ifOverflow="visible" />
            ))}
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted">
        <span className="inline-flex items-center gap-1"><span className="h-0.5 w-3 rounded bg-[var(--s1)]" />you</span>
        <span className="inline-flex items-center gap-1" title="The rest of the month as your last six months spent it, one-offs included"><span className="w-3 border-t-2 border-dashed" style={{ borderColor: over ? 'rgb(var(--warn))' : 'var(--s1)' }} />expected</span>
        <span className="inline-flex items-center gap-1" title="This month's spending money spent in the rhythm of your typical month"><span className="h-0.5 w-3 rounded bg-[var(--s-other)]" />on plan</span>
        {events.length > 0 && <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full border-2" style={{ borderColor: 'var(--s7)' }} />due · <span className="h-2 w-2 rounded-full" style={{ background: 'var(--s7)' }} />paid</span>}
      </div>
    </div>
  )
}

/** The whole month's income as one bar: what is committed (fixed, saving,
 *  funds — solid once paid, striped while still to go), what was spent, and
 *  what is left. The tick marks where spending would be today at your
 *  typical month's pace. */
function MonthBar({ p, progress, warn }: { p: Overview['plan']; progress: number; warn: boolean }) {
  const fixedPaid = Math.min(p.fixed_spent, p.fixed_planned)
  const savedDone = Math.max(0, Math.min(p.saved_actual, p.saving_planned)) // a payout month nets below zero
  const segs = [
    // Commitments are decided already: one quiet colour, striped while still to go.
    { key: 'fixed-paid', label: 'Fixed paid', v: fixedPaid, color: 'var(--s-other)', faded: false },
    { key: 'fixed-due', label: 'Fixed still to pay', v: p.fixed_planned - fixedPaid, color: 'var(--s-other)', faded: true },
    { key: 'saved', label: 'Saved', v: savedDone, color: 'var(--s-other)', faded: false },
    { key: 'saving-due', label: 'Saving still to do', v: p.saving_planned - savedDone, color: 'var(--s-other)', faded: true },
    { key: 'funds', label: 'Set aside in funds', v: p.fund_set_aside, color: 'var(--s-other)', faded: false },
    { key: 'overdrawn', label: 'Paid beyond a fund', v: p.fund_overdrawn ?? 0, color: 'rgb(var(--bad))', faded: false },
    { key: 'spent', label: 'Spent', v: p.free_spent, color: p.safe_to_spend < 0 ? 'rgb(var(--bad))' : warn ? 'rgb(var(--warn))' : 'var(--s1)', faded: false },
  ].filter((x) => x.v > 0.005)
  // A fund overdraft is spending nothing was set aside for: committed first.
  const committed = p.fixed_planned + p.saving_planned + p.fund_set_aside + (p.fund_overdrawn ?? 0)
  const total = Math.max(p.income_base, committed + p.free_spent, 1)
  const pctOf = (v: number) => `${(v / total) * 100}%`
  // Free money = what was spent + what is left; progress is the share a
  // typical month has spent by today.
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
        <div className="absolute -top-1 h-[18px] w-0.5 rounded bg-ink/70" style={{ left: `${tick * 100}%` }} title="where spending would be today at your typical month's pace" />
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted">
        <span className="inline-flex items-center gap-1" title="Fixed obligations, saving and fund set-asides — striped while still to pay">
          <span className="h-2 w-2.5 rounded-sm" style={hatch('var(--s-other)')} />Committed <span className="tnum text-ink2">{eur(p.fixed_planned + p.saving_planned + p.fund_set_aside)}</span>
        </span>
        {(p.fund_overdrawn ?? 0) > 0.005 && <Swatch color="rgb(var(--bad))" label="Beyond funds" v={p.fund_overdrawn!} />}
        <Swatch color="var(--s1)" label="Spent" v={p.free_spent} />
        <Swatch color="rgb(var(--sunken))" label="Left" v={Math.max(p.safe_to_spend, 0)} border />
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
 *  that means per day. Same figure as Plan's "left to spend" — with what is
 *  still to pay (alimony, loan) in view and the sum behind it one tap away. */
function LeftToSpend({ p, progress, month }: { p: Overview['plan']; progress: number; month: string }) {
  const [open, setOpen] = useState(() => { try { return localStorage.getItem('home-left-open') === '1' } catch { return false } })
  const toggle = () => { setOpen(!open); try { localStorage.setItem('home-left-open', open ? '0' : '1') } catch { /* private mode */ } }
  const left = p.safe_to_spend
  const total = Math.max(p.free_spent + Math.max(left, 0), 1)
  const used = Math.min(1, Math.max(0, p.free_spent / total))
  const today = (p.days ?? []).filter((d) => d.cum != null).length
  const pace = p.days?.length ? typicalShare(p, Math.max(today, 1)) : progress
  const fixed = p.fixed ?? []
  const due = fixed.filter((f) => f.spent < f.budgeted).sort((a, b) => (a.due ?? '9').localeCompare(b.due ?? '9'))
  const rows: { label: string; value: number; sub?: string; sign: '' | '−' | '=' }[] = [
    { label: 'Income this month', value: p.income_base, sub: p.income_actual < p.income_base ? `${eur(p.income_actual)} received so far` : 'received', sign: '' },
    { label: 'Fixed obligations', value: p.fixed_planned, sub: `${eur(p.fixed_spent)} paid${due.length ? ` · ${due.map((f) => `${f.name}${f.due ? ` ${shortDate(f.due)}` : ''}`).join(', ')} still to pay` : ''}`, sign: '−' },
    { label: 'Saving & investing', value: p.saving_planned, sub: `${eur(p.saved_actual)} done`, sign: '−' },
    ...(p.fund_set_aside > 0 ? [{ label: 'Set aside in funds', value: p.fund_set_aside, sub: 'trips, car and other lumpy costs', sign: '−' as const }] : []),
    ...((p.fund_overdrawn ?? 0) > 0.005 ? [{ label: 'Paid beyond funds', value: p.fund_overdrawn!, sub: (p.funds_over ?? []).map((f) => f.name).join(', ') || 'more than a fund held', sign: '−' as const }] : []),
    { label: 'Spent so far', value: p.free_spent, sub: (p.unreviewed ?? 0) > 0 ? `day-to-day, incl. ${eur(p.unreviewed!)} not reviewed yet` : 'day-to-day, outside funds', sign: '−' },
  ]
  return (
    // Phones: one column. Wide screens: the month chart on the left at full
    // height, so its lines separate; figures and obligations on the right.
    <section className="card lg:grid lg:grid-cols-2 lg:grid-flow-row-dense" style={{ backgroundImage: 'linear-gradient(135deg, color-mix(in oklab, var(--s1) 12%, transparent), transparent 55%)' }}>
      <Link to="/plan" className="block px-4 pt-3 hover:bg-sunken/30 lg:col-start-2" title="Left to spend this month — the same figure as on Plan">
        <div className="flex items-center gap-3">
          <IconTile name="wallet" color="var(--s1)" size={32} />
          <div className="min-w-0 flex-1">
            <div className="text-xs text-ink2">Left to spend in {month}</div>
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className={clsx('text-xl font-semibold tracking-tight tnum', left < 0 ? 'text-bad' : p.projected_left < 0 ? 'text-warn' : 'text-good')}>{eur(left)}</span>
              <span className="text-xs text-muted">{left > 0 ? <>≈ <b className="tnum text-ink">{eur(p.per_day_left)}</b>/day · {p.days_left} days left</> : <span className="text-bad">over budget</span>}</span>
            </div>
          </div>
          <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
        </div>
        <MonthBar p={p} progress={pace} warn={used > pace + 0.1} />
        <Verdict p={p} />
      </Link>
      <div className="min-w-0 px-4 pb-3 lg:col-start-1 lg:row-span-6 lg:row-start-1 lg:border-r lg:border-line lg:py-3"><MonthTimeline p={p} /></div>
      <TrackRecord p={p} />
      <button type="button" onClick={toggle} aria-expanded={open} className="flex w-full items-center justify-between border-t border-line px-4 py-2 text-left text-xs text-accent hover:bg-sunken/30 lg:col-start-2">
        <span>How {eur(left)} is worked out</span>
        <Icon name="chevronD" size={14} className={clsx('transition', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-1.5 px-4 pb-3 text-sm lg:col-start-2">
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

/** One honest outlook instead of three numbers: where the month is heading
 *  at your usual rhythm (one-offs included), and the reasons behind it as
 *  rows that add up — biggest first. */
function Verdict({ p }: { p: Overview['plan'] }) {
  const left = p.safe_to_spend
  const rest = p.expected_rest ?? 0
  const vsTypical = p.typical_day > 0 ? p.avg_day / p.typical_day - 1 : 0
  const runOut = p.run_out_day ? `${p.run_out_day} ${monthLabel(todayISO().slice(0, 7)).split(' ')[0]}` : ''
  const tone = left < 0 ? 'text-bad' : p.projected_left < 0 ? 'text-warn' : 'text-good'
  const headline = left < 0 ? <>Already <b className="tnum">{eur(-left)}</b> over this month</>
    : p.projected_left < 0 ? <>Heading for <b className="tnum">{eur(-p.projected_left)}</b> over by month end</>
    : <>On track — about <b className="tnum">{eur(p.projected_left)}</b> left at month end</>
  const funds = p.funds_over ?? []
  const rows: { key: string; label: string; sub: string; value: ReactNode; tone?: string }[] = [
    ...funds.map((f) => ({ key: f.name, label: `${f.name} fund overdrawn`, sub: `held ${eur(f.budgeted)}, paid ${eur(f.spent)}`, value: `−${eur(-f.remaining)}`, tone: 'text-bad' })),
    ...((p.unreviewed ?? 0) > 0 ? [{ key: 'unreviewed', label: 'Not reviewed yet', sub: `${p.unreviewed_count} bank ${p.unreviewed_count === 1 ? 'payment' : 'payments'} in the inbox, already counted`, value: `−${eur(p.unreviewed!)}` }] : []),
    ...(left > 0 && rest > 0 ? [{ key: 'rest', label: 'Rest of a usual month', sub: [(p.typical_one_offs ?? 0) > 0 && 'incl. one-offs', p.projected_left < 0 && runOut && `runs out ~${runOut}`].filter(Boolean).join(' · '), value: `−${eur(rest)}` }] : []),
    ...(p.typical_day > 0 && p.avg_day > 0 ? [{ key: 'pace', label: 'Day-to-day pace', sub: `${eur(p.typical_day)}/day usual`,
      value: <>{eur(p.avg_day)}/day <span className={clsx(vsTypical > 0.1 ? 'text-bad' : vsTypical < -0.1 ? 'text-good' : 'text-muted')}>{vsTypical >= 0 ? '+' : '−'}{pct(Math.abs(vsTypical), 0)}</span></> }] : []),
  ]
  return (
    <div className="mt-2.5 pb-2.5">
      <div className={clsx('text-sm font-medium', tone)}>{headline}</div>
      {rows.length > 0 && (
        <div className="mt-1.5 space-y-1 text-xs">
          {rows.map((r) => (
            <div key={r.key} className="flex items-baseline gap-2">
              <span className="min-w-0 flex-1 text-ink2">{r.label}{r.sub && <span className="block text-muted sm:inline"><span className="hidden sm:inline"> · </span>{r.sub}</span>}</span>
              <span className={clsx('shrink-0 tnum', r.tone ?? 'text-ink')}>{r.value}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/** The plan's own test: how recent months ended against it, as bars above
 *  (left over) or below (over) a zero line. A plan every month misses isn't
 *  a budget, it's a wish — say so, with the gap and what to do. */
function TrackRecord({ p }: { p: Overview['plan'] }) {
  const t = p.track ?? []
  if (t.length < 3) return null
  const overs = t.filter((m) => m.left < 0)
  const med = (xs: number[]) => { const s = [...xs].sort((a, b) => a - b); const n = s.length; return n ? (n % 2 ? s[(n - 1) / 2] : (s[n / 2 - 1] + s[n / 2]) / 2) : 0 }
  const spent = med(t.map((m) => m.spent))
  const planned = p.free_planned ?? 0
  const up = Math.max(0, ...t.map((m) => m.left))
  const down = Math.max(0, ...t.map((m) => -m.left))
  const span = Math.max(up + down, 1)
  const H = 44 // px of bar area
  const zero = (up / span) * H
  const fits = overs.length <= 1
  return (
    <div className="border-t border-line px-4 py-2.5 lg:col-start-2">
      <div className="mb-2 flex items-baseline justify-between text-xs">
        <span className="font-medium text-ink2">Last {t.length} months against this plan</span>
        <span className={clsx('tnum', fits ? 'text-good' : 'text-warn')}>{overs.length ? `${overs.length} of ${t.length} over` : 'all within'}</span>
      </div>
      <div className="flex gap-1.5" role="img" aria-label={t.map((m) => `${monthLabel(m.month)} ${m.left < 0 ? 'over' : 'left'} ${eur(Math.abs(m.left))}`).join(', ')}>
        {t.map((m) => {
          const h = Math.max(2, (Math.abs(m.left) / span) * H)
          return (
            <div key={m.month} className="flex min-w-0 flex-1 flex-col items-center" title={`${monthLabel(m.month, true)}: ${m.left < 0 ? `${eur(-m.left)} over` : `${eur(m.left)} left`} · day-to-day ${eur(m.spent)}`}>
              <div className="relative w-full" style={{ height: H + 24 }}>
                <div className="absolute inset-x-0 border-t border-axis" style={{ top: zero + 7 }} />
                <div className={clsx('absolute left-1/2 w-full max-w-7 -translate-x-1/2 rounded-[3px]', m.left < 0 ? 'bg-bad/75' : 'bg-good/75')}
                  style={m.left < 0 ? { top: zero + 7, height: h } : { top: zero + 7 - h, height: h }} />
                <span className={clsx('absolute inset-x-0 text-center text-[10px] tnum leading-none', m.left < 0 ? 'text-bad' : 'text-good')}
                  style={m.left < 0 ? { top: zero + 7 + h + 3 } : { top: Math.max(zero + 7 - h - 12, 0) }}>{m.left < 0 ? '−' : '+'}{eurk(Math.round(Math.abs(m.left))).replace('€', '')}</span>
              </div>
              <span className="mt-1 text-[10px] text-muted">{monthLabel(m.month).split(' ')[0]}</span>
            </div>
          )
        })}
      </div>
      {!fits && planned > 0 && (
        <>
          <p className="mt-2 text-xs text-muted">
            Day-to-day spending runs about <b className="tnum text-ink">{eur(spent)}</b> a month; the plan leaves <b className="tnum text-ink">{eur(planned)}</b>. The lines are too tight, or saving is set higher than the month can fund.
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Link to="/plan" className="rounded-lg bg-accent/10 px-2.5 py-1.5 text-xs font-medium text-accent hover:bg-accent/15">Rebalance the plan</Link>
            <AskCFO q={`My plan has ended over in ${overs.length} of the last ${t.length} months: day-to-day spending runs about ${eur(spent)} a month and the plan leaves ${eur(planned)}. Which lines should change, and is my saving target realistic?`} label="Ask CFO why" />
          </div>
        </>
      )}
    </div>
  )
}

type Entry = { date: string; label: string; account?: string; amount: number; kind: CashItem['kind'] | 'move' | 'save'; move?: CashPlan['moves'][number] }

/** Everything that moves money until the next salary, in one dated list:
 *  obligations, bills, standing orders and the top-ups that keep every
 *  account above its buffer (each as late as is safe, so savings keep
 *  earning). Per-account low points and buffers sit below. Read-only advice:
 *  recording a move opens an ordinary transfer for review. */
function UntilPayday({ c }: { c: CashPlan }) {
  const editor = useTxEditor()
  const [all, setAll] = useState(false)
  if (!c.payday || !c.accounts.length) return null
  const record = (m: CashPlan['moves'][number], note: string) =>
    editor.open({ kind: 'transfer', date: todayISO(), amount: m.amount, account_id: m.from, to_account_id: m.to, category: 'transfer.internal', note })
  const days = Math.round((new Date(c.payday + 'T00:00:00').getTime() - new Date(todayISO() + 'T00:00:00').getTime()) / 86400000)
  const salary = c.accounts.find((a) => a.id === c.salary_account)?.name
  const entries: Entry[] = [
    ...c.accounts.flatMap((a) => (a.items ?? []).filter((it) => it.kind !== 'spending' && !it.within).map((it) => ({ date: it.date, label: it.label, account: a.name, amount: it.amount, kind: it.kind }))),
    ...c.moves.map((m) => ({ date: m.by, label: `${m.from_name} → ${m.to_name}`, account: m.for ? `for ${m.for.charAt(0).toLowerCase()}${m.for.slice(1)}` : undefined, amount: m.amount, kind: 'move' as const, move: m })),
  ].sort((a, b) => a.date.localeCompare(b.date) || (a.kind === 'move' ? -1 : b.kind === 'move' ? 1 : 0))
  const shown = all ? entries : entries.slice(0, 7)
  const day = (d: string) => (d <= todayISO() ? 'Today' : shortDate(d).replace(/ \d{2}$/, ''))
  return (
    <section id="cash" className="card scroll-mt-16">
      <div className="flex items-start gap-3 px-4 pt-3">
        <IconTile name="calendar" color="var(--s7)" size={32} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">Until payday</div>
          <div className="text-xs text-muted">Salary by <b className="text-ink2">{shortDate(c.payday)}</b>{salary && <> into {salary}</>} · {days} days · day-to-day at {eur(c.daily)}/day</div>
        </div>
      </div>
      <div className="px-4 py-3">
        {c.moves.length === 0 && <div className="mb-2 flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every account covers what's due until payday</div>}
        {c.short > 0 && (
          <div className={clsx('mb-2 flex items-start gap-1.5 text-sm', c.cash >= c.short ? 'text-warn' : 'text-bad')}>
            <Icon name="alert" size={16} className="mt-0.5 shrink-0" />
            <span>Savings fall <b className="tnum">{eur(c.short)}</b> short — {c.cash >= c.short ? <>deposit that much of your {eur(c.cash)} cash, or lower a buffer below</> : 'lower a buffer below or spend less until payday'}.</span>
          </div>
        )}
        <div className="divide-y divide-line">
          {shown.map((e, i) => (
            <div key={i} className={clsx('flex items-center gap-3 py-1.5 text-sm', e.kind === 'move' && '-mx-2 rounded-lg border-0 bg-accent/10 px-2')}>
              <span className={clsx('w-12 shrink-0 text-xs', e.date <= todayISO() ? 'font-semibold text-warn' : 'text-muted')}>{e.kind === 'move' ? 'by ' : ''}{day(e.date)}</span>
              <span className="min-w-0 flex-1">
                <span className={clsx('block truncate', e.kind === 'move' && 'font-medium')}>{e.label}</span>
                {e.account && <span className="block truncate text-xs text-muted">{e.account}</span>}
              </span>
              {e.move ? (
                <>
                  <span className="shrink-0 font-medium tnum">{eur(e.amount)}</span>
                  <button className="btn-ghost h-7 shrink-0 px-2 text-xs" onClick={() => record(e.move!, `Top-up for ${e.move!.for.charAt(0).toLowerCase()}${e.move!.for.slice(1)}`)} title="Made the transfer? Record it">Record</button>
                </>
              ) : (
                <span className={clsx('shrink-0 tnum', e.amount > 0 && 'text-good')}>{e.amount > 0 ? '+' : '−'}{eur(Math.abs(e.amount))}</span>
              )}
            </div>
          ))}
        </div>
        {entries.length > 7 && <button className="mt-1 text-xs text-accent" onClick={() => setAll(!all)}>{all ? 'Show less' : `Show all ${entries.length}`}</button>}
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
