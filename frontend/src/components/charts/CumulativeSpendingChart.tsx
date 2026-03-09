import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'

interface Props {
  transactions: Transaction[]
}

const MONTH_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#ec4899', '#06b6d4', '#84cc16', '#f97316', '#6366f1',
]

export default function CumulativeSpendingChart({ transactions }: Props) {
  // Group expenses by "YYYY-MM" month key
  const byMonth: Record<string, Record<number, number>> = {}

  for (const tx of transactions) {
    if (tx.type !== 'expense') continue
    const d = new Date(tx.date)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
    const day = d.getDate()
    if (!byMonth[key]) byMonth[key] = {}
    byMonth[key][day] = (byMonth[key][day] ?? 0) + tx.amount
  }

  const monthKeys = Object.keys(byMonth).sort()
  if (monthKeys.length === 0) return null

  // Build chart rows: one per day 1-31 with cumulative sums per month
  const chartData = Array.from({ length: 31 }, (_, i) => {
    const day = i + 1
    const row: Record<string, number> = { day }
    for (const mk of monthKeys) {
      const prev = day > 1 ? (row[mk] ?? 0) : 0  // will be filled below
      void prev
      row[mk] = 0
    }
    return row
  })

  // Today's month key and day — used to cap the current month
  const now = new Date()
  const todayKey = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const todayDay = now.getDate()

  // Accumulate day by day per month; leave future days as undefined for current month
  for (const mk of monthKeys) {
    let running = 0
    for (let day = 1; day <= 31; day++) {
      if (mk === todayKey && day > todayDay) {
        // Don't set value — Recharts will render a gap / stop the line
        chartData[day - 1][mk] = undefined as unknown as number
        continue
      }
      running += byMonth[mk][day] ?? 0
      chartData[day - 1][mk] = running
    }
  }

  // Remove trailing all-zero rows (days beyond any month's last entry)
  const lastNonZero = chartData.reduce((last, row, i) => {
    const hasData = monthKeys.some((mk) => row[mk] > 0)
    return hasData ? i : last
  }, 0)
  const trimmed = chartData.slice(0, lastNonZero + 1)

  const monthLabel = (mk: string) => {
    const [year, month] = mk.split('-')
    return new Date(Number(year), Number(month) - 1).toLocaleString('default', { month: 'short', year: '2-digit' })
  }

  return (
    <ResponsiveContainer width="100%" height={320}>
      <LineChart data={trimmed} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis
          dataKey="day"
          tickFormatter={(v) => `Day ${v}`}
          tick={{ fontSize: 11 }}
        />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} tick={{ fontSize: 11 }} />
        <Tooltip formatter={(v: number) => formatEuro(v)} labelFormatter={(l) => `Day ${l}`} />
        <Legend />
        {monthKeys.map((mk, i) => (
          <Line
            key={mk}
            type="monotone"
            dataKey={mk}
            name={monthLabel(mk)}
            stroke={MONTH_COLORS[i % MONTH_COLORS.length]}
            strokeWidth={2}
            dot={false}
            connectNulls={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
