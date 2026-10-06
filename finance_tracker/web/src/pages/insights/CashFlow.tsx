import { useMemo } from 'react'
import clsx from 'clsx'
import { Bar, BarChart, CartesianGrid, Cell, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useCashflow, usePeriod } from '../../lib/hooks'
import { addMonths, eur, eurc, eurk, monthLabel, pct, thisMonth, todayISO } from '../../lib/format'
import type { Flow } from '../../lib/types'
import { Card, Loading, Segmented, Stat } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'


// ── cash flow ───────────────────────────────────────────────────────

/** Chart spans in months; YTD counts the months of this year so far. */
export const SPANS = [{ value: '3', label: '3M' }, { value: '6', label: '6M' }, { value: 'ytd', label: 'YTD' }, { value: '12', label: '12M' }, { value: '24', label: '24M' }, { value: '60', label: '5Y' }]
export const spanMonths = (v: string) => (v === 'ytd' ? new Date().getMonth() + 1 : Number(v))

export function CashFlow() {
  const [span, setSpan] = usePeriod('cashflow', '24', SPANS.map((s) => s.value))
  const from = addMonths(thisMonth(), -spanMonths(span) + 1) + '-01'
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
        <Stat icon="briefcase" color="var(--s6)" label="Income · 12 mo" value={eur(inc)} sub={`${eur(inc / 12)}/month`} />
        <Stat icon="bag" color="var(--s2)" label="Spending · 12 mo" value={eur(sp)} sub={`${eur(sp / 12)}/month`} />
        <Stat icon="piggy" color="var(--s3)" label="Savings rate" value={pct(inc ? (inc - sp) / inc : 0)} sub={`${eur((inc - sp) / 12)}/month saved`} tone={inc - sp < 0 ? 'bad' : 'good'} />
        <Stat icon="trend" color="var(--s7)" label="Invested · 12 mo" value={eur(sum('invested'))} sub={`incl. ${eur(sum('principal'))} loan principal`} />
      </div>
      <Card title="Income vs spending" action={<Segmented size="sm" value={span} onChange={setSpan} options={SPANS} />}>
        <div className="h-64">
          <ResponsiveContainer>
            <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barGap={2} barCategoryGap="20%">
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="label" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (() => {
                const f = payload[0].payload as Flow
                // Spending is net of refunds; show the gross and everything that left
                // the accounts too, so a heavy month reads as heavy.
                const refunds = f.refunds ?? 0
                return <TooltipBox title={monthLabel(f.period, true)} rows={[
                  { color: 'var(--s1)', label: 'Income', value: eurc(f.income) },
                  { color: 'var(--s2)', label: refunds ? 'Spending (net)' : 'Spending', value: eurc(f.spending) },
                  ...(refunds ? [{ label: <span className="pl-3 text-muted">gross, before refunds</span>, value: <span className="text-muted">{eurc(f.spending + refunds)}</span> },
                    { label: <span className="pl-3 text-muted">refunds &amp; gifts deducted</span>, value: <span className="text-muted">−{eurc(refunds)}</span> }] : []),
                  { label: 'Essential', value: eurc(f.essential) }, { label: 'Saved', value: eurc(f.saved), bold: true },
                  { label: 'Savings rate', value: pct(f.savings_rate) }, { label: 'Invested (incl. loan principal)', value: eurc(f.invested) },
                  { label: 'Total out', value: eurc(f.spending + refunds + f.invested), bold: true }]} />
              })() : null} />
              {/* The month in progress is drawn lighter: it is not comparable yet. */}
              <Bar dataKey="income" name="Income" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false}>
                {rows.map((r) => <Cell key={r.period} fillOpacity={r.period === thisMonth() ? 0.4 : 1} />)}
              </Bar>
              <Bar dataKey="spending" name="Spending" fill="var(--s2)" radius={[4, 4, 0, 0]} isAnimationActive={false}>
                {rows.map((r) => <Cell key={r.period} fillOpacity={r.period === thisMonth() ? 0.4 : 1} />)}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-2 flex flex-wrap items-center justify-between gap-2"><Legend items={[{ color: 'var(--s1)', label: 'Income' }, { color: 'var(--s2)', label: 'Spending' }]} />
          <span className="text-xs text-muted">Lighter bars: {monthLabel(thisMonth())} so far</span></div>
      </Card>
      <Card title="Savings rate by month">
        <div className="h-40">
          <ResponsiveContainer>
            <BarChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="label" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
              <YAxis {...axisProps} tickFormatter={(v) => `${v}%`} width={40} domain={[-50, 100]} ticks={[-50, 0, 50, 100]} allowDataOverflow />
              <ReferenceLine y={0} stroke="var(--chart-axis)" />
              <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? <TooltipBox title={monthLabel(payload[0].payload.period, true)} rows={[{ label: 'Savings rate', value: pct(payload[0].payload.savings_rate), bold: true }]} /> : null} />
              <Bar dataKey="rate" name="Savings rate" fill="var(--s3)" radius={[4, 4, 0, 0]} isAnimationActive={false}>
                {rows.map((r) => <Cell key={r.period} fillOpacity={r.period === thisMonth() ? 0.4 : 1} />)}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-1 text-xs text-muted">Salary paid on the 30th sometimes lands on the 1st — read single months with that in mind; the 12-month figure above is the honest one.</div>
      </Card>
      <Card pad={false} title="Every year">
        <div className="overflow-x-auto">
          <table className="w-full text-xs tnum sm:text-sm [&_td]:px-2 [&_td:first-child]:pl-4 [&_td:last-child]:pr-4 sm:[&_td]:px-4 [&_th]:px-2 [&_th:first-child]:pl-4 [&_th:last-child]:pr-4 sm:[&_th]:px-4">
            <thead className="text-xs text-muted">
              <tr className="border-y border-line">
                {['Year', 'Income', 'Spending', 'Saved', 'Rate', 'Invested'].map((h) => <th key={h} className={clsx('py-2 font-medium', h === 'Year' ? 'text-left' : 'text-right')}>{h}</th>)}
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {[...(years ?? [])].reverse().filter((y) => y.income > 0 || y.spending > 1000).map((y) => (
                <tr key={y.period}>
                  <td className="py-2 font-medium">{y.period}</td>
                  <td className="py-2 text-right">{eur(y.income)}</td>
                  <td className="py-2 text-right">{eur(y.spending)}</td>
                  <td className={clsx('py-2 text-right', y.saved < 0 && 'text-bad')}>{eur(y.saved)}</td>
                  <td className="py-2 text-right">{y.income > 0 ? pct(y.savings_rate) : '—'}</td>
                  <td className="py-2 text-right">{eur(y.invested)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
