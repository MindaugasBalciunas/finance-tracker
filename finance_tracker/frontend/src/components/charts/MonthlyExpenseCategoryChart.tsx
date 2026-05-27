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
import type { Transaction, MonthlySummary } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  transactions: Transaction[]
  topN?: number
  monthTotals?: MonthlySummary[]
}

function fmt(v: number) {
  if (Math.abs(v) >= 1000) return `€${(v / 1000).toFixed(1)}k`
  return `€${v.toFixed(0)}`
}

function CustomTick({ x, y, payload, totalsMap, avg }: any) {
  const data: MonthlySummary | undefined = totalsMap?.[payload?.value]
  const aboveAvg = data && avg != null && data.expenses > avg
  const belowAvg = data && avg != null && data.expenses < avg
  const expColor = aboveAvg ? '#dc2626' : belowAvg ? '#16a34a' : '#374151'
  return (
    <g transform={`translate(${x},${y})`}>
      <text x={0} y={0} dy={12} textAnchor="middle" fill="#6b7280" fontSize={10}>{payload?.value}</text>
      {data && (
        <>
          <text x={0} y={0} dy={25} textAnchor="middle" fill="#15803d" fontSize={9}>{fmt(data.income)}</text>
          <text x={0} y={0} dy={36} textAnchor="middle" fill={expColor} fontSize={9}>{fmt(data.expenses)}</text>
          {data.investments > 0 && (
            <text x={0} y={0} dy={47} textAnchor="middle" fill="#1d4ed8" fontSize={9}>{fmt(data.investments)}</text>
          )}
        </>
      )}
    </g>
  )
}

const COLORS = [
  '#ef4444', '#f97316', '#eab308', '#84cc16',
  '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899', '#6b7280',
]

function buildChartData(transactions: Transaction[], topN: number) {
  // Group by month key
  const monthMap: Record<string, Record<string, number>> = {}

  for (const tx of transactions) {
    const d = new Date(tx.date)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
    if (!monthMap[key]) monthMap[key] = {}
    const cat = tx.category as string
    monthMap[key][cat] = (monthMap[key][cat] ?? 0) + tx.amount.value
  }

  // Find top N categories by all-time total
  const catTotals: Record<string, number> = {}
  for (const cats of Object.values(monthMap)) {
    for (const [cat, amt] of Object.entries(cats)) {
      catTotals[cat] = (catTotals[cat] ?? 0) + amt
    }
  }
  const topCats = Object.entries(catTotals)
    .sort((a, b) => b[1] - a[1])
    .slice(0, topN)
    .map(([cat]) => cat)

  const months = Object.keys(monthMap).sort()
  const rows = months.map((key) => {
    const [year, month] = key.split('-')
    const date = new Date(parseInt(year), parseInt(month) - 1)
    const label = date.toLocaleDateString('en', { month: 'short', year: '2-digit' })
    const row: Record<string, any> = { name: label }
    let other = 0
    for (const [cat, amt] of Object.entries(monthMap[key])) {
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

  return { rows, categories: allCats }
}

export default function MonthlyExpenseCategoryChart({ transactions, topN = 8, monthTotals }: Props) {
  const { rows, categories } = buildChartData(transactions, topN)

  // Build label -> MonthlySummary map using the same label format as buildChartData
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
  const tickHeight = monthTotals ? (hasInvestments ? 62 : 50) : 28

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
    <ResponsiveContainer width="100%" height={320 + (tickHeight - 28)}>
      <BarChart data={rows} margin={{ top: 5, right: 20, left: 0, bottom: tickHeight - 20 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis
          dataKey="name"
          interval={0}
          height={tickHeight}
          tick={monthTotals
            ? <CustomTick totalsMap={totalsMap} avg={avg} />
            : { fontSize: 11 } as any
          }
        />
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
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}
