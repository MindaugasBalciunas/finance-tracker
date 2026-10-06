import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { usePeriod } from '../../lib/hooks'
import { catColor, CORE, useCats } from '../../lib/categories'
import { eur, eurk, monthLabel } from '../../lib/format'
import { Card, Loading, Segmented } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { SPANS, spanMonths } from './CashFlow'

// ── trends ──────────────────────────────────────────────────────────

export function Trends() {
  const [months, setMonths] = usePeriod('trends', '24', SPANS.map((s) => s.value))
  const [parent, setParent] = useState('')
  const cats = useCats()
  const n = String(spanMonths(months))
  const { data, isLoading } = useQuery({ queryKey: ['trends', n, parent], queryFn: () => api.get<any[]>('/insights/trends', { months: n, parent }) })
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
        <Segmented size="sm" value={months} onChange={setMonths} options={SPANS} />
        <select className="input select-pad h-8 w-auto text-xs" value={parent} onChange={(e) => setParent(e.target.value)}>
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
