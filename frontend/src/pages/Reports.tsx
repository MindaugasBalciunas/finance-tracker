import { useState } from 'react'
import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import SavingsRateTrendChart from '../components/charts/SavingsRateTrendChart'
import NetCashFlowChart from '../components/charts/NetCashFlowChart'
import MonthlyExpenseCategoryChart from '../components/charts/MonthlyExpenseCategoryChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import DateRangeFilter from '../components/ui/DateRangeFilter'
import type { DateRange } from '../components/ui/DateRangeFilter'
import { formatEuro } from '../utils/format'

export default function Reports() {
  const [dateRange, setDateRange] = useState<DateRange>({})

  const { data: summary, isLoading } = useTransactionSummary(dateRange)
  const { data: allExpenses } = useAllExpenses(dateRange)

  const savings = summary ? summary.total_income - summary.total_expenses : 0
  const savingsRate = summary && summary.total_income > 0
    ? (savings / summary.total_income) * 100
    : 0

  const investmentRate = summary && summary.total_income > 0
    ? (summary.total_investments / summary.total_income) * 100
    : 0

  const expenseRatio = summary && summary.total_income > 0
    ? (summary.total_expenses / summary.total_income) * 100
    : 0

  const avgMonthlySpend = summary && summary.by_month.length > 0
    ? summary.total_expenses / summary.by_month.length
    : 0

  const avgMonthlySavings = summary && summary.by_month.length > 0
    ? savings / summary.by_month.length
    : 0

  const positiveMonths = summary?.by_month.filter(m => m.income > m.expenses).length ?? 0
  const totalMonths = summary?.by_month.length ?? 0

  const topExpenseCategory = summary?.by_category
    .filter(c => c.type === 'expense')
    .sort((a, b) => b.total - a.total)[0]

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between flex-wrap gap-4">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Reports</h2>
          <p className="text-sm text-gray-500 mt-1">Financial analytics and breakdowns</p>
        </div>
        <DateRangeFilter value={dateRange} onChange={setDateRange} />
      </div>

      {isLoading ? <LoadingSpinner /> : summary ? (
        <>
          {/* Summary KPIs */}
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
            {[
              { label: 'Total Income', value: formatEuro(summary.total_income), color: 'text-green-600' },
              { label: 'Total Expenses', value: formatEuro(summary.total_expenses), color: 'text-red-600' },
              { label: 'Net Savings', value: formatEuro(savings), color: savings >= 0 ? 'text-green-600' : 'text-red-600' },
              { label: 'Savings Rate', value: `${savingsRate.toFixed(1)}%`, color: savingsRate >= 20 ? 'text-green-600' : 'text-yellow-600' },
            ].map(({ label, value, color }) => (
              <div key={label} className="bg-white rounded-xl border border-gray-200 p-5">
                <p className="text-sm font-medium text-gray-500">{label}</p>
                <p className={`text-2xl font-bold mt-1 ${color}`}>{value}</p>
              </div>
            ))}
          </div>

          {/* Financial health panel */}
          {summary.total_income > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-6">
              <h3 className="text-base font-semibold text-gray-900 mb-4">Financial Health</h3>
              <div className="grid grid-cols-2 lg:grid-cols-3 gap-6">

                {/* Savings rate indicator */}
                <div>
                  <div className="flex justify-between text-xs text-gray-500 mb-1">
                    <span>Savings Rate</span>
                    <span className={savingsRate >= 20 ? 'text-green-600 font-semibold' : 'text-yellow-600'}>{savingsRate.toFixed(1)}%</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full transition-all ${savingsRate >= 20 ? 'bg-green-500' : savingsRate >= 10 ? 'bg-yellow-400' : 'bg-red-400'}`} style={{ width: `${Math.min(savingsRate, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{savingsRate >= 20 ? 'Excellent — above 20% target' : savingsRate >= 10 ? 'OK — aim for 20%+' : 'Low — under 10%'}</p>
                </div>

                {/* Investment rate */}
                <div>
                  <div className="flex justify-between text-xs text-gray-500 mb-1">
                    <span>Investment Rate</span>
                    <span className={investmentRate >= 10 ? 'text-blue-600 font-semibold' : 'text-gray-500'}>{investmentRate.toFixed(1)}%</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full ${investmentRate >= 10 ? 'bg-blue-500' : 'bg-gray-300'}`} style={{ width: `${Math.min(investmentRate, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{investmentRate >= 10 ? 'Good — investing 10%+' : 'Consider investing more'}</p>
                </div>

                {/* Expense ratio */}
                <div>
                  <div className="flex justify-between text-xs text-gray-500 mb-1">
                    <span>Expense Ratio</span>
                    <span className={expenseRatio <= 70 ? 'text-green-600 font-semibold' : 'text-red-500'}>{expenseRatio.toFixed(1)}%</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full ${expenseRatio <= 70 ? 'bg-green-400' : expenseRatio <= 85 ? 'bg-yellow-400' : 'bg-red-400'}`} style={{ width: `${Math.min(expenseRatio, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{expenseRatio <= 70 ? 'Healthy — under 70%' : expenseRatio <= 85 ? 'OK — aim to reduce' : 'High — exceeds 85%'}</p>
                </div>

                {/* Avg monthly savings */}
                <div>
                  <p className="text-xs text-gray-500">Avg Monthly Savings</p>
                  <p className={`text-lg font-bold mt-0.5 ${avgMonthlySavings >= 0 ? 'text-green-600' : 'text-red-600'}`}>{formatEuro(avgMonthlySavings)}</p>
                </div>

                {/* Avg monthly spend */}
                <div>
                  <p className="text-xs text-gray-500">Avg Monthly Spend</p>
                  <p className="text-lg font-bold mt-0.5 text-gray-800">{formatEuro(avgMonthlySpend)}</p>
                </div>

                {/* Positive months */}
                {totalMonths > 0 && (
                  <div>
                    <p className="text-xs text-gray-500">Profitable Months</p>
                    <p className="text-lg font-bold mt-0.5 text-gray-800">
                      {positiveMonths}/{totalMonths}
                      <span className={`text-xs font-normal ml-1 ${positiveMonths / totalMonths >= 0.75 ? 'text-green-600' : 'text-yellow-600'}`}>
                        ({((positiveMonths / totalMonths) * 100).toFixed(0)}%)
                      </span>
                    </p>
                  </div>
                )}
              </div>

              {topExpenseCategory && (
                <div className="mt-4 pt-4 border-t border-gray-100 flex items-center gap-2 text-xs text-gray-500">
                  <span>Top expense category:</span>
                  <span className="font-semibold text-red-600">{topExpenseCategory.category}</span>
                  <span>—</span>
                  <span>{formatEuro(topExpenseCategory.total)} ({((topExpenseCategory.total / summary.total_expenses) * 100).toFixed(1)}% of all expenses)</span>
                </div>
              )}
            </div>
          )}

          {/* Expense breakdown by category per month */}
          {allExpenses && allExpenses.data.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-6">
              <h3 className="text-base font-semibold text-gray-900 mb-1">Expense Breakdown by Month</h3>
              <p className="text-xs text-gray-400 mb-4">Stacked by category — see how spending composition changes over time</p>
              <MonthlyExpenseCategoryChart transactions={allExpenses.data} />
            </div>
          )}

          {/* Net cash flow + savings rate */}
          {summary.by_month?.length > 1 && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <div className="bg-white rounded-xl border border-gray-200 p-6">
                <h3 className="text-base font-semibold text-gray-900 mb-1">Net Cash Flow by Month</h3>
                <p className="text-xs text-gray-400 mb-4">Green = saved money, red = spent more than earned</p>
                <NetCashFlowChart data={summary.by_month} />
              </div>
              <div className="bg-white rounded-xl border border-gray-200 p-6">
                <h3 className="text-base font-semibold text-gray-900 mb-1">Savings Rate Trend</h3>
                <p className="text-xs text-gray-400 mb-4">Monthly % of income kept — 20% is the recommended minimum</p>
                <SavingsRateTrendChart data={summary.by_month} />
              </div>
            </div>
          )}

          {/* Monthly income/expense/investment bar chart */}
          {summary.by_month?.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-6">
              <h3 className="text-base font-semibold text-gray-900 mb-4">Monthly Breakdown</h3>
              <MonthlyBarChart data={summary.by_month} />
            </div>
          )}

          {/* Category donuts */}
          {summary.by_category?.length > 0 && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <div className="bg-white rounded-xl border border-gray-200 p-6">
                <h3 className="text-base font-semibold text-gray-900 mb-4">Expense Categories</h3>
                <CategoryDonutChart data={summary.by_category} type="expense" />
              </div>
              <div className="bg-white rounded-xl border border-gray-200 p-6">
                <h3 className="text-base font-semibold text-gray-900 mb-4">Investment Categories</h3>
                <CategoryDonutChart data={summary.by_category} type="investment" />
              </div>
            </div>
          )}

          {/* Category detail table */}
          {summary.by_category?.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
              <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
                <h3 className="text-sm font-semibold text-gray-700">Category Detail</h3>
              </div>
              <table className="w-full text-sm">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className="text-left px-4 py-2 font-semibold text-gray-600">Category</th>
                    <th className="text-left px-4 py-2 font-semibold text-gray-600">Type</th>
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">Total</th>
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">% of type</th>
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">Count</th>
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">Avg</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {[...summary.by_category]
                    .sort((a, b) => b.total - a.total)
                    .map((cat, i) => {
                      const typeTotal = cat.type === 'expense'
                        ? summary.total_expenses
                        : cat.type === 'income'
                        ? summary.total_income
                        : summary.total_investments
                      const pct = typeTotal > 0 ? (cat.total / typeTotal) * 100 : 0
                      return (
                        <tr key={i} className="hover:bg-gray-50">
                          <td className="px-4 py-2 text-gray-700">{cat.category}</td>
                          <td className="px-4 py-2">
                            <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${
                              cat.type === 'expense' ? 'bg-red-100 text-red-700' :
                              cat.type === 'income' ? 'bg-green-100 text-green-700' :
                              'bg-blue-100 text-blue-700'
                            }`}>
                              {cat.type}
                            </span>
                          </td>
                          <td className="px-4 py-2 text-right font-semibold">{formatEuro(cat.total)}</td>
                          <td className="px-4 py-2 text-right text-gray-500">{pct.toFixed(1)}%</td>
                          <td className="px-4 py-2 text-right text-gray-500">{cat.count}</td>
                          <td className="px-4 py-2 text-right text-gray-500">{formatEuro(cat.total / cat.count)}</td>
                        </tr>
                      )
                    })}
                </tbody>
              </table>
            </div>
          )}
        </>
      ) : (
        <div className="bg-white rounded-xl border border-dashed border-gray-300 p-16 text-center">
          <p className="text-gray-500">No transaction data available yet.</p>
        </div>
      )}
    </div>
  )
}
