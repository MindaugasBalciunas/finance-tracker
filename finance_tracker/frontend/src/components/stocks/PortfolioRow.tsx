import { useState } from 'react'
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
  ReferenceLine, ReferenceDot, ResponsiveContainer,
} from 'recharts'
import { useStockPrice, useStockHistory } from '../../hooks/useStocks'
import type { StockHolding, StockTrade } from '../../types'
import { formatEuro, formatUsd, formatPercent, gainColor, gainBg } from '../../utils/format'

export const RANGES = ['1mo', '3mo', '6mo', '1y', '2y', '5y'] as const
export type Range = typeof RANGES[number]

export function DualAmount({ usd, usdToEur, gain = false }: {
  usd: number
  usdToEur: (n: number) => number | null
  gain?: boolean
}) {
  const eur = usdToEur(usd)
  const prefix = gain && usd !== 0 ? (usd >= 0 ? '+' : '') : ''
  return (
    <div>
      <div>{prefix}{eur != null ? formatEuro(eur) : formatUsd(usd)}</div>
      {eur != null && <div className="text-xs text-gray-400">{prefix}{formatUsd(usd)}</div>}
    </div>
  )
}

export function DualAmountEur({ eur, eurToUsd, gain = false }: {
  eur: number
  eurToUsd: (n: number) => number | null
  gain?: boolean
}) {
  const usd = eurToUsd(eur)
  const prefix = gain && eur !== 0 ? (eur >= 0 ? '+' : '') : ''
  return (
    <div>
      <div>{prefix}{formatEuro(eur)}</div>
      {usd != null && <div className="text-xs text-gray-400">{prefix}{formatUsd(usd)}</div>}
    </div>
  )
}

interface HoldingProps {
  holding: StockHolding
  usdToEur: (n: number) => number | null
  eurToUsd: (n: number) => number | null
  totalCostEur: number | null
  trades: StockTrade[]
}

// Live-price derived numbers shared by the desktop row and the mobile card.
function useHoldingLive(holding: StockHolding, usdToEur: (n: number) => number | null, totalCostEur: number | null) {
  const { data: priceData } = useStockPrice(holding.ticker, holding.shares > 0)
  const currentPrice = priceData?.price ?? null
  const currentValue = currentPrice != null ? holding.shares * currentPrice : null
  const unrealizedGain = currentValue != null ? currentValue - holding.total_cost.value : null
  const returnPct = unrealizedGain != null && holding.total_cost.value > 0
    ? (unrealizedGain / holding.total_cost.value) * 100 : null
  const isEur = holding.currency === 'EUR'
  const costEur = isEur ? holding.total_cost.value : usdToEur(holding.total_cost.value)
  const weightPct = costEur != null && totalCostEur != null && totalCostEur > 0
    ? (costEur / totalCostEur) * 100 : null
  return { currentPrice, currentValue, unrealizedGain, returnPct, isEur, weightPct }
}

// Range switcher + price history chart shown when a position is expanded.
function PriceHistoryPanel({ holding, trades, isEur }: {
  holding: StockHolding
  trades: StockTrade[]
  isEur: boolean
}) {
  const [range, setRange] = useState<Range>('1y')
  const { data: histData, isLoading: histLoading, isError: histError } = useStockHistory(holding.ticker, range, true)
  const points = histData?.points ?? []
  const first = points[0]?.close
  const last = points[points.length - 1]?.close
  const chartColor = first != null && last != null && last >= first ? '#10b981' : '#ef4444'
  const formatCcy = isEur ? (v: number) => `€${v.toFixed(2)}` : (v: number) => `$${v.toFixed(2)}`

  return (
    <div>
      <div className="flex items-center gap-2 mb-3 flex-wrap">
        <span className="text-xs font-semibold text-gray-500 mr-1">Price history</span>
        <div className="flex bg-white border border-gray-200 rounded-lg p-0.5">
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                range === r ? 'bg-blue-600 text-white' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              {r.toUpperCase()}
            </button>
          ))}
        </div>
      </div>

      {histLoading && <div className="h-48 flex items-center justify-center text-gray-400 text-sm">Loading…</div>}
      {histError && <div className="h-48 flex items-center justify-center text-red-400 text-sm">Failed to load price history</div>}
      {!histLoading && !histError && points.length > 0 && (
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={points} margin={{ top: 4, right: 12, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id={`grad-${holding.ticker}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={chartColor} stopOpacity={0.2} />
                <stop offset="95%" stopColor={chartColor} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="date" tick={{ fontSize: 10 }} tickLine={false}
              tickFormatter={(d) => d.slice(5)} interval="preserveStartEnd" />
            <YAxis tick={{ fontSize: 10 }} tickLine={false} axisLine={false}
              tickFormatter={formatCcy} width={65} domain={['auto', 'auto']} />
            <Tooltip
              formatter={(v: number) => [formatCcy(v), 'Close']}
              contentStyle={{ fontSize: 11, borderRadius: 6 }}
            />
            <ReferenceLine y={holding.avg_cost.value} stroke="#6366f1" strokeDasharray="4 2"
              label={{ value: `Avg ${formatCcy(holding.avg_cost.value)}`, position: 'insideTopRight', fontSize: 10, fill: '#6366f1' }} />
            <Area type="monotone" dataKey="close" stroke={chartColor} strokeWidth={2}
              fill={`url(#grad-${holding.ticker})`} dot={false} activeDot={{ r: 3 }} />
            {trades.map((t) => {
              const date = t.date.slice(0, 10)
              if (!points.some((p) => p.date === date)) return null
              const isBuy = t.action === 'buy'
              return (
                <ReferenceDot
                  key={t.id}
                  x={date}
                  y={t.price_per_share.value}
                  r={6}
                  fill={isBuy ? '#10b981' : '#ef4444'}
                  stroke="white"
                  strokeWidth={1.5}
                  label={{ value: isBuy ? 'B' : 'S', position: 'top', fontSize: 9, fontWeight: 'bold', fill: isBuy ? '#10b981' : '#ef4444' }}
                />
              )
            })}
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}

export default function PortfolioRow({ holding, usdToEur, eurToUsd, totalCostEur, trades }: HoldingProps) {
  const [expanded, setExpanded] = useState(false)
  const { currentPrice, currentValue, unrealizedGain, returnPct, isEur, weightPct } =
    useHoldingLive(holding, usdToEur, totalCostEur)

  function AmountCell({ amount, gain = false }: { amount: number; gain?: boolean }) {
    return isEur
      ? <DualAmountEur eur={amount} eurToUsd={eurToUsd} gain={gain} />
      : <DualAmount usd={amount} usdToEur={usdToEur} gain={gain} />
  }

  const colSpan = 9

  return (
    <>
      <tr className={`hover:bg-gray-50 ${expanded ? 'bg-blue-50/30' : ''}`}>
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setExpanded((v) => !v)}
              className="font-bold text-gray-900 hover:text-blue-600 text-left"
            >
              {holding.ticker}
            </button>
            <span className={`text-xs transition-transform ${expanded ? 'rotate-90' : ''} text-gray-400`}>▶</span>
          </div>
        </td>
        <td className="px-4 py-3 text-right text-gray-500 text-xs">{holding.currency}</td>
        <td className="px-4 py-3 text-right text-gray-700">{holding.shares.toFixed(4)}</td>
        <td className="px-4 py-3 text-right text-gray-700">
          <AmountCell amount={holding.avg_cost.value} />
        </td>
        <td className="px-4 py-3 text-right text-gray-700">
          <div>
            <AmountCell amount={holding.total_cost.value} />
            {weightPct != null && (
              <div className="mt-1 flex items-center gap-1">
                <div className="h-1.5 bg-gray-100 rounded-full flex-1 overflow-hidden">
                  <div className="h-full bg-blue-400 rounded-full" style={{ width: `${weightPct}%` }} />
                </div>
                <span className="text-xs text-gray-400">{weightPct.toFixed(0)}%</span>
              </div>
            )}
          </div>
        </td>
        <td className="px-4 py-3 text-right text-gray-500">
          {currentPrice != null ? <AmountCell amount={currentPrice} /> : <span className="text-xs text-gray-300">loading…</span>}
        </td>
        <td className="px-4 py-3 text-right font-semibold">
          {currentValue != null ? (
            <div className="text-blue-700"><AmountCell amount={currentValue} /></div>
          ) : <span className="text-xs text-gray-300">—</span>}
        </td>
        <td className={`px-4 py-3 text-right font-semibold ${unrealizedGain != null ? gainColor(unrealizedGain) : 'text-gray-300'}`}>
          {unrealizedGain != null ? (
            <div>
              <AmountCell amount={unrealizedGain} gain />
              {returnPct != null && (
                <div className="flex items-center justify-end gap-1 mt-0.5">
                  <div className="h-1 w-10 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full ${gainBg(returnPct)}`} style={{ width: `${Math.min(Math.abs(returnPct), 100)}%` }} />
                  </div>
                  <span className={`text-xs ${gainColor(returnPct)}`}>{formatPercent(returnPct)}</span>
                </div>
              )}
            </div>
          ) : '—'}
        </td>
        <td className={`px-4 py-3 text-right font-semibold ${gainColor(holding.realized_gain.value)}`}>
          {holding.realized_gain.value !== 0 ? <AmountCell amount={holding.realized_gain.value} gain /> : '—'}
        </td>
      </tr>

      {expanded && (
        <tr>
          <td colSpan={colSpan} className="px-4 py-3 bg-gray-50 border-b border-gray-100">
            <PriceHistoryPanel holding={holding} trades={trades} isEur={isEur} />
          </td>
        </tr>
      )}
    </>
  )
}

// Compact card variant of PortfolioRow for phone screens.
export function PortfolioCard({ holding, usdToEur, totalCostEur, trades }: HoldingProps) {
  const [expanded, setExpanded] = useState(false)
  const { currentPrice, currentValue, unrealizedGain, returnPct, isEur, weightPct } =
    useHoldingLive(holding, usdToEur, totalCostEur)

  const toEur = (amount: number) => (isEur ? amount : usdToEur(amount))
  const fmtNative = isEur ? formatEuro : formatUsd
  const fmt = (amount: number, gain = false) => {
    const eur = toEur(amount)
    const prefix = gain && amount !== 0 ? (amount >= 0 ? '+' : '') : ''
    return `${prefix}${eur != null ? formatEuro(eur) : fmtNative(amount)}`
  }

  return (
    <div className="px-4 py-3">
      <div className="flex items-center justify-between gap-2">
        <button
          onClick={() => setExpanded((v) => !v)}
          className="flex items-center gap-1.5 font-bold text-gray-900 text-left"
        >
          {holding.ticker}
          <span className="text-xs font-normal text-gray-400">{holding.currency}</span>
          <span className={`text-xs transition-transform ${expanded ? 'rotate-90' : ''} text-gray-400`}>▶</span>
        </button>
        <div className="text-right">
          <p className="text-sm font-bold text-blue-700">
            {currentValue != null ? fmt(currentValue) : '—'}
          </p>
          {unrealizedGain != null && returnPct != null && (
            <p className={`text-xs font-semibold ${gainColor(unrealizedGain)}`}>
              {fmt(unrealizedGain, true)} · {formatPercent(returnPct)}
            </p>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-x-6 gap-y-1 mt-2 text-xs">
        <div className="flex justify-between">
          <span className="text-gray-400">Shares</span>
          <span className="text-gray-700">{holding.shares.toFixed(4)}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-400">Avg cost</span>
          <span className="text-gray-700">{fmtNative(holding.avg_cost.value)}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-400">Cost basis</span>
          <span className="text-gray-700">
            {fmt(holding.total_cost.value)}
            {weightPct != null && <span className="text-gray-400"> · {weightPct.toFixed(0)}%</span>}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-400">Price</span>
          <span className="text-gray-700">{currentPrice != null ? fmtNative(currentPrice) : 'loading…'}</span>
        </div>
        {holding.realized_gain.value !== 0 && (
          <div className="flex justify-between">
            <span className="text-gray-400">Realized</span>
            <span className={`font-medium ${gainColor(holding.realized_gain.value)}`}>
              {fmt(holding.realized_gain.value, true)}
            </span>
          </div>
        )}
      </div>

      {expanded && (
        <div className="mt-3 bg-gray-50 rounded-lg p-3 -mx-1">
          <PriceHistoryPanel holding={holding} trades={trades} isEur={isEur} />
        </div>
      )}
    </div>
  )
}
