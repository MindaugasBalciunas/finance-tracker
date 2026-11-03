import { Area, CartesianGrid, ComposedChart, Line, ReferenceDot, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useWide } from '../lib/hooks'
import { eur, eurk } from '../lib/format'
import type { Overview } from '../lib/types'
import { axisProps, gridProps, TooltipBox } from './charts'

// The current month day by day, shared by Home (in its "left to spend" card)
// and Plan (full width, taller).

/** The day labels under the month chart: every day when it is wide;
 *  otherwise every other day and the last, skipping the day before it so
 *  "29 30" don't run together. */
export function dayTicks(dim: number, wide: boolean) {
  return Array.from({ length: dim }, (_, i) => i + 1).filter((d) => wide || (d % 2 === 1 && d !== dim - 1) || d === dim)
}

/** How much of a month's free spending a typical month has done by each day
 *  (0–1), from the typical-month curve — so pace follows your usual rhythm
 *  (rent-week, weekends, payday) instead of a straight line. Falls back to
 *  an even split when there is no history. */
export function typicalShare(p: Overview['plan'], day: number) {
  const days = p.days ?? []
  const end = days[days.length - 1]?.typical ?? 0
  if (!days.length || end <= 0) return days.length ? day / days.length : 0
  return Math.min(1, Math.max(0, (days[Math.min(day, days.length) - 1]?.typical ?? 0) / end))
}

/** The month day by day: free spending so far against your typical month's
 *  rhythm through this month's free money, and where this pace ends up; dated
 *  events (obligations due or paid, income) sit on the axis. */
export function MonthTimeline({ p, big = false }: { p: Overview['plan']; big?: boolean }) {
  // Every day labelled only when the chart is wide: half the card on a
  // laptop (lg); on a desktop (xl) it sits in Home's narrow month column.
  // Plan's runs the page's full width from lg up.
  const lg = useWide('(min-width: 1024px)')
  const xl = useWide()
  const wide = big ? lg : lg && !xl
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
    <div className={big ? 'flex flex-col' : 'mt-3 flex flex-col lg:mt-0 lg:h-full xl:h-auto'}>
      {/* Home, lg: fills the left half of the card; xl: the card stacks in a column, so a fixed height. */}
      <div className={big ? 'h-72 sm:h-80 lg:h-96' : 'h-52 sm:h-60 lg:h-auto lg:min-h-72 lg:flex-1 xl:h-60 xl:min-h-0 xl:flex-none'}>
        <ResponsiveContainer>
          <ComposedChart data={rows} margin={{ top: 14, right: 8, bottom: 0, left: 0 }}>
            <defs>
              <linearGradient id="mt-cum" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--s1)" stopOpacity={0.25} />
                <stop offset="100%" stopColor="var(--s1)" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid {...gridProps} />
            {/* Every day of this month (28–31), labelled; phones every other day. */}
            <XAxis dataKey="day" {...axisProps} type="number" domain={[1, dim]} interval={0}
              ticks={dayTicks(dim, wide)}
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
            {/* On the last day "today" would hang off the right edge; shift it in. */}
            <ReferenceLine x={today} stroke="rgb(var(--ink))" strokeOpacity={0.5} label={{ value: 'today', position: 'top', dx: today === dim ? -10 : 0, fontSize: 10, fill: 'var(--chart-text)' }} />
            {/* What is left today: the gap from your spending up to the free money.
                Its label sits on the roomier side of today, so early in the
                month it doesn't run into the euro axis, and a halo in the
                card's colour keeps it readable where a line crosses it. */}
            <ReferenceLine segment={[{ x: today, y: cumToday }, { x: today, y: free }]} stroke={p.safe_to_spend < 0 ? 'rgb(var(--bad))' : 'rgb(var(--good))'} strokeWidth={3} ifOverflow="extendDomain"
              label={{ value: p.safe_to_spend < 0 ? `${eur(-p.safe_to_spend)} over` : `${eur(p.safe_to_spend)} left`, position: today <= dim / 2 ? 'right' : 'left', fontSize: 11, fontWeight: 600, stroke: 'var(--chart-surface)', strokeWidth: 3, paintOrder: 'stroke', fill: p.safe_to_spend < 0 ? 'rgb(var(--bad))' : 'rgb(var(--good))' }} />
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

