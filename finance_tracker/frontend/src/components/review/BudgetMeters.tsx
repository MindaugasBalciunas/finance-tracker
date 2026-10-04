import { Link } from 'react-router-dom'
import type { ReviewBudgetLine } from '../../api/review'
import { formatEuro } from '../../utils/format'

// One meter per monthly budget line. The fill carries the state — within,
// close, over — and every over line also says "over" with an icon, so the
// state never rests on color alone.
export default function BudgetMeters({ lines, safeToSpend }: { lines: ReviewBudgetLine[]; safeToSpend?: number }) {
  if (lines.length === 0) return null
  const over = lines.filter((l) => l.spent > l.budgeted + 0.5).length
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="text-sm font-semibold text-gray-900">Budget</h3>
        <Link to="/budget" className="text-xs text-blue-600 hover:underline">Details →</Link>
      </div>
      <p className="text-xs text-gray-500 mt-0.5 mb-3">
        {lines.length - over} of {lines.length} monthly lines within budget
        {safeToSpend != null && <> · safe to spend <span className={safeToSpend < 0 ? 'text-red-600 font-semibold' : ''}>{formatEuro(safeToSpend)}</span></>}
      </p>
      <ul className="space-y-2.5">
        {lines.map((l) => {
          const ratio = l.budgeted > 0 ? l.spent / l.budgeted : 0
          const state = ratio > 1.005 ? 'over' : ratio >= 0.9 ? 'close' : 'ok'
          const fill = state === 'over' ? 'bg-red-500' : state === 'close' ? 'bg-amber-500' : 'bg-blue-500'
          const track = state === 'over' ? 'bg-red-100' : state === 'close' ? 'bg-amber-100' : 'bg-blue-100'
          return (
            <li key={l.name}>
              <div className="flex flex-wrap items-baseline justify-between gap-x-2 text-sm">
                <span className="text-gray-700">{state === 'over' && <span aria-label="over budget">⚠ </span>}{l.name}</span>
                <span className="text-xs text-gray-500 ml-auto">
                  <span className="font-semibold text-gray-900">{formatEuro(l.spent)}</span> / {formatEuro(l.budgeted)}
                  {state === 'over' && <span className="ml-1 text-red-600 font-semibold">+{formatEuro(l.spent - l.budgeted)} over</span>}
                </span>
              </div>
              <div className={`mt-1 h-2.5 rounded-full ${track}`}>
                <div className={`h-2.5 rounded-full ${fill}`} style={{ width: `${Math.min(ratio, 1) * 100}%` }} />
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
