import { memo, useMemo, useState } from 'react'
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { BalanceTrend } from '../../types'
import { formatEuro } from '../../utils/format'
import { MIN_VALID_BTC_PRICE } from '../../utils/btc'
import { useIsMobile } from '../../hooks/useIsMobile'

interface Props {
  trend: BalanceTrend
  btcPrice?: number | null
}

// Brand hues per institution — Swedbank orange, SEB green (pension a deep
// forest step of it), Revolut indigo, IBKR red, Bitcoin gold, Artea violet,
// Luminor blue, cash a neutral slate — with in-family lightness steps.
// Adjacent-pair separation validated for protanopia/deuteranopia and normal
// vision in the default stack order — don't nudge shades by eye.
const ACCOUNT_COLORS: Record<string, string> = {
  swed:        '#f97316',
  swed_etf:    '#c2410c',
  seb:         '#047857',
  seb_pen:     '#14532d',
  cash:        '#cbd5e1',
  rev_m:       '#312e81',
  rev_r:       '#a5b4fc',
  rev_stocks:  '#6366f1',
  ibkr_stocks: '#dc2626',
  art:         '#7c3aed',
  m_btc:       '#b45309',
  r_btc:       '#fbbf24',
  luminor:     '#0284c7',
}

const ACCOUNT_LABELS: Record<string, string> = {
  seb:         'SEB',
  swed:        'Swedbank',
  luminor:     'Luminor',
  cash:        'Cash',
  swed_etf:    'Swedbank ETF',
  seb_pen:     'SEB pension',
  art:         'Artea pension',
  rev_m:       'Revolut M',
  rev_r:       'Revolut R',
  r_btc:       'R BTC',
  m_btc:       'M BTC',
  rev_stocks:  'Rev stocks',
  ibkr_stocks: 'IBKR stocks',
}


function buildYTicks(maxVal: number): number[] {
  const candidates = [0, 1000, 5000, 10000, 25000, 50000, 75000, 100000, 125000, 150000, 175000, 200000, 250000, 300000, 400000, 500000]
  const ticks = candidates.filter((v) => v <= maxVal * 1.05)
  // Top out at the series' actual peak (rounded up to €100) so the highest
  // point reads off the axis instead of floating above the last round tick.
  const peak = Math.ceil(maxVal / 100) * 100
  if (peak > 0 && peak > (ticks[ticks.length - 1] ?? 0)) {
    ticks.push(peak)
  }
  return ticks
}

// Month-boundary ticks for the time axis, thinned to at most ~12 labels so
// multi-year histories stay readable.
function buildTimeTicks(minTs: number, maxTs: number): number[] {
  const ticks: number[] = []
  const d = new Date(minTs)
  d.setDate(1); d.setHours(0, 0, 0, 0)
  if (d.getTime() < minTs) d.setMonth(d.getMonth() + 1)
  while (d.getTime() <= maxTs) {
    ticks.push(d.getTime())
    d.setMonth(d.getMonth() + 1)
  }
  const step = Math.ceil(ticks.length / 12)
  return ticks.filter((_, i) => i % step === 0)
}

function formatTick(ts: number): string {
  return new Date(ts).toLocaleDateString('lt-LT', { year: '2-digit', month: 'short' })
}

interface TooltipProps {
  active?: boolean
  payload?: any[]
  label?: string
  hiddenKeys: Set<string>
}

function CustomTooltip({ active, payload, hiddenKeys }: TooltipProps) {
  if (!active || !payload?.length) return null
  const rawDate = payload[0]?.payload?.date as string | undefined
  const label = rawDate ? String(rawDate).slice(0, 10) : ''

  // Individual segment values are in payload[i].payload[dataKey], not payload[i].value (which is cumulative)
  const items = [...payload]
    .filter((p) => !hiddenKeys.has(p.dataKey))
    .map((p) => ({ name: p.name as string, value: p.payload[p.dataKey] as number, color: p.fill as string, dataKey: p.dataKey as string }))
    .filter((i) => i.value > 0)
    .reverse()

  const total = items.reduce((s, i) => s + i.value, 0)

  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs max-w-56">
      <p className="font-semibold text-gray-700 mb-1">{label}</p>
      {items.map((i) => (
        <div key={i.dataKey} className="flex justify-between gap-3">
          <span style={{ color: i.color }}>{i.name}</span>
          <span className="font-medium">{formatEuro(i.value)}</span>
        </div>
      ))}
      <div className="border-t border-gray-100 mt-1 pt-1 flex justify-between font-bold text-gray-800">
        <span>Total</span>
        <span>{formatEuro(total)}</span>
      </div>
    </div>
  )
}

interface LegendEntry { dataKey: string; value: string; color: string }

interface LegendProps {
  payload?: LegendEntry[]
  hiddenKeys: Set<string>
  latestValues: Record<string, number>
  onToggle: (key: string) => void
  horizontal?: boolean
}

function CustomLegend({ payload, hiddenKeys, latestValues, onToggle, horizontal }: LegendProps) {
  if (!payload) return null
  // Show top of stack first in legend
  const reversed = [...payload].reverse()
  return (
    <ul
      className={
        horizontal
          ? 'flex flex-row flex-wrap justify-center gap-x-3 gap-y-1 text-xs pt-2'
          : 'flex flex-col gap-1 text-xs pl-2 max-h-80 overflow-y-auto'
      }
    >
      {reversed.map((entry) => {
        const hidden = hiddenKeys.has(entry.dataKey)
        const latest = latestValues[entry.dataKey] ?? 0
        return (
          <li
            key={entry.dataKey}
            onClick={() => onToggle(entry.dataKey)}
            className="flex items-center gap-1.5 cursor-pointer select-none"
            style={{ opacity: hidden ? 0.3 : 1 }}
          >
            <span className="inline-block w-5 h-3 flex-shrink-0 rounded-sm" style={{ backgroundColor: entry.color }} />
            <span className={hidden ? 'line-through text-gray-400' : 'text-gray-700'}>
              {entry.value}
              {latest > 0 && <span className="ml-1 text-gray-400">({formatEuro(latest)})</span>}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

const BalanceTrendChart = ({ trend, btcPrice }: Props) => {
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())
  // On phones the side legend would eat half the plot width — stack it below.
  const isMobile = useIsMobile()

  const data = useMemo(() => trend.dates.map((date, i) => {
    const row: Record<string, number | string> = { date, ts: new Date(date).getTime() }
    Object.keys(trend.accounts).forEach((acc) => {
      let value = trend.accounts[acc][i] ?? 0
      if ((acc === 'r_btc' || acc === 'm_btc') && btcPrice && btcPrice >= MIN_VALID_BTC_PRICE) {
        if (value > 0 && value < 1) value = value * btcPrice
      }
      row[acc] = value
    })
    return row
  }).sort((a, b) => (a.ts as number) - (b.ts as number)), [trend, btcPrice])

  const activeAccounts = useMemo(() => Object.keys(trend.accounts).filter((acc) =>
    trend.accounts[acc].some((v) => v > 0)
  ), [trend])

  // Compute latest values first so we can sort by them
  const latestValues = useMemo(() => {
    const values: Record<string, number> = {}
    if (data.length > 0) {
      const last = data[data.length - 1]
      for (const acc of activeAccounts) {
        values[acc] = typeof last[acc] === 'number' ? (last[acc] as number) : 0
      }
    }
    return values
  }, [data, activeAccounts])

  // Stack order: a fixed, familiar floor to read the chart against — SEB
  // pension at the very bottom, then cash, IBKR stocks and Revolut stocks.
  // The rest sorts by stability (smallest average move between snapshots)
  // so jumpy day-to-day accounts ride on top where their wiggle doesn't
  // distort everything above them.
  const orderedAccounts = useMemo(() => {
    const pinned = ['seb_pen', 'cash', 'ibkr_stocks', 'rev_stocks']
    const volatility: Record<string, number> = {}
    for (const acc of activeAccounts) {
      let sum = 0
      let n = 0
      let prev: number | null = null
      for (const row of data) {
        const val = typeof row[acc] === 'number' ? (row[acc] as number) : 0
        if (prev != null) {
          sum += Math.abs(val - prev)
          n++
        }
        prev = val
      }
      volatility[acc] = n > 0 ? sum / n : 0
    }
    return [
      ...pinned.filter((a) => activeAccounts.includes(a)),
      ...activeAccounts
        .filter((a) => !pinned.includes(a))
        .sort((a, b) => (volatility[a] ?? 0) - (volatility[b] ?? 0)),
    ]
  }, [activeAccounts, data])

  const yTicks = useMemo(() => {
    const maxTotal = Math.max(...data.map((d) =>
      activeAccounts.reduce((s, acc) => s + (typeof d[acc] === 'number' ? (d[acc] as number) : 0), 0)
    ))
    return buildYTicks(maxTotal)
  }, [data, activeAccounts])

  const timeTicks = useMemo(() => (
    data.length > 0 ? buildTimeTicks(data[0].ts as number, data[data.length - 1].ts as number) : undefined
  ), [data])

  const toggleKey = (key: string) => {
    setHiddenKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

  return (
    <ResponsiveContainer width="100%" height={isMobile ? 480 : 400}>
      <AreaChart data={data} margin={{ top: 5, right: isMobile ? 8 : 20, left: isMobile ? 0 : 10, bottom: 5 }}>
        <defs>
          {orderedAccounts.map((acc) => {
            const color = ACCOUNT_COLORS[acc] ?? '#94a3b8'
            return (
              <linearGradient key={acc} id={`sg-${acc}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.95} />
                <stop offset="100%" stopColor={color} stopOpacity={0.80} />
              </linearGradient>
            )
          })}
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis
          dataKey="ts"
          type="number"
          scale="time"
          domain={['dataMin', 'dataMax']}
          ticks={timeTicks}
          tickFormatter={formatTick}
          tick={{ fontSize: 11 }}
        />
        <YAxis
          domain={[0, (dataMax: number) => Math.ceil(dataMax / 100) * 100]}
          ticks={yTicks}
          tickFormatter={(v) => v === 0 ? '€0' : `€${(v / 1000).toFixed(0)}k`}
          tick={{ fontSize: 11 }}
          width={52}
        />
        <Tooltip content={<CustomTooltip hiddenKeys={hiddenKeys} />} />
        <Legend
          layout={isMobile ? 'horizontal' : 'vertical'}
          align={isMobile ? 'center' : 'right'}
          verticalAlign={isMobile ? 'bottom' : 'middle'}
          content={
            <CustomLegend
              hiddenKeys={hiddenKeys}
              latestValues={latestValues}
              onToggle={toggleKey}
              horizontal={isMobile}
            />
          }
        />
        {orderedAccounts.map((acc) => (
          <Area
            key={acc}
            type="monotone"
            dataKey={acc}
            stackId="nw"
            isAnimationActive={false}
            stroke={ACCOUNT_COLORS[acc] ?? '#94a3b8'}
            strokeWidth={acc === 'swed' ? 2 : 0.5}
            strokeOpacity={acc === 'swed' ? 0.9 : 0.4}
            fill={`url(#sg-${acc})`}
            name={ACCOUNT_LABELS[acc] ?? acc}
            hide={hiddenKeys.has(acc)}
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  )
}

export default memo(BalanceTrendChart)
