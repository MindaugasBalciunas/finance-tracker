import { useState } from 'react'
import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import { useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import StatCard from '../components/ui/StatCard'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import CumulativeSpendingChart from '../components/charts/CumulativeSpendingChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro } from '../utils/format'

export default function Dashboard() {
  const [trendRange] = useState<{ date_from?: string; date_to?: string }>({})

  const { data: summary, isLoading: summaryLoading } = useTransactionSummary()
  const { data: latestBalance, isLoading: balanceLoading } = useLatestBalance()
  const { data: trend, isLoading: trendLoading } = useBalanceTrend(trendRange)
  const { data: allocations, isLoading: allocLoading } = useAccountAllocation()
  const { data: allExpenses } = useAllExpenses()

  const isLoading = summaryLoading || balanceLoading || trendLoading || allocLoading

  if (isLoading) return <LoadingSpinner message="Loading dashboard..." />

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Dashboard</h2>
        <p className="text-sm text-gray-500 mt-1">Your financial overview</p>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Net Worth"
          value={latestBalance ? formatEuro(latestBalance.total) : '—'}
          subtitle="Latest balance snapshot"
          color="blue"
        />
        <StatCard
          title="Total Income"
          value={summary ? formatEuro(summary.total_income) : '—'}
          subtitle="All recorded income"
          color="green"
        />
        <StatCard
          title="Total Expenses"
          value={summary ? formatEuro(summary.total_expenses) : '—'}
          subtitle="All recorded expenses"
          color="red"
        />
        <StatCard
          title="Invested"
          value={summary ? formatEuro(summary.total_investments) : '—'}
          subtitle="Total investments"
          color="purple"
        />
      </div>

      {/* Balance trend */}
      {trend && (
        <div className="bg-white rounded-xl border border-gray-200 p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-4">Net Worth Over Time</h3>
          <BalanceTrendChart trend={trend} />
        </div>
      )}

      {/* Cumulative spending comparison */}
      {allExpenses && allExpenses.data.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Monthly Spending Pace</h3>
          <p className="text-xs text-gray-400 mb-4">Cumulative expenses by day — compare spending speed across months</p>
          <CumulativeSpendingChart transactions={allExpenses.data} />
        </div>
      )}

      {/* Monthly income vs expenses + allocation */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {summary?.by_month && summary.by_month.length > 0 && (
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Monthly Overview</h3>
            <MonthlyBarChart data={summary.by_month} />
          </div>
        )}

        {allocations && allocations.length > 0 && (
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Account Allocation</h3>
            <AllocationPieChart allocations={allocations} />
          </div>
        )}
      </div>

      {/* Expense breakdown */}
      {summary?.by_category && summary.by_category.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Expenses by Category</h3>
            <CategoryDonutChart data={summary.by_category} type="expense" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Income by Category</h3>
            <CategoryDonutChart data={summary.by_category} type="income" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Investments by Category</h3>
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
