import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import { useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import { useBtcEur } from '../hooks/useBtcPrice'
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

  const { data: summary, isLoading: summaryLoading } = useTransactionSummary(dateRange)
  const { data: latestBalance, isLoading: balanceLoading } = useLatestBalance()
  const { data: trend, isLoading: trendLoading } = useBalanceTrend(dateRange)
  const { data: allocations, isLoading: allocLoading } = useAccountAllocation()
  const { data: allExpenses } = useAllExpenses(dateRange)
  const { price: liveBtcPrice } = useBtcEur()
  const storedBtcPrice = latestBalance?.btc_price ?? 0
  const btcPrice: number | null = liveBtcPrice ?? (storedBtcPrice > 0 ? storedBtcPrice : null)

  const isLoading = summaryLoading || balanceLoading || trendLoading || allocLoading

  const savingsRate = summary && summary.total_income > 0
    ? ((summary.total_income - summary.total_expenses) / summary.total_income) * 100
    : null

  const netSaved = summary ? summary.total_income - summary.total_expenses : null

  const avgMonthlySpend = summary && summary.by_month?.length > 0
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

  const calMonths = summary?.by_month ?? []
  const calAvg = calMonths.length > 0
    ? calMonths.reduce((s, m) => s + m.expenses, 0) / calMonths.length
    : null
  const calMultiYear = calMonths.some((m) => m.year !== calMonths[0]?.year)

  return (
    <div className="space-y-4 sm:space-y-8">

      {/* Monthly calendar grid */}
      {calMonths.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <div className="flex items-baseline justify-between mb-3">
            <h3 className="text-base font-semibold text-gray-900">Monthly Overview</h3>
            <span className="text-xs text-gray-400">
              avg spend {calAvg != null ? formatEuro(calAvg) : '—'}/mo
            </span>
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-2">
            {calMonths.map((m) => {
              const aboveAvg = calAvg != null && m.expenses > calAvg
              const belowAvg = calAvg != null && m.expenses < calAvg
              const diffPct = calAvg != null && calAvg > 0
                ? ((m.expenses - calAvg) / calAvg) * 100
                : null
              const accentColor = aboveAvg ? 'bg-red-400' : belowAvg ? 'bg-green-400' : 'bg-gray-200'
              const borderColor = aboveAvg ? 'border-red-100' : belowAvg ? 'border-green-100' : 'border-gray-200'
              const expenseColor = aboveAvg ? 'text-red-600' : belowAvg ? 'text-green-600' : 'text-gray-700'
              const diffColor = aboveAvg ? 'text-red-400' : 'text-green-500'
              return (
                <div key={`${m.year}-${m.month}`} className={`rounded-lg border ${borderColor} overflow-hidden`}>
                  <div className={`${accentColor} h-0.5`} />
                  <div className="px-2.5 py-2">
                    <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400 mb-1.5">
                      {m.month_name.slice(0, 3)}{calMultiYear ? ` ${m.year}` : ''}
                    </p>
                    <div className="space-y-0.5">
                      <div className="flex items-baseline justify-between gap-1">
                        <span className="text-[10px] text-gray-400 shrink-0">Inc</span>
                        <span className="text-xs font-semibold text-green-700 tabular-nums">{formatEuro(m.income)}</span>
                      </div>
                      <div className="flex items-baseline justify-between gap-1">
                        <span className="text-[10px] text-gray-400 shrink-0">Exp</span>
                        <span className={`text-xs font-semibold tabular-nums ${expenseColor}`}>{formatEuro(m.expenses)}</span>
                      </div>
                      {m.investments > 0 && (
                        <div className="flex items-baseline justify-between gap-1">
                          <span className="text-[10px] text-gray-400 shrink-0">Inv</span>
                          <span className="text-xs font-semibold text-blue-700 tabular-nums">{formatEuro(m.investments)}</span>
                        </div>
                      )}
                    </div>
                    {diffPct != null && (
                      <p className={`text-[10px] font-medium mt-1 ${diffColor}`}>
                        {diffPct >= 0 ? '+' : ''}{diffPct.toFixed(0)}%
                      </p>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

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
            value={formatEuro(latestBalance.seb + latestBalance.swed + latestBalance.luminor + latestBalance.cash + latestBalance.rev_m + latestBalance.rev_r)}
            subtitle="Banks + Cash + Revolut"
            color="green"
          />
          <StatCard
            title="Investments"
            value={formatEuro(latestBalance.swed_etf + latestBalance.rev_stocks + latestBalance.ibkr_stocks)}
            subtitle="ETF + Revolut + IBKR"
            color="blue"
          />
          <StatCard
            title="Pensions"
            value={formatEuro(latestBalance.seb_pen + latestBalance.art)}
            subtitle="2nd + 3rd Pillar"
            color="purple"
          />
          <StatCard
            title="Crypto"
            value={formatEuro(
              btcPrice != null
                ? (latestBalance.r_btc + latestBalance.m_btc) * btcPrice
                : (latestBalance.r_btc_eur ?? 0) + (latestBalance.m_btc_eur ?? 0)
            )}
            subtitle={btcPrice != null
              ? `${(latestBalance.r_btc + latestBalance.m_btc).toFixed(8)} BTC · €${btcPrice.toLocaleString()} /BTC`
              : `${(latestBalance.r_btc + latestBalance.m_btc).toFixed(8)} BTC`}
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
              value={savingsRate != null ? `${savingsRate.toFixed(1)}%` : '—'}
              subtitle="Of income kept"
              color={savingsRate != null && savingsRate >= 20 ? 'green' : savingsRate != null && savingsRate >= 0 ? 'yellow' : 'red'}
            />
            <StatCard
              title="Avg Monthly Spend"
              value={avgMonthlySpend != null ? formatEuro(avgMonthlySpend) : '—'}
              subtitle={`Over ${summary.by_month?.length ?? 0} month${(summary.by_month?.length ?? 0) !== 1 ? 's' : ''}`}
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
          <BalanceTrendChart trend={trend} btcPrice={btcPrice} />
        </div>
      )}

      {/* Expense breakdown by category per month */}
      {allExpenses && allExpenses.data.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Where Money Goes Each Month</h3>
          <p className="text-xs text-gray-400 mb-4">Stacked expense breakdown by category — see which categories dominate each month</p>
          <MonthlyExpenseCategoryChart transactions={allExpenses.data} />
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
