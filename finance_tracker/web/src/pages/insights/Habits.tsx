import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import clsx from 'clsx'
import { Bar, BarChart, CartesianGrid, Cell, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurc, eurk, monthLabel, pct } from '../../lib/format'
import { ledgerLink, monthEnd, type LedgerQuery } from '../../lib/links'
import { useCats } from '../../lib/categories'
import type { Habits as HabitsData, HabitMonth } from '../../lib/types'
import { Card, Loading } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'

// Habits: how often, how big, when — on spending by choice (alimony, the
// mortgage and finance costs left out), complete months only.

const DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const round = (v: number) => Math.round(v)
const SMALL = 9.99 // "under €10", inclusive
const MEAL = 199.99 // a meal of €200 or more is an occasion, not eating-out habit

/** The transactions behind each figure: spending by choice in a range,
 *  optionally only some categories or amounts. */
function useLinks() {
  const cats = useCats()
  const food = cats.tree.find((c) => c.id === 'food')?.children.map((c) => c.id) ?? []
  const eatingOut = food.filter((c) => c !== 'food.groceries')
  const base = (from: string, to: string, q: Partial<LedgerQuery> = {}) => ledgerLink({ kind: 'expense', choices: true, from, to, ...q })
  const month = (m: string, q: Partial<LedgerQuery> = {}) => base(`${m}-01`, monthEnd(m), q)
  return { ready: cats.ready && eatingOut.length > 0, base, month, eatingOut, small: { max: SMALL }, eating: { category: eatingOut, max: MEAL } }
}

// How each comparison reads. `more` says which way is the costlier habit
// (coloured); the rest are just shown as they moved.
const CHANGES: Record<HabitsData['changes'][number]['key'], { label: string; fmt: (v: number) => string; more?: 'bad' }> = {
  spent: { label: 'Spent by choice', fmt: (v) => `${eur(round(v))}/mo`, more: 'bad' },
  purchases: { label: 'Purchases', fmt: (v) => `${round(v)}/mo` },
  median: { label: 'Usual purchase', fmt: (v) => eurc(v) },
  small: { label: 'Buys under €10', fmt: (v) => `${round(v)}/mo`, more: 'bad' },
  eating_out: { label: 'Eating out', fmt: (v) => `${eur(round(v))}/mo`, more: 'bad' },
  weekend: { label: 'On weekends', fmt: (v) => pct(v) },
  days: { label: 'Days you spent', fmt: (v) => `${round(v)}/mo` },
}

export function Habits({ months }: { months: number }) {
  const n = String(Math.max(1, months))
  const { data, isLoading } = useQuery({ queryKey: ['habits', n], queryFn: () => api.get<HabitsData>('/insights/habits', { months: n }), enabled: months > 0 })
  const nav = useNavigate()
  const L = useLinks()
  // Year to date in January: no month of this year is complete yet.
  if (months <= 0) return <p className="text-sm text-muted">No complete month this year yet — pick 3M or 12M.</p>
  if (isLoading) return <Loading />
  if (!data || data.months.length === 0) return <p className="text-sm text-muted">Habits show once a month of spending is in.</p>
  const rows = data.months.map((m) => ({ ...m, big: m.purchases - m.small }))
  // Bars hand their row to onClick; line charts give the hovered month.
  const open = (q: Partial<LedgerQuery> = {}) => (d: { month?: string }) => d?.month && nav(L.month(d.month, q))
  const openLine = (q: Partial<LedgerQuery> = {}) => (s: { activeLabel?: string | number } | null) => s?.activeLabel && nav(L.month(String(s.activeLabel), q))
  const part = (from: number, to?: number) => (d: { month?: string }) => d?.month && nav(L.base(`${d.month}-${String(from).padStart(2, '0')}`, to ? `${d.month}-${to}` : monthEnd(d.month)))
  // A calendar month opens its latest complete one: last November in November.
  const seasonOpen = (d: { month?: number }) => {
    if (!d?.month) return
    const now = new Date()
    const y = d.month < now.getMonth() + 1 ? now.getFullYear() : now.getFullYear() - 1
    nav(L.month(`${y}-${String(d.month).padStart(2, '0')}`))
  }
  const tick = (m: string) => monthLabel(m)
  const thisMonth = new Date().getMonth() + 1
  return (
    <div className="space-y-4">
      {data.changes.length > 0 && <Compare data={data} links={L} />}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title="How often you buy">
          <div className="h-52">
            <ResponsiveContainer>
              <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="15%">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tickFormatter={tick} minTickGap={24} />
                <YAxis {...axisProps} width={32} allowDecimals={false} />
                <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (() => {
                  const m = payload[0].payload as HabitMonth
                  return <TooltipBox title={monthLabel(m.month, true)} rows={[
                    { color: 'var(--s1)', label: '€10 and more', value: String(m.purchases - m.small) },
                    { color: 'var(--s4)', label: 'Under €10', value: `${m.small} · ${eurc(m.small_spent)}` },
                    { label: 'Purchases', value: String(m.purchases), bold: true },
                    { label: 'Days you spent', value: String(m.days) },
                  ]} />
                })() : null} />
                <Bar dataKey="big" stackId="n" fill="var(--s1)" stroke="var(--chart-surface)" strokeWidth={1} isAnimationActive={false} cursor="pointer" onClick={open({ min: 10 })} />
                <Bar dataKey="small" stackId="n" fill="var(--s4)" stroke="var(--chart-surface)" strokeWidth={1} radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={open(L.small)} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3"><Legend items={[{ color: 'var(--s1)', label: '€10 and more' }, { color: 'var(--s4)', label: 'Under €10' }]} /></div>
          <div className="mt-1 text-xs text-muted">Purchases a month; a split receipt counts once (the Ledger lists its parts). Tap a colour for those purchases.</div>
        </Card>

        <Card title="Your usual purchase">
          <div className="h-52">
            <ResponsiveContainer>
              <LineChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }} onClick={openLine({ sort: 'amount' })} className="cursor-pointer">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tickFormatter={tick} minTickGap={24} />
                <YAxis {...axisProps} width={40} tickFormatter={(v) => `€${v}`} />
                <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) => active && payload?.length ? (() => {
                  const m = payload[0].payload as HabitMonth
                  return <TooltipBox title={monthLabel(m.month, true)} rows={[{ color: 'var(--s7)', label: 'Usual purchase', value: eurc(m.median), bold: true }, { label: 'Spent by choice', value: eur(m.spent) }]} />
                })() : null} />
                <Line dataKey="median" stroke="var(--s7)" strokeWidth={2} dot={{ r: 3, fill: 'var(--s7)', strokeWidth: 0 }} activeDot={{ r: 5 }} isAnimationActive={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-1 text-xs text-muted">The middle purchase of the month: half cost less, half more. Big one-offs don't move it. Tap a month for its purchases, largest first.</div>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title="When in the month">
          <div className="h-52">
            <ResponsiveContainer>
              <BarChart data={rows} stackOffset="expand" margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="15%">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tickFormatter={tick} minTickGap={24} />
                <YAxis {...axisProps} width={36} tickFormatter={(v) => pct(v)} />
                <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (() => {
                  const m = payload[0].payload as HabitMonth
                  const s = (v: number) => `${eur(v)} · ${pct(m.spent ? v / m.spent : 0)}`
                  return <TooltipBox title={monthLabel(m.month, true)} rows={[
                    { color: 'var(--s1)', label: 'Days 1–10', value: s(m.early) },
                    { color: 'var(--s2)', label: 'Days 11–20', value: s(m.mid) },
                    { color: 'var(--s3)', label: 'Days 21–end', value: s(m.late) },
                  ]} />
                })() : null} />
                <Bar dataKey="early" stackId="p" fill="var(--s1)" stroke="var(--chart-surface)" strokeWidth={1} isAnimationActive={false} cursor="pointer" onClick={part(1, 10)} />
                <Bar dataKey="mid" stackId="p" fill="var(--s2)" stroke="var(--chart-surface)" strokeWidth={1} isAnimationActive={false} cursor="pointer" onClick={part(11, 20)} />
                <Bar dataKey="late" stackId="p" fill="var(--s3)" stroke="var(--chart-surface)" strokeWidth={1} radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={part(21)} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3"><Legend items={[{ color: 'var(--s1)', label: 'Days 1–10' }, { color: 'var(--s2)', label: 'Days 11–20' }, { color: 'var(--s3)', label: 'Days 21–end' }]} /></div>
          <div className="mt-1 text-xs text-muted">Where each month's spending by choice landed — a heavy first third is payday spending. Tap a colour for those days.</div>
        </Card>

        <Card title="Days of the week">
          <div className="h-52">
            <ResponsiveContainer>
              <BarChart data={data.weekdays} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="20%">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="day" {...axisProps} tickFormatter={(d) => DAYS[d - 1]} />
                <YAxis {...axisProps} width={44} tickFormatter={eurk} />
                <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (
                  <TooltipBox title={DAYS[(payload[0].payload as HabitsData['weekdays'][number]).day - 1]} rows={[{ color: 'var(--s1)', label: 'On average', value: eur((payload[0].payload as HabitsData['weekdays'][number]).average), bold: true }]} />) : null} />
                <Bar dataKey="average" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false}>
                  {data.weekdays.map((w) => <Cell key={w.day} fillOpacity={w.day >= 6 ? 1 : 0.55} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-1 text-xs text-muted">What a day costs on average over the span; weekends in full colour. Card payments count on the day you paid.</div>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title="Eating out and groceries">
          <div className="h-52">
            <ResponsiveContainer>
              <LineChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }} onClick={L.ready ? openLine({ category: [...L.eatingOut, 'food.groceries'] }) : undefined} className="cursor-pointer">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tickFormatter={tick} minTickGap={24} />
                <YAxis {...axisProps} width={44} tickFormatter={eurk} />
                <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) => active && payload?.length ? (() => {
                  const m = payload[0].payload as HabitMonth
                  return <TooltipBox title={monthLabel(m.month, true)} rows={[{ color: 'var(--s2)', label: 'Eating out', value: eur(m.eating_out) }, { color: 'var(--s3)', label: 'Groceries', value: eur(m.groceries) }]} />
                })() : null} />
                <Line dataKey="eating_out" stroke="var(--s2)" strokeWidth={2} dot={{ r: 3, fill: 'var(--s2)', strokeWidth: 0 }} isAnimationActive={false} />
                <Line dataKey="groceries" stroke="var(--s3)" strokeWidth={2} dot={{ r: 3, fill: 'var(--s3)', strokeWidth: 0 }} isAnimationActive={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-3"><Legend items={[{ color: 'var(--s2)', label: 'Eating out' }, { color: 'var(--s3)', label: 'Groceries' }]} /></div>
          <div className="mt-1 text-xs text-muted">Eating out is every food category but groceries — restaurants, fast food, coffee, lunches at work, delivery — leaving out single meals of €200 or more (an occasion, not a habit). Tap a month for all its food purchases.</div>
        </Card>

        <Card title="Your year">
          <div className="h-52">
            <ResponsiveContainer>
              <BarChart data={data.season} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="15%">
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tick={{ ...axisProps.tick, fontSize: 10 }} tickFormatter={(m) => MONTHS[m - 1]} interval={0} />
                <YAxis {...axisProps} width={44} tickFormatter={eurk} />
                <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (() => {
                  const s = payload[0].payload as HabitsData['season'][number]
                  return <TooltipBox title={MONTHS[s.month - 1]} rows={[{ color: 'var(--s1)', label: `Average of ${s.years} year${s.years === 1 ? '' : 's'}`, value: eur(s.average), bold: true }]} />
                })() : null} />
                <Bar dataKey="average" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={seasonOpen}>
                  {data.season.map((s) => <Cell key={s.month} fillOpacity={s.month === thisMonth ? 1 : 0.55} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-1 text-xs text-muted">Each month's spending by choice, averaged over the last three years — the dear months to plan for. This month in full colour; tap a month for its latest one.</div>
        </Card>
      </div>
    </div>
  )
}

/** The last three complete months against the same three a year earlier. */
function Compare({ data, links: L }: { data: HabitsData; links: ReturnType<typeof useLinks> }) {
  const nav = useNavigate()
  const span = (r: [string, string]) => `${monthLabel(r[0])}–${monthLabel(r[1])}`
  // A tile opens the last three months' transactions behind it.
  const from = `${data.recent[0]}-01`, to = monthEnd(data.recent[1])
  const q: Partial<Record<HabitsData['changes'][number]['key'], Partial<LedgerQuery>>> = { median: { sort: 'amount' }, small: L.small, eating_out: L.eating }
  return (
    <Card title="Habits now and a year ago">
      <div className="text-xs text-muted">{span(data.recent)} against {span(data.year_ago)}, a month's worth each, refunds not taken off and card holds not yet booked left out. Tap one for its transactions.</div>
      <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
        {data.changes.map((c) => {
          const d = CHANGES[c.key]
          const ch = c.then > 0 ? c.now / c.then - 1 : 0
          const up = ch > 0.005, down = ch < -0.005
          const tone = d.more && (up || down) ? (up ? 'text-bad' : 'text-good') : 'text-ink2'
          return (
            <button key={c.key} className="min-w-0 rounded-xl bg-sunken/60 p-2.5 text-left hover:bg-sunken last:col-span-2 sm:last:col-span-1"
              onClick={() => (c.key !== 'eating_out' || L.ready) && nav(L.base(from, to, q[c.key]))}>
              <div className="truncate text-xs text-ink2">{d.label}</div>
              <div className="tnum mt-0.5 truncate font-semibold">{d.fmt(c.now)}</div>
              <div className="mt-0.5 flex flex-wrap items-center gap-x-1 text-xs">
                {(up || down) && c.then > 0 && <span className={clsx('inline-flex items-center tnum', tone)}><Icon name={up ? 'up' : 'down'} size={12} />{Math.abs(Math.round(ch * 100))}%</span>}
                <span className="text-muted">was {d.fmt(c.then)}</span>
              </div>
            </button>
          )
        })}
      </div>
    </Card>
  )
}
