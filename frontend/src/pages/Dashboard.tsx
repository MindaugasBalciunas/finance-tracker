import { useState, useMemo } from 'react'
import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import { useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import { useBtcEur, BTC_HOLDINGS } from '../hooks/useBtcPrice'
import StatCard from '../components/ui/StatCard'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import CumulativeSpendingChart from '../components/charts/CumulativeSpendingChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import AIInsightCard from '../components/ui/AIInsightCard'
import { formatEuro } from '../utils/format'
import clsx from 'clsx'

type Period = 'all' | '1y' | '6m' | '3m' | '1m'

const PERIODS: { key: Period; label: string }[] = [
  { key: 'all', label: 'All time' },
  { key: '1y',  label: '1 Year' },
  { key: '6m',  label: '6 Months' },
  { key: '3m',  label: '3 Months' },
  { key: '1m',  label: '1 Month' },
]

function getDateRange(period: Period): { date_from?: string; date_to?: string } {
  if (period === 'all') return {}
  const now = new Date()
  const from = new Date(now)
  if (period === '1y') from.setFullYear(now.getFullYear() - 1)
  else if (period === '6m') from.setMonth(now.getMonth() - 6)
  else if (period === '3m') from.setMonth(now.getMonth() - 3)
  else if (period === '1m') from.setMonth(now.getMonth() - 1)
  return { date_from: from.toISOString().slice(0, 10) }
}

// Convert BTC amounts to EUR using live price (preferred) or snapshot price
function getBtcEurValue(balance: any, liveBtcPrice: number | null): number {
  const btcAmount = balance.r_btc + balance.m_btc
  
  // Always prefer live price for visualization
  if (liveBtcPrice && liveBtcPrice > 0) {
    return btcAmount * liveBtcPrice
  }
  
  // Fall back to snapshot price if available
  if (balance.btc_price && balance.btc_price > 0) {
    return btcAmount * balance.btc_price
  }
  
  // Legacy format - assume already in EUR
  return btcAmount
}

export default function Dashboard() {
  const [period, setPeriod] = useState<Period>('all')
  const dateRange = useMemo(() => getDateRange(period), [period])

  const { data: summary, isLoading: summaryLoading } = useTransactionSummary(dateRange)
  const { data: latestBalance, isLoading: balanceLoading } = useLatestBalance()
  const { data: trend, isLoading: trendLoading } = useBalanceTrend(dateRange)
  const { data: allocations, isLoading: allocLoading } = useAccountAllocation()
  const { data: allExpenses } = useAllExpenses(dateRange)
  const btc = useBtcEur()

  const isLoading = summaryLoading || balanceLoading || trendLoading || allocLoading

  // Computed insights
  const savingsRate = summary && summary.total_income > 0
    ? ((summary.total_income - summary.total_expenses) / summary.total_income) * 100
    : null

  const netSaved = summary ? summary.total_income - summary.total_expenses : null

  const avgMonthlySpend = summary && summary.by_month.length > 0
    ? summary.total_expenses / summary.by_month.length
    : null

  const netWorthChange = trend && trend.totals.length >= 2
    ? trend.totals[trend.totals.length - 1] - trend.totals[0]
    : null

  const netWorthChangePct = netWorthChange != null && trend && trend.totals[0] > 0
    ? (netWorthChange / trend.totals[0]) * 100
    : null

  const topCategory = summary?.by_category
    .filter((c) => c.type === 'expense')
    .sort((a, b) => b.total - a.total)[0] ?? null

  if (isLoading) return <LoadingSpinner message="Loading dashboard..." />

  return (
    <div className="space-y-8">

      {/* Header + period picker */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Dashboard</h2>
          <p className="text-sm text-gray-500 mt-1">Your financial overview</p>
        </div>
        <div className="flex items-center gap-1 bg-gray-100 rounded-lg p-1">
          {PERIODS.map((p) => (
            <button
              key={p.key}
              onClick={() => setPeriod(p.key)}
              className={clsx(
                'px-3 py-1.5 text-sm font-medium rounded-md transition-colors',
                period === p.key
                  ? 'bg-white text-gray-900 shadow-sm'
                  : 'text-gray-500 hover:text-gray-700'
              )}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {/* Balance KPI cards — always from latest snapshot */}
      {latestBalance && (
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
          <StatCard
            title="Net Worth"
            value={formatEuro(latestBalance.total)}
            subtitle="All accounts combined"
            color="blue"
          />
          <StatCard
            title="Free Cash"
            value={formatEuro(latestBalance.seb + latestBalance.swed + latestBalance.luminor + latestBalance.cash + latestBalance.rev_m + latestBalance.rev_r)}
            subtitle="Banks + Cash + Revolut"
            color="green"
          />
          <StatCard
            title="Investments"
            value={formatEuro(latestBalance.swed_etf + latestBalance.rev_stocks)}
            subtitle="ETF + Revolut Stocks"
            color="blue"
          />
          <StatCard
            title="Pensions"
            value={formatEuro(latestBalance.swed_pen + latestBalance.art)}
            subtitle="2nd + 3rd Pillar"
            color="purple"
          />
          <StatCard
            title="Crypto"
            value={formatEuro(getBtcEurValue(latestBalance, btc.price))}
            subtitle={btc.price != null
              ? `${(latestBalance.r_btc + latestBalance.m_btc).toFixed(8)} BTC · €${btc.price.toLocaleString()} /BTC`
              : `${(latestBalance.r_btc + latestBalance.m_btc).toFixed(8)} BTC`}
            color="yellow"
          />
        </div>
      )}

      {/* Period insights */}
      {summary && (
        <div>
          <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-3">
            {period === 'all' ? 'All-time' : PERIODS.find((p) => p.key === period)?.label} period insights
          </p>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
            <StatCard
              title="Net Saved"
              value={netSaved != null ? formatEuro(netSaved) : '—'}
              subtitle="Income minus expenses"
              color={netSaved != null && netSaved >= 0 ? 'green' : 'red'}
            />
            <StatCard
              title="Savings Rate"
              value={savingsRate != null ? `${savingsRate.toFixed(1)}%` : '—'}
              subtitle="Of income kept"
              color={savingsRate != null && savingsRate >= 20 ? 'green' : savingsRate != null && savingsRate >= 0 ? 'yellow' : 'red'}
            />
            <StatCard
              title="Avg Monthly Spend"
              value={avgMonthlySpend != null ? formatEuro(avgMonthlySpend) : '—'}
              subtitle={`Over ${summary.by_month.length} month${summary.by_month.length !== 1 ? 's' : ''}`}
              color="blue"
            />
            <StatCard
              title="Net Worth Change"
              value={netWorthChange != null ? formatEuro(netWorthChange) : '—'}
              subtitle={netWorthChangePct != null ? `${netWorthChangePct >= 0 ? '+' : ''}${netWorthChangePct.toFixed(1)}% in period` : 'From first to last snapshot'}
              color={netWorthChange != null && netWorthChange >= 0 ? 'green' : 'red'}
            />
          </div>
          <div className="grid grid-cols-2 lg:grid-cols-3 gap-4 mt-4">
            <StatCard
              title="Total Income"
              value={formatEuro(summary.total_income)}
              subtitle="All recorded income"
              color="green"
            />
            <StatCard
              title="Total Expenses"
              value={formatEuro(summary.total_expenses)}
              subtitle="All recorded expenses"
              color="red"
            />
            <StatCard
              title="Total Invested"
              value={formatEuro(summary.total_investments)}
              subtitle={topCategory ? `Top spend: ${topCategory.category}` : 'Investment transactions'}
              color="purple"
            />
          </div>
        </div>
      )}

      {/* AI Financial Overview */}
      <AIInsightCard />

      {/* Net Worth Over Time */}
      {trend && (
        <div className="bg-white rounded-xl border border-gray-200 p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Net Worth Over Time</h3>
          <p className="text-xs text-gray-400 mb-4">Click legend items to show/hide accounts. Hover a line to highlight it.</p>
          <BalanceTrendChart trend={trend} />
        </div>
      )}

      {/* Monthly spending pace */}
      {allExpenses && allExpenses.data.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Monthly Spending Pace</h3>
          <p className="text-xs text-gray-400 mb-4">Cumulative daily expenses per month — steeper slope = faster spending</p>
          <CumulativeSpendingChart transactions={allExpenses.data} />
        </div>
      )}

      {/* Monthly cash flow + allocation */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {summary?.by_month && summary.by_month.length > 0 && (
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Monthly Cash Flow</h3>
            <p className="text-xs text-gray-400 mb-4">Income vs expenses by month</p>
            <MonthlyBarChart data={summary.by_month} />
          </div>
        )}

        {allocations && allocations.length > 0 && (
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Current Allocation</h3>
            <p className="text-xs text-gray-400 mb-4">Share of net worth per account</p>
            <AllocationPieChart allocations={allocations} />
          </div>
        )}
      </div>

      {/* Category breakdown */}
      {summary?.by_category && summary.by_category.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Expenses by Category</h3>
            <p className="text-xs text-gray-400 mb-4">Where money is spent</p>
            <CategoryDonutChart data={summary.by_category} type="expense" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Income by Category</h3>
            <p className="text-xs text-gray-400 mb-4">Where money comes from</p>
            <CategoryDonutChart data={summary.by_category} type="income" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Investments by Category</h3>
            <p className="text-xs text-gray-400 mb-4">Where capital is deployed</p>
            <CategoryDonutChart data={summary.by_category} type="investment" />
          </div>
        </div>
      )}

      {!summary && !latestBalance && (
        <div className="bg-white rounded-xl border border-dashed border-gray-300 p-16 text-center">
          <p className="text-gray-500 text-lg">No data yet.</p>
          <p className="text-gray-400 text-sm mt-2">Add transactions and balance snapshots to see your dashboard.</p>
        </div>
      )}
    </div>
  )
}
