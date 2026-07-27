import { memo, useMemo } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'
import { txLabels, isCommitted } from '../../utils/labels'
import { pickGranularity, bucketKey, bucketLabel, GRANULARITY_NOTE } from '../../utils/timeBuckets'

interface Props {
  transactions: Transaction[]
  topN?: number
}

const COLORS = [
  '#6366f1', '#f97316', '#10b981', '#eab308',
  '#06b6d4', '#ec4899', '#8b5cf6', '#84cc16', '#3b82f6',
]
const OTHER_COLOR = '#9ca3af'
const UNLABELED_COLOR = '#d1d5db'

// Attributes each transaction to its FIRST label so the stacked bars sum to
// the real monthly spend — counting a multi-label transaction once per label
// would inflate the totals. Fixed obligations are excluded: they repeat at
// the same size every month and would compress the interesting variation.
function buildChartData(transactions: Transaction[], topN: number) {
  // Long periods aggregate to quarters/years so the bars stay readable.
  const monthSet = new Set<string>()
  for (const tx of transactions) {
    if (!isCommitted(tx)) monthSet.add(tx.date.slice(0, 7))
  }
  const granularity = pickGranularity(monthSet.size)

  const bucketMap: Record<string, Record<string, number>> = {}
  for (const tx of transactions) {
    if (isCommitted(tx)) continue
    const key = bucketKey(tx.date, granularity)
    if (!bucketMap[key]) bucketMap[key] = {}
    const labels = txLabels(tx)
    const bucket = labels.length > 0 ? labels[0] : 'unlabeled'
    bucketMap[key][bucket] = (bucketMap[key][bucket] ?? 0) + tx.amount.value
  }

  const labelTotals: Record<string, number> = {}
  for (const labels of Object.values(bucketMap)) {
    for (const [l, amt] of Object.entries(labels)) {
      if (l === 'unlabeled') continue
      labelTotals[l] = (labelTotals[l] ?? 0) + amt
    }
  }
  const topLabels = Object.entries(labelTotals)
    .sort((a, b) => b[1] - a[1])
    .slice(0, topN)
    .map(([l]) => l)

  const buckets = Object.keys(bucketMap).sort()
  const rows = buckets.map((key) => {
    const row: Record<string, any> = { name: bucketLabel(key, granularity) }
    let other = 0
    for (const [l, amt] of Object.entries(bucketMap[key])) {
      if (topLabels.includes(l)) row[l] = amt
      else if (l === 'unlabeled') row['unlabeled'] = amt
      else other += amt
    }
    if (other > 0) row['other labels'] = other
    return row
  })

  const series = [...topLabels]
  if (rows.some((r) => r['other labels'])) series.push('other labels')
  if (rows.some((r) => r['unlabeled'])) series.push('unlabeled')

  return { rows, series, granularity }
}

const MonthlyLabelChart = ({ transactions, topN = 8 }: Props) => {
  const { rows, series, granularity } = useMemo(() => buildChartData(transactions, topN), [transactions, topN])

  const CustomTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    const total = payload.reduce((s: number, p: any) => s + (p.value ?? 0), 0)
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs max-w-48">
        <p className="font-semibold text-gray-700 mb-1">{label}</p>
        {[...payload].reverse().map((p: any) => (
          <div key={p.name} className="flex justify-between gap-3">
            <span style={{ color: p.fill }}>{p.name}</span>
            <span className="font-medium">{formatEuro(p.value)}</span>
          </div>
        ))}
        <div className="border-t border-gray-100 mt-1 pt-1 flex justify-between font-semibold">
          <span className="text-gray-600">Total</span>
          <span>{formatEuro(total)}</span>
        </div>
      </div>
    )
  }

  if (rows.length < 2) return null

  const colorFor = (name: string, i: number) => {
    if (name === 'other labels') return OTHER_COLOR
    if (name === 'unlabeled') return UNLABELED_COLOR
    return COLORS[i % COLORS.length]
  }

  return (
    <div>
      {GRANULARITY_NOTE[granularity] && (
        <p className="text-[11px] text-gray-400 mb-1">{GRANULARITY_NOTE[granularity]}</p>
      )}
      <ResponsiveContainer width="100%" height={320}>
        <BarChart data={rows} margin={{ top: 8, right: 20, left: 0, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis dataKey="name" tick={{ fontSize: 11 }} />
          <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
          <Tooltip content={<CustomTooltip />} />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {series.map((name, i) => (
            <Bar
              key={name}
              dataKey={name}
              stackId="a"
              fill={colorFor(name, i)}
              radius={i === series.length - 1 ? [4, 4, 0, 0] : [0, 0, 0, 0]}
              isAnimationActive={false}
            />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}

export default memo(MonthlyLabelChart)
