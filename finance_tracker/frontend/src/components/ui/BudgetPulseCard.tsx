import { Link } from 'react-router-dom'
import { useBudgets, useBudgetStatus } from '../../hooks/useBudgets'
import { formatEuro } from '../../utils/format'

// The home-page answer to "what can I still spend this month?" — always the
// CURRENT month regardless of the global date filter, since that's the only
// month a budget decision can still change.
export default function BudgetPulseCard() {
  const { data: budgets } = useBudgets()
  const { data: plan } = useBudgetStatus()

  if (!budgets || budgets.filter((b) => b.kind !== 'trip').length === 0) {
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
  const safe = plan?.safe_to_spend ?? null
  // Over a monthly cap, or a fund drawn below zero.
  const warnings = (plan?.lines ?? [])
    .filter((l) => l.kind === 'spending')
    .map((l) => {
      if (l.fund) return l.remaining < -0.5 ? { id: l.id, text: `${l.name} fund ${formatEuro(l.remaining)}`, ratio: 99 } : null
      if (l.period === 'yearly') return l.remaining < -0.5 ? { id: l.id, text: `${l.name} over the year`, ratio: 98 } : null
      return l.budgeted > 0 && l.spent > l.budgeted
        ? { id: l.id, text: `${l.name} +${Math.round((l.spent / l.budgeted - 1) * 100)}%`, ratio: l.spent / l.budgeted }
        : null
    })
    .filter((w): w is { id: number; text: string; ratio: number } => w != null)
    .sort((a, b) => b.ratio - a.ratio)
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
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan?.fixed_planned ?? 0)}</p>
        </div>
        <div className="bg-gray-50 rounded-lg py-1.5">
          <p className="text-[11px] text-gray-400">Investing</p>
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan?.investment_planned ?? 0)}</p>
        </div>
        <div className="bg-gray-50 rounded-lg py-1.5">
          <p className="text-[11px] text-gray-400">Spent free</p>
          <p className="text-sm font-semibold text-gray-700">{formatEuro(plan?.discretionary_spent ?? 0)}</p>
        </div>
      </div>

      {warnings.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {warnings.map((w) => (
            <span key={w.id} className="text-[11px] font-medium bg-red-50 text-red-700 border border-red-200 rounded-full px-2 py-0.5">
              ⚠ {w.text}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
