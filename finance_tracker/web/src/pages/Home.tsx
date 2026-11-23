import { useState, type CSSProperties, type ReactNode } from 'react'
import clsx from 'clsx'
import type { CashCalendar, Overview } from '../lib/types'
import { Link } from 'react-router-dom'
import { Area, AreaChart, ReferenceDot, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { useCashCalendar, useNetWorthHistory, useOverview, usePeriod, usePrefs, useWide } from '../lib/hooks'
import { RANGES, rangeFrom, rangeLabel, rangeStep, SHORT } from '../lib/periods'
import { eur, eurk, monthLabel, pct, shortDate, todayISO } from '../lib/format'
import { CountUp, Card, Delta, ErrorBox, hue, Loading, Segmented, Toggle } from '../components/ui'
import { GroupBar, TooltipBox } from '../components/charts'
import { MonthTimeline, typicalShare } from '../components/MonthChart'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { QuickActions } from '../components/QuickActions'
import { PaydayStrip } from './plan/Cash'
import { Icon, IconTile } from '../components/Icon'
import { GROUPS, LIQUID_GROUPS } from '../lib/categories'

/** Home is a one-page review, read top to bottom: where you stand (net
 *  worth), what needs you today, how the month is going, cash until payday,
 *  and what was spent. Charts and detail live on their pages, one tap away.
 *  (It was a big net-worth chart and four equal cards; nothing said what
 *  needed attention.) */
export default function Home() {
  const { data: o, isLoading, error } = useOverview()
  const editor = useTxEditor()
  const { data: cal } = useCashCalendar()
  const wide = useWide()
  if (isLoading) return <Loading />
  if (error || !o) return <ErrorBox error={error} />
  const month = monthLabel(o.date.slice(0, 7), true)

  const worth = <NetWorthHero o={o} />
  const today = <TodayCard o={o} cal={cal} />
  const thisMonth = <LeftToSpend p={o.plan} progress={o.month_progress} month={month} />
  const cash = o.cash ? <PaydayStrip c={o.cash} /> : null
  const recent = (
    <Card pad={false} icon="list" color="var(--s2)" title="Recent" action={
      <div className="flex gap-1">
        <button className="btn-ghost h-8 px-2.5 text-xs" onClick={editor.scan}><Icon name="camera" size={16} />Scan</button>
        <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => editor.open()}><Icon name="plus" size={16} />Add</button>
      </div>}>
      <div className="divide-y divide-line pb-1">
        {(o.recent ?? []).slice(0, 6).map((t) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
      </div>
      <Link to="/ledger" className="block border-t border-line px-4 py-3 text-center text-sm text-accent">All transactions</Link>
    </Card>
  )
  return (
    <div className="flex flex-col gap-4">
      <QuickActions inboxOpen={o.inbox_open} />
      {wide ? (
        // Desktop: where you stand and what was spent on the left; the review
        // column on the right — what needs you, the month, cash until payday —
        // so both columns run the page's height (a short right column and a
        // card squeezed beside the month left large empty areas).
        <div className="grid grid-cols-3 items-start gap-4">
          <div className="col-span-2 space-y-4">{worth}{recent}</div>
          <div className="space-y-4">{today}{thisMonth}{cash}</div>
        </div>
      ) : <>
        {worth}
        {today}
        {thisMonth}
        {cash}
        {recent}
      </>}
    </div>
  )
}

/** Where you stand: net worth (or liquid) with its changes, the chart over
 *  the period you pick, and each group one tap from its page. Kept at its
 *  original size: the owner's favourite part of Home. */
function NetWorthHero({ o }: { o: Overview }) {
  const { prefs, set: setPrefs } = usePrefs()
  const liquid = prefs.liquid_only
  // The chart's period, remembered like every other chart's.
  const [range, setRange] = usePeriod('home', '1y', RANGES.map((r) => r.value))
  const { data: hist } = useNetWorthHistory(rangeFrom(range), rangeStep(range))
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
  const dotLabel = (m: { i: number; p: (typeof spark)[number] }, above: boolean) => (props: { viewBox?: unknown }) => {
    const vb = (props.viewBox ?? {}) as { x?: number; y?: number; width?: number; height?: number }
    const x = (vb.x ?? 0) + (vb.width ?? 0) / 2
    const y = (vb.y ?? 0) + (above ? -8 : (vb.height ?? 0) + 14)
    return (
      <text x={x} y={y} textAnchor={anchor(m.i)} fontSize={11} fill="var(--chart-text)">
        <tspan fontWeight={600} fill="rgb(var(--ink))">{eurk(m.p[key])}</tspan>
        <tspan dx={4}>{SHORT.includes(range) ? shortDate(m.p.date) : monthLabel(m.p.date.slice(0, 7))}</tspan>
      </text>
    )
  }

  const hero = (
    <section className="hero overflow-hidden">
      <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-end sm:justify-between sm:p-6">
        <div>
          <div className="flex items-center justify-between gap-4">
            <div className="text-sm text-ink2">{liquid ? 'Liquid assets' : 'Net worth'}</div>
            <span className="text-xs sm:hidden"><Toggle checked={liquid} onChange={(v) => setPrefs({ liquid_only: v })} label="Liquid only" /></span>
          </div>
          <div className="grad-text mt-1 text-4xl font-bold tracking-tight sm:text-5xl"><CountUp value={liquid ? o.liquid : o.net_worth} format={eur} /></div>
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
        // Each group opens its page: Investments was the page most often
        // reached only after several hops (Home → Insights → Wealth → tab).
        <div className="border-t border-line px-4 py-3 sm:px-6">
          <GroupBar parts={assets} debt={liquid ? 0 : o.debt} link={(id) => (id === 'investments' ? '/wealth/investments' : id === 'real_assets' ? '/wealth/ownership' : '/wealth')} />
          <div className="mt-2 flex items-center justify-between gap-3 text-xs text-muted">
            <span className="min-w-0">{liquid ? `Liquid total ${eur(assetTotal)} · house and car excluded` : `Assets ${eur(assetTotal)} − debt ${eur(o.debt)} = ${eur(o.net_worth)}`}</span>
            <span className="flex shrink-0 gap-3">
              <Link to="/wealth/investments" className="inline-flex items-center gap-0.5 text-accent">Investments<Icon name="chevronR" size={14} /></Link>
              <Link to="/wealth" className="inline-flex items-center gap-0.5 text-accent">Wealth<Icon name="chevronR" size={14} /></Link>
            </span>
          </div>
        </div>
      )}
    </section>
  )
  return hero
}

/** Today: one sentence on where things stand, then what needs you — the
 *  same actions as the bell, most urgent first, each a tap from its page. */
function TodayCard({ o, cal }: { o: Overview; cal?: CashCalendar }) {
  const p = o.plan
  const free = p.free_spent + (p.fund_overdrawn ?? 0) + p.safe_to_spend
  const shortPay = (p.free_planned ?? free) - free > 0.5 ? (p.free_planned ?? free) - free : 0
  const now = todayISO()
  const day = Number(now.slice(8, 10))
  const dim = p.days?.length || 30
  // The headline: cash risk first (it's what can go wrong today), then the month.
  const cashLine = cal?.first_short ? { tone: 'bad', text: <>Cash runs out {cal.first_short <= now ? 'today' : `on ${shortDate(cal.first_short)}`}.</> }
    : cal?.first_below && cal.first_below <= now ? { tone: 'warn', text: <>Cash is under your {eur(cal.floor)} emergency floor today ({eur(cal.start)}).</> }
    : null
  const monthLine = p.safe_to_spend < 0
    ? shortPay > 0
      ? { tone: 'warn', text: <>The month is {eur(-p.safe_to_spend)} over because this month's pay was {eur(shortPay)} short. At your usual pay you'd have {eur(shortPay + p.safe_to_spend)} left.</> }
      : { tone: 'bad', text: <>The month is {eur(-p.safe_to_spend)} over budget.</> }
    : p.projected_left < 0
      ? { tone: 'warn', text: <>{eur(p.safe_to_spend)} left, but heading for {eur(-p.projected_left)} over by month end.</> }
      : { tone: 'good', text: <>On track: {eur(p.safe_to_spend)} left, about {eur(p.per_day_left)} a day.</> }
  const lines = [cashLine, monthLine].filter(Boolean) as { tone: string; text: ReactNode }[]
  const rank = { bad: 0, warn: 1, info: 2 } as Record<string, number>
  const actions = [...(o.actions ?? [])].sort((a, b) => (rank[a.level] ?? 3) - (rank[b.level] ?? 3) || (a.due ?? '9').localeCompare(b.due ?? '9'))
  if (o.inbox_open > 0 && !actions.some((a) => a.kind === 'inbox')) actions.unshift({ level: 'warn', kind: 'inbox', text: `${o.inbox_open} bank ${o.inbox_open === 1 ? 'payment' : 'payments'} to review`, link: '/ledger/inbox' })
  const tone = (t: string) => (t === 'bad' ? 'text-bad' : t === 'warn' ? 'text-warn' : t === 'good' ? 'text-good' : 'text-ink2')
  return (
    <section className="card">
      <div className="px-4 pt-3">
        <div className="text-xs text-muted">Today · {shortDate(now)} · day {day} of {dim}</div>
        <div className="mt-1.5 space-y-1.5">
          {lines.map((l, i) => (
            <p key={i} className={clsx('flex items-start gap-2 text-sm font-medium', tone(l.tone))}>
              <Icon name={l.tone === 'good' ? 'check' : 'alert'} size={16} className="mt-0.5 shrink-0" /><span>{l.text}</span>
            </p>
          ))}
        </div>
      </div>
      <div className="mt-3 border-t border-line">
        <div className="px-4 pt-2.5 text-xs font-semibold uppercase tracking-wide text-muted">Needs you</div>
        {actions.length === 0 ? (
          <div className="flex items-center gap-2 px-4 py-3 text-sm text-good"><Icon name="check" size={16} />Nothing to do today</div>
        ) : (
          <ul className="divide-y divide-line">
            {actions.slice(0, 5).map((a, i) => (
              <li key={i}>
                <Link to={a.link && a.link !== 'cash' ? a.link : '/plan/cash'} className="flex items-center gap-3 px-4 py-2.5 hover:bg-sunken/40">
                  <span className={clsx('h-2 w-2 shrink-0 rounded-full', a.level === 'bad' ? 'bg-bad' : a.level === 'warn' ? 'bg-warn' : 'bg-accent')} />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm">{a.text}</span>
                    {a.detail && <span className="block text-xs text-muted">{a.detail}</span>}
                  </span>
                  <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
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
    // The day-by-day chart and the months against the plan are on Plan → Budget.
    <section className="card" style={hue(left < 0 ? 'var(--hue-bad)' : p.projected_left < 0 ? 'var(--hue-warn)' : 'var(--hue-good)')}>
      <Link to="/plan" className="block px-4 pt-3 hover:bg-sunken/30" title="Left to spend this month — the same figure as on Plan">
        <div className="flex items-center gap-3">
          <IconTile name="wallet" color="var(--s1)" size={32} />
          <div className="min-w-0 flex-1">
            <div className="text-xs text-ink2">Left to spend in {month}</div>
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className={clsx('text-xl font-semibold tracking-tight tnum', left < 0 ? 'text-bad' : p.projected_left < 0 ? 'text-warn' : 'text-good')}><CountUp value={left} format={eur} /></span>
              <span className="text-xs text-muted">{left > 0 ? <>≈ <b className="tnum text-ink">{eur(p.per_day_left)}</b>/day · {p.days_left} days left</> : <span className="text-bad">over budget</span>}</span>
            </div>
          </div>
          <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
        </div>
        <MonthBar p={p} progress={pace} warn={used > pace + 0.1} />
        <Verdict p={p} />
      </Link>
      {/* The month day by day, as on Plan: back on Home in 3.0.1 — the owner
          missed it after 3.0.0 left only the bar and the verdict here. */}
      <div className="min-w-0 px-4 pb-3"><MonthTimeline p={p} /></div>
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

/** One honest outlook instead of three numbers: where the month is heading
 *  at your usual rhythm (one-offs included), and the reasons behind it as
 *  rows that add up — biggest first. */
function Verdict({ p }: { p: Overview['plan'] }) {
  const left = p.safe_to_spend
  const rest = p.expected_rest ?? 0
  const vsTypical = p.typical_day > 0 ? p.avg_day / p.typical_day - 1 : 0
  const runOut = p.run_out_day ? `${p.run_out_day} ${monthLabel(todayISO().slice(0, 7)).split(' ')[0]}` : ''
  // A short pay (the first, partial one after a job change) is why the month
  // is over, not the spending: say so, as Today and the month chart do.
  const free = p.free_spent + (p.fund_overdrawn ?? 0) + left
  const shortPay = (p.free_planned ?? free) - free > 0.5 ? (p.free_planned ?? free) - free : 0
  const tone = left < 0 ? (shortPay > 0 ? 'text-warn' : 'text-bad') : p.projected_left < 0 ? 'text-warn' : 'text-good'
  const headline = left < 0 && shortPay > 0 ? <>Short pay: <b className="tnum">{eur(shortPay)}</b> less this month. At usual pay, <b className="tnum">{eur(shortPay + left)}</b> left</>
    : left < 0 ? <>Already <b className="tnum">{eur(-left)}</b> over this month</>
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

