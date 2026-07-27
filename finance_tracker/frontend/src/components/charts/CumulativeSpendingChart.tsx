import { memo, useMemo, useState } from 'react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'
import { useIsMobile } from '../../hooks/useIsMobile'

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
  horizontal?: boolean
}

function CustomLegend({ payload, hiddenKeys, latestValues, onToggle, horizontal }: CustomLegendProps) {
  if (!payload) return null
  return (
    <ul
      className={
        horizontal
          ? 'flex flex-row flex-wrap justify-center gap-x-3 gap-y-1 text-xs pt-2'
          : 'flex flex-col gap-1 text-xs pl-2 max-h-72 overflow-y-auto'
      }
    >
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

// Past this many months, per-month cumulative lines are an unreadable
// tangle — switch to a day × month heat map of daily spending instead.
const HEATMAP_THRESHOLD = 12

function SpendingHeatmap({ byMonth, monthKeys }: { byMonth: Record<string, Record<number, number>>; monthKeys: string[] }) {
  // Color scale anchored at the 95th percentile so one huge day doesn't
  // wash out everything else.
  const daily: number[] = []
  for (const mk of monthKeys) {
    for (const v of Object.values(byMonth[mk])) daily.push(v)
  }
  const sorted = [...daily].sort((a, b) => a - b)
  const p95 = sorted[Math.floor(sorted.length * 0.95)] || 1

  const monthTotals: Record<string, number> = {}
  for (const mk of monthKeys) {
    monthTotals[mk] = Object.values(byMonth[mk]).reduce((s, v) => s + v, 0)
  }

  const cellColor = (v: number) => {
    if (v <= 0) return '#f3f4f6'
    const a = Math.min(1, 0.15 + (v / p95) * 0.85)
    return `rgba(220, 38, 38, ${a.toFixed(2)})`
  }

  const DAY_LABELS = new Set([1, 8, 15, 22, 29])

  return (
    <div>
      <div className="overflow-x-auto pb-1">
        <div className="inline-block">
          {/* Year markers above January columns; the first column is only
              labelled when no January follows soon enough to collide. */}
          <div className="flex ml-7">
            {monthKeys.map((mk, i) => (
              <div key={mk} className="w-3 text-[10px] text-gray-400 overflow-visible whitespace-nowrap">
                {(mk.endsWith('-01') || (i === 0 && !monthKeys.slice(0, 4).some((k) => k.endsWith('-01')))) ? mk.slice(0, 4) : ''}
              </div>
            ))}
          </div>
          {Array.from({ length: 31 }, (_, i) => i + 1).map((day) => (
            <div key={day} className="flex items-center">
              <div className="w-7 pr-1 text-right text-[9px] text-gray-400 leading-none">
                {DAY_LABELS.has(day) ? day : ''}
              </div>
              {monthKeys.map((mk) => {
                const v = byMonth[mk][day] ?? 0
                return (
                  <div
                    key={mk}
                    className="w-3 h-[7px] border border-white rounded-[1px]"
                    style={{ backgroundColor: cellColor(v) }}
                    title={`${mk}-${String(day).padStart(2, '0')}: ${formatEuro(v)}${monthTotals[mk] ? ` (month ${formatEuro(monthTotals[mk])})` : ''}`}
                  />
                )
              })}
            </div>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-1.5 mt-2 text-[10px] text-gray-400">
        <span>Daily spend:</span>
        <span>€0</span>
        {[0.15, 0.35, 0.6, 0.85, 1].map((a) => (
          <span key={a} className="w-3 h-2 rounded-[1px] inline-block" style={{ backgroundColor: `rgba(220,38,38,${a})` }} />
        ))}
        <span>{formatEuro(p95)}+</span>
        <span className="ml-2">· one column per month, one row per day of month</span>
      </div>
    </div>
  )
}

const CumulativeSpendingChart = ({ transactions }: Props) => {
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())
  // Side legend would halve the plot width on phones — stack it below instead.
  const isMobile = useIsMobile()

  const { monthKeys, byMonth, trimmed, latestValues } = useMemo(() => {
    // Group expenses by "YYYY-MM" month key
    const byMonth: Record<string, Record<number, number>> = {}

    for (const tx of transactions) {
      if (tx.type !== 'expense') continue
      const d = new Date(tx.date)
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
      const day = d.getDate()
      if (!byMonth[key]) byMonth[key] = {}
      byMonth[key][day] = (byMonth[key][day] ?? 0) + tx.amount.value
    }

    const monthKeys = Object.keys(byMonth).sort()

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

    // Latest cumulative total per month (last defined value)
    const latestValues: Record<string, number> = {}
    for (const mk of monthKeys) {
      const lastDefined = [...trimmed].reverse().find((row) => row[mk] != null && row[mk] > 0)
      latestValues[mk] = lastDefined?.[mk] ?? 0
    }

    return { monthKeys, byMonth, trimmed, latestValues }
  }, [transactions])

  if (monthKeys.length === 0) return null

  if (monthKeys.length > HEATMAP_THRESHOLD) {
    return <SpendingHeatmap byMonth={byMonth} monthKeys={monthKeys} />
  }

  const monthLabel = (mk: string) => {
    const [year, month] = mk.split('-')
    return new Date(Number(year), Number(month) - 1).toLocaleString('default', { month: 'short', year: '2-digit' })
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
          layout={isMobile ? 'horizontal' : 'vertical'}
          align={isMobile ? 'center' : 'right'}
          verticalAlign={isMobile ? 'bottom' : 'middle'}
          content={<CustomLegend hiddenKeys={hiddenKeys} latestValues={latestValues} onToggle={toggleKey} horizontal={isMobile} />}
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
            isAnimationActive={false}
            onMouseEnter={() => setActiveKey(mk)}
            onMouseLeave={() => setActiveKey(null)}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}

export default memo(CumulativeSpendingChart)
