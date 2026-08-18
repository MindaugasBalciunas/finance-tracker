import { useState, useMemo } from 'react'
import { useTransactionSummary, useAllExpenses, useAllTransactions } from '../hooks/useTransactions'
import MonthlyBarChart from '../components/charts/MonthlyBarChart'
import CategoryDonutChart from '../components/charts/CategoryDonutChart'
import SavingsRateTrendChart from '../components/charts/SavingsRateTrendChart'
import NetCashFlowChart from '../components/charts/NetCashFlowChart'
import MonthlyExpenseCategoryChart from '../components/charts/MonthlyExpenseCategoryChart'
import MonthlyLabelChart from '../components/charts/MonthlyLabelChart'
import CategoryTransactionsModal from '../components/ui/CategoryTransactionsModal'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import { formatEuro } from '../utils/format'
import { useDateRange } from '../context/DateRangeContext'
import { txLabels, FIXED_LABELS } from '../utils/labels'
import type { Category, TransactionType } from '../types'

// Grocery-store labels applied by backend rules; shown in their own
// comparison card (and therefore excluded from the generic label list).
const STORE_NAMES: Record<string, string> = {
  maxima: 'Maxima',
  lidl: 'Lidl',
  iki: 'IKI',
  rimi: 'Rimi',
  norfa: 'Norfa',
  barbora: 'Barbora (delivery)',
}

// Clickable table header for client-side sorting: first click applies the
// column's natural direction, second click flips it.
type SortState = { key: string; dir: 1 | -1 }
function SortTh({ label, k, sort, setSort, natural = -1, className = '' }: {
  label: string
  k: string
  sort: SortState
  setSort: (s: SortState) => void
  natural?: 1 | -1
  className?: string
}) {
  const active = sort.key === k
  return (
    <th className={`px-3 sm:px-4 py-2 font-semibold ${className}`}>
      <button
        onClick={() => setSort(active ? { key: k, dir: sort.dir === 1 ? -1 : 1 } : { key: k, dir: natural })}
        className={`inline-flex items-center gap-0.5 ${active ? 'text-gray-900' : 'text-gray-600'} hover:text-gray-900`}
        title={`Sort by ${label.toLowerCase()}`}
      >
        {label}
        <span className="w-3 text-center text-xs">{active ? (sort.dir === 1 ? '↑' : '↓') : ''}</span>
      </button>
    </th>
  )
}

function sortRows<T>(rows: T[], sort: SortState, get: Record<string, (r: T) => string | number>): T[] {
  const accessor = get[sort.key]
  if (!accessor) return rows
  return [...rows].sort((a, b) => {
    const av = accessor(a), bv = accessor(b)
    const cmp = typeof av === 'string' ? av.localeCompare(String(bv)) : Number(av) - Number(bv)
    return cmp * sort.dir
  })
}

export default function Reports() {
  const { dateRange } = useDateRange()
  const [selectedCategory, setSelectedCategory] = useState<{
    category: Category
    type: TransactionType
  } | null>(null)
  const [selectedLabel, setSelectedLabel] = useState<{
    label?: string
    type: TransactionType
    title?: string
    unlabeled?: boolean
  } | null>(null)
  const [showAllLabels, setShowAllLabels] = useState(false)
  const [catSort, setCatSort] = useState<SortState>({ key: 'total', dir: -1 })
  const [labelSort, setLabelSort] = useState<SortState>({ key: 'total', dir: -1 })

  const { data: summary, isLoading, isError, error, refetch } = useTransactionSummary(dateRange)
  const { data: allExpenses } = useAllExpenses(dateRange)
  const { data: allIncome } = useAllTransactions({ type: 'income', ...dateRange })

  // Per-label expense stats — a transaction with several labels counts in
  // each (labels are overlapping views, not splits), so label sums must never
  // be added together; transaction-level subtotals are tracked separately.
  const labelStats = useMemo(() => {
    type Stat = { label: string; total: number; count: number; discTotal: number; discCount: number; perMonth: Record<string, number>; committed: boolean; store: boolean }
    const sums: Record<string, Stat> = {}
    const monthSet = new Set<string>()
    const discMonthSet = new Set<string>()
    let expenseTotal = 0
    let committedTotal = 0
    let unlabeledSum = 0
    let unlabeledCount = 0
    for (const tx of allExpenses?.data ?? []) {
      const mk = tx.date.slice(0, 7)
      monthSet.add(mk)
      expenseTotal += tx.amount.value
      const labels = txLabels(tx)
      if (labels.length === 0) {
        unlabeledSum += tx.amount.value
        unlabeledCount += 1
        discMonthSet.add(mk)
        continue
      }
      const committedTx = labels.some((l) => FIXED_LABELS.includes(l))
      if (committedTx) committedTotal += tx.amount.value
      else discMonthSet.add(mk)
      for (const l of labels) {
        if (!sums[l]) sums[l] = { label: l, total: 0, count: 0, discTotal: 0, discCount: 0, perMonth: {}, committed: FIXED_LABELS.includes(l), store: !!STORE_NAMES[l] }
        sums[l].total += tx.amount.value
        sums[l].count += 1
        sums[l].perMonth[mk] = (sums[l].perMonth[mk] ?? 0) + tx.amount.value
        // Money from committed transactions must not resurface in the
        // Discretionary list via an innocent co-label (e.g. "loan,house").
        if (!committedTx) {
          sums[l].discTotal += tx.amount.value
          sums[l].discCount += 1
        }
      }
    }
    const all = Object.values(sums).sort((a, b) => b.total - a.total)
    return {
      all,
      months: [...monthSet].sort(),
      // Months that contain non-committed spending — the label trend chart
      // excludes fixed obligations, so its guard must count these, not all.
      discMonthCount: discMonthSet.size,
      expenseTotal,
      committedTotal,
      discretionaryTotal: expenseTotal - committedTotal - unlabeledSum,
      unlabeledSum,
      unlabeledCount,
    }
  }, [allExpenses])

  // Latest data month vs the average of the earlier months in the period.
  const labelTrend = (perMonth: Record<string, number>): number | null => {
    const { months } = labelStats
    if (months.length < 2) return null
    const curKey = months[months.length - 1]
    const prior = months.slice(0, -1)
    const cur = perMonth[curKey] ?? 0
    const avg = prior.reduce((s, m) => s + (perMonth[m] ?? 0), 0) / prior.length
    if (avg < 10 && cur < 10) return null
    if (avg <= 0) return null
    return ((cur - avg) / avg) * 100
  }

  // Income grouped by label — employer labels make this an income-source view.
  const incomeByLabel = useMemo(() => {
    type Row = { label: string; total: number; count: number; first: string; last: string; monthCount: number }
    const sums: Record<string, Row & { months: Set<string> }> = {}
    let incomeTotal = 0
    for (const tx of allIncome?.data ?? []) {
      incomeTotal += tx.amount.value
      for (const l of txLabels(tx)) {
        if (!sums[l]) sums[l] = { label: l, total: 0, count: 0, first: tx.date, last: tx.date, monthCount: 0, months: new Set() }
        const s = sums[l]
        s.total += tx.amount.value
        s.count += 1
        if (tx.date < s.first) s.first = tx.date
        if (tx.date > s.last) s.last = tx.date
        s.months.add(tx.date.slice(0, 7))
      }
    }
    const rows = Object.values(sums)
      .map(({ months, ...r }) => ({ ...r, monthCount: months.size }))
      .sort((a, b) => b.total - a.total)
    return { rows, incomeTotal }
  }, [allIncome])

  // Per-store totals, visit counts and average basket size.
  const storeSpend = useMemo(() => {
    const sums: Record<string, { total: number; count: number }> = {}
    for (const tx of allExpenses?.data ?? []) {
      for (const l of txLabels(tx)) {
        if (!STORE_NAMES[l]) continue
        if (!sums[l]) sums[l] = { total: 0, count: 0 }
        sums[l].total += tx.amount.value
        sums[l].count += 1
      }
    }
    return Object.entries(sums)
      .map(([store, s]) => ({ store, ...s, avg: s.total / s.count }))
      .sort((a, b) => b.total - a.total)
  }, [allExpenses])

  // Basket comparison only means something with a few visits per store.
  const comparableStores = storeSpend.filter(s => s.count >= 5)
  const cheapestBasket = comparableStores.length >= 2
    ? [...comparableStores].sort((a, b) => a.avg - b.avg)[0]
    : null
  const priciestBasket = comparableStores.length >= 2
    ? [...comparableStores].sort((a, b) => b.avg - a.avg)[0]
    : null

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

      {isError ? <QueryError error={error} onRetry={() => refetch()} /> : isLoading ? <LoadingSpinner /> : summary ? (
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
                    <SortTh label="Category" k="category" sort={catSort} setSort={setCatSort} natural={1} className="text-left" />
                    <th className="text-left px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">Type</th>
                    <SortTh label="Total" k="total" sort={catSort} setSort={setCatSort} className="text-right" />
                    <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">% of type</th>
                    <SortTh label="Count" k="count" sort={catSort} setSort={setCatSort} className="text-right" />
                    <SortTh label="Avg" k="avg" sort={catSort} setSort={setCatSort} className="text-right hidden sm:table-cell" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {sortRows([...summary.by_category], catSort, {
                    category: (c) => c.category,
                    total: (c) => c.total,
                    count: (c) => c.count,
                    avg: (c) => c.count > 0 ? c.total / c.count : 0,
                  })
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

      {/* Label overview: coverage + discretionary/committed split */}
      {(labelStats.all.length > 0 || labelStats.unlabeledSum > 0) && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">🏷 Spending by Label</h3>
          <p className="text-xs text-gray-400 mb-4">Labels across all categories in the period — click any label to see its transactions</p>

          {/* Label coverage */}
          {labelStats.expenseTotal > 0 && (
            <div className="mb-5">
              {(() => {
                const labeledPct = ((labelStats.expenseTotal - labelStats.unlabeledSum) / labelStats.expenseTotal) * 100
                return (
                  <>
                    <div className="flex items-center justify-between text-xs text-gray-500 mb-1">
                      <span>Label coverage — {labeledPct.toFixed(0)}% of spending is labeled</span>
                      {labelStats.unlabeledSum > 0 && (
                        <button
                          onClick={() => setSelectedLabel({ type: 'expense', unlabeled: true, title: 'Unlabeled expenses' })}
                          className="text-blue-600 hover:underline"
                        >
                          {formatEuro(labelStats.unlabeledSum)} unlabeled ({labelStats.unlabeledCount}) →
                        </button>
                      )}
                    </div>
                    <div className="h-2 bg-gray-200 rounded-full overflow-hidden">
                      <div className={`h-full rounded-full ${labeledPct >= 90 ? 'bg-green-500' : labeledPct >= 70 ? 'bg-yellow-400' : 'bg-red-400'}`} style={{ width: `${labeledPct}%` }} />
                    </div>
                  </>
                )
              })()}
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-10 gap-y-5">
            {/* Discretionary labels */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <h4 className="text-sm font-semibold text-gray-700">Discretionary</h4>
                <span className="text-xs text-gray-400">{formatEuro(labelStats.discretionaryTotal)} of choices</span>
              </div>
              <div className="space-y-2">
                {(() => {
                  const disc = labelStats.all
                    .filter((s) => !s.committed && !s.store && s.discTotal > 0)
                    .sort((a, b) => b.discTotal - a.discTotal)
                  const shown = showAllLabels ? disc : disc.slice(0, 10)
                  const max = disc[0]?.discTotal ?? 1
                  return (
                    <>
                      {shown.map((s) => {
                        const trend = labelTrend(s.perMonth)
                        return (
                          <button key={s.label} onClick={() => setSelectedLabel({ label: s.label, type: 'expense' })} className="w-full text-left group">
                            <div className="flex items-center justify-between text-sm mb-0.5">
                              <span className="font-medium text-indigo-700 group-hover:text-indigo-900">{s.label}</span>
                              <span className="text-gray-700 font-semibold">
                                {trend != null && Math.abs(trend) >= 15 && (
                                  <span className={`text-xs font-semibold mr-1.5 ${trend > 0 ? 'text-amber-600' : 'text-green-600'}`}>
                                    {trend > 0 ? '▲' : '▼'}{Math.abs(trend) >= 995 ? '>10x' : `${Math.abs(trend).toFixed(0)}%`}
                                  </span>
                                )}
                                {formatEuro(s.discTotal)} <span className="text-xs text-gray-400 font-normal">({s.discCount})</span>
                              </span>
                            </div>
                            <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                              <div className="h-full bg-indigo-400 rounded-full group-hover:bg-indigo-600" style={{ width: `${(s.discTotal / max) * 100}%` }} />
                            </div>
                          </button>
                        )
                      })}
                      {disc.length > 10 && (
                        <button onClick={() => setShowAllLabels((v) => !v)} className="text-xs text-blue-600 hover:underline">
                          {showAllLabels ? 'Show top 10' : `Show all ${disc.length} labels`}
                        </button>
                      )}
                    </>
                  )
                })()}
              </div>
            </div>

            {/* Committed labels (fixed obligations) */}
            {labelStats.all.some((s) => s.committed) && (
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-semibold text-gray-700">🔒 Committed</h4>
                  <span className="text-xs text-gray-400">{formatEuro(labelStats.committedTotal)} pre-decided</span>
                </div>
                <div className="space-y-2">
                  {(() => {
                    const fixed = labelStats.all.filter((s) => s.committed)
                    const max = fixed[0]?.total ?? 1
                    return fixed.map((s) => (
                      <button key={s.label} onClick={() => setSelectedLabel({ label: s.label, type: 'expense' })} className="w-full text-left group">
                        <div className="flex items-center justify-between text-sm mb-0.5">
                          <span className="font-medium text-slate-600 group-hover:text-slate-900">{s.label}</span>
                          <span className="text-gray-700 font-semibold">{formatEuro(s.total)} <span className="text-xs text-gray-400 font-normal">({s.count})</span></span>
                        </div>
                        <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                          <div className="h-full bg-slate-400 rounded-full group-hover:bg-slate-600" style={{ width: `${(s.total / max) * 100}%` }} />
                        </div>
                      </button>
                    ))
                  })()}
                </div>
                <p className="text-[11px] text-gray-400 mt-2">
                  Fixed obligations ({FIXED_LABELS.join(', ')}) — kept apart so they don't drown out spending choices.
                </p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Label trend over months */}
      {(allExpenses?.data.length ?? 0) > 0 && labelStats.discMonthCount > 1 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">Labels by Month</h3>
          <p className="text-xs text-gray-400 mb-4">
            Discretionary spending stacked by label — each transaction counted under its first label, fixed obligations excluded
          </p>
          <MonthlyLabelChart transactions={allExpenses!.data} />
        </div>
      )}

      {/* Label detail table */}
      {labelStats.all.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
            <h3 className="text-sm font-semibold text-gray-700">Label Detail</h3>
            <p className="text-xs text-gray-400 mt-0.5">Every label in the period — click a row to see its transactions</p>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <SortTh label="Label" k="label" sort={labelSort} setSort={setLabelSort} natural={1} className="text-left" />
                  <SortTh label="Total" k="total" sort={labelSort} setSort={setLabelSort} className="text-right" />
                  <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600 hidden sm:table-cell">% of expenses</th>
                  <SortTh label="Count" k="count" sort={labelSort} setSort={setLabelSort} className="text-right" />
                  <SortTh label="Avg" k="avg" sort={labelSort} setSort={setLabelSort} className="text-right hidden sm:table-cell" />
                  <th className="text-right px-3 sm:px-4 py-2 font-semibold text-gray-600">Trend</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {sortRows(labelStats.all, labelSort, {
                  label: (s) => s.label,
                  total: (s) => s.total,
                  count: (s) => s.count,
                  avg: (s) => s.count > 0 ? s.total / s.count : 0,
                }).map((s) => {
                  const pct = labelStats.expenseTotal > 0 ? (s.total / labelStats.expenseTotal) * 100 : 0
                  const trend = labelTrend(s.perMonth)
                  return (
                    <tr
                      key={s.label}
                      className="hover:bg-gray-50 cursor-pointer"
                      onClick={() => setSelectedLabel({ label: s.label, type: 'expense', title: s.store ? `Store: ${STORE_NAMES[s.label]}` : undefined })}
                    >
                      <td className="px-3 sm:px-4 py-2">
                        <span className="font-medium text-indigo-700">{s.label}</span>
                        {s.committed && <span className="ml-1.5 text-[10px] font-semibold bg-slate-100 text-slate-600 rounded-full px-1.5 py-0.5">fixed</span>}
                        {s.store && <span className="ml-1.5 text-[10px] font-semibold bg-emerald-100 text-emerald-700 rounded-full px-1.5 py-0.5">store</span>}
                      </td>
                      <td className="px-3 sm:px-4 py-2 text-right font-semibold">{formatEuro(s.total)}</td>
                      <td className="px-3 sm:px-4 py-2 text-right text-gray-500 hidden sm:table-cell">{pct.toFixed(1)}%</td>
                      <td className="px-3 sm:px-4 py-2 text-right text-gray-500">{s.count}</td>
                      <td className="px-3 sm:px-4 py-2 text-right text-gray-500 hidden sm:table-cell">{formatEuro(s.total / s.count)}</td>
                      <td className="px-3 sm:px-4 py-2 text-right">
                        {trend != null && Math.abs(trend) >= 15 ? (
                          <span className={`text-xs font-semibold ${trend > 0 ? 'text-amber-600' : 'text-green-600'}`}>
                            {trend > 0 ? '▲' : '▼'}{Math.abs(trend) >= 995 ? '>10x' : `${Math.abs(trend).toFixed(0)}%`}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-300">—</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <p className="text-[11px] text-gray-400 px-4 py-2 border-t border-gray-100">
            A transaction with several labels counts toward each — label totals overlap, so they don't sum to total expenses.
            Trend compares the latest month in the period against the average of the earlier months.
          </p>
        </div>
      )}

      {/* Income sources by label */}
      {incomeByLabel.rows.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">💼 Income by Label</h3>
          <p className="text-xs text-gray-400 mb-4">Employers and other income sources in the period — click one to see its payments</p>
          <div className="space-y-3">
            {incomeByLabel.rows.map((r) => {
              const max = incomeByLabel.rows[0].total
              const share = incomeByLabel.incomeTotal > 0 ? (r.total / incomeByLabel.incomeTotal) * 100 : 0
              return (
                <button key={r.label} onClick={() => setSelectedLabel({ label: r.label, type: 'income' })} className="w-full text-left group">
                  <div className="flex items-center justify-between gap-2 text-sm mb-0.5">
                    <span className="font-medium text-green-700 group-hover:text-green-900 truncate">{r.label}</span>
                    <span className="text-gray-700 font-semibold shrink-0">
                      {formatEuro(r.total)} <span className="text-xs text-gray-400 font-normal">({share.toFixed(0)}%)</span>
                    </span>
                  </div>
                  <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                    <div className="h-full bg-green-400 rounded-full group-hover:bg-green-600" style={{ width: `${(r.total / max) * 100}%` }} />
                  </div>
                  <p className="text-[11px] text-gray-400 mt-0.5">
                    {r.count} {r.count === 1 ? 'payment' : 'payments'} · {r.first.slice(0, 7)} → {r.last.slice(0, 7)}
                    {r.monthCount > 1 && <> · avg {formatEuro(r.total / r.monthCount)}/mo over {r.monthCount} months</>}
                  </p>
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* Grocery store comparison */}
      {storeSpend.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-1">🛒 Grocery Stores</h3>
          <p className="text-xs text-gray-400 mb-4">
            Totals and average basket per store in the period — click a store to see its transactions
          </p>
          <div className="space-y-3">
            {storeSpend.map((s) => {
              const max = storeSpend[0].total
              const isCheapest = cheapestBasket?.store === s.store
              const isPriciest = priciestBasket?.store === s.store
              return (
                <button key={s.store} onClick={() => setSelectedLabel({ label: s.store, type: 'expense', title: `Store: ${STORE_NAMES[s.store]}` })} className="w-full text-left group">
                  <div className="flex items-center justify-between gap-2 text-sm mb-0.5">
                    <span className="flex items-center gap-2 min-w-0">
                      <span className="font-medium text-emerald-700 group-hover:text-emerald-900 truncate">{STORE_NAMES[s.store]}</span>
                      {isCheapest && (
                        <span className="shrink-0 px-1.5 py-0.5 rounded-full bg-green-100 text-green-700 text-[10px] font-semibold">cheapest basket</span>
                      )}
                      {isPriciest && (
                        <span className="shrink-0 px-1.5 py-0.5 rounded-full bg-amber-100 text-amber-700 text-[10px] font-semibold">priciest basket</span>
                      )}
                    </span>
                    <span className="text-gray-700 font-semibold shrink-0">{formatEuro(s.total)}</span>
                  </div>
                  <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                    <div className="h-full bg-emerald-400 rounded-full group-hover:bg-emerald-600" style={{ width: `${(s.total / max) * 100}%` }} />
                  </div>
                  <p className="text-[11px] text-gray-400 mt-0.5">
                    {s.count} {s.count === 1 ? 'visit' : 'visits'} · avg basket {formatEuro(s.avg)}
                  </p>
                </button>
              )
            })}
          </div>
          {cheapestBasket && priciestBasket && cheapestBasket.store !== priciestBasket.store && (
            <p className="mt-3 text-xs text-gray-500 bg-emerald-50 border border-emerald-100 rounded-lg px-3 py-2">
              💡 Average basket at <span className="font-semibold">{STORE_NAMES[priciestBasket.store]}</span> is{' '}
              {formatEuro(priciestBasket.avg)} vs {formatEuro(cheapestBasket.avg)} at{' '}
              <span className="font-semibold">{STORE_NAMES[cheapestBasket.store]}</span>. Basket sizes differ per trip,
              but shifting routine runs toward the cheaper store adds up.
            </p>
          )}
        </div>
      )}

      {selectedLabel && (
        <CategoryTransactionsModal
          title={selectedLabel.title ?? (selectedLabel.label ? `Label: ${selectedLabel.label}` : undefined)}
          label={selectedLabel.label}
          unlabeled={selectedLabel.unlabeled}
          type={selectedLabel.type}
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
