import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../../api/insights'
import { useAISettings } from '../../hooks/useInsights'
import { useDateRange } from '../../context/DateRangeContext'

const COLLAPSE_KEY = 'view-insight-collapsed'

// ViewInsightBar shows a short auto-generated AI review of the tab the user
// is currently on, scoped to the selected date range. It only renders when
// the gateway is configured; failures stay silent (the page must never feel
// broken because a nicety couldn't load). Server caches per view+period, so
// revisits within 15 minutes are free.
export default function ViewInsightBar({ view }: { view: string }) {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model
  const { dateRange } = useDateRange()
  const qc = useQueryClient()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(COLLAPSE_KEY) === '1')
  const [refreshing, setRefreshing] = useState(false)

  const queryKey = ['view-summary', view, dateRange.date_from ?? '', dateRange.date_to ?? '']
  const { data: summary, isLoading, isError } = useQuery({
    queryKey,
    queryFn: () => aiApi.viewSummary(view, dateRange),
    enabled: configured && !collapsed,
    staleTime: 15 * 60_000,
    retry: false,
  })

  if (!configured || isError) return null

  const toggle = () => {
    const next = !collapsed
    setCollapsed(next)
    localStorage.setItem(COLLAPSE_KEY, next ? '1' : '0')
  }

  const refresh = async () => {
    setRefreshing(true)
    try {
      const fresh = await aiApi.viewSummary(view, dateRange, true)
      qc.setQueryData(queryKey, fresh)
    } catch {
      // Silent — the stale blurb stays.
    } finally {
      setRefreshing(false)
    }
  }

  if (collapsed) {
    return (
      <button
        onClick={toggle}
        className="mb-3 text-[11px] text-indigo-400 hover:text-indigo-600"
        title="Show the AI review of this view"
      >
        ✦ AI review
      </button>
    )
  }

  return (
    <div className="mb-3 flex items-start gap-2 rounded-xl border border-indigo-100 bg-gradient-to-r from-indigo-50/70 to-blue-50/70 px-3 py-2">
      <span className="text-indigo-500 mt-0.5">✦</span>
      <p className="flex-1 min-w-0 text-[13px] leading-snug text-gray-700">
        {isLoading || refreshing
          ? <span className="text-gray-400 animate-pulse">Reviewing this view…</span>
          : summary}
      </p>
      <span className="flex items-center gap-0.5 shrink-0">
        <button
          onClick={refresh}
          disabled={isLoading || refreshing}
          aria-label="Refresh AI review"
          title="Re-run the review"
          className="p-1 text-indigo-300 hover:text-indigo-600 disabled:opacity-40"
        >
          ↻
        </button>
        <button
          onClick={toggle}
          aria-label="Hide AI review"
          title="Hide (remembers your choice)"
          className="p-1 text-indigo-300 hover:text-indigo-600"
        >
          ✕
        </button>
      </span>
    </div>
  )
}