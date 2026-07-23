import { memo } from 'react'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import type { CategorySummary } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  data: CategorySummary[]
  type: 'expense' | 'income' | 'investment'
  onSelect?: (category: CategorySummary['category']) => void
}

const COLORS = [
  '#ef4444', '#f97316', '#f59e0b', '#10b981', '#3b82f6',
  '#8b5cf6', '#ec4899', '#14b8a6', '#6366f1', '#d97706',
]

function CustomTooltip({ active, payload }: any) {
  if (!active || !payload?.length) return null
  const d = payload[0].payload
  const pct: number = payload[0].payload.__pct ?? 0
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
      <p className="font-semibold text-gray-800 mb-1">{d.category}</p>
      <p className="text-gray-700">{formatEuro(d.total)}</p>
      <p className="text-gray-500">{pct.toFixed(1)}%</p>
    </div>
  )
}

const CategoryDonutChart = ({ data, type, onSelect }: Props) => {
  const filtered = data.filter((d) => d.type === type).sort((a, b) => b.total - a.total)

  if (filtered.length === 0) {
    return <p className="text-center text-gray-400 py-10 text-sm">No data</p>
  }

  const total = filtered.reduce((s, d) => s + d.total, 0)
  const chartData = filtered.map((d) => ({ ...d, __pct: total > 0 ? (d.total / total) * 100 : 0 }))

  return (
    <div>
      <ResponsiveContainer width="100%" height={300}>
        <PieChart>
          <Pie
            data={chartData}
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
            onClick={onSelect ? (d: any) => onSelect(d.category) : undefined}
            style={onSelect ? { cursor: 'pointer' } : undefined}
          >
            {chartData.map((_, i) => (
              <Cell key={i} fill={COLORS[i % COLORS.length]} />
            ))}
          </Pie>
          <Tooltip content={<CustomTooltip />} />
        </PieChart>
      </ResponsiveContainer>

      <table className="w-full text-xs mt-3 border-collapse">
        <thead>
          <tr className="text-gray-400 border-b border-gray-100">
            <th className="text-left font-medium py-1.5 pr-2">Category</th>
            <th className="text-right font-medium py-1.5 pr-2">Amount</th>
            <th className="text-right font-medium py-1.5">%</th>
          </tr>
        </thead>
        <tbody>
          {chartData.map((d, i) => (
            <tr
              key={d.category}
              className={`border-b border-gray-50 hover:bg-gray-50 ${onSelect ? 'cursor-pointer' : ''}`}
              onClick={onSelect ? () => onSelect(d.category) : undefined}
            >
              <td className="py-1.5 pr-2">
                <span className="inline-flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm flex-shrink-0" style={{ backgroundColor: COLORS[i % COLORS.length] }} />
                  {d.category}
                </span>
              </td>
              <td className="text-right py-1.5 pr-2 font-medium text-gray-700">{formatEuro(d.total)}</td>
              <td className="text-right py-1.5 text-gray-500">{d.__pct.toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default memo(CategoryDonutChart)
