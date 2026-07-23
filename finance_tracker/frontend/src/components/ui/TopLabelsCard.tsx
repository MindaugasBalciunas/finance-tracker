import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import type { Transaction } from '../../types'
import { formatEuro } from '../../utils/format'

// Money that isn't a choice — kept out of the "where does my money go" story.
const FIXED_LABELS = new Set(['loan', 'alimony', 'leasing', 'evelina'])

interface Props {
  expenses: Transaction[]
}

// Compact "where the money actually goes" panel driven by labels — the
// discretionary story the category donut can't tell (a Food euro at Lidl and
// one at a restaurant are different decisions).
export default function TopLabelsCard({ expenses }: Props) {
  const top = useMemo(() => {
    const sums: Record<string, { total: number; count: number }> = {}
    for (const tx of expenses) {
      const labels = (tx.labels ?? '').split(',').map((l) => l.trim()).filter(Boolean)
      if (labels.some((l) => FIXED_LABELS.has(l))) continue
      for (const l of labels) {
        if (FIXED_LABELS.has(l)) continue
        if (!sums[l]) sums[l] = { total: 0, count: 0 }
        sums[l].total += tx.amount.value
        sums[l].count += 1
      }
    }
    return Object.entries(sums)
      .sort((a, b) => b[1].total - a[1].total)
      .slice(0, 7)
  }, [expenses])

  if (top.length === 0) return null
  const max = top[0][1].total

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-center justify-between mb-1">
        <h3 className="text-base font-semibold text-gray-900">🏷 Where money goes</h3>
        <Link to="/reports" className="text-xs text-blue-600 hover:underline">All labels →</Link>
      </div>
      <p className="text-xs text-gray-400 mb-3">Top labels in the period — fixed obligations excluded</p>
      <div className="space-y-2">
        {top.map(([label, { total, count }]) => (
          <div key={label}>
            <div className="flex items-center justify-between text-sm mb-0.5">
              <span className="font-medium text-indigo-700">{label}</span>
              <span className="text-gray-700 font-semibold">
                {formatEuro(total)} <span className="text-xs text-gray-400 font-normal">({count})</span>
              </span>
            </div>
            <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
              <div className="h-full bg-indigo-400 rounded-full" style={{ width: `${(total / max) * 100}%` }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
