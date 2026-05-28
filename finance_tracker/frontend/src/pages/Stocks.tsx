import { useState } from 'react'
import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, ReferenceLine, ReferenceDot, BarChart, Bar } from 'recharts'
import { useStockTrades, useStockPortfolio, useCreateStockTrade, useUpdateStockTrade, useDeleteStockTrade, useStockPrice, useStockHistory, useUsdEurRate } from '../hooks/useStocks'
import StockTradeForm from '../components/forms/StockTradeForm'
import StockForecastSection from '../components/charts/StockForecastSection'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatDate } from '../utils/format'
import type { CreateStockTradeInput, StockHolding, StockTrade } from '../types'

interface SellPnLEntry {
  id: number
  date: string
  ticker: string
  shares: number
  sellPrice: number
  avgCost: number
  pnl: number
  currency: string
}

function computeSellPnL(trades: StockTrade[]): SellPnLEntry[] {
  const byTicker: Record<string, StockTrade[]> = {}
  for (const t of trades) {
    if (!byTicker[t.ticker]) byTicker[t.ticker] = []
    byTicker[t.ticker].push(t)
  }
  const results: SellPnLEntry[] = []
  for (const tickerTrades of Object.values(byTicker)) {
    const sorted = [...tickerTrades].sort((a, b) => a.date.localeCompare(b.date))
    let runningShares = 0
    let runningCost = 0
    for (const t of sorted) {
      if (t.action === 'buy') {
        runningShares += t.shares
        runningCost += t.shares * t.price_per_share.value
      } else if (t.action === 'sell' && runningShares > 0) {
        const avgCost = runningCost / runningShares
        const pnl = t.shares * (t.price_per_share.value - avgCost)
        results.push({ id: t.id, date: t.date.slice(0, 10), ticker: t.ticker, shares: t.shares, sellPrice: t.price_per_share.value, avgCost, pnl, currency: t.currency })
        runningCost -= t.shares * avgCost
        runningShares -= t.shares
      }
    }
  }
  return results.sort((a, b) => a.date.localeCompare(b.date))
}

const CHART_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#14b8a6', '#f97316', '#ec4899', '#6366f1', '#84cc16',
]

function formatUsd(amount: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount)
}
function formatEur(amount: number) {
  return new Intl.NumberFormat('en-EU', { style: 'currency', currency: 'EUR' }).format(amount)
}
function formatPct(pct: number) {
  return `${pct >= 0 ? '+' : ''}${pct.toFixed(2)}%`
}
function gainColor(value: number) {
  if (value > 0) return 'text-green-600'
  if (value < 0) return 'text-red-600'
  return 'text-gray-500'
}
function gainBg(value: number) {
  if (value > 0) return 'bg-green-500'
  if (value < 0) return 'bg-red-500'
  return 'bg-gray-300'
}

function DualAmount({ usd, usdToEur, gain = false }: { usd: number; usdToEur: (n: number) => number | null; gain?: boolean }) {
  const eur = usdToEur(usd)
  const prefix = gain && usd !== 0 ? (usd >= 0 ? '+' : '') : ''
  return (
    <div>
      <div>{prefix}{eur != null ? formatEur(eur) : formatUsd(usd)}</div>
      {eur != null && <div className="text-xs text-gray-400">{prefix}{formatUsd(usd)}</div>}
    </div>
  )
}

function DualAmountEur({ eur, eurToUsd, gain = false }: { eur: number; eurToUsd: (n: number) => number | null; gain?: boolean }) {
  const usd = eurToUsd(eur)
  const prefix = gain && eur !== 0 ? (eur >= 0 ? '+' : '') : ''
  return (
    <div>
      <div>{prefix}{formatEur(eur)}</div>
      {usd != null && <div className="text-xs text-gray-400">{prefix}{formatUsd(usd)}</div>}
    </div>
  )
}

const RANGES = ['1mo', '3mo', '6mo', '1y', '2y', '5y'] as const
type Range = typeof RANGES[number]

function PortfolioRow({ holding, usdToEur, eurToUsd, totalCostEur, trades }: {
  holding: StockHolding
  usdToEur: (n: number) => number | null
  eurToUsd: (n: number) => number | null
  totalCostEur: number | null
  trades: StockTrade[]
}) {
  const [expanded, setExpanded] = useState(false)
  const [range, setRange] = useState<Range>('1y')
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

  function AmountCell({ amount, gain = false }: { amount: number; gain?: boolean }) {
    return isEur
      ? <DualAmountEur eur={amount} eurToUsd={eurToUsd} gain={gain} />
      : <DualAmount usd={amount} usdToEur={usdToEur} gain={gain} />
  }

  // Inline chart state (only fetched when expanded)
  const { data: histData, isLoading: histLoading, isError: histError } = useStockHistory(holding.ticker, range, expanded)
  const points = histData?.points ?? []
  const first = points[0]?.close
  const last = points[points.length - 1]?.close
  const chartColor = first != null && last != null && last >= first ? '#10b981' : '#ef4444'
  const formatCcy = isEur ? (v: number) => `€${v.toFixed(2)}` : (v: number) => `$${v.toFixed(2)}`
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
                  <span className={`text-xs ${gainColor(returnPct)}`}>{formatPct(returnPct)}</span>
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
            {/* Range tabs */}
            <div className="flex items-center gap-2 mb-3">
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
          </td>
        </tr>
      )}
    </>
  )
}

export default function Stocks() {
  const [showForm, setShowForm] = useState(false)
  const [editingTrade, setEditingTrade] = useState<StockTrade | null>(null)

  const { data: trades, isLoading: tradesLoading } = useStockTrades()
  const { data: portfolio, isLoading: portfolioLoading } = useStockPortfolio()
  const createMutation = useCreateStockTrade()
  const updateMutation = useUpdateStockTrade()
  const deleteMutation = useDeleteStockTrade()
  const { usdToEur, eurToUsd } = useUsdEurRate()

  const handleCreate = async (input: CreateStockTradeInput) => {
    await createMutation.mutateAsync({ ...input, ticker: input.ticker.toUpperCase() })
    setShowForm(false)
  }
  const handleUpdate = async (input: CreateStockTradeInput) => {
    if (!editingTrade) return
    await updateMutation.mutateAsync({ id: editingTrade.id, input: { ...input, ticker: input.ticker.toUpperCase() } })
    setEditingTrade(null)
  }
  const handleDelete = async (id: number) => {
    if (confirm('Delete this trade?')) await deleteMutation.mutateAsync(id)
  }

  if (tradesLoading || portfolioLoading) return <LoadingSpinner message="Loading stocks & ETFs…" />

  const activeHoldings = (portfolio?.holdings ?? []).filter((h) => h.shares > 0.0001)
  const closedHoldings = (portfolio?.holdings ?? []).filter((h) => h.shares <= 0.0001 && h.realized_gain.value !== 0)
  const allHoldings = portfolio?.holdings ?? []

  function toEur(amount: number, currency: string): number | null {
    return currency === 'EUR' ? amount : usdToEur(amount)
  }

  const totalInvestedEur = activeHoldings.every((h) => toEur(h.total_cost.value, h.currency) != null)
    ? activeHoldings.reduce((sum, h) => sum + toEur(h.total_cost.value, h.currency)!, 0)
    : null
  const totalRealizedEur = allHoldings.every((h) => toEur(h.realized_gain.value, h.currency) != null)
    ? allHoldings.reduce((sum, h) => sum + toEur(h.realized_gain.value, h.currency)!, 0)
    : null

  // Allocation chart data (sorted by EUR cost basis desc, slices < 5% grouped into "Other")
  const allocationData = totalInvestedEur != null && totalInvestedEur > 0
    ? (() => {
        const items = [...activeHoldings]
          .map((h) => ({ name: h.ticker, value: toEur(h.total_cost.value, h.currency) ?? 0 }))
          .sort((a, b) => b.value - a.value)
        const threshold = totalInvestedEur * 0.05
        const main = items.filter((i) => i.value >= threshold)
        const other = items.filter((i) => i.value < threshold)
        if (other.length > 0) {
          main.push({ name: `Other (${other.length})`, value: other.reduce((s, i) => s + i.value, 0) })
        }
        return main
      })()
    : []

  return (
    <div className="space-y-6">

      {/* Header */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-gray-500">Track your stock and ETF trades and portfolio performance</p>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 shrink-0 ml-3"
        >
          + Add
        </button>
      </div>

      {/* Add trade modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Stock / ETF Trade</h3>
            <StockTradeForm onSubmit={handleCreate} onCancel={() => setShowForm(false)} isSubmitting={createMutation.isPending} />
          </div>
        </div>
      )}

      {/* Edit trade modal */}
      {editingTrade && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Edit Trade — {editingTrade.ticker}</h3>
            <StockTradeForm
              key={editingTrade.id}
              onSubmit={handleUpdate}
              onCancel={() => setEditingTrade(null)}
              isSubmitting={updateMutation.isPending}
              defaultValues={{
                date: editingTrade.date.slice(0, 10),
                action: editingTrade.action,
                ticker: editingTrade.ticker,
                shares: editingTrade.shares,
                price_per_share: editingTrade.price_per_share.value,
                currency: editingTrade.currency,
                source: editingTrade.source,
                notes: editingTrade.notes,
              }}
            />
          </div>
        </div>
      )}

      {/* Summary cards */}
      {portfolio && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Invested (open)</p>
            <p className="text-xl font-bold text-gray-900">
              {totalInvestedEur != null ? formatEur(totalInvestedEur) : '—'}
            </p>
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Realized gain/loss</p>
            {totalRealizedEur != null ? (
              <p className={`text-xl font-bold ${gainColor(totalRealizedEur)}`}>
                {totalRealizedEur >= 0 ? '+' : ''}{formatEur(totalRealizedEur)}
              </p>
            ) : <p className="text-xl font-bold text-gray-400">—</p>}
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Open positions</p>
            <p className="text-xl font-bold text-gray-900">{activeHoldings.length}</p>
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Total trades</p>
            <p className="text-xl font-bold text-gray-900">{trades?.length ?? 0}</p>
          </div>
        </div>
      )}

      {/* Realized Gains Analysis */}
      {trades && trades.filter((t) => t.action === 'sell').length > 0 && (() => {
        const sellPnL = computeSellPnL(trades)
        if (sellPnL.length === 0) return null

        const withEur = sellPnL.map((s) => ({
          ...s,
          pnlEur: s.currency === 'EUR' ? s.pnl : (usdToEur(s.pnl) ?? s.pnl),
        }))

        let running = 0
        const cumData = withEur.map((s) => {
          running += s.pnlEur
          return { date: s.date, cumulative: running, trade: s.pnlEur, ticker: s.ticker }
        })

        const tickerTotals: Record<string, number> = {}
        for (const s of withEur) tickerTotals[s.ticker] = (tickerTotals[s.ticker] ?? 0) + s.pnlEur
        const tickerBars = Object.entries(tickerTotals)
          .map(([ticker, pnl]) => ({ ticker, pnl }))
          .sort((a, b) => b.pnl - a.pnl)

        const totalPnlEur = withEur.reduce((s, x) => s + x.pnlEur, 0)
        const best = tickerBars[0]
        const worst = tickerBars[tickerBars.length - 1]
        const lineColor = totalPnlEur >= 0 ? '#10b981' : '#ef4444'

        return (
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6 space-y-5">
            <div>
              <h3 className="text-base font-semibold text-gray-900 mb-1">Realized Gains Analysis</h3>
              <p className="text-xs text-gray-400">Per-sell P&L via average cost method · amounts converted to EUR</p>
            </div>

            {/* Stats */}
            <div className="grid grid-cols-3 gap-3">
              <div className="bg-gray-50 rounded-lg p-3">
                <p className="text-xs text-gray-500 mb-1">Total Realized</p>
                <p className={`text-lg font-bold ${gainColor(totalPnlEur)}`}>{totalPnlEur >= 0 ? '+' : ''}{formatEur(totalPnlEur)}</p>
                <p className="text-xs text-gray-400">{sellPnL.length} sell trade{sellPnL.length !== 1 ? 's' : ''}</p>
              </div>
              <div className={`rounded-lg p-3 ${best && best.pnl > 0 ? 'bg-green-50' : 'bg-gray-50'}`}>
                <p className="text-xs text-gray-500 mb-1">Best Performer</p>
                {best ? (
                  <>
                    <p className={`text-lg font-bold ${gainColor(best.pnl)}`}>{best.ticker}</p>
                    <p className={`text-xs ${gainColor(best.pnl)}`}>{best.pnl >= 0 ? '+' : ''}{formatEur(best.pnl)}</p>
                  </>
                ) : <p className="text-gray-400 text-sm">—</p>}
              </div>
              <div className={`rounded-lg p-3 ${worst && worst.pnl < 0 ? 'bg-red-50' : 'bg-gray-50'}`}>
                <p className="text-xs text-gray-500 mb-1">Worst Performer</p>
                {worst && worst !== best ? (
                  <>
                    <p className={`text-lg font-bold ${gainColor(worst.pnl)}`}>{worst.ticker}</p>
                    <p className={`text-xs ${gainColor(worst.pnl)}`}>{worst.pnl >= 0 ? '+' : ''}{formatEur(worst.pnl)}</p>
                  </>
                ) : <p className="text-gray-400 text-sm">—</p>}
              </div>
            </div>

            {/* Cumulative P&L chart */}
            <div>
              <p className="text-xs font-medium text-gray-500 mb-2">Cumulative Realized P&L</p>
              <ResponsiveContainer width="100%" height={200}>
                <AreaChart data={cumData} margin={{ top: 4, right: 12, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="cumGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={lineColor} stopOpacity={0.2} />
                      <stop offset="95%" stopColor={lineColor} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                  <XAxis dataKey="date" tick={{ fontSize: 10 }} tickLine={false}
                    tickFormatter={(d) => d.slice(0, 7)} interval="preserveStartEnd" />
                  <YAxis tick={{ fontSize: 10 }} tickLine={false} axisLine={false}
                    tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} width={58} />
                  <ReferenceLine y={0} stroke="#d1d5db" strokeDasharray="4 2" />
                  <Tooltip
                    formatter={(v: number, name: string) => [
                      `${v >= 0 ? '+' : ''}${formatEur(v)}`,
                      name === 'cumulative' ? 'Cumulative P&L' : 'This trade',
                    ]}
                    labelFormatter={(l: string) => `${l}`}
                    contentStyle={{ fontSize: 11, borderRadius: 6 }}
                  />
                  <Area type="monotone" dataKey="cumulative" stroke={lineColor} strokeWidth={2}
                    fill="url(#cumGrad)" dot={{ r: 3, fill: lineColor, strokeWidth: 0 }} activeDot={{ r: 4 }} />
                </AreaChart>
              </ResponsiveContainer>
            </div>

            {/* Per-ticker P&L bars */}
            {tickerBars.length > 1 && (
              <div>
                <p className="text-xs font-medium text-gray-500 mb-2">Realized P&L by Ticker</p>
                <ResponsiveContainer width="100%" height={Math.max(tickerBars.length * 36, 80)}>
                  <BarChart data={tickerBars} layout="vertical" margin={{ top: 0, right: 60, left: 0, bottom: 0 }}>
                    <XAxis type="number" tick={{ fontSize: 10 }} tickLine={false} axisLine={false}
                      tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} />
                    <YAxis type="category" dataKey="ticker" tick={{ fontSize: 11, fontWeight: 600 }} tickLine={false} axisLine={false} width={52} />
                    <ReferenceLine x={0} stroke="#d1d5db" />
                    <Tooltip formatter={(v: number) => [`${v >= 0 ? '+' : ''}${formatEur(v)}`, 'P&L']} contentStyle={{ fontSize: 11, borderRadius: 6 }} />
                    <Bar dataKey="pnl" radius={[0, 3, 3, 0]} label={{ position: 'right', fontSize: 10, formatter: (v: number) => `${v >= 0 ? '+' : ''}${formatEur(v)}` }}>
                      {tickerBars.map((entry, i) => (
                        <Cell key={i} fill={entry.pnl >= 0 ? '#10b981' : '#ef4444'} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}

            {/* Sell trades with P&L */}
            <div>
              <p className="text-xs font-medium text-gray-500 mb-2">Sell Trade Breakdown</p>
              <div className="overflow-x-auto">
                <table className="min-w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-100">
                      <th className="text-left py-1.5 pr-4 text-xs font-semibold text-gray-500">Date</th>
                      <th className="text-left py-1.5 pr-4 text-xs font-semibold text-gray-500">Ticker</th>
                      <th className="text-right py-1.5 pr-4 text-xs font-semibold text-gray-500">Shares</th>
                      <th className="text-right py-1.5 pr-4 text-xs font-semibold text-gray-500">Sell Price</th>
                      <th className="text-right py-1.5 pr-4 text-xs font-semibold text-gray-500">Avg Cost</th>
                      <th className="text-right py-1.5 text-xs font-semibold text-gray-500">P&L</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-50">
                    {[...withEur].reverse().map((s) => {
                      const fmt = s.currency === 'EUR' ? formatEur : formatUsd
                      return (
                        <tr key={s.id} className="hover:bg-gray-50">
                          <td className="py-2 pr-4 text-gray-600 text-xs">{s.date}</td>
                          <td className="py-2 pr-4 font-bold text-gray-900">{s.ticker}</td>
                          <td className="py-2 pr-4 text-right text-gray-600 text-xs">{s.shares}</td>
                          <td className="py-2 pr-4 text-right text-gray-600 text-xs">{fmt(s.sellPrice)}</td>
                          <td className="py-2 pr-4 text-right text-gray-500 text-xs">{fmt(s.avgCost)}</td>
                          <td className={`py-2 text-right font-semibold text-sm ${gainColor(s.pnlEur)}`}>
                            {s.pnlEur >= 0 ? '+' : ''}{formatEur(s.pnlEur)}
                            {s.currency !== 'EUR' && (
                              <div className="text-xs font-normal text-gray-400">{s.pnl >= 0 ? '+' : ''}{fmt(s.pnl)}</div>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )
      })()}

      {/* Portfolio allocation chart + breakdown */}
      {allocationData.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 sm:gap-4">
          {/* Donut chart */}
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <h3 className="text-sm font-semibold text-gray-700 mb-3">Allocation by Cost Basis</h3>
            <ResponsiveContainer width="100%" height={260}>
              <PieChart>
                <Pie
                  data={allocationData}
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={100}
                  dataKey="value"
                  nameKey="name"
                  paddingAngle={2}
                >
                  {allocationData.map((_, i) => (
                    <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip formatter={(v: number) => formatEur(v)} />
                <Legend />
              </PieChart>
            </ResponsiveContainer>
          </div>

          {/* Allocation bars breakdown */}
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <h3 className="text-sm font-semibold text-gray-700 mb-3">Position Weights</h3>
            <div className="space-y-3">
              {allocationData.map((item, i) => {
                const pct = totalInvestedEur! > 0 ? (item.value / totalInvestedEur!) * 100 : 0
                return (
                  <div key={item.name}>
                    <div className="flex justify-between items-center mb-1">
                      <span className="text-sm font-semibold text-gray-800">{item.name}</span>
                      <div className="text-right">
                        <span className="text-sm text-gray-700">{formatEur(item.value)}</span>
                        <span className="text-xs text-gray-400 ml-2">{pct.toFixed(1)}%</span>
                      </div>
                    </div>
                    <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                      <div
                        className="h-full rounded-full transition-all"
                        style={{ width: `${pct}%`, backgroundColor: CHART_COLORS[i % CHART_COLORS.length] }}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Open positions */}
      {activeHoldings.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
            <h3 className="text-sm font-semibold text-gray-700">Open Positions</h3>
            <p className="text-xs text-gray-400 mt-0.5">Live prices from Yahoo Finance · EUR primary · USD secondary</p>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className="text-left px-4 py-2 font-semibold text-gray-600">Ticker</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Ccy</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Shares</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Avg Cost</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Cost Basis</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Current Price</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Market Value</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Unrealized P&L</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Realized P&L</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {activeHoldings.map((h) => (
                  <PortfolioRow
                    key={h.ticker}
                    holding={h}
                    usdToEur={usdToEur}
                    eurToUsd={eurToUsd}
                    totalCostEur={totalInvestedEur}
                    trades={(trades ?? []).filter((t) => t.ticker === h.ticker)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* 1-year scenario forecast vs VWCE */}
      {activeHoldings.length > 0 && (
        <StockForecastSection holdings={activeHoldings} usdToEur={usdToEur} />
      )}

      {/* Closed positions */}
      {closedHoldings.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
            <h3 className="text-sm font-semibold text-gray-700">Closed Positions</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className="text-left px-4 py-2 font-semibold text-gray-600">Ticker</th>
                  <th className="text-right px-4 py-2 font-semibold text-gray-600">Realized P&L</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {closedHoldings.map((h) => (
                  <tr key={h.ticker} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-bold text-gray-900">{h.ticker}</td>
                    <td className={`px-4 py-3 text-right font-semibold ${gainColor(h.realized_gain.value)}`}>
                      {h.currency === 'EUR'
                        ? <DualAmountEur eur={h.realized_gain.value} eurToUsd={eurToUsd} gain />
                        : <DualAmount usd={h.realized_gain.value} usdToEur={usdToEur} gain />}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Trade history */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
          <h3 className="text-sm font-semibold text-gray-700">Trade History</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Date</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Broker</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Action</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Ticker</th>
                <th className="text-right px-4 py-3 font-semibold text-gray-600">Shares</th>
                <th className="text-right px-4 py-3 font-semibold text-gray-600">Price/Share</th>
                <th className="text-right px-4 py-3 font-semibold text-gray-600">Total</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Notes</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {[...(trades ?? [])].reverse().map((t) => {
                const price = t.price_per_share.value
                const total = t.shares * price
                const isEur = t.currency === 'EUR'
                return (
                  <tr key={t.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-gray-700">{formatDate(t.date)}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                        t.source === 'IBKR' ? 'bg-purple-100 text-purple-700' : 'bg-gray-100 text-gray-600'
                      }`}>
                        {t.source}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ${
                        t.action === 'buy' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                      }`}>
                        {t.action.toUpperCase()}
                      </span>
                    </td>
                    <td className="px-4 py-3 font-bold text-gray-900">{t.ticker}</td>
                    <td className="px-4 py-3 text-right text-gray-700">{t.shares}</td>
                    <td className="px-4 py-3 text-right text-gray-700">
                      {isEur
                        ? <DualAmountEur eur={price} eurToUsd={eurToUsd} />
                        : <DualAmount usd={price} usdToEur={usdToEur} />}
                    </td>
                    <td className="px-4 py-3 text-right font-semibold text-gray-900">
                      {isEur
                        ? <DualAmountEur eur={total} eurToUsd={eurToUsd} />
                        : <DualAmount usd={total} usdToEur={usdToEur} />}
                    </td>
                    <td className="px-4 py-3 text-gray-500 text-xs">{t.notes || '—'}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button onClick={() => setEditingTrade(t)} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
                        <button onClick={() => handleDelete(t.id)} className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
                      </div>
                    </td>
                  </tr>
                )
              })}
              {!trades?.length && (
                <tr>
                  <td colSpan={8} className="px-4 py-12 text-center text-gray-400">
                    No trades yet. Add your first trade above.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
