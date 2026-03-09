import { useState } from 'react'
import { useTransactionSummary } from '../hooks/useTransactions'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro } from '../utils/format'

export default function Reports() {
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

  const filter = {
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
  }

  const { data: summary, isLoading } = useTransactionSummary(filter)

  const savings = summary ? summary.total_income - summary.total_expenses : 0
  const savingsRate = summary && summary.total_income > 0
    ? (savings / summary.total_income) * 100
    : 0

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">Reports</h2>
        <p className="text-sm text-gray-500 mt-1">Financial analytics and breakdowns</p>
      </div>

      {/* Date range filter */}
      <div className="bg-white rounded-xl border border-gray-200 p-4 flex items-center gap-4">
        <span className="text-sm font-medium text-gray-600">Period:</span>
        <input
          type="date"
          value={dateFrom}
          onChange={(e) => setDateFrom(e.target.value)}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        />
        <span className="text-gray-400">to</span>
        <input
          type="date"
          value={dateTo}
          onChange={(e) => setDateTo(e.target.value)}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        />
        <button
          onClick={() => { setDateFrom(''); setDateTo('') }}
          className="text-sm text-gray-500 hover:text-gray-800 underline"
        >
          All time
        </button>
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

          {/* Monthly chart */}
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

          {/* Category table */}
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
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">Count</th>
                    <th className="text-right px-4 py-2 font-semibold text-gray-600">Avg</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {[...summary.by_category]
                    .sort((a, b) => b.total - a.total)
                    .map((cat, i) => (
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
                        <td className="px-4 py-2 text-right text-gray-500">{cat.count}</td>
                        <td className="px-4 py-2 text-right text-gray-500">{formatEuro(cat.total / cat.count)}</td>
                      </tr>
                    ))}
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
