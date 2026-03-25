import { useState } from 'react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
} from 'recharts'
import type { BalanceTrend } from '../../types'
import { formatEuro } from '../../utils/format'
import { MIN_VALID_BTC_PRICE } from '../../utils/btc'

interface Props {
  trend: BalanceTrend
  btcPrice?: number | null
}

const ACCOUNT_COLORS: Record<string, string> = {
  seb:        '#3b82f6',
  swed:       '#60a5fa',
  luminor:    '#93c5fd',
  cash:       '#bfdbfe',
  swed_etf:   '#7c3aed',
  seb_pen:   '#a78bfa',
  art:        '#c4b5fd',
  rev_m:      '#f97316',
  rev_r:      '#fb923c',
  r_btc:      '#fbbf24',
  m_btc:      '#fcd34d',
  rev_stocks: '#10b981',
}

const ACCOUNT_LABELS: Record<string, string> = {
  seb:        'Seb',
  swed:       'Swedbank',
  luminor:    'Luminor',
  cash:       'Cash',
  swed_etf:   'Swedbank ETF',
  seb_pen:   'SEB 2nd pillar pension',
  art:        'Artea 3rd pillar pension',
  rev_m:      'Revolut M account',
  rev_r:      'Revolut R account',
  r_btc:      'Revolut R account BTC',
  m_btc:      'Revolut M account BTC',
  rev_stocks: 'Revolut M account stocks',
}

const ACCOUNT_DASH: Record<string, string> = {
  seb:        '0',
  swed:       '0',
  luminor:    '0',
  cash:       '0',
  swed_etf:   '6 2',
  seb_pen:   '6 2',
  art:        '6 2',
  rev_m:      '3 3',
  rev_r:      '3 3',
  r_btc:      '3 3',
  m_btc:      '3 3',
  rev_stocks: '3 3',
}

function buildYTicks(maxVal: number): number[] {
  const candidates = [0, 1000, 5000, 10000, 25000, 50000, 75000, 100000, 125000, 150000, 175000, 200000, 250000, 300000, 400000, 500000]
  return candidates.filter((v) => v <= maxVal * 1.05)
}

interface TooltipPayloadItem {
  dataKey: string
  name: string
  value: number
  color: string
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipPayloadItem[]
  label?: string
  activeKey: string | null
  hiddenKeys: Set<string>
}

function CustomTooltip({ active, payload, label, activeKey, hiddenKeys }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0) return null
  const visible = payload.filter((p) => !hiddenKeys.has(p.dataKey))
  const items = activeKey ? visible.filter((p) => p.dataKey === activeKey) : visible
  return (
    <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
      <p className="font-semibold text-gray-700 mb-1">{label}</p>
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
    <ul className="flex flex-col gap-1 text-xs pl-2 max-h-80 overflow-y-auto">
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

export default function BalanceTrendChart({ trend, btcPrice }: Props) {
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [hiddenKeys, setHiddenKeys] = useState<Set<string>>(new Set())

  const data = trend.dates.map((date, i) => {
    const row: Record<string, number | string> = { date }
    let convertedTotal = 0

    Object.keys(trend.accounts).forEach((acc) => {
      let value = trend.accounts[acc][i] ?? 0

      // Convert BTC to EUR using live price only if price is valid
      if ((acc === 'r_btc' || acc === 'm_btc') && btcPrice && btcPrice >= MIN_VALID_BTC_PRICE) {
        // Only convert small values that look like BTC amounts
        if (value > 0 && value < 1) {
          value = value * btcPrice
        }
      }
      
      row[acc] = value
      convertedTotal += value
    })
    
    // Use sum of all converted accounts as total (this is more accurate than backend totals)
    row['total'] = convertedTotal
    return row
  })

  const activeAccounts = Object.keys(trend.accounts).filter((acc) =>
    trend.accounts[acc].some((v) => v > 0)
  )

  // Latest non-zero value per key for legend - use converted data so BTC shows in EUR
  const latestValues: Record<string, number> = {}
  if (data.length > 0) {
    const lastRow = data[data.length - 1]
    latestValues['total'] = typeof lastRow['total'] === 'number' ? lastRow['total'] : 0
    for (const acc of activeAccounts) {
      const val = lastRow[acc]
      latestValues[acc] = typeof val === 'number' ? val : 0
    }
  } else {
    latestValues['total'] = 0
    for (const acc of activeAccounts) {
      latestValues[acc] = 0
    }
  }

  // Dynamic Y-axis ticks based on actual max from converted data
  const maxTotal = Math.max(...data.map((d) => typeof d['total'] === 'number' ? d['total'] : 0).filter((v) => v != null))
  const yTicks = buildYTicks(maxTotal)

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
    <ResponsiveContainer width="100%" height={400}>
      <LineChart data={data} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="date" tick={{ fontSize: 11 }} />
        <YAxis
          domain={[0, 'auto']}
          ticks={yTicks}
          tickFormatter={(v) => v === 0 ? '€0' : `€${(v / 1000).toFixed(0)}k`}
          tick={{ fontSize: 11 }}
          width={52}
        />
        <Tooltip content={<CustomTooltip activeKey={activeKey} hiddenKeys={hiddenKeys} />} />
        <Legend
          layout="vertical"
          align="right"
          verticalAlign="middle"
          content={<CustomLegend hiddenKeys={hiddenKeys} latestValues={latestValues} onToggle={toggleKey} />}
        />
        <Line
          type="monotone"
          dataKey="total"
          stroke="#1d4ed8"
          strokeWidth={activeKey === 'total' ? 3 : 2.5}
          strokeOpacity={lineOpacity('total')}
          dot={false}
          name="Total"
          hide={hiddenKeys.has('total')}
          onMouseEnter={() => setActiveKey('total')}
          onMouseLeave={() => setActiveKey(null)}
        />
        {activeAccounts.map((acc) => (
          <Line
            key={acc}
            type="monotone"
            dataKey={acc}
            stroke={ACCOUNT_COLORS[acc] ?? '#94a3b8'}
            strokeWidth={activeKey === acc ? 2.5 : 1.5}
            strokeOpacity={lineOpacity(acc)}
            dot={false}
            name={ACCOUNT_LABELS[acc] ?? acc}
            strokeDasharray={ACCOUNT_DASH[acc] ?? '4 2'}
            hide={hiddenKeys.has(acc)}
            onMouseEnter={() => setActiveKey(acc)}
            onMouseLeave={() => setActiveKey(null)}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
