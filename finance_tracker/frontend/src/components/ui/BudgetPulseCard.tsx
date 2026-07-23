import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useAllTransactions, useTransactionSummary } from '../../hooks/useTransactions'
import { useBudgets, useBudgetSettings } from '../../hooks/useBudgets'
import { computeMonthPlan, medianMonthlyIncome } from '../../utils/budget'
import { ltNetSalary } from '../../utils/ltSalary'
import { formatEuro } from '../../utils/format'

function currentMonthRange() {
  const now = new Date()
  const y = now.getFullYear()
  const m = now.getMonth()
  const first = `${y}-${String(m + 1).padStart(2, '0')}-01`
  const last = new Date(y, m + 1, 0)
  const lastStr = `${y}-${String(m + 1).padStart(2, '0')}-${String(last.getDate()).padStart(2, '0')}`
  return { date_from: first, date_to: lastStr }
}

// The home-page answer to "what can I still spend this month?" — always the
// CURRENT month regardless of the global date filter, since that's the only
// month a budget decision can still change.
export default function BudgetPulseCard() {
  const { data: budgets } = useBudgets()
  const { data: settings } = useBudgetSettings()
  const { data: monthTxs } = useAllTransactions(currentMonthRange())
  const { data: allTimeSummary } = useTransactionSummary({})

  const incomeBase = useMemo(() => {
    if (settings?.income_mode === 'manual' && settings.manual_income > 0) return settings.manual_income
    if (settings?.income_mode === 'gross' && settings.gross_salary > 0) {
      return ltNetSalary(settings.gross_salary, settings.monthly_deductions).netAfterDeductions
    }
    return medianMonthlyIncome(allTimeSummary?.by_month ?? [], new Date())
  }, [settings, allTimeSummary])

  const plan = useMemo(
    () => computeMonthPlan(budgets ?? [], monthTxs?.data ?? [], incomeBase),
    [budgets, monthTxs, incomeBase]
  )

  if (!budgets || budgets.length === 0) {
    return (
      <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6 flex flex-col justify-center">
        <h3 className="text-base font-semibold text-gray-900 mb-1">🎯 Budget</h3>
        <p className="text-sm text-gray-500">
          No budgets yet — set spending limits and investment targets to see your safe-to-spend here.
        </p>
        <Link to="/budget" className="text-sm text-blue-600 hover:underline mt-2">Set up budgets →</Link>
      </div>
    )
  }

  const now = new Date()
  const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate()
  const daysLeft = daysInMonth - now.getDate() + 1
  const safe = plan.safeToSpend
  const overBudget = plan.spending
    .filter((s) => s.budget.amount > 0 && s.actual > s.budget.amount)
    .sort((a, b) => b.actual / b.budget.amount - a.actual / a.budget.amount)
    .slice(0, 3)

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-center justify-between mb-1">
        <h3 className="text-base font-semibold text-gray-900">
          🎯 Budget — {now.toLocaleDateString('en', { month: 'long' })}
        </h3>
        <Link to="/budget" className="text-xs text-blue-600 hover:underline">Details →</Link>
      </div>

      <p className="text-xs text-gray-400 mb-3">Safe to spend after fixed costs, investment targets and what's already gone</p>
      <p className={`text-3xl font-bold ${safe !== null && safe < 0 ? 'text-red-600' : 'text-emerald-600'}`}>
        {safe !== null ? formatEuro(safe) : '—'}
      </p>
      {safe !== null && safe > 0 && daysLeft > 0 && (
        <p className="text-xs text-gray-500 mt-0.5">≈ {formatEuro(safe / daysLeft)}/day for the next {daysLeft} days</p>
      )}
      {safe !== null && safe < 0 && (
        <p className="text-xs text-red-500 mt-0.5">over the month's plan — every euro now comes from savings</p>
      )}

      <div className="mt-3 grid grid-cols-3 gap-2 text-center">
        <div className="bg-gray-50 rounded-lg py-1.5">
          <p className="text-[11px] text-gray-400">Fixed</p>
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan.fixedPlanned)}</p>
        </div>
        <div className="bg-gray-50 rounded-lg py-1.5">
          <p className="text-[11px] text-gray-400">Investing</p>
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan.investmentPlanned)}</p>
        </div>
        <div className="bg-gray-50 rounded-lg py-1.5">
          <p className="text-[11px] text-gray-400">Spent free</p>
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan.discretionarySpent)}</p>
        </div>
      </div>

      {overBudget.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {overBudget.map((s) => (
            <span key={s.budget.id} className="text-[11px] font-medium bg-red-50 text-red-700 border border-red-200 rounded-full px-2 py-0.5">
              ⚠ {s.budget.name} +{Math.round((s.actual / s.budget.amount - 1) * 100)}%
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
