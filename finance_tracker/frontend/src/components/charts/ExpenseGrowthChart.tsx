import { memo, useMemo, useState } from 'react'
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'
import { txLabels, isCommitted } from '../../utils/labels'
import { useIsMobile } from '../../hooks/useIsMobile'

interface Props {
  transactions: Transaction[]
  mode: 'category' | 'label'
  topN?: number
}

// Palettes mirror the monthly stacked-bar charts (both rank series by total,
// so a category/label keeps its color between the "by month" and "growth"
// views).
const CATEGORY_COLORS = [
  '#ef4444', '#f97316', '#eab308', '#84cc16',
  '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899', '#6b7280',
]
const LABEL_COLORS = [
  '#6366f1', '#f97316', '#10b981', '#eab308',
  '#06b6d4', '#ec4899', '#8b5cf6', '#84cc16', '#3b82f6',
]
const OTHER_COLOR = '#9ca3af'
const UNLABELED_COLOR = '#d1d5db'

const DAY_MS = 86_400_000

// Snap a date string to its bucket start: the day itself, or the Monday of
// its week.
function bucketStart(date: string, granularity: 'day' | 'week'): number {
  const d = new Date(date.slice(0, 10))
  d.setHours(0, 0, 0, 0)
  if (granularity === 'week') {
    d.setDate(d.getDate() - ((d.getDay() + 6) % 7))
  }
  return d.getTime()
}

export type GrowthView = 'monthly' | 'cumulative'

// Running sum per group through the period, at daily resolution for short
// ranges and weekly beyond ~4 months — fine enough that salary-day bursts
// and quiet weeks show as real movement instead of one smooth monthly ramp.
// Empty buckets are kept so no-spend stretches read as flat plateaus. The
// 'monthly' view resets the sums at each month boundary: every month climbs
// from zero, so peak height = month total and shapes compare directly.
// Label mode follows the MonthlyLabelChart conventions: each transaction
// counts under its FIRST label (so the stack sums to real spend) and fixed
// obligations are excluded — their identical monthly steps would drown the
// variation the chart exists to show.
type Row = Record<string, number | string | null>

function buildSeries(transactions: Transaction[], mode: 'category' | 'label', topN: number, view: GrowthView) {
  const empty = { rows: [] as Row[], series: [] as string[], seriesTotals: {} as Record<string, number> }
  const relevant = transactions.filter((tx) => !(mode === 'label' && isCommitted(tx)))
  if (relevant.length === 0) return empty

  let minTs = Infinity
  let maxTs = -Infinity
  for (const tx of relevant) {
    const ts = bucketStart(tx.date, 'day')
    if (ts < minTs) minTs = ts
    if (ts > maxTs) maxTs = ts
  }
  const granularity: 'day' | 'week' = (maxTs - minTs) / DAY_MS <= 120 ? 'day' : 'week'

  const byBucket: Record<number, Record<string, number>> = {}
  for (const tx of relevant) {
    const ts = bucketStart(tx.date, granularity)
    const group = mode === 'category'
      ? (tx.category as string)
      : (txLabels(tx)[0] ?? 'unlabeled')
    if (!byBucket[ts]) byBucket[ts] = {}
    byBucket[ts][group] = (byBucket[ts][group] ?? 0) + tx.amount.value
  }

  const totals: Record<string, number> = {}
  for (const groups of Object.values(byBucket)) {
    for (const [g, amt] of Object.entries(groups)) {
      if (g === 'unlabeled') continue
      totals[g] = (totals[g] ?? 0) + amt
    }
  }
  const top = Object.entries(totals)
    .sort((a, b) => b[1] - a[1])
    .slice(0, topN)
    .map(([g]) => g)

  const otherName = mode === 'label' ? 'other labels' : 'Other'
  const allKeys = [...top, otherName, 'unlabeled']
  const running: Record<string, number> = {}
  const seriesTotals: Record<string, number> = {}
  let hasOther = false
  let hasUnlabeled = false
  const rows: Row[] = []
  const fillRow = (ts: number, value: number | null): Row => {
    const row: Row = { ts }
    for (const g of allKeys) row[g] = value
    return row
  }
  // Advance via setDate so bucket timestamps stay at local midnight across
  // DST switches — raw +86400000·n drifts an hour and stops matching the
  // byBucket keys.
  const cursor = new Date(minTs)
  if (granularity === 'week') cursor.setDate(cursor.getDate() - ((cursor.getDay() + 6) % 7))
  let prevMonth = ''
  while (cursor.getTime() <= maxTs) {
    const ts = cursor.getTime()
    const monthKey = `${cursor.getFullYear()}-${cursor.getMonth()}`
    cursor.setDate(cursor.getDate() + (granularity === 'day' ? 1 : 7))
    if (view === 'monthly' && monthKey !== prevMonth) {
      for (const k of Object.keys(running)) running[k] = 0
      // Clean cut between months: an all-null row breaks the area paths
      // (connectNulls is off), and a zero anchor makes the new month rise
      // from the baseline instead of inheriting the previous month's peak.
      if (prevMonth !== '') rows.push(fillRow(ts - 2, null))
      rows.push(fillRow(ts - 1, 0))
      prevMonth = monthKey
    }
    for (const [g, amt] of Object.entries(byBucket[ts] ?? {})) {
      const key = top.includes(g) ? g : g === 'unlabeled' ? 'unlabeled' : otherName
      running[key] = (running[key] ?? 0) + amt
      seriesTotals[key] = (seriesTotals[key] ?? 0) + amt
      if (key === otherName) hasOther = true
      if (key === 'unlabeled') hasUnlabeled = true
    }
    const row: Row = { ts }
    for (const g of allKeys) row[g] = running[g] ?? 0
    rows.push(row)
  }
  if (rows.length < 2) return empty

  // Biggest spender at the bottom of the stack; catch-all buckets on top.
  const series = [...top]
  if (hasOther) series.push(otherName)
  if (hasUnlabeled) series.push('unlabeled')
  return { rows, series, seriesTotals }
}

function dateLabel(ts: number): string {
  return new Date(ts).toLocaleDateString('en', { day: 'numeric', month: 'short', year: '2-digit' })
}

interface TooltipProps {
  active?: boolean
  payload?: any[]
  hiddenKeys: Set<string>
}

function CustomTooltip({ active, payload, hiddenKeys }: TooltipProps) {
  if (!active || !payload?.length) return null
  const ts = payload[0]?.payload?.ts as number | undefined
  // Segment values live in payload[i].payload[dataKey]; payload[i].value is
  // the stacked offset, not the series' own amount.
  const items = [...payload]
    .filter((p) => !hiddenKeys.has(p.dataKey))
    .map((p) => ({ name: p.name as string, value: p.payload[p.dataKey] as number, color: p.stroke as string, dataKey: p.dataKey as string }))
    .filter((i) => i.value > 0)
    .reverse()
  const total = items.reduce((s, i) => s + i.value, 0)
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs max-w-56">
      <p className="font-semibold text-gray-700 mb-1">Through {ts != null ? dateLabel(ts) : ''}</p>
      {items.map((i) => (
        <div key={i.dataKey} className="flex justify-between gap-3">
          <span style={{ color: i.color }}>{i.name}</span>
          <span className="font-medium">{formatEuro(i.value)}</span>
        </div>
      ))}
      <div className="border-t border-gray-100 mt-1 pt-1 flex justify-between font-bold text-gray-800">
        <span>Total</span>
        <span>{formatEuro(total)}</span>
      </div>
    </div>
  )
}

interface LegendEntry { dataKey: string; value: string; color: string }

interface LegendProps {
  payload?: LegendEntry[]
  hiddenKeys: Set<string>
  latestValues: Record<string, number>
  onToggle: (key: string) => void
  horizontal?: boolean
}

function CustomLegend({ payload, hiddenKeys, latestValues, onToggle, horizontal }: LegendProps) {
  if (!payload) return null
  // Top of stack first, matching how the areas read visually.
  const reversed = [...payload].reverse()
  return (
    <ul
      className={
        horizontal
          ? 'flex flex-row flex-wrap justify-center gap-x-3 gap-y-1 text-xs pt-2'
          : 'flex flex-col gap-1 text-xs pl-2 max-h-80 overflow-y-auto'
      }
    >
      {reversed.map((entry) => {
        const hidden = hiddenKeys.has(entry.dataKey)
        const latest = latestValues[entry.dataKey] ?? 0
        return (
          <li
            key={entry.dataKey}
            onClick={() => onToggle(entry.dataKey)}
            className="flex items-center gap-1.5 cursor-pointer select-none"
            style={{ opacity: hidden ? 0.3 : 1 }}
          >
            <span className="inline-block w-5 h-3 flex-shrink-0 rounded-sm" style={{ backgroundColor: entry.color }} />
            <span className={hidden ? 'line-through text-gray-400' : 'text-gray-700'}>
              {entry.value}
              {latest > 0 && <span className="ml-1 text-gray-400">({formatEuro(latest)})</span>}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

const ExpenseGrowthChart = ({ transactions, mode, topN = 8 }: Props) => {
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())
  const [view, setView] = useState<GrowthView>('monthly')
  // Side legend would halve the plot width on phones — stack it below.
  const isMobile = useIsMobile()

  const { rows, series, seriesTotals } = useMemo(
    () => buildSeries(transactions, mode, topN, view),
    [transactions, mode, topN, view]
  )

  // Bucket points thinned to at most ~12 axis labels; short ranges get
  // day-level labels, long ones month/year.
  const { timeTicks, tickFmt } = useMemo(() => {
    const all = rows.map((r) => r.ts as number)
    const step = Math.ceil(all.length / 12)
    const spanDays = all.length > 1 ? (all[all.length - 1] - all[0]) / 86_400_000 : 0
    const tickFmt = spanDays <= 200
      ? (ts: number) => new Date(ts).toLocaleDateString('lt-LT', { month: 'short', day: 'numeric' })
      : (ts: number) => new Date(ts).toLocaleDateString('lt-LT', { year: '2-digit', month: 'short' })
    return { timeTicks: all.filter((_, i) => i % step === 0), tickFmt }
  }, [rows])

  if (rows.length === 0) return null

  const palette = mode === 'category' ? CATEGORY_COLORS : LABEL_COLORS
  const colorFor = (name: string, i: number) => {
    if (name === 'other labels' || name === 'Other') return OTHER_COLOR
    if (name === 'unlabeled') return UNLABELED_COLOR
    return palette[i % palette.length]
  }

  const toggleKey = (key: string) => {
    setHiddenKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

  return (
    <div>
      <div className="flex justify-end mb-1">
        <div className="flex items-center gap-0.5 bg-gray-100 rounded-lg p-0.5">
          {(['monthly', 'cumulative'] as GrowthView[]).map((v) => (
            <button
              key={v}
              onClick={() => setView(v)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md capitalize ${view === v ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700'}`}
              title={v === 'monthly' ? 'Restart at zero each month — compare month shapes' : 'Keep growing through the whole period'}
            >
              {v === 'monthly' ? 'Monthly reset' : 'Cumulative'}
            </button>
          ))}
        </div>
      </div>
      <ResponsiveContainer width="100%" height={isMobile ? 460 : 380}>
      <AreaChart data={rows} margin={{ top: 5, right: isMobile ? 8 : 20, left: isMobile ? 0 : 10, bottom: 5 }}>
        <defs>
          {series.map((name, i) => {
            const color = colorFor(name, i)
            return (
              <linearGradient key={name} id={`eg-${mode}-${i}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.9} />
                <stop offset="100%" stopColor={color} stopOpacity={0.75} />
              </linearGradient>
            )
          })}
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis
          dataKey="ts"
          type="number"
          scale="time"
          domain={['dataMin', 'dataMax']}
          ticks={timeTicks}
          tickFormatter={tickFmt}
          tick={{ fontSize: 11 }}
        />
        <YAxis
          tickFormatter={(v) => (v === 0 ? '€0' : `€${(v / 1000).toFixed(v < 10000 ? 1 : 0)}k`)}
          tick={{ fontSize: 11 }}
          width={52}
        />
        <Tooltip content={<CustomTooltip hiddenKeys={hiddenKeys} />} />
        <Legend
          layout={isMobile ? 'horizontal' : 'vertical'}
          align={isMobile ? 'center' : 'right'}
          verticalAlign={isMobile ? 'bottom' : 'middle'}
          content={
            <CustomLegend
              hiddenKeys={hiddenKeys}
              latestValues={seriesTotals}
              onToggle={toggleKey}
              horizontal={isMobile}
            />
          }
        />
        {series.map((name, i) => (
          <Area
            key={name}
            // Linear keeps the monthly sawtooth's drops crisp; monotone
            // smoothing suits the ever-growing cumulative curve.
            type={view === 'monthly' ? 'linear' : 'monotone'}
            dataKey={name}
            stackId="growth"
            connectNulls={false}
            isAnimationActive={false}
            stroke={colorFor(name, i)}
            strokeWidth={0.5}
            strokeOpacity={0.5}
            fill={`url(#eg-${mode}-${i})`}
            name={name}
            hide={hiddenKeys.has(name)}
          />
        ))}
      </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

export default memo(ExpenseGrowthChart)
