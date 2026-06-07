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

  const main = sorted.filter((a) => a.percentage >= 5)
  const small = sorted.filter((a) => a.percentage < 5)
  const data = small.length > 0
    ? [
        ...main,
        {
          account: 'Other',
          amount: small.reduce((sum, a) => sum + a.amount, 0),
          percentage: small.reduce((sum, a) => sum + a.percentage, 0),
        },
      ]
    : main

  return (
    <ResponsiveContainer width="100%" height={360}>
      <PieChart>
        <Pie
          data={data}
          cx="50%"
          cy="50%"
          outerRadius={110}
          dataKey="amount"
          nameKey="account"
          label={({ account, percentage, amount }) =>
            `${account} ${formatEuro(amount)} (${percentage.toFixed(1)}%)`
          }
          labelLine={true}
        >
          {data.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip formatter={(v: number) => formatEuro(v)} />
        <Legend />
      </PieChart>
    </ResponsiveContainer>
  )
}
