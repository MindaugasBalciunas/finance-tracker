import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { AccountAllocation } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  allocations: AccountAllocation[]
}

const COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#14b8a6', '#f97316', '#ec4899', '#6366f1', '#d97706',
  '#06b6d4', '#84cc16',
]

export default function AllocationPieChart({ allocations }: Props) {
  const sorted = [...allocations].sort((a, b) => b.amount - a.amount)

  return (
    <ResponsiveContainer width="100%" height={320}>
      <PieChart>
        <Pie
          data={sorted}
          cx="50%"
          cy="50%"
          outerRadius={110}
          dataKey="amount"
          nameKey="account"
          label={({ account, percentage }) => `${account} ${percentage.toFixed(1)}%`}
          labelLine={true}
        >
          {sorted.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip formatter={(v: number) => formatEuro(v)} />
        <Legend />
      </PieChart>
    </ResponsiveContainer>
  )
}
