import { useState } from 'react'
import type { ReviewCategory } from '../../api/review'
import { formatEuro } from '../../utils/format'
import { SPENDING } from './colors'

// Where the money went: one bar per category on a shared scale, with a tick
// at the category's own 6-month average — over the tick means above normal.
// Tap a category to see the payments behind it.
export default function CategoryBars({ rows }: { rows: ReviewCategory[] }) {
  const [all, setAll] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  if (rows.length === 0) return null
  const shown = all ? rows : rows.slice(0, 8)
  const max = Math.max(...rows.map((r) => Math.max(r.spent, r.average)), 1)
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-gray-900">Where the money went</h3>
          <p className="text-xs text-gray-400">Tap a category to see its biggest payments</p>
        </div>
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
            <li key={r.category}>
              <button
                type="button"
                onClick={() => setOpen((o) => (o === r.category ? null : r.category))}
                className="w-full text-left"
                aria-expanded={open === r.category}
                disabled={!r.top?.length}
              >
              <div className="flex items-baseline justify-between gap-2 text-sm">
                <span className="text-gray-700 truncate">
                  {r.top?.length ? <span className="text-gray-400 text-xs mr-1">{open === r.category ? '▾' : '▸'}</span> : null}
                  {r.category}
                </span>
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
              </button>
              {open === r.category && r.top && (
                <ul className="mt-2 ml-3 pl-3 border-l-2 border-gray-100 space-y-1">
                  {r.top.map((t) => (
                    <li key={t.id} className="flex items-baseline justify-between gap-2 text-xs">
                      <span className="min-w-0 text-gray-600">
                        <span className="text-gray-400 mr-1.5">{t.date.slice(5)}</span>
                        {t.comment || t.category}
                        {t.moved_from && <span className="ml-1.5 rounded px-1 py-0.5 bg-amber-50 text-amber-700">was {t.moved_from}</span>}
                        {t.recurring && !t.moved_from && <span className="ml-1.5 rounded px-1 py-0.5 bg-gray-100 text-gray-500">regular</span>}
                      </span>
                      <span className="font-semibold text-gray-800 flex-shrink-0">{formatEuro(t.amount)}</span>
                    </li>
                  ))}
                  {(r.count ?? 0) > r.top.length && (
                    <li className="text-xs text-gray-400">+ {(r.count ?? 0) - r.top.length} smaller payment{(r.count ?? 0) - r.top.length === 1 ? '' : 's'}</li>
                  )}
                </ul>
              )}
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
