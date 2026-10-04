import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { reviewApi } from '../api/review'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import { formatEuro } from '../utils/format'
import KpiTile from '../components/review/KpiTile'
import HighlightsCard from '../components/review/HighlightsCard'
import TwelveMonthChart from '../components/review/TwelveMonthChart'
import CategoryBars from '../components/review/CategoryBars'
import BudgetMeters from '../components/review/BudgetMeters'
import SpendCalendar from '../components/review/SpendCalendar'
import { ACCENT, INCOME, SPENDING } from '../components/review/colors'
import { lastCompleteMonth } from '../utils/month'

function shiftMonth(ym: string, n: number): string {
  const [y, m] = ym.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1 + n, 1))
  return d.toISOString().slice(0, 7)
}

function monthName(ym: string): string {
  const [y, m] = ym.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString('en-GB', { month: 'long', year: 'numeric', timeZone: 'UTC' })
}

// The month-end review: how the month went against the one before and the
// half-year average, where the budget slipped, and what in the data needs
// attention. Computed on the server from the ledger — no AI involved.
export default function Review() {
  const [params, setParams] = useSearchParams()
  const month = params.get('month') ?? lastCompleteMonth()
  const { data: r, isLoading, isError, error, refetch, isPlaceholderData } = useQuery({
    queryKey: ['review', month],
    queryFn: () => reviewApi.get(month),
    // Switching months keeps the last render (dimmed) instead of a spinner flash.
    placeholderData: (prev) => prev,
  })
  const go = (n: number) => setParams({ month: shiftMonth(month, n) })
  const atLatest = month >= new Date().toISOString().slice(0, 7)

  return (
    <div className={`p-4 sm:p-6 space-y-4 max-w-5xl mx-auto transition-opacity ${isPlaceholderData ? 'opacity-60' : ''}`}>
      <div className="flex items-center justify-between gap-2">
        <button onClick={() => go(-1)} className="px-3 py-1.5 text-sm border border-gray-300 rounded-lg hover:bg-gray-50" aria-label="Previous month">‹</button>
        <div className="text-center">
          <h1 className="text-xl font-bold text-gray-900">{monthName(month)}</h1>
          <p className="text-xs text-gray-400">Month review{r && !r.complete && ' · month still running'}</p>
        </div>
        <button onClick={() => go(1)} disabled={atLatest} className="px-3 py-1.5 text-sm border border-gray-300 rounded-lg hover:bg-gray-50 disabled:opacity-30" aria-label="Next month">›</button>
      </div>

      {isError ? <QueryError error={error} onRetry={() => refetch()} /> : isLoading || !r ? <LoadingSpinner /> : (
        <>
          <HighlightsCard r={r} />

          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            <KpiTile label="Income" value={r.income} previous={r.six_month_avg.income} refLabel="vs usual" goodWhenUp accent={INCOME}
              trend={r.trend?.map((t) => t.income)} />
            <KpiTile label="Spending" value={r.spending} previous={r.six_month_avg.spending} refLabel="vs usual" goodWhenUp={false} accent={SPENDING}
              trend={r.trend?.map((t) => t.spending)} />
            <KpiTile label="Net saved" value={r.net_saved} previous={r.six_month_avg.net_saved} refLabel="vs usual" goodWhenUp accent={ACCENT}
              trend={r.trend?.map((t) => t.net_saved)} />
            {r.net_worth ? (
              <KpiTile label="Net worth change" value={r.net_worth.change} goodWhenUp accent={ACCENT}
                note={`now ${formatEuro(r.net_worth.end)}`} trend={r.net_worth.points?.map((p) => p.value)} />
            ) : (
              <KpiTile label="Invested" value={r.invested} previous={r.six_month_avg.invested} refLabel="vs usual" goodWhenUp accent={ACCENT}
                trend={r.trend?.map((t) => t.invested)} />
            )}
          </div>

          {r.trend && r.trend.length > 1 && (
            <TwelveMonthChart rows={r.trend} current={r.month} onPick={(m) => setParams({ month: m })} />
          )}

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
            <CategoryBars rows={r.by_category ?? r.categories} />
            {r.budget?.lines && r.budget.lines.length > 0 && (
              <BudgetMeters lines={r.budget.lines} safeToSpend={r.budget.safe_to_spend} />
            )}
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
            {r.daily && <SpendCalendar days={r.daily} />}

            {r.top_expenses.length > 0 && (
              <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
                <h3 className="text-sm font-semibold text-gray-900 mb-2">Largest expenses</h3>
                <ul className="space-y-2">
                  {r.top_expenses.map((e) => (
                    <li key={e.id} className="text-sm">
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="min-w-0 truncate text-gray-700">{e.comment || e.category}</span>
                        <span className="font-semibold text-gray-900 flex-shrink-0">−{formatEuro(e.amount)}</span>
                      </div>
                      <div className="flex items-center gap-2 mt-1">
                        <div className="flex-1 h-1.5 rounded-full bg-gray-100">
                          <div className="h-1.5 rounded-full" style={{ width: `${(e.amount / r.top_expenses[0].amount) * 100}%`, backgroundColor: SPENDING }} />
                        </div>
                        <span className="text-[11px] text-gray-400 flex-shrink-0">{e.date.slice(5)} · {e.category}</span>
                      </div>
                    </li>
                  ))}
                </ul>
                {r.spending > 0 && (
                  <p className="text-xs text-gray-400 mt-3">
                    These {r.top_expenses.length} are {Math.round((r.top_expenses.reduce((s, e) => s + e.amount, 0) / r.spending) * 100)}% of the month's spending.
                  </p>
                )}
              </div>
            )}
          </div>

          {r.checks.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
              <h3 className="text-sm font-semibold text-gray-900 mb-2">Housekeeping</h3>
              <ul className="space-y-1.5">
                {r.checks.map((c) => (
                  <li key={c.text} className="flex items-start gap-2 text-sm">
                    <span className={c.level === 'warn' ? 'text-amber-600' : 'text-gray-400'}>{c.level === 'warn' ? '⚠' : 'ℹ'}</span>
                    <span className="flex-1 text-gray-700">
                      {c.text}{' '}
                      {c.link && <Link to={c.link} className="text-blue-600 hover:underline whitespace-nowrap">Open →</Link>}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {Math.abs(r.owed_to_you) >= 0.01 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5 flex items-baseline justify-between">
              <h3 className="text-sm font-semibold text-gray-900">Owed to you (all time)</h3>
              <span className="text-base font-semibold text-gray-900">{formatEuro(r.owed_to_you)}</span>
            </div>
          )}
        </>
      )}
    </div>
  )
}
