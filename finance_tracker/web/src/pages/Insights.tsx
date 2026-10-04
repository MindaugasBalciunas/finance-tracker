import { useMemo, useState } from 'react'
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../lib/api'
import { useCashflow } from '../lib/hooks'
import { catColor, CORE, useCats } from '../lib/categories'
import { addMonths, eur, eurc, eurk, monthLabel, pct, shortDate, thisMonth, todayISO } from '../lib/format'
import type { Flow, Recurring, Tx } from '../lib/types'
import { AskCFO, Card, Empty, Loading, PageHeader, Segmented, Stat, Tabs } from '../components/ui'
import { axisProps, gridProps, Legend, ShareBar, TooltipBox } from '../components/charts'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { Icon } from '../components/Icon'

const TABS = [
  { value: 'cashflow', label: 'Cash flow' }, { value: 'spending', label: 'Spending' }, { value: 'trends', label: 'Trends' },
  { value: 'recurring', label: 'Recurring' }, { value: 'fi', label: 'Independence' }, { value: 'review', label: 'Year review' },
]

export default function Insights() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'cashflow'
  return (
    <div>
      <PageHeader title="Insights" actions={<AskCFO q={`Review my ${TABS.find((t) => t.value === tab)?.label.toLowerCase() ?? 'finances'}: what stands out, what changed, and one or two concrete actions.`} />} />
      <Tabs value={tab} onChange={(v) => nav(`/insights/${v}`)} tabs={TABS} />
      <Routes>
        <Route path="/" element={<CashFlow />} />
        <Route path="/cashflow" element={<CashFlow />} />
        <Route path="/spending" element={<Spending />} />
        <Route path="/trends" element={<Trends />} />
        <Route path="/recurring" element={<RecurringView />} />
        <Route path="/fi" element={<FI />} />
        <Route path="/review" element={<Review />} />
      </Routes>
    </div>
  )
}

// ── cash flow ───────────────────────────────────────────────────────

function CashFlow() {
  const [span, setSpan] = useState('24')
  const from = addMonths(thisMonth(), -Number(span) + 1) + '-01'
  const { data: months, isLoading } = useCashflow(from, todayISO(), 'month')
  const { data: years } = useCashflow('', '', 'year')
  const rows = useMemo(() => (months ?? []).map((f) => ({ ...f, label: f.period, spendNeg: -f.spending, rate: f.income > 0 ? f.savings_rate * 100 : null })), [months])
  const complete = (months ?? []).filter((f) => f.period < thisMonth())
  const last12 = complete.slice(-12)
  const sum = (k: keyof Flow) => last12.reduce((a, f) => a + (f[k] as number), 0)
  const inc = sum('income')
  const sp = sum('spending')
  if (isLoading) return <Loading />
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Income · last 12 months" value={eur(inc)} sub={`${eur(inc / 12)}/month`} />
        <Stat label="Spending · last 12 months" value={eur(sp)} sub={`${eur(sp / 12)}/month`} />
        <Stat label="Savings rate" value={pct(inc ? (inc - sp) / inc : 0)} sub={`${eur((inc - sp) / 12)}/month saved`} tone={inc - sp < 0 ? 'bad' : 'good'} />
        <Stat label="Invested & principal" value={eur(sum('invested'))} sub={`incl. ${eur(sum('principal'))} mortgage principal`} />
      </div>
      <Card title="Income vs spending" action={<Segmented size="sm" value={span} onChange={setSpan} options={[{ value: '12', label: '12M' }, { value: '24', label: '24M' }, { value: '60', label: '5Y' }]} />}>
        <div className="h-64">
          <ResponsiveContainer>
            <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barGap={2} barCategoryGap="20%">
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="label" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (() => {
                const f = payload[0].payload as Flow
                return <TooltipBox title={monthLabel(f.period, true)} rows={[
                  { color: 'var(--s1)', label: 'Income', value: eurc(f.income) }, { color: 'var(--s2)', label: 'Spending', value: eurc(f.spending) },
                  { label: 'Essential', value: eurc(f.essential) }, { label: 'Saved', value: eurc(f.saved), bold: true },
                  { label: 'Savings rate', value: pct(f.savings_rate) }, { label: 'Invested', value: eurc(f.invested) }]} />
              })() : null} />
              <Bar dataKey="income" name="Income" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false} />
              <Bar dataKey="spending" name="Spending" fill="var(--s2)" radius={[4, 4, 0, 0]} isAnimationActive={false} />
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'Income' }, { color: 'var(--s2)', label: 'Spending' }]} /></div>
      </Card>
      <Card title="Savings rate by month">
        <div className="h-40">
          <ResponsiveContainer>
            <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="label" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={(v) => `${v}%`} width={40} domain={[-50, 100]} allowDataOverflow />
              <ReferenceLine y={0} stroke="var(--chart-axis)" />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? <TooltipBox title={monthLabel(payload[0].payload.period, true)} rows={[{ label: 'Savings rate', value: pct(payload[0].payload.savings_rate), bold: true }]} /> : null} />
              <Bar dataKey="rate" name="Savings rate" fill="var(--s3)" radius={[4, 4, 0, 0]} isAnimationActive={false} />
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-1 text-xs text-muted">Salary paid on the 30th sometimes lands on the 1st — read single months with that in mind; the 12-month figure above is the honest one.</div>
      </Card>
      <Card pad={false} title="Every year">
        <div className="overflow-x-auto">
          <table className="w-full text-sm tnum">
            <thead className="text-xs text-muted">
              <tr className="border-y border-line">
                {['Year', 'Income', 'Spending', 'Saved', 'Rate', 'Invested'].map((h) => <th key={h} className={clsx('px-4 py-2 font-medium', h === 'Year' ? 'text-left' : 'text-right')}>{h}</th>)}
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {[...(years ?? [])].reverse().filter((y) => y.income > 0 || y.spending > 1000).map((y) => (
                <tr key={y.period}>
                  <td className="px-4 py-2 font-medium">{y.period}</td>
                  <td className="px-4 py-2 text-right">{eur(y.income)}</td>
                  <td className="px-4 py-2 text-right">{eur(y.spending)}</td>
                  <td className={clsx('px-4 py-2 text-right', y.saved < 0 && 'text-bad')}>{eur(y.saved)}</td>
                  <td className="px-4 py-2 text-right">{y.income > 0 ? pct(y.savings_rate) : '—'}</td>
                  <td className="px-4 py-2 text-right">{eur(y.invested)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}

// ── spending ────────────────────────────────────────────────────────

const PRESETS = [{ value: 'month', label: 'This month' }, { value: 'last_month', label: 'Last month' }, { value: '3m', label: '3 months' }, { value: 'ytd', label: 'This year' }, { value: '12m', label: '12 months' }, { value: 'last_year', label: 'Last year' }]

function Spending() {
  const [preset, setPreset] = useState('12m')
  const [open, setOpen] = useState<string | null>(null)
  const cats = useCats()
  const editor = useTxEditor()
  const { data, isLoading } = useQuery({ queryKey: ['breakdown', preset], queryFn: () => api.get<any>('/insights/breakdown', { preset }) })
  if (isLoading || !data) return <Loading />
  const total = data.categories.reduce((a: number, c: any) => a + c.total, 0)
  const max = Math.max(...data.categories.map((c: any) => c.total), 1)
  return (
    <div className="space-y-4">
      <div className="no-scrollbar -mx-4 flex gap-1.5 overflow-x-auto px-4 sm:mx-0 sm:px-0">
        {PRESETS.map((p) => <button key={p.value} className={preset === p.value ? 'chip-on' : 'chip'} onClick={() => setPreset(p.value)}>{p.label}</button>)}
      </div>
      <PaceCard />
      <div className="text-sm text-muted">{shortDate(data.from)} – {shortDate(data.to)} · <b className="text-ink">{eur(total)}</b> spent, compared with the same length before</div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card pad={false} title="By category" className="lg:col-span-2">
          <div className="divide-y divide-line border-t border-line">
            {data.categories.map((c: any) => (
              <div key={c.category}>
                <button className="w-full px-4 py-2.5 text-left hover:bg-sunken/40" onClick={() => setOpen(open === c.category ? null : c.category)}>
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="flex items-center gap-2 text-sm font-medium"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(c.category) }} />{cats.name(c.category)}</span>
                    <span className="tnum text-sm"><b>{eur(c.total)}</b> <span className="text-xs text-muted">{pct(c.share)}</span></span>
                  </div>
                  <div className="mt-1.5 flex items-center gap-3">
                    <ShareBar value={c.total} max={max} />
                    <span className={clsx('w-16 shrink-0 text-right text-xs tnum', c.change > 0.1 ? 'text-bad' : c.change < -0.1 ? 'text-good' : 'text-muted')}>
                      {c.previous > 0 ? `${c.change > 0 ? '+' : ''}${pct(c.change)}` : 'new'}
                    </span>
                  </div>
                  <div className="mt-0.5 text-xs text-muted">{eur(c.monthly_avg)}/mo{c.top_merchant ? ` · mostly ${c.top_merchant}` : ''}</div>
                </button>
                {open === c.category && c.children?.length > 0 && (
                  <div className="bg-sunken/40 px-4 py-2">
                    {c.children.map((ch: any) => (
                      <a key={ch.category} href={`#/ledger?category=${ch.category}&from=${data.from}&to=${data.to}`} className="flex items-center justify-between py-1 text-sm">
                        <span className="text-ink2">{cats.name(ch.category)}</span>
                        <span className="tnum">{eur(ch.total)} <span className="text-xs text-muted">{ch.previous > 0 ? `${ch.change > 0 ? '+' : ''}${pct(ch.change)}` : ''}</span></span>
                      </a>
                    ))}
                    <a href={`#/ledger?category=${c.category}&from=${data.from}&to=${data.to}`} className="mt-1 inline-flex items-center gap-1 text-xs text-accent">Transactions<Icon name="chevronR" size={12} /></a>
                  </div>
                )}
              </div>
            ))}
          </div>
        </Card>
        <div className="space-y-4">
          <Card pad={false} title="Top merchants">
            <div className="divide-y divide-line border-t border-line">
              {data.merchants.slice(0, 12).map((m: any) => (
                <a key={m.name} href={`#/ledger?merchant=${encodeURIComponent(m.name)}&from=${data.from}&to=${data.to}`} className="flex items-center justify-between px-4 py-2 text-sm hover:bg-sunken/40">
                  <span className="min-w-0 truncate">{m.name} <span className="text-xs text-muted">×{m.count}</span></span>
                  <span className="tnum">{eur(m.amount)}</span>
                </a>
              ))}
            </div>
          </Card>
          <Card pad={false} title="Largest expenses">
            <div className="divide-y divide-line border-t border-line">
              {data.largest.slice(0, 8).map((t: Tx) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}

function PaceCard() {
  const { data } = useQuery({ queryKey: ['pace'], queryFn: () => api.get<{ day: number; current?: number; last_month: number; typical: number }[]>('/insights/pace') })
  if (!data) return null
  const today = data.filter((p) => p.current != null).pop()
  const ahead = today ? today.current! - today.typical : 0
  return (
    <Card title="This month so far" action={today && <span className={clsx('text-sm tnum', ahead > 0 ? 'text-bad' : 'text-good')}>{ahead > 0 ? `${eur(ahead)} ahead of` : `${eur(-ahead)} below`} a typical month</span>}>
      <div className="h-48">
        <ResponsiveContainer>
          <LineChart data={data} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="day" {...axisProps} interval={4} />
            <YAxis {...axisProps} tickFormatter={eurk} width={48} />
            <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
              <TooltipBox title={`Day ${label}`} rows={payload.filter((p: any) => p.value != null).map((p: any) => ({ color: p.stroke, label: p.name, value: eur(p.value) }))} />) : null} />
            <Line dataKey="typical" name="Typical (6-month avg)" stroke="var(--s-other)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Line dataKey="last_month" name="Last month" stroke="var(--s2)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Line dataKey="current" name="This month" stroke="var(--s1)" strokeWidth={2.5} dot={false} isAnimationActive={false} connectNulls={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'This month' }, { color: 'var(--s2)', label: 'Last month' }, { color: 'var(--s-other)', label: 'Typical (6-month avg)' }]} /></div>
    </Card>
  )
}

// ── trends ──────────────────────────────────────────────────────────

function Trends() {
  const [months, setMonths] = useState('24')
  const [parent, setParent] = useState('')
  const cats = useCats()
  const { data, isLoading } = useQuery({ queryKey: ['trends', months, parent], queryFn: () => api.get<any[]>('/insights/trends', { months, parent }) })
  const keys = useMemo(() => {
    const totals: Record<string, number> = {}
    for (const r of data ?? []) for (const [k, v] of Object.entries(r)) if (k !== 'month') totals[k] = (totals[k] ?? 0) + (v as number)
    return Object.entries(totals).sort((a, b) => b[1] - a[1]).map(([k]) => k)
  }, [data])
  // Top level: the seven core categories keep their own colour, the rest fold
  // into Other. Drill-down: one category, its leaves in slot order (≤7 + Other).
  const series = useMemo(() => {
    if (!parent) {
      const core = CORE.filter((c) => keys.includes(c))
      return { keys: core, other: keys.filter((k) => !CORE.includes(k)), color: (k: string) => catColor(k) }
    }
    const top = keys.slice(0, 7)
    return { keys: top, other: keys.slice(7), color: (k: string) => `var(--s${top.indexOf(k) + 1})` }
  }, [keys, parent])
  const rows = useMemo(() => (data ?? []).map((r) => {
    const row: any = { month: r.month }
    for (const k of series.keys) row[k] = r[k] ?? 0
    row.__other = series.other.reduce((a, k) => a + (r[k] ?? 0), 0)
    return row
  }), [data, series])
  if (isLoading) return <Loading />
  const all = [...series.keys, ...(series.other.length ? ['__other'] : [])]
  const label = (k: string) => (k === '__other' ? 'Other' : cats.name(k))
  const color = (k: string) => (k === '__other' ? 'var(--s-other)' : series.color(k))
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Segmented size="sm" value={months} onChange={setMonths} options={[{ value: '12', label: '12M' }, { value: '24', label: '24M' }, { value: '60', label: '5Y' }]} />
        <select className="input h-8 w-auto text-xs" value={parent} onChange={(e) => setParent(e.target.value)}>
          <option value="">All categories</option>
          {cats.tree.filter((c) => c.kind === 'expense').map((c) => <option key={c.id} value={c.id}>{c.name} breakdown</option>)}
        </select>
      </div>
      <Card title={parent ? `${cats.name(parent)} by subcategory` : 'Monthly spending by category'}>
        <div className="h-72">
          <ResponsiveContainer>
            <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="15%">
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="month" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload, label: l }) => active && payload?.length ? (
                <TooltipBox title={monthLabel(l as string, true)} rows={[...payload].reverse().filter((p: any) => p.value).map((p: any) => ({ color: p.fill, label: label(p.dataKey), value: eur(p.value) }))}
                  footer={<div className="flex justify-between"><span>Total</span><span className="tnum text-ink">{eur(payload.reduce((a: number, p: any) => a + (p.value || 0), 0))}</span></div>} />) : null} />
              {all.map((k, i) => (
                <Bar key={k} dataKey={k} stackId="s" fill={color(k)} stroke="var(--chart-surface)" strokeWidth={1} radius={i === all.length - 1 ? [4, 4, 0, 0] : 0} isAnimationActive={false} />
              ))}
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-3"><Legend items={all.map((k) => ({ color: color(k), label: label(k) }))} /></div>
      </Card>
    </div>
  )
}

// ── recurring ───────────────────────────────────────────────────────

function RecurringView() {
  const { data, isLoading } = useQuery({ queryKey: ['recurring'], queryFn: () => api.get<{ items: Recurring[]; monthly_total: number }>('/insights/recurring') })
  const cats = useCats()
  if (isLoading || !data) return <Loading />
  if (!data.items.length) return <Empty title="Nothing recurring found" />
  return (
    <div className="space-y-4">
      <Stat label="Recurring costs" value={`${eur(data.monthly_total)}/mo`} sub={`${eur(data.monthly_total * 12)} a year across ${data.items.length} merchants`} />
      <Card pad={false}>
        <div className="divide-y divide-line">
          {data.items.map((r) => (
            <a key={r.merchant} href={`#/ledger?period=365&merchant=${encodeURIComponent(r.merchant)}`} className="flex items-center gap-3 px-4 py-2.5 hover:bg-sunken/40">
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{r.merchant}{r.cadence === 'yearly' && <span className="ml-1.5 rounded-full bg-sunken px-1.5 text-[10px] text-ink2">yearly</span>}</div>
                <div className="truncate text-xs text-muted">{cats.path(r.category)} · next ~{shortDate(r.next)}</div>
              </div>
              <div className="text-right">
                <div className="tnum text-sm font-semibold">{eur(r.amount)}</div>
                {r.changed && <div className="text-xs text-warn tnum">last {eur(r.last_amount)}</div>}
              </div>
            </a>
          ))}
        </div>
      </Card>
    </div>
  )
}

// ── FI ──────────────────────────────────────────────────────────────

function FI() {
  const { data: f, isLoading } = useQuery({ queryKey: ['fi'], queryFn: () => api.get<any>('/insights/fi') })
  if (isLoading || !f) return <Loading />
  return (
    <div className="space-y-4">
      <section className="card p-4">
        <div className="text-sm text-ink2">Progress to financial independence</div>
        <div className="mt-1 flex items-baseline gap-3">
          <div className="text-3xl font-semibold">{pct(f.progress, 1)}</div>
          <div className="text-sm text-muted">{eur(f.investable)} of {eur(f.target)}</div>
        </div>
        <div className="mt-3 h-2 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(f.progress, 1) * 100}%` }} /></div>
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><div className="text-xs text-muted">At current pace</div><div className="font-semibold">{f.years_to_fi >= 0 ? `${f.years_to_fi} years` : 'not reached'}</div>{f.fi_date && <div className="text-xs text-muted">{monthLabel(f.fi_date, true)}{f.fi_age ? ` · age ${Math.round(f.fi_age)}` : ''}</div>}</div>
          <div><div className="text-xs text-muted">Saving now</div><div className="font-semibold">{eur(f.monthly_invested)}/mo</div></div>
          <div><div className="text-xs text-muted">Needed for target age {f.target_age || '—'}</div><div className="font-semibold">{f.required_monthly ? `${eur(f.required_monthly)}/mo` : 'set it in Plan'}</div></div>
          <div><div className="text-xs text-muted">Assumptions</div><div className="font-semibold">{f.withdrawal_rate}% draw · {f.expected_return}% real</div></div>
        </div>
        <div className="mt-2 text-xs text-muted">Target = {f.spend_source} ({eur(f.annual_spend)}/yr) ÷ {f.withdrawal_rate}%. Investable = brokers, pension, crypto and cash above the emergency fund — the house is not counted.</div>
      </section>
      <Card title="Projection">
        <div className="h-64">
          <ResponsiveContainer>
            <AreaChart data={f.projection} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="year" {...axisProps} />
              <YAxis {...axisProps} tickFormatter={eurk} width={52} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={label} rows={[{ color: 'var(--s1)', label: 'Investable', value: eur(payload[0].payload.investable), bold: true }, { label: 'Target', value: eur(payload[0].payload.target) }]} /> : null} />
              <ReferenceLine y={f.target} stroke="var(--s2)" label={{ value: 'FI target', fill: 'var(--chart-text)', fontSize: 11, position: 'insideTopLeft' }} />
              <Area dataKey="investable" stroke="var(--s1)" strokeWidth={2} fill="var(--s1)" fillOpacity={0.15} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </Card>
      <Card title="Emergency fund">
        <div className="grid grid-cols-3 gap-3 text-sm">
          <div><div className="text-xs text-muted">Cash</div><div className="font-semibold">{eur(f.emergency.cash)}</div></div>
          <div><div className="text-xs text-muted">Essential spend</div><div className="font-semibold">{eur(f.emergency.monthly_essential)}/mo</div></div>
          <div><div className="text-xs text-muted">Covers</div><div className={clsx('font-semibold', f.emergency.months < f.emergency.target_months ? 'text-warn' : 'text-good')}>{f.emergency.months} months</div><div className="text-xs text-muted">target {f.emergency.target_months}</div></div>
        </div>
      </Card>
    </div>
  )
}

// ── year review ─────────────────────────────────────────────────────

function Review() {
  const [year, setYear] = useState(todayISO().slice(0, 4))
  const { data: r, isLoading } = useQuery({ queryKey: ['review', year], queryFn: () => api.get<any>('/insights/review', { year }) })
  const cats = useCats()
  const editor = useTxEditor()
  if (isLoading || !r) return <Loading />
  const years = (r.years as Flow[]).filter((y) => y.income > 0).map((y) => y.period).reverse()
  const f = r.flow as Flow
  const p = r.previous as Flow
  const nwChange = r.net_worth_end - r.net_worth_start
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <select className="input h-9 w-auto" value={year} onChange={(e) => setYear(e.target.value)}>{years.map((y) => <option key={y}>{y}</option>)}</select>
        <span className="text-sm text-muted">vs {Number(year) - 1}</span>
      </div>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Income" value={eur(f.income)} sub={p.income ? `${f.income >= p.income ? '+' : ''}${pct(f.income / p.income - 1)} vs last year` : ''} />
        <Stat label="Spending" value={eur(f.spending)} sub={p.spending ? `${f.spending >= p.spending ? '+' : ''}${pct(f.spending / p.spending - 1)} vs last year` : ''} />
        <Stat label="Savings rate" value={pct(f.savings_rate)} sub={`last year ${pct(p.savings_rate)}`} tone={f.saved < 0 ? 'bad' : 'good'} />
        <Stat label="Net worth change" value={eur(nwChange)} sub={`${eur(r.net_worth_start)} → ${eur(r.net_worth_end)}`} tone={nwChange < 0 ? 'bad' : 'good'} />
      </div>
      <Card title="Month by month">
        <div className="h-52">
          <ResponsiveContainer>
            <LineChart data={r.months} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="period" {...axisProps} tickFormatter={(m) => monthLabel(m).slice(0, 3)} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={monthLabel(label as string, true)} rows={payload.map((x: any) => ({ color: x.stroke, label: x.name, value: eur(x.value) }))} /> : null} />
              <Line dataKey="income" name="Income" stroke="var(--s1)" strokeWidth={2} dot={{ r: 3 }} isAnimationActive={false} />
              <Line dataKey="spending" name="Spending" stroke="var(--s2)" strokeWidth={2} dot={{ r: 3 }} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'Income' }, { color: 'var(--s2)', label: 'Spending' }]} /></div>
      </Card>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card pad={false} title="Where it went">
          <div className="divide-y divide-line border-t border-line">
            {r.categories.slice(0, 10).map((c: any) => (
              <div key={c.category} className="flex items-center justify-between px-4 py-2 text-sm">
                <span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(c.category) }} />{cats.name(c.category)}</span>
                <span className="tnum">{eur(c.total)} <span className={clsx('text-xs', c.change > 0.1 ? 'text-bad' : c.change < -0.1 ? 'text-good' : 'text-muted')}>{c.previous ? `${c.change > 0 ? '+' : ''}${pct(c.change)}` : ''}</span></span>
              </div>
            ))}
          </div>
        </Card>
        <Card pad={false} title="Biggest expenses">
          <div className="divide-y divide-line border-t border-line">
            {r.largest.map((t: Tx) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
          </div>
        </Card>
      </div>
    </div>
  )
}
