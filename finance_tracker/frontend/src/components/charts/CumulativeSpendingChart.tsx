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
// tangle — switch to a calendar of mini month charts instead.
const CALENDAR_THRESHOLD = 12

const MONTH_ABBR = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// One ribbon per year (recent first): each month's spending ramps up from
// zero inside its own slot, so the row reads as a skyline — peak height =
// the month's total, slope = how fast it was spent. All years share one
// vertical scale; month color compares its total to the period's average.
function SpendingCalendar({ byMonth, monthKeys }: { byMonth: Record<string, Record<number, number>>; monthKeys: string[] }) {
  const now = new Date()
  const currentKey = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`

  const totals: Record<string, number> = {}
  for (const mk of monthKeys) {
    totals[mk] = Object.values(byMonth[mk]).reduce((s, v) => s + v, 0)
  }
  const maxTotal = Math.max(...Object.values(totals), 1)
  const complete = monthKeys.filter((mk) => mk !== currentKey)
  const avgTotal = complete.length > 0
    ? complete.reduce((s, mk) => s + totals[mk], 0) / complete.length
    : 0

  const firstYear = parseInt(monthKeys[0].slice(0, 4), 10)
  const lastYear = parseInt(monthKeys[monthKeys.length - 1].slice(0, 4), 10)
  const years: number[] = []
  for (let y = lastYear; y >= firstYear; y--) years.push(y)

  // Row geometry: 12 month slots of 100 units, drawn edge to edge.
  const MW = 100
  const W = MW * 12
  const H = 52
  const PAD_TOP = 4

  const rampPath = (mk: string, slot: number): string | null => {
    const days = byMonth[mk]
    if (!days) return null
    const x0 = slot * MW
    let cum = 0
    let path = `M${x0},${H}`
    let lastX = x0
    for (let d = 1; d <= 31; d++) {
      if (mk === currentKey && d > now.getDate()) break
      cum += days[d] ?? 0
      lastX = x0 + (d / 31) * MW
      const y = H - (cum / maxTotal) * (H - PAD_TOP)
      path += ` L${lastX.toFixed(1)},${y.toFixed(1)}`
    }
    // Close straight down to the baseline: the ramp becomes a filled shape.
    path += ` L${lastX.toFixed(1)},${H} Z`
    return path
  }

  const colorFor = (mk: string) => {
    // The running month is judged against nothing — it isn't finished yet.
    if (mk === currentKey || avgTotal <= 0) return { line: '#6366f1', from: 'rgba(99,102,241,0.45)', to: 'rgba(99,102,241,0.08)' }
    const r = totals[mk] / avgTotal
    if (r < 0.85) return { line: '#10b981', from: 'rgba(16,185,129,0.45)', to: 'rgba(16,185,129,0.08)' }
    if (r <= 1.15) return { line: '#818cf8', from: 'rgba(129,140,248,0.40)', to: 'rgba(129,140,248,0.07)' }
    return { line: '#ef4444', from: 'rgba(239,68,68,0.50)', to: 'rgba(239,68,68,0.10)' }
  }

  const fmtShort = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(0)}k` : v.toFixed(0))

  const yearTotal = (year: number) =>
    MONTH_ABBR.reduce((s, _, i) => s + (totals[`${year}-${String(i + 1).padStart(2, '0')}`] ?? 0), 0)

  return (
    <div>
      {/* Shared month axis */}
      <div className="flex items-center gap-2 mb-0.5">
        <span className="w-9 shrink-0" />
        <div className="flex-1 grid grid-cols-12">
          {MONTH_ABBR.map((m) => (
            <span key={m} className="text-[9px] text-gray-400 text-center">
              <span className="hidden sm:inline">{m}</span>
              <span className="sm:hidden">{m[0]}</span>
            </span>
          ))}
        </div>
        <span className="w-11 shrink-0" />
      </div>

      <div className={years.length > 8 ? 'max-h-[30rem] overflow-y-auto pr-1' : ''}>
        {years.map((year) => (
          <div key={year} className="flex items-center gap-2 mb-1">
            <span className="w-9 shrink-0 text-[10px] font-semibold text-gray-500 text-right">{year}</span>
            <svg viewBox={`0 0 ${W} ${H}`} className="flex-1 h-12 block rounded bg-gray-50" preserveAspectRatio="none">
              <defs>
                {MONTH_ABBR.map((_, i) => {
                  const mk = `${year}-${String(i + 1).padStart(2, '0')}`
                  if (totals[mk] == null) return null
                  const { from, to } = colorFor(mk)
                  return (
                    <linearGradient key={mk} id={`ramp-${mk}`} x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={from} />
                      <stop offset="100%" stopColor={to} />
                    </linearGradient>
                  )
                })}
              </defs>
              {/* month slot separators */}
              {Array.from({ length: 11 }, (_, i) => (
                <line key={i} x1={(i + 1) * MW} y1={0} x2={(i + 1) * MW} y2={H} stroke="#e5e7eb" strokeWidth={1} vectorEffect="non-scaling-stroke" />
              ))}
              {/* average-month height guide */}
              {avgTotal > 0 && (
                <line x1={0} y1={H - (avgTotal / maxTotal) * (H - PAD_TOP)} x2={W} y2={H - (avgTotal / maxTotal) * (H - PAD_TOP)} stroke="#9ca3af" strokeWidth={1} strokeDasharray="3 4" vectorEffect="non-scaling-stroke" opacity={0.6} />
              )}
              {MONTH_ABBR.map((name, i) => {
                const mk = `${year}-${String(i + 1).padStart(2, '0')}`
                const d = rampPath(mk, i)
                if (!d) return null
                const { line } = colorFor(mk)
                return (
                  <g key={mk}>
                    <title>{`${name} ${year}: ${formatEuro(totals[mk])}${mk === currentKey ? ' (in progress)' : ''}`}</title>
                    <path d={d} fill={`url(#ramp-${mk})`} stroke={line} strokeWidth={1.5} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
                  </g>
                )
              })}
            </svg>
            <span className="w-11 shrink-0 text-[10px] text-gray-500 font-medium tabular-nums">{fmtShort(yearTotal(year))}</span>
          </div>
        ))}
      </div>
      <p className="mt-2 text-[10px] text-gray-400">
        One row per year, one hill per month: spending climbs from zero — the peak is the month's total, the slope is how fast it went.
        All years share one scale; the dashed line is your average month ({formatEuro(avgTotal)}).{' '}
        <span className="text-emerald-600 font-medium">Green</span> ≥15% below it, <span className="text-red-500 font-medium">red</span> ≥15% above.
        Hover a hill for the exact amount; year totals on the right.
      </p>
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

  if (monthKeys.length > CALENDAR_THRESHOLD) {
    return <SpendingCalendar byMonth={byMonth} monthKeys={monthKeys} />
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
