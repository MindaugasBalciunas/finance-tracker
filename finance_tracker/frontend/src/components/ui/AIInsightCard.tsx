import { useLatestInsight, useGenerateInsight } from '../../hooks/useInsights'

export default function AIInsightCard() {
  const { data: insight, isLoading } = useLatestInsight()
  const generate = useGenerateInsight()

  const formattedDate = insight
    ? new Date(insight.created_at).toLocaleString('default', {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: '2-digit', minute: '2-digit',
      })
    : null

  return (
    <div className="bg-gradient-to-br from-blue-50 to-indigo-50 rounded-xl border border-blue-100 p-6">
      <div className="flex items-start justify-between gap-4 mb-4">
        <div className="flex items-center gap-2">
          <span className="text-xl">✦</span>
          <div>
            <h3 className="text-base font-semibold text-gray-900">AI Financial Overview</h3>
            {formattedDate && (
              <p className="text-xs text-gray-400 mt-0.5">Generated {formattedDate}</p>
            )}
          </div>
        </div>
        <button
          onClick={() => generate.mutate()}
          disabled={generate.isPending}
          className="flex-shrink-0 px-3 py-1.5 text-xs font-medium rounded-lg bg-white border border-blue-200 text-blue-700 hover:bg-blue-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors shadow-sm"
        >
          {generate.isPending ? 'Generating…' : insight ? 'Refresh' : 'Generate'}
        </button>
      </div>

      {generate.isPending && (
        <div className="flex items-center gap-2 text-sm text-blue-600 py-4">
          <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
          </svg>
          Analysing your finances…
        </div>
      )}

      {!generate.isPending && insight && (
        <div className="text-sm text-gray-700 leading-relaxed whitespace-pre-wrap">
          {insight.content}
        </div>
      )}

      {!generate.isPending && !insight && !isLoading && (
        <p className="text-sm text-gray-500 italic">
          Click "Generate" to get a personalised AI overview of your finances.
        </p>
      )}

      {generate.error && (
        <p className="mt-3 text-xs text-red-500">
          Error: {(generate.error as Error).message}
        </p>
      )}

      <p className="mt-4 text-xs text-gray-400">
        Runs through your configured AI gateway · Analysis based on all data in your database
      </p>
    </div>
  )
}
