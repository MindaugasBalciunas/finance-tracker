import { useState } from 'react'
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

interface TooltipPayloadItem {
  dataKey: string
  name: string
  value: number
  color: string
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipPayloadItem[]
  label?: number
  activeKey: string | null
  hiddenKeys: Set<string>
}

function CustomTooltip({ active, payload, label, activeKey, hiddenKeys }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0) return null
  const visible = payload.filter((p) => !hiddenKeys.has(p.dataKey))
  const items = activeKey ? visible.filter((p) => p.dataKey === activeKey) : visible
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
      <p className="font-semibold text-gray-700 mb-1">Day {label}</p>
      {items.map((p) => (
        <p key={p.dataKey} style={{ color: p.color }}>
          {p.name}: {formatEuro(p.value)}
        </p>
      ))}
    </div>
  )
}

interface LegendEntry {
  dataKey: string
  value: string
  color: string
}

interface CustomLegendProps {
  payload?: LegendEntry[]
  hiddenKeys: Set<string>
  latestValues: Record<string, number>
  onToggle: (key: string) => void
}

function CustomLegend({ payload, hiddenKeys, latestValues, onToggle }: CustomLegendProps) {
  if (!payload) return null
  return (
    <ul className="flex flex-col gap-1 text-xs pl-2 max-h-72 overflow-y-auto">
      {payload.map((entry) => {
        const hidden = hiddenKeys.has(entry.dataKey)
        const latest = latestValues[entry.dataKey]
        return (
          <li
            key={entry.dataKey}
            onClick={() => onToggle(entry.dataKey)}
            className="flex items-center gap-1.5 cursor-pointer select-none"
            style={{ opacity: hidden ? 0.35 : 1 }}
          >
            <span
              className="inline-block w-5 h-0.5 flex-shrink-0"
              style={{ backgroundColor: entry.color }}
            />
            <span className={hidden ? 'line-through text-gray-400' : 'text-gray-700'}>
              {entry.value}
              {latest != null && (
                <span className="ml-1 text-gray-400">({formatEuro(latest)})</span>
              )}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

export default function CumulativeSpendingChart({ transactions }: Props) {
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())

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
      void mk
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
        chartData[day - 1][mk] = undefined as unknown as number
        continue
      }
      running += byMonth[mk][day] ?? 0
      chartData[day - 1][mk] = running
    }
  }

  // Remove trailing all-zero rows
  const lastNonZero = chartData.reduce((last, row, i) => {
    const hasData = monthKeys.some((mk) => row[mk] > 0)
    return hasData ? i : last
  }, 0)
  const trimmed = chartData.slice(0, lastNonZero + 1)

  const monthLabel = (mk: string) => {
    const [year, month] = mk.split('-')
    return new Date(Number(year), Number(month) - 1).toLocaleString('default', { month: 'short', year: '2-digit' })
  }

  // Latest cumulative total per month (last defined value)
  const latestValues: Record<string, number> = {}
  for (const mk of monthKeys) {
    const lastDefined = [...trimmed].reverse().find((row) => row[mk] != null && row[mk] > 0)
    latestValues[mk] = lastDefined?.[mk] ?? 0
  }

  const toggleKey = (key: string) => {
    setHiddenKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const lineOpacity = (key: string) => {
    if (hiddenKeys.has(key)) return 0
    return activeKey === null || activeKey === key ? 1 : 0.15
  }

  return (
    <ResponsiveContainer width="100%" height={340}>
      <LineChart data={trimmed} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="day" tickFormatter={(v) => `Day ${v}`} tick={{ fontSize: 11 }} />
        <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} tick={{ fontSize: 11 }} width={52} />
        <Tooltip content={<CustomTooltip activeKey={activeKey} hiddenKeys={hiddenKeys} />} />
        <Legend
          layout="vertical"
          align="right"
          verticalAlign="middle"
          content={<CustomLegend hiddenKeys={hiddenKeys} latestValues={latestValues} onToggle={toggleKey} />}
        />
        {monthKeys.map((mk, i) => (
          <Line
            key={mk}
            type="monotone"
            dataKey={mk}
            name={monthLabel(mk)}
            stroke={MONTH_COLORS[i % MONTH_COLORS.length]}
            strokeWidth={activeKey === mk ? 3 : 2}
            strokeOpacity={lineOpacity(mk)}
            dot={false}
            connectNulls={false}
            hide={hiddenKeys.has(mk)}
            onMouseEnter={() => setActiveKey(mk)}
            onMouseLeave={() => setActiveKey(null)}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
