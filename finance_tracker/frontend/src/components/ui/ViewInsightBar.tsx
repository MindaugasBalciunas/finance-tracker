import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../../api/insights'
import { useAIAvailable } from '../../hooks/useInsights'
import { useDateRange } from '../../context/DateRangeContext'

// ViewInsightBar: on-demand AI review of the tab the user is on, scoped to
// the selected date range. Nothing is fetched until the user clicks — tokens
// are only spent on request. A review already fetched this session (same
// view + period) shows again instantly from the client cache, and the server
// additionally caches per view+period for 15 minutes.
export default function ViewInsightBar({ view }: { view: string }) {
  const configured = useAIAvailable()
  const { dateRange } = useDateRange()
  const qc = useQueryClient()
  const [refreshing, setRefreshing] = useState(false)

  const queryKey = ['view-summary', view, dateRange.date_from ?? '', dateRange.date_to ?? '']
  const { data: review, isFetching, isError, refetch } = useQuery({
    queryKey,
    queryFn: () => aiApi.viewSummary(view, dateRange),
    enabled: false, // manual only — the ✦ button below triggers it
    staleTime: Infinity,
    retry: false,
  })

  if (!configured) return null

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

  // Idle: a tiny prompt instead of a bar — nothing has been requested yet.
  if (review === undefined && !isFetching) {
    return (
      <button
        onClick={() => refetch()}
        className={`mb-3 text-[11px] ${isError ? 'text-red-400 hover:text-red-600' : 'text-indigo-400 hover:text-indigo-600'}`}
        title="Ask the AI to review this view (uses tokens)"
      >
        ✦ AI review{isError ? ' — failed, try again' : ''}
      </button>
    )
  }

  return (
    <div className="mb-3 flex items-start gap-2 rounded-xl border border-indigo-100 bg-gradient-to-r from-indigo-50/70 to-blue-50/70 px-3 py-2">
      <span className="text-indigo-500 mt-0.5">✦</span>
      <div className="flex-1 min-w-0">
        <p className="text-[13px] leading-snug text-gray-700">
          {isFetching || refreshing
            ? <span className="text-gray-400 animate-pulse">Reviewing this view…</span>
            : review?.summary}
        </p>
        {/* What this review cost — mirrors the chat's per-answer badge. */}
        {!isFetching && !refreshing && review && ((review.costUsd ?? 0) > 0 || (review.inputTokens ?? 0) + (review.outputTokens ?? 0) > 0) && (
          <p className="mt-0.5 text-[11px] text-gray-400">
            {[
              (review.costUsd ?? 0) > 0 ? `$${parseFloat(review.costUsd.toFixed(4))}` : null,
              (review.inputTokens ?? 0) + (review.outputTokens ?? 0) > 0
                ? `${(() => { const n = (review.inputTokens ?? 0) + (review.outputTokens ?? 0); return n >= 1_000_000 ? `${parseFloat((n / 1_000_000).toFixed(1))}M` : n >= 1_000 ? `${parseFloat((n / 1_000).toFixed(1))}k` : `${n}` })()} tokens`
                : null,
            ].filter(Boolean).join(' · ')}
          </p>
        )}
      </div>
      <span className="flex items-center gap-0.5 shrink-0">
        <button
          onClick={refresh}
          disabled={isFetching || refreshing}
          aria-label="Refresh AI review"
          title="Re-run the review (bypasses the cache)"
          className="p-1 text-indigo-300 hover:text-indigo-600 disabled:opacity-40"
        >
          ↻
        </button>
        <button
          onClick={() => qc.removeQueries({ queryKey })}
          aria-label="Dismiss AI review"
          title="Dismiss"
          className="p-1 text-indigo-300 hover:text-indigo-600"
        >
          ✕
        </button>
      </span>
    </div>
  )
}
