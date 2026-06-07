import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import { useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import { useBtcEur } from '../hooks/useBtcPrice'
import { freeCash, investments, pensions, cryptoEur, cryptoSubtitle } from '../utils/balanceGroups'
import StatCard from '../components/ui/StatCard'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import CumulativeSpendingChart from '../components/charts/CumulativeSpendingChart'
import SavingsRateTrendChart from '../components/charts/SavingsRateTrendChart'
import MonthlyExpenseCategoryChart from '../components/charts/MonthlyExpenseCategoryChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro } from '../utils/format'
import { useDateRange } from '../context/DateRangeContext'

export default function Dashboard() {
  const { dateRange } = useDateRange()

  const { price: liveBtcPrice } = useBtcEur()
  const { data: summary, isLoading: summaryLoading } = useTransactionSummary(dateRange)
  const { data: allTimeSummary } = useTransactionSummary({})
  const { data: latestBalance, isLoading: balanceLoading } = useLatestBalance(liveBtcPrice)
  const { data: trend, isLoading: trendLoading } = useBalanceTrend(dateRange)
  const { data: allocations, isLoading: allocLoading } = useAccountAllocation()
  const { data: allExpenses } = useAllExpenses(dateRange)

  const isLoading = summaryLoading || balanceLoading || trendLoading || allocLoading

  const netSaved = summary ? summary.total_income - summary.total_expenses : null

  const now = new Date()
  const completeMonths = summary?.by_month?.filter(
    (m) => !(m.year === now.getFullYear() && m.month === now.getMonth() + 1)
  ) ?? []

  // True when the selected period extends into the current calendar month
  const curYM = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const periodIncludesCurrentMonth = !dateRange.date_to || dateRange.date_to >= curYM

  // All-time complete months (excluding current) — used for median fallback
  const allTimeCompleteMonths = allTimeSummary?.by_month?.filter(
    (m) => !(m.year === now.getFullYear() && m.month === now.getMonth() + 1)
  ) ?? []

  // Median of per-month savings rates across all historical complete months
  const medianSavingsRate = (() => {
    if (allTimeCompleteMonths.length === 0) return null
    const rates = allTimeCompleteMonths
      .filter((m) => m.income > 0)
      .map((m) => ((m.income - m.expenses) / m.income) * 100)
      .sort((a, b) => a - b)
    if (rates.length === 0) return null
    const mid = Math.floor(rates.length / 2)
    return rates.length % 2 === 1 ? rates[mid] : (rates[mid - 1] + rates[mid]) / 2
  })()

  // Median monthly income — threshold for "salary has dropped this month"
  const allTimeMedianIncome = (() => {
    if (allTimeCompleteMonths.length === 0) return null
    const sorted = [...allTimeCompleteMonths].map((m) => m.income).sort((a, b) => a - b)
    const mid = Math.floor(sorted.length / 2)
    return sorted.length % 2 === 1 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
  })()

  // Current month entry in the period data (may exist even for "this month" filter)
  const currentMonthEntry = summary?.by_month?.find(
    (m) => m.year === now.getFullYear() && m.month === now.getMonth() + 1
  )
  // Salary considered dropped once current month income >= 50% of median
  const salaryDropped =
    currentMonthEntry != null &&
    allTimeMedianIncome != null &&
    allTimeMedianIncome > 0 &&
    currentMonthEntry.income >= allTimeMedianIncome * 0.5

  // Savings rate logic:
  // • Period doesn't touch current month → use raw summary totals
  // • Period includes current month AND salary has dropped → use summary totals (current data is reliable)
  // • Period includes current month AND salary still pending → use complete months from period,
  //   or fall back to median if none (e.g. "this month" view early in the month)
  const completeTotalIncome = completeMonths.reduce((s, m) => s + m.income, 0)
  const completeTotalExpenses = completeMonths.reduce((s, m) => s + m.expenses, 0)
  const savingsRate = (() => {
    if (!periodIncludesCurrentMonth) {
      return summary && summary.total_income > 0
        ? ((summary.total_income - summary.total_expenses) / summary.total_income) * 100
        : null
    }
    if (salaryDropped) {
      return summary && summary.total_income > 0
        ? ((summary.total_income - summary.total_expenses) / summary.total_income) * 100
        : null
    }
    // Salary still pending — use only complete months in the period
    return completeMonths.length > 0 && completeTotalIncome > 0
      ? ((completeTotalIncome - completeTotalExpenses) / completeTotalIncome) * 100
      : null // → medianSavingsRate fallback in the card
  })()

  const avgMonthlySpend = completeMonths.length > 0
    ? completeMonths.reduce((s, m) => s + m.expenses, 0) / completeMonths.length
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
    <div className="space-y-4 sm:space-y-8">

      {/* Balance KPI cards — always from latest snapshot */}
      {latestBalance && (
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4">
          <StatCard
            title="Net Worth"
            value={formatEuro(latestBalance.total)}
            subtitle="All accounts combined"
            color="blue"
          />
          <StatCard
            title="Free Cash"
            value={formatEuro(freeCash(latestBalance))}
            subtitle="Banks + Cash + Revolut"
            color="green"
          />
          <StatCard
            title="Investments"
            value={formatEuro(investments(latestBalance))}
            subtitle="ETF + Revolut + IBKR"
            color="blue"
          />
          <StatCard
            title="Pensions"
            value={formatEuro(pensions(latestBalance))}
            subtitle="2nd + 3rd Pillar"
            color="purple"
          />
          <StatCard
            title="Crypto"
            value={formatEuro(cryptoEur(latestBalance))}
            subtitle={cryptoSubtitle(latestBalance, liveBtcPrice)}
            color="yellow"
          />
        </div>
      )}

      {/* Period insights */}
      {summary && (
        <div>
          <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-3">
            {dateRange.date_from ? `From ${dateRange.date_from}${dateRange.date_to ? ` to ${dateRange.date_to}` : ''}` : 'All-time'} period insights
          </p>
          <div className="grid grid-cols-2 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
            <StatCard
              title="Net Saved"
              value={netSaved != null ? formatEuro(netSaved) : '—'}
              subtitle="Income minus expenses"
              color={netSaved != null && netSaved >= 0 ? 'green' : 'red'}
            />
            <StatCard
              title="Savings Rate"
              value={
                savingsRate != null
                  ? `${savingsRate.toFixed(1)}%`
                  : medianSavingsRate != null
                  ? `${medianSavingsRate.toFixed(1)}%`
                  : '—'
              }
              subtitle={
                savingsRate != null
                  ? (periodIncludesCurrentMonth && !salaryDropped ? 'Complete months only' : periodIncludesCurrentMonth && salaryDropped ? 'Month in progress' : 'Of income kept')
                  : medianSavingsRate != null
                  ? '⚠ Median rate — salary pending'
                  : 'No data yet'
              }
              color={
                savingsRate != null
                  ? (savingsRate >= 20 ? 'green' : savingsRate >= 0 ? 'yellow' : 'red')
                  : medianSavingsRate != null
                  ? (medianSavingsRate >= 20 ? 'green' : medianSavingsRate >= 0 ? 'yellow' : 'red')
                  : 'yellow'
              }
            />
            <StatCard
              title="Avg Monthly Spend"
              value={avgMonthlySpend != null ? formatEuro(avgMonthlySpend) : '—'}
              subtitle={`Over ${completeMonths.length} complete month${completeMonths.length !== 1 ? 's' : ''}`}
              color="blue"
            />
            <StatCard
              title="Net Worth Change"
              value={netWorthChange != null ? formatEuro(netWorthChange) : '—'}
              subtitle={netWorthChangePct != null ? `${netWorthChangePct >= 0 ? '+' : ''}${netWorthChangePct.toFixed(1)}% in period` : 'From first to last snapshot'}
              color={netWorthChange != null && netWorthChange >= 0 ? 'green' : 'red'}
            />
          </div>
          <div className="grid grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4 mt-3 sm:mt-4">
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

      {/* Net Worth Over Time */}
      {trend && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Net Worth Over Time</h3>
          <p className="text-xs text-gray-400 mb-4">Click legend items to show/hide accounts. Hover a line to highlight it.</p>
          <BalanceTrendChart trend={trend} btcPrice={liveBtcPrice} />
        </div>
      )}

      {/* Expense breakdown by category per month — numbers embedded in X-axis ticks */}
      {allExpenses && allExpenses.data.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Where Money Goes Each Month</h3>
          <p className="text-xs text-gray-400 mb-4">
            Stacked expense breakdown by category · axis shows <span className="text-green-700 font-medium">income</span> / <span className="text-red-600 font-medium">expenses</span>{summary?.by_month?.some((m) => m.investments > 0) ? <> / <span className="text-blue-700 font-medium">invested</span></> : null} per month
          </p>
          <MonthlyExpenseCategoryChart
            transactions={allExpenses.data}
            monthTotals={summary?.by_month}
          />
        </div>
      )}

      {/* Monthly spending pace */}
      {allExpenses && allExpenses.data.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Monthly Spending Pace</h3>
          <p className="text-xs text-gray-400 mb-4">Cumulative daily expenses per month — steeper slope = faster spending</p>
          <CumulativeSpendingChart transactions={allExpenses.data} />
        </div>
      )}

      {/* Savings rate trend + allocation */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
        {summary?.by_month && summary.by_month.length > 1 && (
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Savings Rate Trend</h3>
            <p className="text-xs text-gray-400 mb-4">Monthly % of income kept after expenses — 20% is the recommended minimum</p>
            <SavingsRateTrendChart data={summary.by_month} />
          </div>
        )}

        {allocations && allocations.length > 0 && (
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Current Allocation</h3>
            <p className="text-xs text-gray-400 mb-4">Share of net worth per account</p>
            <AllocationPieChart allocations={allocations} />
          </div>
        )}
      </div>

      {/* Monthly cash flow */}
      {summary?.by_month && summary.by_month.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Monthly Cash Flow</h3>
          <p className="text-xs text-gray-400 mb-4">Income vs expenses vs investments by month</p>
          <MonthlyBarChart data={summary.by_month} />
        </div>
      )}

      {/* Category breakdown */}
      {summary?.by_category && summary.by_category.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Expenses by Category</h3>
            <p className="text-xs text-gray-400 mb-4">Where money is spent</p>
            <CategoryDonutChart data={summary.by_category} type="expense" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-1">Income by Category</h3>
            <p className="text-xs text-gray-400 mb-4">Where money comes from</p>
            <CategoryDonutChart data={summary.by_category} type="income" />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
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
