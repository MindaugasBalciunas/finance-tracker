import {
  BarChart,
  Bar,
  Cell,
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
  const now = new Date()
  const sorted = [...data].sort((a, b) => a.year !== b.year ? a.year - b.year : a.month - b.month)
  const chartData = sorted.map((d) => {
    const partial = d.year === now.getFullYear() && d.month === now.getMonth() + 1
    return {
      name: partial ? `${d.month_name} ${d.year}*` : `${d.month_name} ${d.year}`,
      Income: d.income,
      Expenses: d.expenses,
      Investments: d.investments,
      partial,
    }
  })

  const CustomTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    const partial = payload[0]?.payload?.partial as boolean
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
        <p className="font-semibold text-gray-700 mb-1">{label}</p>
        {partial && <p className="text-amber-500 mb-1">* Month in progress</p>}
        {payload.map((p: any) => (
          <div key={p.dataKey} className="flex justify-between gap-3">
            <span style={{ color: p.fill }}>{p.dataKey}</span>
            <span className="font-medium">{formatEuro(p.value)}</span>
          </div>
        ))}
      </div>
    )
  }

  return (
    <ResponsiveContainer width="100%" height={320}>
      <BarChart data={chartData} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
        <Tooltip content={<CustomTooltip />} />
        <Legend />
        <Bar dataKey="Income" fill="#10b981" radius={[4, 4, 0, 0]}>
          {chartData.map((d, i) => (
            <Cell key={i} fill="#10b981" fillOpacity={d.partial ? 0.4 : 1} />
          ))}
        </Bar>
        <Bar dataKey="Expenses" fill="#ef4444" radius={[4, 4, 0, 0]}>
          {chartData.map((d, i) => (
            <Cell key={i} fill="#ef4444" fillOpacity={d.partial ? 0.4 : 1} />
          ))}
        </Bar>
        <Bar dataKey="Investments" fill="#3b82f6" radius={[4, 4, 0, 0]}>
          {chartData.map((d, i) => (
            <Cell key={i} fill="#3b82f6" fillOpacity={d.partial ? 0.4 : 1} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
