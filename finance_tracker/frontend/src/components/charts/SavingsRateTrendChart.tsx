import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  ResponsiveContainer,
  Legend,
} from 'recharts'
import type { MonthlySummary } from '../../types'

interface Props {
  data: MonthlySummary[]
}

function isCurrentMonth(d: MonthlySummary): boolean {
  const now = new Date()
  return d.year === now.getFullYear() && d.month === now.getMonth() + 1
}

export default function SavingsRateTrendChart({ data }: Props) {
  const sorted = [...data]
    .filter(d => !isCurrentMonth(d)) // exclude in-progress month — distorts scale before salary drops
    .sort((a, b) => a.year !== b.year ? a.year - b.year : a.month - b.month)

  const chartData = sorted.map((d) => ({
    name: `${d.month_name} '${String(d.year).slice(2)}`,
    rate: d.income > 0 ? ((d.income - d.expenses) / d.income) * 100 : 0,
  }))

  const CustomTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    const rate = payload[0].value as number
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
        <p className="font-semibold text-gray-700 mb-1">{label}</p>
        <p className={rate >= 20 ? 'text-green-600' : rate >= 0 ? 'text-yellow-600' : 'text-red-600'}>
          Savings rate: <strong>{rate.toFixed(1)}%</strong>
        </p>
        <p className="text-gray-400 mt-0.5">{rate >= 20 ? 'Above target ✓' : rate >= 0 ? 'Below 20% target' : 'Deficit month'}</p>
      </div>
    )
  }

  return (
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={chartData} margin={{ top: 10, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis
          tickFormatter={(v) => `${v.toFixed(0)}%`}
          tick={{ fontSize: 11 }}
          domain={['auto', 'auto']}
        />
        <Tooltip content={<CustomTooltip />} />
        <Legend formatter={() => 'Savings Rate'} />
        <ReferenceLine y={20} stroke="#10b981" strokeDasharray="6 3" strokeWidth={1.5} label={{ value: '20% target', position: 'insideTopRight', fontSize: 10, fill: '#10b981' }} />
        <ReferenceLine y={0} stroke="#ef4444" strokeDasharray="3 3" strokeWidth={1} />
        <Line
          type="monotone"
          dataKey="rate"
          stroke="#3b82f6"
          strokeWidth={2}
          dot={{ r: 3, fill: '#3b82f6' }}
          activeDot={{ r: 5 }}
          name="Savings Rate"
        />
      </LineChart>
    </ResponsiveContainer>
  )
}
