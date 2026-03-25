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

interface Props {
  transactions: Transaction[]
  topN?: number
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

export default function MonthlyExpenseCategoryChart({ transactions, topN = 8 }: Props) {
  const { rows, categories } = buildChartData(transactions, topN)

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
    <ResponsiveContainer width="100%" height={320}>
      <BarChart data={rows} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
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
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}
