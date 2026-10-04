import { useState } from 'react'
import { Link } from 'react-router-dom'
import { lastCompleteMonth, markReviewNudgeSeen, reviewNudgeSeen } from '../../utils/month'

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

// First week of a month: last month's review is the thing to read. Closing it
// (or opening the review) hides it until next month's review is ready.
export default function ReviewNudge() {
  const month = lastCompleteMonth()
  const [hidden, setHidden] = useState(() => reviewNudgeSeen(month))
  if (hidden || new Date().getDate() > 7) return null
  const name = MONTHS[Number(month.slice(5, 7)) - 1]
  const close = () => {
    markReviewNudgeSeen(month)
    setHidden(true)
  }
  return (
    <div className="flex items-center gap-2 bg-blue-50 border border-blue-100 rounded-xl pl-4 pr-1.5 py-1.5">
      <Link
        to={`/review?month=${month}`}
        onClick={() => markReviewNudgeSeen(month)}
        className="flex-1 flex items-center justify-between gap-3 py-1.5 min-w-0"
      >
        <span className="text-sm text-blue-900 truncate">🗓️ Your {name} review is ready</span>
        <span className="text-sm text-blue-700 flex-shrink-0">Open →</span>
      </Link>
      <button
        onClick={close}
        className="p-2 rounded-lg text-blue-400 hover:text-blue-700 hover:bg-blue-100"
        aria-label={`Hide the ${name} review until next month`}
        title="Hide until next month"
      >
        ✕
      </button>
    </div>
  )
}
