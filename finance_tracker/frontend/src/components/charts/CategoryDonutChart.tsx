import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { CategorySummary } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  data: CategorySummary[]
  type: 'expense' | 'income' | 'investment'
}

const COLORS = [
  '#ef4444', '#f97316', '#f59e0b', '#10b981', '#3b82f6',
  '#8b5cf6', '#ec4899', '#14b8a6', '#6366f1', '#d97706',
]

export default function CategoryDonutChart({ data, type }: Props) {
  const filtered = data.filter((d) => d.type === type).sort((a, b) => b.total - a.total)

  if (filtered.length === 0) {
    return <p className="text-center text-gray-400 py-10 text-sm">No data</p>
  }

  const total = filtered.reduce((s, d) => s + d.total, 0)

  return (
    <ResponsiveContainer width="100%" height={320}>
      <PieChart>
        <Pie
          data={filtered}
          cx="50%"
          cy="50%"
          innerRadius={60}
          outerRadius={100}
          dataKey="total"
          nameKey="category"
          label={({ category, total: value, percent }) =>
            percent >= 0.05
              ? `${category} ${formatEuro(value)} (${(percent * 100).toFixed(1)}%)`
              : ''
          }
          labelLine={true}
        >
          {filtered.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip formatter={(v: number) => [formatEuro(v), `${((v / total) * 100).toFixed(1)}%`]} />
        <Legend formatter={(value) => <span className="text-xs">{value}</span>} />
      </PieChart>
    </ResponsiveContainer>
  )
}
