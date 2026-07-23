import { useMemo } from 'react'
import type { Transaction, MonthlySummary } from '../../types'
import { buildInsights, type InsightTone } from '../../utils/insights'

interface Props {
  expenses: Transaction[]
  byMonth: MonthlySummary[]
}

const TONE_STYLES: Record<InsightTone, { dot: string; text: string }> = {
  good: { dot: 'bg-green-500', text: 'text-green-700' },
  warn: { dot: 'bg-yellow-500', text: 'text-yellow-700' },
  bad: { dot: 'bg-red-500', text: 'text-red-700' },
  info: { dot: 'bg-blue-400', text: 'text-gray-700' },
}

export default function InsightsPanel({ expenses, byMonth }: Props) {
  const insights = useMemo(() => buildInsights(expenses, byMonth, new Date()), [expenses, byMonth])

  if (insights.length === 0) return null

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <h3 className="text-base font-semibold text-gray-900 mb-1">💡 Insights</h3>
      <p className="text-xs text-gray-400 mb-3">Computed from your transactions in the selected period</p>
      <ul className="space-y-2">
        {insights.map((ins, i) => {
          const style = TONE_STYLES[ins.tone]
          return (
            <li key={i} className="flex items-start gap-2.5">
              <span className={`mt-1.5 w-2 h-2 rounded-full flex-shrink-0 ${style.dot}`} />
              <span className="text-sm text-gray-700 leading-snug">
                <span className={`font-semibold mr-1 ${style.text}`}>{ins.icon}</span>
                {ins.text}
              </span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
