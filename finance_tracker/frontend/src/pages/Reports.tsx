import { useState, useMemo } from 'react'
import { useTransactionSummary, useAllExpenses } from '../hooks/useTransactions'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import SavingsRateTrendChart from '../components/charts/SavingsRateTrendChart'
import NetCashFlowChart from '../components/charts/NetCashFlowChart'
import MonthlyExpenseCategoryChart from '../components/charts/MonthlyExpenseCategoryChart'
import CategoryTransactionsModal from '../components/ui/CategoryTransactionsModal'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro } from '../utils/format'
import { useDateRange } from '../context/DateRangeContext'
import type { Category, TransactionType } from '../types'

export default function Reports() {
  const { dateRange } = useDateRange()
  const [selectedCategory, setSelectedCategory] = useState<{
    category: Category
    type: TransactionType
  } | null>(null)
  const [selectedLabel, setSelectedLabel] = useState<string | null>(null)

  const { data: summary, isLoading } = useTransactionSummary(dateRange)
  const { data: allExpenses } = useAllExpenses(dateRange)

  // Spending grouped by label — a transaction with several labels counts in each.
  const labelSpend = useMemo(() => {
    const sums: Record<string, { total: number; count: number }> = {}
    for (const tx of allExpenses?.data ?? []) {
      for (const l of (tx.labels ?? '').split(',').filter(Boolean)) {
        if (!sums[l]) sums[l] = { total: 0, count: 0 }
        sums[l].total += tx.amount.value
        sums[l].count += 1
      }
    }
    return Object.entries(sums).sort((a, b) => b[1].total - a[1].total).slice(0, 14)
  }, [allExpenses])

  const savings = summary ? summary.total_income - summary.total_expenses : null
  const savingsRate = summary && summary.total_income > 0
    ? ((savings ?? 0) / summary.total_income) * 100
    : null

  const investmentRate = summary && summary.total_income > 0
    ? (summary.total_investments / summary.total_income) * 100
    : null

  const expenseRatio = summary && summary.total_income > 0
    ? (summary.total_expenses / summary.total_income) * 100
    : null

  const now = new Date()
  const completeMonths = summary?.by_month?.filter(
    (m) => !(m.year === now.getFullYear() && m.month === now.getMonth() + 1)
  ) ?? []

  const avgMonthlySpend = completeMonths.length > 0
    ? completeMonths.reduce((s, m) => s + m.expenses, 0) / completeMonths.length
    : null

  const avgMonthlySavings = completeMonths.length > 0
    ? completeMonths.reduce((s, m) => s + (m.income - m.expenses), 0) / completeMonths.length
    : null

  const positiveMonths = completeMonths.filter(m => m.income > m.expenses).length
  const totalMonths = completeMonths.length

  const topExpenseCategory = summary?.by_category
    .filter(c => c.type === 'expense')
    .sort((a, b) => b.total - a.total)[0]

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Reports</h2>
        <p className="text-sm text-gray-500 mt-1">Financial analytics and breakdowns</p>
      </div>

      {isLoading ? <LoadingSpinner /> : summary ? (
        <>
          {/* Summary KPIs */}
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-3">
              {[
                { label: 'Total Income', value: formatEuro(summary.total_income), color: 'text-green-600' },
                { label: 'Total Expenses', value: formatEuro(summary.total_expenses), color: 'text-red-600' },
                { label: 'Net Savings', value: savings != null ? formatEuro(savings) : '—', color: savings != null && savings >= 0 ? 'text-green-600' : 'text-red-600' },
                { label: 'Savings Rate', value: savingsRate != null ? `${savingsRate.toFixed(1)}%` : '—', color: savingsRate != null && savingsRate >= 20 ? 'text-green-600' : 'text-yellow-600' },
              ].map(({ label, value, color }) => (
                <div key={label}>
                  <p className="text-xs font-medium text-gray-500">{label}</p>
                  <p className={`text-lg sm:text-xl font-bold mt-0.5 ${color}`}>{value}</p>
                </div>
              ))}
            </div>
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
                    <span className={(savingsRate ?? 0) >= 20 ? 'text-green-600 font-semibold' : 'text-yellow-600'}>{savingsRate != null ? `${savingsRate.toFixed(1)}%` : '—'}</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full transition-all ${(savingsRate ?? 0) >= 20 ? 'bg-green-500' : (savingsRate ?? 0) >= 10 ? 'bg-yellow-400' : 'bg-red-400'}`} style={{ width: `${Math.min(savingsRate ?? 0, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{(savingsRate ?? 0) >= 20 ? 'Excellent — above 20% target' : (savingsRate ?? 0) >= 10 ? 'OK — aim for 20%+' : 'Low — under 10%'}</p>
                </div>

                {/* Investment rate */}
                <div>
                  <div className="flex justify-between text-xs text-gray-500 mb-1">
                    <span>Investment Rate</span>
                    <span className={(investmentRate ?? 0) >= 10 ? 'text-blue-600 font-semibold' : 'text-gray-500'}>{investmentRate != null ? `${investmentRate.toFixed(1)}%` : '—'}</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full ${(investmentRate ?? 0) >= 10 ? 'bg-blue-500' : 'bg-gray-300'}`} style={{ width: `${Math.min(investmentRate ?? 0, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{(investmentRate ?? 0) >= 10 ? 'Good — investing 10%+' : 'Consider investing more'}</p>
                </div>

                {/* Expense ratio */}
                <div>
                  <div className="flex justify-between text-xs text-gray-500 mb-1">
                    <span>Expense Ratio</span>
                    <span className={(expenseRatio ?? 101) <= 70 ? 'text-green-600 font-semibold' : 'text-red-500'}>{expenseRatio != null ? `${expenseRatio.toFixed(1)}%` : '—'}</span>
                  </div>
                  <div className="h-2 bg-gray-100 rounded-full overflow-hidden">
                    <div className={`h-full rounded-full ${(expenseRatio ?? 0) <= 70 ? 'bg-green-400' : (expenseRatio ?? 0) <= 85 ? 'bg-yellow-400' : 'bg-red-400'}`} style={{ width: `${Math.min(expenseRatio ?? 0, 100)}%` }} />
                  </div>
                  <p className="text-xs text-gray-400 mt-1">{(expenseRatio ?? 101) <= 70 ? 'Healthy — under 70%' : (expenseRatio ?? 101) <= 85 ? 'OK — aim to reduce' : 'High — exceeds 85%'}</p>
                </div>

                {/* Avg monthly savings */}
                <div>
                  <p className="text-xs text-gray-500">Avg Monthly Savings</p>
                  <p className={`text-lg font-bold mt-0.5 ${(avgMonthlySavings ?? 0) >= 0 ? 'text-green-600' : 'text-red-600'}`}>{avgMonthlySavings != null ? formatEuro(avgMonthlySavings) : '—'}</p>
                </div>

                {/* Avg monthly spend */}
                <div>
                  <p className="text-xs text-gray-500">Avg Monthly Spend</p>
                  <p className="text-lg font-bold mt-0.5 text-gray-800">{avgMonthlySpend != null ? formatEuro(avgMonthlySpend) : '—'}</p>
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
                <h3 className="text-base font-semibold text-gray-900 mb-1">Expense Categories</h3>
                <p className="text-xs text-gray-400 mb-4">Click a category to see its transactions</p>
                <CategoryDonutChart
                  data={summary.by_category}
                  type="expense"
                  onSelect={(category) => setSelectedCategory({ category, type: 'expense' })}
                />
              </div>
              <div className="bg-white rounded-xl border border-gray-200 p-6">
                <h3 className="text-base font-semibold text-gray-900 mb-1">Investment Categories</h3>
                <p className="text-xs text-gray-400 mb-4">Click a category to see its transactions</p>
                <CategoryDonutChart
                  data={summary.by_category}
                  type="investment"
                  onSelect={(category) => setSelectedCategory({ category, type: 'investment' })}
                />
              </div>
            </div>
          )}

          {/* Category detail table */}
          {summary.by_category?.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
              <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
                <h3 className="text-sm font-semibold text-gray-700">Category Detail</h3>
                <p className="text-xs text-gray-400 mt-0.5">Click a row to see its transactions</p>
              </div>
              <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className="text-left px-3 sm:px-4 py-2 font-semibold text-gray-600">Category</th>
                    <th className="text-left px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">Type</th>
                    <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600">Total</th>
                    <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">% of type</th>
                    <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600">Count</th>
                    <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">Avg</th>
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
                        <tr
                          key={i}
                          className="hover:bg-gray-50 cursor-pointer"
                          onClick={() => setSelectedCategory({ category: cat.category, type: cat.type })}
                        >
                          <td className="px-3 sm:px-4 py-2 text-gray-700">{cat.category}</td>
                          <td className="px-3 sm:px-4 py-2 hidden sm:table-cell">
                            <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${
                              cat.type === 'expense' ? 'bg-red-100 text-red-700' :
                              cat.type === 'income' ? 'bg-green-100 text-green-700' :
                              'bg-blue-100 text-blue-700'
                            }`}>
                              {cat.type}
                            </span>
                          </td>
                          <td className="px-3 sm:px-4 py-2 text-right font-semibold">{formatEuro(cat.total)}</td>
                          <td className="px-3 sm:px-4 py-2 text-right text-gray-500 hidden sm:table-cell">{pct.toFixed(1)}%</td>
                          <td className="px-3 sm:px-4 py-2 text-right text-gray-500">{cat.count}</td>
                          <td className="px-3 sm:px-4 py-2 text-right text-gray-500 hidden sm:table-cell">{formatEuro(cat.total / cat.count)}</td>
                        </tr>
                      )
                    })}
                </tbody>
              </table>
              </div>
            </div>
          )}
        </>
      ) : (
        <div className="bg-white rounded-xl border border-dashed border-gray-300 p-16 text-center">
          <p className="text-gray-500">No transaction data available yet.</p>
        </div>
      )}

      {/* Spending by label */}
      {labelSpend.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Spending by Label</h3>
          <p className="text-xs text-gray-400 mb-4">Labels across all categories in the period — click one to see its transactions</p>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-10 gap-y-2">
            {labelSpend.map(([label, { total, count }]) => {
              const max = labelSpend[0][1].total
              return (
                <button key={label} onClick={() => setSelectedLabel(label)} className="text-left group">
                  <div className="flex items-center justify-between text-sm mb-0.5">
                    <span className="font-medium text-indigo-700 group-hover:text-indigo-900">{label}</span>
                    <span className="text-gray-700 font-semibold">{formatEuro(total)} <span className="text-xs text-gray-400 font-normal">({count})</span></span>
                  </div>
                  <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                    <div className="h-full bg-indigo-400 rounded-full group-hover:bg-indigo-600" style={{ width: `${(total / max) * 100}%` }} />
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      )}

      {selectedLabel && (
        <CategoryTransactionsModal
          title={`Label: ${selectedLabel}`}
          label={selectedLabel}
          type="expense"
          dateRange={dateRange}
          onClose={() => setSelectedLabel(null)}
        />
      )}

      {selectedCategory && (
        <CategoryTransactionsModal
          category={selectedCategory.category}
          type={selectedCategory.type}
          dateRange={dateRange}
          onClose={() => setSelectedCategory(null)}
        />
      )}
    </div>
  )
}
