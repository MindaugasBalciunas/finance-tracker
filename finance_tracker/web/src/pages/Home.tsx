import clsx from 'clsx'
import type { Overview } from '../lib/types'
import { Link, useNavigate } from 'react-router-dom'
import { Area, AreaChart, ReferenceDot, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { useNetWorthHistory, useOverview, usePeriod, usePrefs } from '../lib/hooks'
import { RANGES, rangeFrom, rangeLabel, rangeStep, SHORT } from '../lib/periods'
import { eur, eurk, monthLabel, pct, shortDate, signed } from '../lib/format'
import { AskCFO, Card, Delta, ErrorBox, Loading, Meter, Segmented, Stat, Toggle } from '../components/ui'
import { TooltipBox } from '../components/charts'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { Icon, IconTile } from '../components/Icon'
import { catIcon, useCats, GROUPS, LIQUID_GROUPS } from '../lib/categories'
import { Checks } from './Insights'

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

  const m = o.month
  const avg = o.avg12
  const spendPace = avg.spending > 0 ? m.spending / (avg.spending * Math.max(o.month_progress, 0.05)) : 0
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
    <div className="space-y-4">
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
          <span className="text-xs text-muted">{rangeLabel(range)} <Delta value={periodChange} /></span>
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

      {o.inbox_open > 0 && (
        <button onClick={() => nav('/ledger/inbox')} className="card flex w-full items-center gap-3 p-3.5 text-left hover:bg-sunken/50">
          <span className="grid h-9 w-9 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="inbox" /></span>
          <span className="flex-1 text-sm"><b>{o.inbox_open}</b> bank {o.inbox_open === 1 ? 'transaction waits' : 'transactions wait'} for review</span>
          <Icon name="chevronR" className="text-muted" />
        </button>
      )}

      {/* This month */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon="bag" color="var(--s2)" label={`Spent in ${monthLabel(m.period || o.date.slice(0, 7))}`} value={eur(m.spending)}
          sub={<>typical month {eur(avg.spending)} · {pct(o.month_progress)} through</>}
          tone={spendPace > 1.15 ? 'bad' : undefined} onClick={() => nav('/insights/spending')} />
        <Stat icon="piggy" color="var(--s6)" label="Saved this month" value={eur(m.saved)} sub={m.income > 0 ? `${pct(m.savings_rate)} of income · avg ${pct(avg.savings_rate)}` : 'no income booked yet'}
          tone={m.saved < 0 ? 'bad' : undefined} onClick={() => nav('/insights/cashflow')} />
        <Stat icon="briefcase" color="var(--s6)" label="Income this month" value={eur(m.income)} sub={`typical month ${eur(avg.income)}`}
          onClick={() => nav('/insights/cashflow')} />
        <Stat icon="umbrella" color="var(--s3)" label="Emergency fund" value={`${o.emergency.months.toFixed(1)} mo`} sub={`${eur(o.emergency.cash)} cash · target ${o.emergency.target_months} mo`}
          tone={o.emergency.months < o.emergency.target_months ? 'warn' : undefined} onClick={() => nav('/insights/fi')} />
      </div>

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
          ) : (
            <div className="mt-3 flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every budget is on track</div>
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
            <div className="text-sm text-muted">No recurring charges expected in the next two weeks.</div>
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
          <div className="mt-3 grid grid-cols-2 gap-3 border-t border-line pt-3 text-sm">
            <div><div className="text-muted text-xs">FI progress</div><div className="font-semibold">{pct(o.fi_progress, 1)}</div></div>
            <div><div className="text-muted text-xs">Years to FI at current pace</div><div className="font-semibold">{o.years_to_fi >= 0 ? o.years_to_fi.toFixed(1) : '—'}</div></div>
          </div>
        </Card>
      </div>

      {o.review_month && (
        <Link to={`/insights/review?month=${o.review_month}`} className="card flex items-center gap-3 p-3.5 hover:bg-sunken/50">
          <span className="grid h-9 w-9 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="chart" /></span>
          <span className="flex-1 text-sm">Your <b>{monthLabel(o.review_month, true)}</b> review is ready — how the month went and what needs a look.</span>
          <Icon name="chevronR" className="text-muted" />
        </Link>
      )}

      <Checks checks={(o.checks ?? []).filter((c) => !c.link.startsWith('/ledger/inbox'))} />

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

      <button onClick={() => editor.open()} className="fixed bottom-20 right-4 z-20 grid h-14 w-14 place-items-center rounded-full bg-accent text-white shadow-lg sm:hidden" aria-label="Add transaction">
        <Icon name="plus" size={26} />
      </button>
    </div>
  )
}



/** The everyday question: how much free money is left this month, and what
 *  that means per day. Same money as Plan's "safe to spend". */
function LeftToSpend({ p, progress, month }: { p: Overview['plan']; progress: number; month: string }) {
  const left = p.safe_to_spend
  const total = Math.max(p.free_spent + Math.max(left, 0), 1)
  const used = Math.min(1, Math.max(0, p.free_spent / total))
  const vsTypical = p.typical_day > 0 ? p.avg_day / p.typical_day - 1 : 0
  const outlook = left < 0 ? 'text-bad' : p.projected_left < 0 ? 'text-warn' : 'text-good'
  return (
    <Link to="/plan" className="card block px-4 py-3 hover:bg-sunken/30" title="Free money this month — same as Plan's safe to spend">
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
      <div className="relative mt-2.5 h-1.5 rounded-full bg-sunken">
        <div className={clsx('h-full rounded-full', left < 0 ? 'bg-bad' : used > progress + 0.1 ? 'bg-warn' : 'bg-accent')} style={{ width: `${Math.max(2, used * 100)}%` }} />
        <div className="absolute -top-0.5 h-2.5 w-0.5 rounded bg-ink/60" style={{ left: `${Math.min(100, progress * 100)}%` }} title="today" />
      </div>
      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-muted">
        <span>Avg day <b className="tnum text-ink">{eur(p.avg_day)}</b>{p.typical_day > 0 && <> vs <span className="tnum">{eur(p.typical_day)}</span> typical <span className={clsx('tnum', vsTypical > 0.1 ? 'text-bad' : vsTypical < -0.1 ? 'text-good' : '')}>({vsTypical >= 0 ? '+' : '−'}{pct(Math.abs(vsTypical), 0)})</span></>}</span>
        <span title="This month's pace, blended with your typical day while the month is young">Month end <b className={clsx('tnum', outlook)}>{p.projected_left >= 0 ? eur(p.projected_left) : `−${eur(-p.projected_left)}`}</b> {p.projected_left >= 0 ? 'left' : 'short'}</span>
      </div>
    </Link>
  )
}
