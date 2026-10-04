import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { reviewApi, type ReviewTotals } from '../api/review'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import { formatEuro } from '../utils/format'
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

// Signed change against a reference, coloured by whether it is good news.
function Delta({ value, base, goodWhenUp }: { value: number; base: number; goodWhenUp: boolean }) {
  const d = value - base
  if (Math.abs(d) < 0.5) return <span className="text-gray-400">same</span>
  const good = goodWhenUp ? d > 0 : d < 0
  return (
    <span className={good ? 'text-green-600' : 'text-red-600'}>
      {d > 0 ? '▲' : '▼'} {formatEuro(Math.abs(d))}
    </span>
  )
}

function TotalsRow({ label, cur, prev, avg, goodWhenUp }: {
  label: string; cur: number; prev: number; avg: number; goodWhenUp: boolean
}) {
  return (
    <div className="py-2 grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5 items-baseline">
      <span className="text-sm text-gray-600">{label}</span>
      <span className="text-base font-semibold text-gray-900 text-right">{formatEuro(cur)}</span>
      <span className="text-xs text-gray-400 col-span-2 text-right">
        vs last month <Delta value={cur} base={prev} goodWhenUp={goodWhenUp} /> · vs 6-mo avg <Delta value={cur} base={avg} goodWhenUp={goodWhenUp} />
      </span>
    </div>
  )
}

const rate = (t: ReviewTotals) => (t.savings_rate != null ? `${t.savings_rate.toFixed(1)}%` : '—')

// The month-end review: how the month went against the one before and the
// half-year average, where the budget slipped, and what in the data needs
// attention. Computed on the server from the ledger — no AI involved.
export default function Review() {
  const [params, setParams] = useSearchParams()
  const month = params.get('month') ?? lastCompleteMonth()
  const { data: r, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['review', month],
    queryFn: () => reviewApi.get(month),
  })
  const go = (n: number) => setParams({ month: shiftMonth(month, n) })
  const atLatest = month >= new Date().toISOString().slice(0, 7)

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-3xl mx-auto">
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
          {r.checks.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
              <h3 className="text-sm font-semibold text-gray-900 mb-2">Needs attention</h3>
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

          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
            <div>
              <h3 className="text-sm font-semibold text-gray-900">Money in and out</h3>
              <span className="text-xs text-gray-400">Savings rate <span className={`font-semibold ${r.net_saved >= 0 ? 'text-green-600' : 'text-red-600'}`}>{rate(r)}</span> · last month {rate(r.previous)} · avg {rate(r.six_month_avg)}</span>
            </div>
            <div className="divide-y divide-gray-100 mt-1">
              <TotalsRow label="Income" cur={r.income} prev={r.previous.income} avg={r.six_month_avg.income} goodWhenUp />
              <TotalsRow label="Spending" cur={r.spending} prev={r.previous.spending} avg={r.six_month_avg.spending} goodWhenUp={false} />
              <TotalsRow label="Net saved" cur={r.net_saved} prev={r.previous.net_saved} avg={r.six_month_avg.net_saved} goodWhenUp />
              <TotalsRow label="Invested" cur={r.invested} prev={r.previous.invested} avg={r.six_month_avg.invested} goodWhenUp />
            </div>
          </div>

          {r.net_worth && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5 flex items-baseline justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-gray-900">Net worth</h3>
                <p className="text-xs text-gray-400">{r.net_worth.start_date} → {r.net_worth.end_date}</p>
              </div>
              <div className="text-right">
                <p className="text-base font-semibold text-gray-900">{formatEuro(r.net_worth.end)}</p>
                <p className={`text-sm font-semibold ${r.net_worth.change >= 0 ? 'text-green-600' : 'text-red-600'}`}>
                  {r.net_worth.change >= 0 ? '▲' : '▼'} {formatEuro(Math.abs(r.net_worth.change))}
                </p>
              </div>
            </div>
          )}

          {r.categories.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
              <h3 className="text-sm font-semibold text-gray-900 mb-2">Biggest swings vs your 6-month average</h3>
              <ul className="divide-y divide-gray-100">
                {r.categories.map((c) => (
                  <li key={c.category} className="py-1.5 flex items-baseline justify-between gap-2 text-sm">
                    <span className="text-gray-700">{c.category}</span>
                    <span className="text-right">
                      <span className="font-semibold text-gray-900">{formatEuro(c.spent)}</span>
                      <span className={`ml-2 text-xs ${c.delta > 0 ? 'text-red-600' : 'text-green-600'}`}>
                        {c.delta > 0 ? '+' : '−'}{formatEuro(Math.abs(c.delta))}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {r.budget && (r.budget.over.length > 0 || r.budget.within_count > 0) && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
              <div className="flex items-baseline justify-between mb-2">
                <h3 className="text-sm font-semibold text-gray-900">Budget</h3>
                <Link to="/budget" className="text-xs text-blue-600 hover:underline">Details →</Link>
              </div>
              <p className="text-xs text-gray-500 mb-1">
                {r.budget.within_count} monthly line{r.budget.within_count === 1 ? '' : 's'} within budget
                {r.budget.over.length > 0 && `, ${r.budget.over.length} over`}
              </p>
              <ul className="divide-y divide-gray-100">
                {r.budget.over.map((l) => (
                  <li key={l.name} className="py-1.5 flex items-baseline justify-between gap-2 text-sm">
                    <span className="text-gray-700">{l.name}</span>
                    <span className="text-right text-xs text-gray-500">
                      {formatEuro(l.spent)} of {formatEuro(l.budgeted)} <span className="text-red-600 font-semibold">+{formatEuro(l.spent - l.budgeted)}</span>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {r.top_expenses.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
              <h3 className="text-sm font-semibold text-gray-900 mb-2">Largest expenses</h3>
              <ul className="divide-y divide-gray-100">
                {r.top_expenses.map((e) => (
                  <li key={e.id} className="py-1.5 flex items-baseline justify-between gap-3 text-sm">
                    <span className="min-w-0">
                      <span className="block truncate text-gray-700">{e.comment || e.category}</span>
                      <span className="text-xs text-gray-400">{e.date} · {e.category}</span>
                    </span>
                    <span className="font-semibold text-red-600 flex-shrink-0">−{formatEuro(e.amount)}</span>
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
