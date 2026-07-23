import { memo } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  Cell,
  ResponsiveContainer,
  Legend,
} from 'recharts'
import type { MonthlySummary } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  data: MonthlySummary[]
}

const NetCashFlowChart = ({ data }: Props) => {
  const sorted = [...data].sort((a, b) => a.year !== b.year ? a.year - b.year : a.month - b.month)

  const chartData = sorted.map((d) => ({
    name: `${d.month_name} '${String(d.year).slice(2)}`,
    flow: d.income - d.expenses,
    income: d.income,
    expenses: d.expenses,
  }))

  const CustomTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    const { flow, income, expenses } = payload[0].payload
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs space-y-1">
        <p className="font-semibold text-gray-700">{label}</p>
        <p className="text-green-600">Income: <strong>{formatEuro(income)}</strong></p>
        <p className="text-red-500">Expenses: <strong>{formatEuro(expenses)}</strong></p>
        <p className={`border-t border-gray-100 pt-1 font-semibold ${flow >= 0 ? 'text-green-600' : 'text-red-600'}`}>
          Net: {flow >= 0 ? '+' : ''}{formatEuro(flow)}
        </p>
      </div>
    )
  }

  return (
    <ResponsiveContainer width="100%" height={260}>
      <BarChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} />
        <Tooltip content={<CustomTooltip />} />
        <Legend formatter={() => 'Net Cash Flow (Income − Expenses)'} />
        <ReferenceLine y={0} stroke="#9ca3af" strokeWidth={1} />
        <Bar dataKey="flow" radius={[4, 4, 0, 0]} name="Net Cash Flow">
          {chartData.map((entry, i) => (
            <Cell key={i} fill={entry.flow >= 0 ? '#10b981' : '#ef4444'} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

export default memo(NetCashFlowChart)
