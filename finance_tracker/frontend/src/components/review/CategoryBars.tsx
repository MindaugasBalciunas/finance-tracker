import { useState } from 'react'
import type { ReviewCategory } from '../../api/review'
import { formatEuro } from '../../utils/format'
import { SPENDING } from './colors'

// Where the money went: one bar per category on a shared scale, with a tick
// at the category's own 6-month average — over the tick means above normal.
export default function CategoryBars({ rows }: { rows: ReviewCategory[] }) {
  const [all, setAll] = useState(false)
  if (rows.length === 0) return null
  const shown = all ? rows : rows.slice(0, 8)
  const max = Math.max(...rows.map((r) => Math.max(r.spent, r.average)), 1)
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-3">
        <h3 className="text-sm font-semibold text-gray-900">Where the money went</h3>
        <span className="flex items-center gap-3 text-xs text-gray-500">
          <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: SPENDING }} />this month</span>
          <span className="flex items-center gap-1.5"><span className="w-0.5 h-3 bg-gray-800 rounded" />6-mo avg</span>
        </span>
      </div>
      <ul className="space-y-2.5">
        {shown.map((r) => {
          const up = r.delta > 0.5
          const down = r.delta < -0.5
          return (
            <li key={r.category} title={`${r.category}: ${formatEuro(r.spent)} (avg ${formatEuro(r.average)})`}>
              <div className="flex items-baseline justify-between gap-2 text-sm">
                <span className="text-gray-700 truncate">{r.category}</span>
                <span className="flex-shrink-0">
                  <span className="font-semibold text-gray-900">{formatEuro(r.spent)}</span>
                  {(up || down) && (
                    <span className={`ml-1.5 text-xs ${up ? 'text-red-600' : 'text-green-700'}`}>
                      {up ? '▲' : '▼'} {formatEuro(Math.abs(r.delta))}
                    </span>
                  )}
                </span>
              </div>
              <div className="relative mt-1 h-2.5 rounded-full bg-gray-100">
                <div className="h-2.5 rounded-full" style={{ width: `${(r.spent / max) * 100}%`, backgroundColor: SPENDING }} />
                {r.average > 0 && (
                  <div className="absolute -top-0.5 h-3.5 w-0.5 bg-gray-800 rounded" style={{ left: `calc(${(r.average / max) * 100}% - 1px)` }} />
                )}
              </div>
            </li>
          )
        })}
      </ul>
      {rows.length > 8 && (
        <button onClick={() => setAll((v) => !v)} className="mt-3 text-xs text-blue-600 hover:underline">
          {all ? 'Show fewer' : `Show all ${rows.length} categories`}
        </button>
      )}
    </div>
  )
}
