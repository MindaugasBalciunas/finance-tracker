import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { dayLabel, eur, eurk, shortDate } from '../../lib/format'
import type { CashCalendar } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading } from '../../components/ui'
import { axisProps, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'

const WEEK = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

/** The next 60 days, day by day: cash in checking, savings and cash after
 *  each day's bills, transfers, income and usual spending. */
export function CashCalendarCard() {
  const { data: c, isLoading, error } = useQuery({ queryKey: ['calendar'], queryFn: () => api.get<CashCalendar>('/plan/calendar?days=60') })
  const [pick, setPick] = useState<string | null>(null)
  const weeks = useMemo(() => {
    if (!c) return []
    const days = c.days
    const lead = (new Date(days[0].date + 'T00:00:00').getDay() + 6) % 7 // Monday first
    const cells: (CashCalendar['days'][number] | null)[] = [...Array(lead).fill(null), ...days]
    const out = []
    for (let i = 0; i < cells.length; i += 7) out.push(cells.slice(i, i + 7))
    return out
  }, [c])
  if (isLoading) return <Card title="Next 60 days" icon="calendar"><Loading /></Card>
  if (error || !c) return <ErrorBox error={error} />
  const selected = c.days.find((d) => d.date === pick) ?? c.days.find((d) => d.events?.length)
  const tone = c.first_short ? 'var(--hue-bad)' : c.first_below ? 'var(--hue-warn)' : 'var(--hue-good)'
  return (
    <Card title="Next 60 days, day by day" icon="calendar" color={tone}
      action={<AskCFO q="Look at my cash calendar for the next 60 days: which days are tight, and what should I move or delay?" />}>
      {/* Wide screens: what it means and the picked day beside the month grid. */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)] lg:items-start">
        <div className="min-w-0">
        <div className={clsx('flex items-start gap-2 text-sm font-medium', c.first_short ? 'text-bad' : c.first_below ? 'text-warn' : 'text-good')}>
          <Icon name={c.first_short || c.first_below ? 'alert' : 'check'} size={16} className="mt-0.5 shrink-0" />
          <span>{c.first_short ? <>Cash runs out on <b>{dayLabel(c.first_short)}</b>.</>
            : c.first_below ? <>Cash dips under your {eur(c.floor)} emergency floor on <b>{dayLabel(c.first_below)}</b>; lowest {eur(c.low)} on {shortDate(c.low_date)}.</>
            : <>Cash stays above your {eur(c.floor)} floor — lowest {eur(c.low)} on {shortDate(c.low_date)}.</>}</span>
        </div>
        <div className="mt-2 h-28">
          <ResponsiveContainer>
            <AreaChart data={c.days} margin={{ top: 6, right: 4, bottom: 0, left: 0 }}>
              <XAxis dataKey="date" hide />
              <YAxis {...axisProps} tickFormatter={eurk} width={44} domain={[(min: number) => { const lo = Math.min(min, c.floor); return lo - Math.max(Math.abs(lo) * 0.05, 100) }, 'auto']} />
              <Tooltip content={({ active, payload }) => active && payload?.length ? <TooltipBox title={dayLabel(payload[0].payload.date)} rows={[{ label: 'Cash', value: eur(payload[0].payload.balance), bold: true }]} /> : null} />
              <ReferenceLine y={c.floor} stroke="rgb(var(--warn))" strokeDasharray="4 3" />
              <Area type="stepAfter" dataKey="balance" stroke="var(--c)" strokeWidth={2} fill="var(--c)" fillOpacity={0.12} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
        {selected && (
          <div className="mt-3 rounded-xl border border-line bg-sunken/40 p-3 text-sm">
            <div className="flex justify-between font-medium"><span>{dayLabel(selected.date)}</span><span className="tnum">{eur(selected.balance)}</span></div>
            {(selected.events ?? []).length ? (
              <ul className="mt-1 space-y-0.5">
                {selected.events!.map((e, k) => <li key={k} className="flex justify-between gap-2 text-xs"><span className="truncate text-ink2">{e.label}</span><span className={clsx('tnum', e.amount > 0 ? 'text-good' : 'text-ink')}>{e.amount > 0 ? '+' : '−'}{eur(Math.abs(e.amount))}</span></li>)}
              </ul>
            ) : <div className="mt-1 text-xs text-muted">No bills or income — just everyday spending (~{eur(c.daily)} a day).</div>}
          </div>
        )}
          <p className="mt-2 text-xs text-muted">From {eur(c.start)} today; ~{eur(c.daily)} of everyday spending a day, and every known bill, standing order and salary on its date. Moves between your own cash accounts don't change it.</p>
        </div>
        <div className="min-w-0">
        <div className="grid grid-cols-7 gap-1 text-center text-[10px] text-muted">{WEEK.map((w) => <div key={w}>{w}</div>)}</div>
        <div className="mt-1 space-y-1">
          {weeks.map((w, i) => (
            <div key={i} className="grid grid-cols-7 gap-1">
              {w.map((d, j) => d ? (
                <button key={d.date} onClick={() => setPick(d.date)} aria-label={`${dayLabel(d.date)}: ${eur(d.balance)}${d.events?.length ? `, ${d.events.length} payments` : ''}`}
                  className={clsx('min-h-11 rounded-lg border px-1 py-1 text-left transition', selected?.date === d.date ? 'border-accent bg-accent/10' : 'border-line hover:bg-sunken/60',
                    d.short ? 'bg-bad/10' : d.low ? 'bg-warn/10' : '')}>
                  <div className="flex items-center justify-between text-[11px]">
                    <span className={clsx('font-medium', d.date.endsWith('-01') && 'text-accent')}>{Number(d.date.slice(8))}</span>
                    <span className="flex gap-0.5">{(d.events ?? []).slice(0, 3).map((e, k) => <span key={k} className={clsx('h-1.5 w-1.5 rounded-full', e.amount > 0 ? 'bg-good' : 'bg-bad')} />)}</span>
                  </div>
                  <div className={clsx('tnum mt-0.5 truncate text-[10px]', d.short ? 'text-bad' : d.low ? 'text-warn' : 'text-muted')}>
                  {/* Phones: no € sign, so "15.6k" fits a seventh of the screen. */}
                  <span className="sm:hidden">{(d.balance / 1000).toFixed(1)}k</span><span className="hidden sm:inline">{eurk(d.balance)}</span>
                </div>
                </button>
              ) : <div key={`x${j}`} />)}
            </div>
          ))}
        </div>
        </div>
      </div>

    </Card>
  )
}
