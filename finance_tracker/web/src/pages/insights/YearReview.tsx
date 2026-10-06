import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { catColor, useCats } from '../../lib/categories'
import { eur, eurk, monthLabel, pct, todayISO } from '../../lib/format'
import type { Flow, Tx } from '../../lib/types'
import { Card, Loading, Stat } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { TxRow, useTxEditor } from '../../components/TxEditor'


// ── year review ─────────────────────────────────────────────────────

export function Review() {
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
        <select className="input select-pad h-9 w-auto" value={year} onChange={(e) => setYear(e.target.value)}>{years.map((y) => <option key={y}>{y}</option>)}</select>
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
