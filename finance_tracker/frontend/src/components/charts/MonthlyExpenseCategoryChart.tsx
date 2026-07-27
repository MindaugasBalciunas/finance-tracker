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
  Customized,
} from 'recharts'
import type { Transaction, MonthlySummary } from '../../types'
import { formatEuro } from '../../utils/format'
import { useIsMobile } from '../../hooks/useIsMobile'
import { pickGranularity, bucketKey, bucketLabel, GRANULARITY_NOTE } from '../../utils/timeBuckets'

interface Props {
  transactions: Transaction[]
  topN?: number
  monthTotals?: MonthlySummary[]
}

function fmt(v: number) {
  if (Math.abs(v) >= 1000) return `€${(v / 1000).toFixed(1)}k`
  return `€${v.toFixed(0)}`
}

function TopLabels({ xAxisMap, totalsMap, avg, hasInvestments }: any) {
  const xAxis = xAxisMap?.[0]
  if (!xAxis?.scale) return null
  const { scale } = xAxis
  const bw: number = scale.bandwidth ? scale.bandwidth() : 0

  return (
    <g>
      {Object.keys(totalsMap).map((label) => {
        const x: number | undefined = scale(label)
        if (x == null) return null
        const cx = x + bw / 2
        const data: MonthlySummary = totalsMap[label]
        const aboveAvg = avg != null && data.expenses > avg
        const belowAvg = avg != null && data.expenses < avg
        const expColor = aboveAvg ? '#dc2626' : belowAvg ? '#16a34a' : '#374151'
        const lineH = 20
        return (
          <g key={label}>
            <text x={cx} y={lineH} textAnchor="middle">
              <tspan fill="#9ca3af" fontSize={10}>Inc </tspan>
              <tspan fill="#15803d" fontSize={14} fontWeight={700}>{fmt(data.income)}</tspan>
            </text>
            <text x={cx} y={lineH * 2} textAnchor="middle">
              <tspan fill="#9ca3af" fontSize={10}>Exp </tspan>
              <tspan fill={expColor} fontSize={14} fontWeight={700}>{fmt(data.expenses)}</tspan>
            </text>
            {hasInvestments && (
              <text x={cx} y={lineH * 3} textAnchor="middle">
                <tspan fill="#9ca3af" fontSize={10}>Inv </tspan>
                <tspan fill="#1d4ed8" fontSize={14} fontWeight={700}>{data.investments > 0 ? fmt(data.investments) : '—'}</tspan>
              </text>
            )}
          </g>
        )
      })}
    </g>
  )
}

const COLORS = [
  '#ef4444', '#f97316', '#eab308', '#84cc16',
  '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899', '#6b7280',
]

function buildChartData(transactions: Transaction[], topN: number) {
  // Long periods aggregate to quarters/years so the bars stay readable.
  const monthSet = new Set<string>()
  for (const tx of transactions) monthSet.add(tx.date.slice(0, 7))
  const granularity = pickGranularity(monthSet.size)

  const bucketMap: Record<string, Record<string, number>> = {}
  for (const tx of transactions) {
    const key = bucketKey(tx.date, granularity)
    if (!bucketMap[key]) bucketMap[key] = {}
    const cat = tx.category as string
    bucketMap[key][cat] = (bucketMap[key][cat] ?? 0) + tx.amount.value
  }

  const catTotals: Record<string, number> = {}
  for (const cats of Object.values(bucketMap)) {
    for (const [cat, amt] of Object.entries(cats)) {
      catTotals[cat] = (catTotals[cat] ?? 0) + amt
    }
  }
  const topCats = Object.entries(catTotals)
    .sort((a, b) => b[1] - a[1])
    .slice(0, topN)
    .map(([cat]) => cat)

  const buckets = Object.keys(bucketMap).sort()
  const rows = buckets.map((key) => {
    const row: Record<string, any> = { name: bucketLabel(key, granularity) }
    let other = 0
    for (const [cat, amt] of Object.entries(bucketMap[key])) {
      if (topCats.includes(cat)) {
        row[cat] = amt
      } else {
        other += amt
      }
    }
    if (other > 0) row['Other'] = other
    return row
  })

  const allCats = [...topCats]
  const hasOther = rows.some((r) => r['Other'])
  if (hasOther) allCats.push('Other')

  return { rows, categories: allCats, granularity }
}

const MonthlyExpenseCategoryChart = ({ transactions, topN = 8, monthTotals }: Props) => {
  const { rows, categories, granularity } = useMemo(() => buildChartData(transactions, topN), [transactions, topN])
  // The per-month Inc/Exp/Inv labels collide at phone widths — tooltip covers
  // it there. They are month-keyed, so they only apply at month granularity.
  const isMobile = useIsMobile()
  const showTotals = !!monthTotals && !isMobile && granularity === 'month'

  const { totalsMap, avg, hasInvestments } = useMemo(() => {
    const totalsMap: Record<string, MonthlySummary> = {}
    for (const m of monthTotals ?? []) {
      const d = new Date(m.year, m.month - 1)
      const label = d.toLocaleDateString('en', { month: 'short', year: '2-digit' })
      totalsMap[label] = m
    }
    const avg = monthTotals && monthTotals.length > 0
      ? monthTotals.reduce((s, m) => s + m.expenses, 0) / monthTotals.length
      : null
    const hasInvestments = (monthTotals ?? []).some((m) => m.investments > 0)
    return { totalsMap, avg, hasInvestments }
  }, [monthTotals])

  const topMargin = showTotals ? (hasInvestments ? 68 : 48) : 8

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

  if (rows.length === 0) return null

  return (
    <div>
      {GRANULARITY_NOTE[granularity] && (
        <p className="text-[11px] text-gray-400 mb-1">{GRANULARITY_NOTE[granularity]}</p>
      )}
      <ResponsiveContainer width="100%" height={320 + topMargin - 8}>
        <BarChart data={rows} margin={{ top: topMargin, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
        <Tooltip content={<CustomTooltip />} />
        <Legend wrapperStyle={{ fontSize: 11 }} />
        {categories.map((cat, i) => (
          <Bar
            key={cat}
            dataKey={cat}
            stackId="a"
            fill={COLORS[i % COLORS.length]}
            radius={i === categories.length - 1 ? [4, 4, 0, 0] : [0, 0, 0, 0]}
            isAnimationActive={false}
          />
        ))}
        {showTotals && (
          <Customized
            component={(props: any) => (
              <TopLabels {...props} totalsMap={totalsMap} avg={avg} hasInvestments={hasInvestments} />
            )}
          />
        )}
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}

export default memo(MonthlyExpenseCategoryChart)
