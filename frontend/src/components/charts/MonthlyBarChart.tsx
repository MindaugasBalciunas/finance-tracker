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
import type { MonthlySummary } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  data: MonthlySummary[]
}

export default function MonthlyBarChart({ data }: Props) {
  const sorted = [...data].sort((a, b) => a.year !== b.year ? a.year - b.year : a.month - b.month)
  const chartData = sorted.map((d) => ({
    name: `${d.month_name} ${d.year}`,
    Income: d.income,
    Expenses: d.expenses,
    Investments: d.investments,
  }))

  return (
    <ResponsiveContainer width="100%" height={320}>
      <BarChart data={chartData} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
        <Tooltip formatter={(v: number) => formatEuro(v)} />
        <Legend />
        <Bar dataKey="Income" fill="#10b981" radius={[4, 4, 0, 0]} />
        <Bar dataKey="Expenses" fill="#ef4444" radius={[4, 4, 0, 0]} />
        <Bar dataKey="Investments" fill="#3b82f6" radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}
