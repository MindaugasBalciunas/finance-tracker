import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { catColor, useCats } from '../../lib/categories'
import { addMonths, eur, eurc, eurk, monthLabel, pct, thisMonth } from '../../lib/format'
import type { Flow, Tx } from '../../lib/types'
import { Card, Delta, Loading, Meter, Segmented, Stat } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { TxRow, useTxEditor } from '../../components/TxEditor'
import { Icon } from '../../components/Icon'
import { Review } from './YearReview'

// ── month review ──

export function ReviewTabs() {
  const [mode, setMode] = useState<'month' | 'year'>('month')
  return (
    <div className="space-y-4">
      <Segmented value={mode} onChange={setMode} options={[{ value: 'month', label: 'Month' }, { value: 'year', label: 'Year' }]} />
      {mode === 'month' ? <MonthReviewView /> : <Review />}
    </div>
  )
}

function MonthReviewView() {
  const [sp] = useSearchParams()
  const [month, setMonth] = useState(sp.get('month') || addMonths(thisMonth(), -1))
  const { data: r, isLoading } = useQuery({ queryKey: ['month-review', month], queryFn: () => api.get<any>('/insights/month', { month }) })
  const cats = useCats()
  const editor = useTxEditor()
  if (isLoading || !r) return <Loading />
  const f = r.flow as Flow
  const avg = r.six_month_avg as Flow
  const prev = r.previous as Flow
  const toneBorder = r.tone === 'good' ? 'border-l-good' : r.tone === 'bad' ? 'border-l-bad' : 'border-l-axis'
  const rate = Math.max(0, Math.min(1, f.savings_rate))
  const vs = (cur: number, ref: number, goodUp: boolean) => ref ? <Delta value={cur - ref} goodWhenUp={goodUp} /> : null
  const maxCat = Math.max(...r.categories.map((c: any) => Math.max(c.spent, c.average)), 1)
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, -1))} aria-label="Previous month"><Icon name="chevronL" /></button>
        <div className="text-base font-semibold">{monthLabel(month, true)}{!r.complete && <span className="ml-2 text-xs font-normal text-muted">in progress</span>}</div>
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, 1))} disabled={month >= thisMonth()} aria-label="Next month"><Icon name="chevronR" /></button>
      </div>
      <section className={clsx('card border-l-4 p-4', toneBorder)}>
        <div className="section-title">The month in one line</div>
        <h2 className="mt-1 text-lg font-semibold">{r.verdict}</h2>
        {f.income > 0 && (
          <div className="mt-3">
            <div className="flex justify-between text-xs text-muted"><span>Savings rate <b className="text-ink">{pct(f.savings_rate)}</b></span><span>usual {pct(avg.savings_rate)}</span></div>
            <div className="relative mt-1 h-2 rounded-full bg-sunken">
              <div className={clsx('h-full rounded-full', f.savings_rate < avg.savings_rate ? 'bg-warn' : 'bg-good')} style={{ width: `${rate * 100}%` }} />
              <div className="absolute -top-1 h-4 w-0.5 rounded bg-ink" style={{ left: `${Math.max(0, Math.min(1, avg.savings_rate)) * 100}%` }} />
            </div>
          </div>
        )}
        {r.highlights?.length > 0 && (
          <ul className="mt-3 space-y-1.5">
            {r.highlights.map((h: any) => (
              <li key={h.text} className="flex items-start gap-2 text-sm">
                <Icon name={h.tone === 'good' ? 'check' : h.tone === 'bad' ? 'alert' : 'refresh'} size={16} className={clsx('mt-0.5 shrink-0', h.tone === 'good' ? 'text-good' : h.tone === 'bad' ? 'text-bad' : 'text-muted')} />
                <span>{h.text}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Income" value={eur(f.income)} sub={<>vs last month {vs(f.income, prev.income, true)}</>} />
        <Stat label="Spending" value={eur(f.spending)} sub={<>vs 6-mo avg {vs(f.spending, avg.spending, false)}</>} />
        <Stat label="Saved" value={eur(f.saved)} tone={f.saved < 0 ? 'bad' : undefined} sub={<>vs 6-mo avg {vs(f.saved, avg.saved, true)}</>} />
        <Stat label="Net worth" value={eur(r.net_worth.end)} sub={<Delta value={r.net_worth.change} />} />
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title="Where it went" action={<span className="text-xs text-muted">tick = 6-month average</span>}>
          <div className="space-y-2.5">
            {r.categories.slice(0, 10).map((c: any) => (
              <a key={c.category} href={`#/ledger?category=${c.category}&from=${month}-01&to=${month}-31`} className="block">
                <div className="flex justify-between text-sm"><span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(c.category) }} />{cats.name(c.category)}</span>
                  <span className="tnum">{eur(c.spent)} <span className={clsx('text-xs', c.delta > 0 ? 'text-bad' : 'text-good')}>{c.average > 0 ? `${c.delta > 0 ? '+' : '−'}${eur(Math.abs(c.delta))}` : 'new'}</span></span></div>
                <div className="relative mt-1 h-1.5 rounded-full bg-sunken">
                  <div className="h-full rounded-full" style={{ width: `${(c.spent / maxCat) * 100}%`, background: catColor(c.category) }} />
                  {c.average > 0 && <div className="absolute -top-0.5 h-2.5 w-0.5 bg-ink" style={{ left: `${(c.average / maxCat) * 100}%` }} />}
                </div>
              </a>
            ))}
          </div>
        </Card>
        <Card title="Day by day">
          <SpendCalendar days={r.daily} />
          {r.budget && (
            <div className="mt-4 border-t border-line pt-3">
              <div className="mb-2 text-sm"><b>{r.budget.within_count}</b> budgets within limit{r.budget.over?.length ? <>, <b className="text-bad">{r.budget.over.length}</b> over</> : ''}</div>
              <div className="space-y-2">
                {r.budget.lines.slice(0, 6).map((l: any) => (
                  <div key={l.name}><div className="flex justify-between text-xs"><span>{l.name}</span><span className="tnum">{eur(l.spent)} / {eur(l.budgeted)}</span></div><Meter className="mt-1" value={l.spent} max={l.budgeted} /></div>
                ))}
              </div>
            </div>
          )}
          {r.owed_to_you > 0 && <div className="mt-3 text-sm">Owed to you: <b className="tnum">{eurc(r.owed_to_you)}</b></div>}
        </Card>
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card pad={false} title="Biggest expenses"><div className="divide-y divide-line border-t border-line">{r.top_expenses.map((t: Tx) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}</div></Card>
        <Card pad={false} title="Money in"><div className="divide-y divide-line border-t border-line">{(r.top_income ?? []).map((t: Tx) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}</div></Card>
      </div>
      <Card title="Twelve months">
        <div className="h-44">
          <ResponsiveContainer>
            <BarChart data={r.trend} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="period" {...axisProps} tickFormatter={(m) => monthLabel(m).slice(0, 3)} />
              <YAxis {...axisProps} tickFormatter={eurk} width={44} />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? <TooltipBox title={monthLabel(payload[0].payload.period, true)} rows={[{ color: 'var(--s1)', label: 'Income', value: eur(payload[0].payload.income) }, { color: 'var(--s2)', label: 'Spending', value: eur(payload[0].payload.spending) }]} /> : null} />
              <Bar dataKey="income" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false}>{r.trend.map((x: Flow) => <Cell key={x.period} fillOpacity={x.period === month ? 1 : 0.35} />)}</Bar>
              <Bar dataKey="spending" fill="var(--s2)" radius={[4, 4, 0, 0]} isAnimationActive={false}>{r.trend.map((x: Flow) => <Cell key={x.period} fillOpacity={x.period === month ? 1 : 0.35} />)}</Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'Income' }, { color: 'var(--s2)', label: 'Spending' }]} /></div>
      </Card>
      <Checks checks={r.checks} />
    </div>
  )
}

/** Daily spending as a month calendar: one hue, lighter → darker with the amount. */
export function SpendCalendar({ days }: { days: { date: string; spent: number }[] }) {
  const max = Math.max(...days.map((d) => d.spent), 1)
  const first = new Date(days[0].date + 'T00:00:00').getDay()
  const lead = (first + 6) % 7 // Monday-first
  const shade = (v: number) => (v <= 0 ? 'rgb(var(--sunken))' : `color-mix(in oklab, var(--s1) ${Math.round(18 + 82 * Math.sqrt(v / max))}%, rgb(var(--surface)))`)
  return (
    <div>
      <div className="grid grid-cols-7 gap-1 text-center text-[10px] text-muted">{['M', 'T', 'W', 'T', 'F', 'S', 'S'].map((d, i) => <div key={i}>{d}</div>)}</div>
      <div className="mt-1 grid grid-cols-7 gap-1">
        {Array.from({ length: lead }).map((_, i) => <div key={'x' + i} />)}
        {days.map((d) => (
          <div key={d.date} title={`${d.date}: ${eurc(d.spent)}`} className="flex aspect-square flex-col items-center justify-center rounded-md text-[10px]" style={{ background: shade(d.spent), color: d.spent / max > 0.55 ? 'white' : undefined }}>
            <span>{Number(d.date.slice(8))}</span>
            {d.spent > 0 && <span className="tnum text-[8px] leading-none sm:text-[10px]">{d.spent >= 1000 ? eurk(d.spent) : `€${Math.round(d.spent)}`}</span>}
          </div>
        ))}
      </div>
    </div>
  )
}

export function Checks({ checks }: { checks: { level: string; text: string; link: string }[] }) {
  if (!checks?.length) return null
  return (
    <Card title="Worth a look">
      <div className="space-y-2">
        {checks.map((c) => (
          <a key={c.text} href={`#${c.link}`} className="flex items-start gap-2 text-sm hover:text-accent">
            <Icon name={c.level === 'warn' ? 'alert' : 'refresh'} size={16} className={clsx('mt-0.5 shrink-0', c.level === 'warn' ? 'text-warn' : 'text-muted')} />
            <span className="flex-1">{c.text}</span>
            <Icon name="chevronR" size={14} className="mt-1 text-muted" />
          </a>
        ))}
      </div>
    </Card>
  )
}
