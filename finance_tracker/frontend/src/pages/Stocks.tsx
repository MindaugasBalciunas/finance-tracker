import { useState } from 'react'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, ReferenceLine, BarChart, Bar } from 'recharts'
import { useStockTrades, useStockPortfolio, useCreateStockTrade, useUpdateStockTrade, useDeleteStockTrade, useUsdEurRate, useAllStockPrices } from '../hooks/useStocks'
import StockTradeForm from '../components/forms/StockTradeForm'
import StockForecastSection from '../components/charts/StockForecastSection'
import PortfolioRow, { DualAmount, DualAmountEur } from '../components/stocks/PortfolioRow'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatDate, formatEuro, formatUsd, gainColor } from '../utils/format'
import { computeSellPnL } from '../utils/stockCalculations'
import type { CreateStockTradeInput, StockTrade } from '../types'

const CHART_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6',
  '#14b8a6', '#f97316', '#ec4899', '#6366f1', '#84cc16',
]

export default function Stocks() {
  const [showForm, setShowForm] = useState(false)
  const [editingTrade, setEditingTrade] = useState<StockTrade | null>(null)

  const { data: trades, isLoading: tradesLoading } = useStockTrades()
  const { data: portfolio, isLoading: portfolioLoading } = useStockPortfolio()
  const createMutation = useCreateStockTrade()
  const updateMutation = useUpdateStockTrade()
  const deleteMutation = useDeleteStockTrade()
  const { usdToEur, eurToUsd } = useUsdEurRate()
  const activeTickersEarly = (portfolio?.holdings ?? []).filter((h) => h.shares > 0.0001).map((h) => h.ticker)
  const allPriceQueries = useAllStockPrices(activeTickersEarly)

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

  // Price map: ticker → live price (aligns with activeHoldings order)
  const priceMap: Record<string, number | null> = {}
  activeHoldings.forEach((h, i) => { priceMap[h.ticker] = allPriceQueries[i]?.data?.price ?? null })

  // Cost-recovered: tickers where total sell proceeds >= total buy cost (remaining shares are "free money")
  const tickerBuyCostEur: Record<string, number> = {}
  const tickerSellProceedsEur: Record<string, number> = {}
  for (const t of trades ?? []) {
    const eurVal = t.currency === 'EUR'
      ? t.shares * t.price_per_share.value
      : (usdToEur(t.shares * t.price_per_share.value) ?? 0)
    if (t.action === 'buy') tickerBuyCostEur[t.ticker] = (tickerBuyCostEur[t.ticker] ?? 0) + eurVal
    else tickerSellProceedsEur[t.ticker] = (tickerSellProceedsEur[t.ticker] ?? 0) + eurVal
  }
  const costRecoveredSet = new Set(
    Object.keys(tickerBuyCostEur).filter((t) => (tickerSellProceedsEur[t] ?? 0) >= tickerBuyCostEur[t])
  )

  // Total current unrealized P&L in EUR (for combined cumulative extension)
  const totalUnrealizedEur = activeHoldings.reduce((sum, h) => {
    const price = priceMap[h.ticker]
    if (price == null) return sum
    const unrealNative = (price - h.avg_cost.value) * h.shares
    return sum + (toEur(unrealNative, h.currency) ?? unrealNative)
  }, 0)

  // Comparison data: realized + unrealized per ticker, sorted by abs total desc
  const pnlCompData = [
    ...activeHoldings.map((h) => {
      const price = priceMap[h.ticker]
      const unrealNative = price != null ? (price - h.avg_cost.value) * h.shares : null
      const unrealEur = unrealNative != null ? (toEur(unrealNative, h.currency) ?? unrealNative) : null
      return {
        ticker: h.ticker,
        realized: toEur(h.realized_gain.value, h.currency) ?? 0,
        unrealized: unrealEur,
        costRecovered: costRecoveredSet.has(h.ticker),
      }
    }),
    ...closedHoldings.map((h) => ({
      ticker: h.ticker,
      realized: toEur(h.realized_gain.value, h.currency) ?? 0,
      unrealized: 0 as number | null,
      costRecovered: costRecoveredSet.has(h.ticker),
    })),
  ].sort((a, b) => Math.abs((b.realized) + (b.unrealized ?? 0)) - Math.abs((a.realized) + (a.unrealized ?? 0)))

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

      {/* ── SECTION 1: PORTFOLIO SNAPSHOT ─────────────────────────── */}
      <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide">Portfolio snapshot</p>

      {/* Summary KPI cards */}
      {portfolio && (() => {
        const totalUnrealPricesLoaded = activeHoldings.every((h) => priceMap[h.ticker] != null)
        const totalUnrealDisplay = totalUnrealPricesLoaded ? totalUnrealizedEur : null
        const totalPnl = totalRealizedEur != null && totalUnrealDisplay != null
          ? totalRealizedEur + totalUnrealDisplay : null
        return (
          <div className="grid grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4">
            <div className="bg-white rounded-xl border border-gray-200 p-4">
              <p className="text-xs text-gray-500 mb-1">Invested (open)</p>
              <p className="text-xl font-bold text-gray-900">{totalInvestedEur != null ? formatEuro(totalInvestedEur) : '—'}</p>
              <p className="text-xs text-gray-400 mt-1">{activeHoldings.length} position{activeHoldings.length !== 1 ? 's' : ''}</p>
            </div>
            <div className="bg-white rounded-xl border border-gray-200 p-4">
              <p className="text-xs text-gray-500 mb-1">Unrealized P&L</p>
              <p className={`text-xl font-bold ${totalUnrealDisplay != null ? gainColor(totalUnrealDisplay) : 'text-gray-300'}`}>
                {totalUnrealDisplay != null ? `${totalUnrealDisplay >= 0 ? '+' : ''}${formatEuro(totalUnrealDisplay)}` : '…'}
              </p>
              <p className="text-xs text-gray-400 mt-1">Open positions</p>
            </div>
            <div className="bg-white rounded-xl border border-gray-200 p-4">
              <p className="text-xs text-gray-500 mb-1">Realized P&L</p>
              {totalRealizedEur != null ? (
                <p className={`text-xl font-bold ${gainColor(totalRealizedEur)}`}>
                  {totalRealizedEur >= 0 ? '+' : ''}{formatEuro(totalRealizedEur)}
                </p>
              ) : <p className="text-xl font-bold text-gray-400">—</p>}
              <p className="text-xs text-gray-400 mt-1">All closed trades</p>
            </div>
            <div className="bg-white rounded-xl border border-gray-200 p-4">
              <p className="text-xs text-gray-500 mb-1">Total P&L</p>
              <p className={`text-xl font-bold ${totalPnl != null ? gainColor(totalPnl) : 'text-gray-300'}`}>
                {totalPnl != null ? `${totalPnl >= 0 ? '+' : ''}${formatEuro(totalPnl)}` : '…'}
              </p>
              <p className="text-xs text-gray-400 mt-1">Realized + unrealized</p>
            </div>
            <div className="bg-white rounded-xl border border-gray-200 p-4">
              <p className="text-xs text-gray-500 mb-1">Total trades</p>
              <p className="text-xl font-bold text-gray-900">{trades?.length ?? 0}</p>
              <p className="text-xs text-gray-400 mt-1">{(trades ?? []).filter((t) => t.action === 'sell').length} sells</p>
            </div>
          </div>
        )
      })()}

      {/* Portfolio allocation + position weights */}
      {allocationData.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 sm:gap-4">
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <h3 className="text-sm font-semibold text-gray-700 mb-3">Allocation by Cost Basis</h3>
            <ResponsiveContainer width="100%" height={300}>
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
                  label={({ name, value, percent }) =>
                    `${name} ${formatEuro(value)} (${(percent * 100).toFixed(1)}%)`
                  }
                  labelLine={true}
                >
                  {allocationData.map((_, i) => <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />)}
                </Pie>
                <Tooltip formatter={(v: number) => formatEuro(v)} />
              </PieChart>
            </ResponsiveContainer>
          </div>
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
                        <span className="text-sm text-gray-700">{formatEuro(item.value)}</span>
                        <span className="text-xs text-gray-400 ml-2">{pct.toFixed(1)}%</span>
                      </div>
                    </div>
                    <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                      <div className="h-full rounded-full transition-all"
                        style={{ width: `${pct}%`, backgroundColor: CHART_COLORS[i % CHART_COLORS.length] }} />
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Open positions table */}
      {activeHoldings.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
            <h3 className="text-sm font-semibold text-gray-700">Open Positions</h3>
            <p className="text-xs text-gray-400 mt-0.5">Live prices from Yahoo Finance · EUR primary · USD secondary · click ticker to expand price chart</p>
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
                  <PortfolioRow key={h.ticker} holding={h} usdToEur={usdToEur} eurToUsd={eurToUsd}
                    totalCostEur={totalInvestedEur} trades={(trades ?? []).filter((t) => t.ticker === h.ticker)} />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── SECTION 2: P&L PERFORMANCE ────────────────────────────── */}
      {pnlCompData.length > 0 && (
        <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide">P&L performance</p>
      )}

      {/* Realized vs Unrealized comparison — with cost-recovered highlight */}
      {pnlCompData.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Realized vs. Unrealized P&L</h3>
          <p className="text-xs text-gray-400 mb-1">
            Per-ticker in EUR · live prices ·{' '}
            <span className="text-indigo-500 font-medium">■ realized</span>{' '}
            <span className="text-blue-400 font-medium">■ unrealized</span>{' '}
            {costRecoveredSet.size > 0 && <><span className="text-amber-500 font-medium">■ free shares</span>{' '}(cost fully recovered from sells)</>}
          </p>
          {costRecoveredSet.size > 0 && (
            <p className="text-xs text-amber-600 bg-amber-50 rounded-lg px-3 py-1.5 mb-3 inline-block">
              ★ {Array.from(costRecoveredSet).join(', ')} — sell proceeds already covered the full buy cost; remaining shares are pure profit
            </p>
          )}
          <ResponsiveContainer width="100%" height={Math.max(pnlCompData.length * 52, 120)}>
            <BarChart data={pnlCompData} layout="vertical" barCategoryGap="28%" barGap={3}
              margin={{ top: 0, right: 72, left: 0, bottom: 0 }}>
              <XAxis type="number" tick={{ fontSize: 10 }} tickLine={false} axisLine={false}
                tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} />
              <YAxis type="category" dataKey="ticker"
                tick={(props: any) => {
                  const { x, y, payload } = props
                  const isFree = costRecoveredSet.has(payload.value)
                  return (
                    <g transform={`translate(${x},${y})`}>
                      <text x={-4} y={0} dy={4} textAnchor="end" fontSize={11} fontWeight={700} fill={isFree ? '#d97706' : '#374151'}>{payload.value}</text>
                      {isFree && <text x={-4} y={0} dy={16} textAnchor="end" fontSize={8} fill="#d97706">FREE ★</text>}
                    </g>
                  )
                }}
                tickLine={false} axisLine={false} width={58} />
              <ReferenceLine x={0} stroke="#d1d5db" />
              <Tooltip
                formatter={(v: number, name: string) =>
                  [`${v >= 0 ? '+' : ''}${formatEuro(v)}`, name === 'realized' ? 'Realized' : 'Unrealized']
                }
                contentStyle={{ fontSize: 11, borderRadius: 6 }}
              />
              <Bar dataKey="realized" name="Realized" radius={[0, 3, 3, 0]}
                label={{ position: 'right', fontSize: 10, formatter: (v: number) => v !== 0 ? `${v >= 0 ? '+' : ''}${formatEuro(v)}` : '' }}>
                {pnlCompData.map((entry, i) => (
                  <Cell key={`r-${i}`} fill={entry.realized >= 0 ? '#6366f1' : '#a78bfa'} fillOpacity={entry.realized !== 0 ? 1 : 0.15} />
                ))}
              </Bar>
              <Bar dataKey="unrealized" name="Unrealized" radius={[0, 3, 3, 0]}
                label={{ position: 'right', fontSize: 10, formatter: (v: number) => v !== 0 ? `${v >= 0 ? '+' : ''}${formatEuro(v)}` : '' }}>
                {pnlCompData.map((entry, i) => (
                  <Cell key={`u-${i}`}
                    fill={entry.unrealized == null ? '#d1d5db'
                      : entry.costRecovered ? '#f59e0b'
                      : entry.unrealized >= 0 ? '#3b82f6' : '#f97316'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
          {pnlCompData.some((d) => d.unrealized == null) && (
            <p className="text-xs text-gray-400 mt-2">Gray bars = live price loading…</p>
          )}
        </div>
      )}

      {/* Realized Gains Analysis — cumulative chart extended with today's unrealized */}
      {trades && trades.filter((t) => t.action === 'sell').length > 0 && (() => {
        const sellPnL = computeSellPnL(trades)
        if (sellPnL.length === 0) return null

        const withEur = sellPnL.map((s) => ({
          ...s,
          pnlEur: s.currency === 'EUR' ? s.pnl : (usdToEur(s.pnl) ?? s.pnl),
        }))

        let running = 0
        const cumPoints = withEur.map((s) => {
          running += s.pnlEur
          return { date: s.date, realized: running, ticker: s.ticker }
        })

        const finalRealized = cumPoints.length > 0 ? cumPoints[cumPoints.length - 1].realized : 0
        const today = new Date().toISOString().slice(0, 10)
        const lastDate = cumPoints.length > 0 ? cumPoints[cumPoints.length - 1].date : today

        // Extend chart to today: add a "now" point showing realized + unrealized
        const allPricesLoaded = activeHoldings.length === 0 || activeHoldings.every((h) => priceMap[h.ticker] != null)
        const extPoint = allPricesLoaded && lastDate < today
          ? [{ date: today, realized: finalRealized, total: finalRealized + totalUnrealizedEur }]
          : []
        const cumData = [
          ...cumPoints.map((d) => ({ ...d, total: d.realized })),
          ...extPoint,
        ]

        const tickerTotals: Record<string, number> = {}
        for (const s of withEur) tickerTotals[s.ticker] = (tickerTotals[s.ticker] ?? 0) + s.pnlEur
        const tickerBars = Object.entries(tickerTotals)
          .map(([ticker, pnl]) => ({ ticker, pnl }))
          .sort((a, b) => b.pnl - a.pnl)

        const totalPnlEur = finalRealized
        const best = tickerBars[0]
        const worst = tickerBars[tickerBars.length - 1]
        const realColor = totalPnlEur >= 0 ? '#10b981' : '#ef4444'
        const totalColor = (finalRealized + totalUnrealizedEur) >= 0 ? '#3b82f6' : '#f97316'

        return (
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6 space-y-5">
            <div>
              <h3 className="text-base font-semibold text-gray-900 mb-1">Realized Gains Analysis</h3>
              <p className="text-xs text-gray-400">Per-sell P&L via average cost method · EUR</p>
            </div>

            {/* Stats */}
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div className="bg-gray-50 rounded-lg p-3">
                <p className="text-xs text-gray-500 mb-1">Total Realized</p>
                <p className={`text-lg font-bold ${gainColor(totalPnlEur)}`}>{totalPnlEur >= 0 ? '+' : ''}{formatEuro(totalPnlEur)}</p>
                <p className="text-xs text-gray-400">{sellPnL.length} sell{sellPnL.length !== 1 ? 's' : ''}</p>
              </div>
              <div className="bg-blue-50 rounded-lg p-3">
                <p className="text-xs text-gray-500 mb-1">+ Unrealized Today</p>
                <p className={`text-lg font-bold ${gainColor(totalUnrealizedEur)}`}>
                  {totalUnrealizedEur >= 0 ? '+' : ''}{formatEuro(totalUnrealizedEur)}
                </p>
                <p className="text-xs text-gray-400">Open positions</p>
              </div>
              <div className={`rounded-lg p-3 ${best && best.pnl > 0 ? 'bg-green-50' : 'bg-gray-50'}`}>
                <p className="text-xs text-gray-500 mb-1">Best Realized</p>
                {best ? (
                  <>
                    <p className={`text-lg font-bold ${gainColor(best.pnl)}`}>{best.ticker}</p>
                    <p className={`text-xs ${gainColor(best.pnl)}`}>{best.pnl >= 0 ? '+' : ''}{formatEuro(best.pnl)}</p>
                  </>
                ) : <p className="text-gray-400 text-sm">—</p>}
              </div>
              <div className={`rounded-lg p-3 ${worst && worst.pnl < 0 ? 'bg-red-50' : 'bg-gray-50'}`}>
                <p className="text-xs text-gray-500 mb-1">Worst Realized</p>
                {worst && worst !== best ? (
                  <>
                    <p className={`text-lg font-bold ${gainColor(worst.pnl)}`}>{worst.ticker}</p>
                    <p className={`text-xs ${gainColor(worst.pnl)}`}>{worst.pnl >= 0 ? '+' : ''}{formatEuro(worst.pnl)}</p>
                  </>
                ) : <p className="text-gray-400 text-sm">—</p>}
              </div>
            </div>

            {/* Combined cumulative chart: realized history + unrealized extension to today */}
            <div>
              <p className="text-xs font-medium text-gray-500 mb-2">
                Cumulative P&L ·{' '}
                <span style={{ color: realColor }}>— realized</span>
                {extPoint.length > 0 && <>{' '}+ <span style={{ color: totalColor }}>— total (incl. unrealized today)</span></>}
              </p>
              <ResponsiveContainer width="100%" height={220}>
                <AreaChart data={cumData} margin={{ top: 4, right: 12, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="realGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={realColor} stopOpacity={0.25} />
                      <stop offset="95%" stopColor={realColor} stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="totalGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={totalColor} stopOpacity={0.15} />
                      <stop offset="95%" stopColor={totalColor} stopOpacity={0} />
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
                      `${v >= 0 ? '+' : ''}${formatEuro(v)}`,
                      name === 'total' ? 'Total (realized + unrealized)' : 'Cumulative realized',
                    ]}
                    contentStyle={{ fontSize: 11, borderRadius: 6 }}
                  />
                  {/* Total area (realized + unrealized) — rendered first so realized sits on top */}
                  <Area type="monotone" dataKey="total" stroke={totalColor} strokeWidth={1.5}
                    strokeDasharray="5 3" fill="url(#totalGrad)" dot={false} activeDot={{ r: 3 }} />
                  {/* Realized area */}
                  <Area type="monotone" dataKey="realized" stroke={realColor} strokeWidth={2}
                    fill="url(#realGrad)" dot={{ r: 3, fill: realColor, strokeWidth: 0 }} activeDot={{ r: 4 }} />
                </AreaChart>
              </ResponsiveContainer>
            </div>

            {/* Per-ticker realized P&L bars */}
            {tickerBars.length > 1 && (
              <div>
                <p className="text-xs font-medium text-gray-500 mb-2">Realized P&L by Ticker</p>
                <ResponsiveContainer width="100%" height={Math.max(tickerBars.length * 36, 80)}>
                  <BarChart data={tickerBars} layout="vertical" margin={{ top: 0, right: 72, left: 0, bottom: 0 }}>
                    <XAxis type="number" tick={{ fontSize: 10 }} tickLine={false} axisLine={false}
                      tickFormatter={(v) => `€${(v / 1000).toFixed(1)}k`} />
                    <YAxis type="category" dataKey="ticker" tick={{ fontSize: 11, fontWeight: 600 }}
                      tickLine={false} axisLine={false} width={52} />
                    <ReferenceLine x={0} stroke="#d1d5db" />
                    <Tooltip formatter={(v: number) => [`${v >= 0 ? '+' : ''}${formatEuro(v)}`, 'Realized P&L']}
                      contentStyle={{ fontSize: 11, borderRadius: 6 }} />
                    <Bar dataKey="pnl" radius={[0, 3, 3, 0]}
                      label={{ position: 'right', fontSize: 10, formatter: (v: number) => `${v >= 0 ? '+' : ''}${formatEuro(v)}` }}>
                      {tickerBars.map((entry, i) => (
                        <Cell key={i} fill={entry.pnl >= 0 ? '#10b981' : '#ef4444'} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}

            {/* Sell trade breakdown table */}
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
                      const fmt = s.currency === 'EUR' ? formatEuro : formatUsd
                      return (
                        <tr key={s.id} className="hover:bg-gray-50">
                          <td className="py-2 pr-4 text-gray-600 text-xs">{s.date}</td>
                          <td className="py-2 pr-4 font-bold text-gray-900">
                            {s.ticker}
                            {costRecoveredSet.has(s.ticker) && <span className="ml-1 text-amber-500 text-xs">★</span>}
                          </td>
                          <td className="py-2 pr-4 text-right text-gray-600 text-xs">{s.shares}</td>
                          <td className="py-2 pr-4 text-right text-gray-600 text-xs">{fmt(s.sellPrice)}</td>
                          <td className="py-2 pr-4 text-right text-gray-500 text-xs">{fmt(s.avgCost)}</td>
                          <td className={`py-2 text-right font-semibold text-sm ${gainColor(s.pnlEur)}`}>
                            {s.pnlEur >= 0 ? '+' : ''}{formatEuro(s.pnlEur)}
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

      {/* ── SECTION 3: OUTLOOK ────────────────────────────────────── */}
      {activeHoldings.length > 0 && (
        <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide">Outlook</p>
      )}
      {activeHoldings.length > 0 && (
        <StockForecastSection holdings={activeHoldings} usdToEur={usdToEur} />
      )}

      {/* ── SECTION 4: HISTORY ────────────────────────────────────── */}
      <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide">History</p>

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
                    <td className="px-4 py-3 font-bold text-gray-900">
                      {h.ticker}
                      {costRecoveredSet.has(h.ticker) && <span className="ml-1.5 text-xs bg-amber-100 text-amber-700 px-1.5 py-0.5 rounded font-medium">cost recovered</span>}
                    </td>
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
                      }`}>{t.source}</span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold ${
                        t.action === 'buy' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                      }`}>{t.action.toUpperCase()}</span>
                    </td>
                    <td className="px-4 py-3 font-bold text-gray-900">{t.ticker}</td>
                    <td className="px-4 py-3 text-right text-gray-700">{t.shares}</td>
                    <td className="px-4 py-3 text-right text-gray-700">
                      {isEur ? <DualAmountEur eur={price} eurToUsd={eurToUsd} /> : <DualAmount usd={price} usdToEur={usdToEur} />}
                    </td>
                    <td className="px-4 py-3 text-right font-semibold text-gray-900">
                      {isEur ? <DualAmountEur eur={total} eurToUsd={eurToUsd} /> : <DualAmount usd={total} usdToEur={usdToEur} />}
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
                  <td colSpan={8} className="px-4 py-12 text-center text-gray-400">No trades yet. Add your first trade above.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
