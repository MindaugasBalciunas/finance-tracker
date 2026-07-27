import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'
import { txLabels, isCommitted } from '../../utils/labels'

interface Props {
  expenses: Transaction[]
}

// Compact "where the money actually goes" panel driven by labels — the
// discretionary story the category donut can't tell (a Food euro at Lidl and
// one at a restaurant are different decisions). Each row links to the
// transaction list pre-filtered on that label; the arrow compares the latest
// data month against the average of the earlier months in the period.
export default function TopLabelsCard({ expenses }: Props) {
  const { top, months } = useMemo(() => {
    const sums: Record<string, { total: number; count: number; perMonth: Record<string, number> }> = {}
    const monthSet = new Set<string>()
    for (const tx of expenses) {
      if (isCommitted(tx)) continue
      const mk = tx.date.slice(0, 7)
      monthSet.add(mk)
      for (const l of txLabels(tx)) {
        if (!sums[l]) sums[l] = { total: 0, count: 0, perMonth: {} }
        sums[l].total += tx.amount.value
        sums[l].count += 1
        sums[l].perMonth[mk] = (sums[l].perMonth[mk] ?? 0) + tx.amount.value
      }
    }
    return {
      top: Object.entries(sums)
        .sort((a, b) => b[1].total - a[1].total)
        .slice(0, 7),
      months: [...monthSet].sort(),
    }
  }, [expenses])

  if (top.length === 0) return null
  const max = top[0][1].total

  // Latest-month-vs-prior-average delta, only meaningful with 2+ months.
  const curKey = months[months.length - 1]
  const priorMonths = months.slice(0, -1)
  const trendFor = (perMonth: Record<string, number>): number | null => {
    if (priorMonths.length === 0) return null
    const cur = perMonth[curKey] ?? 0
    const avg = priorMonths.reduce((s, m) => s + (perMonth[m] ?? 0), 0) / priorMonths.length
    if (avg < 10 && cur < 10) return null
    if (avg <= 0) return null
    return ((cur - avg) / avg) * 100
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-center justify-between mb-1">
        <h3 className="text-base font-semibold text-gray-900">🏷 Where money goes</h3>
        <Link to="/reports" className="text-xs text-blue-600 hover:underline">All labels →</Link>
      </div>
      <p className="text-xs text-gray-400 mb-3">Top labels in the period — fixed obligations excluded · tap a label to see its transactions</p>
      <div className="space-y-2">
        {top.map(([label, { total, count, perMonth }]) => {
          const trend = trendFor(perMonth)
          return (
            <Link key={label} to={`/transactions?label=${encodeURIComponent(label)}`} className="block group">
              <div className="flex items-center justify-between text-sm mb-0.5">
                <span className="font-medium text-indigo-700 group-hover:text-indigo-900 group-hover:underline">{label}</span>
                <span className="text-gray-700 font-semibold">
                  {trend != null && Math.abs(trend) >= 15 && (
                    <span className={`text-xs font-semibold mr-1.5 ${trend > 0 ? 'text-amber-600' : 'text-green-600'}`}>
                      {trend > 0 ? '▲' : '▼'}{Math.abs(trend) >= 995 ? '>10x' : `${Math.abs(trend).toFixed(0)}%`}
                    </span>
                  )}
                  {formatEuro(total)} <span className="text-xs text-gray-400 font-normal">({count})</span>
                </span>
              </div>
              <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                <div className="h-full bg-indigo-400 group-hover:bg-indigo-600 rounded-full" style={{ width: `${(total / max) * 100}%` }} />
              </div>
            </Link>
          )
        })}
      </div>
    </div>
  )
}
