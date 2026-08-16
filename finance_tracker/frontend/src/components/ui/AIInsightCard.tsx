import { useMemo } from 'react'
import { useLatestInsight, useGenerateInsight } from '../../hooks/useInsights'
import { useDateRange } from '../../context/DateRangeContext'

// The five sections the backend prompt emits, each behind a "## Heading"
// line — matched here so each renders as its own labelled card.
const SECTIONS: { key: string; icon: string }[] = [
  { key: 'Transactions', icon: '💸' },
  { key: 'Balances', icon: '🏦' },
  { key: 'Stocks', icon: '📉' },
  { key: 'Budget', icon: '🎯' },
  { key: 'Reports', icon: '📈' },
]

interface Parsed {
  period: string | null
  sections: { key: string; icon: string; body: string }[]
  preamble: string // anything before the first heading (fallback for old/free-form insights)
}

function parseInsight(content: string): Parsed {
  const lines = content.split('\n')
  let period: string | null = null
  if (lines[0]?.toLowerCase().startsWith('period:')) {
    period = lines.shift()!.slice('period:'.length).trim()
  }
  const rest = lines.join('\n')

  // Split on "## " headings, keeping track of which known section each is.
  const parts = rest.split(/^##\s+/m)
  const preamble = parts[0].trim()
  const sections: Parsed['sections'] = []
  for (let i = 1; i < parts.length; i++) {
    const block = parts[i]
    const nl = block.indexOf('\n')
    const heading = (nl === -1 ? block : block.slice(0, nl)).trim()
    const body = (nl === -1 ? '' : block.slice(nl + 1)).trim()
    const known = SECTIONS.find((s) => heading.toLowerCase().startsWith(s.key.toLowerCase()))
    sections.push({ key: known?.key ?? heading, icon: known?.icon ?? '•', body })
  }
  return { period, sections, preamble }
}

export default function AIInsightCard() {
  const { data: insight, isLoading } = useLatestInsight()
  const generate = useGenerateInsight()
  const { dateRange } = useDateRange()

  const parsed = useMemo(() => (insight ? parseInsight(insight.content) : null), [insight])

  const formattedDate = insight
    ? new Date(insight.created_at).toLocaleString('default', {
        day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit',
      })
    : null

  const run = () => generate.mutate({ date_from: dateRange.date_from, date_to: dateRange.date_to })

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-xl">✦</span>
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-gray-900">AI Financial Overview</h3>
            <p className="text-xs text-gray-400 mt-0.5 truncate">
              {parsed?.period ? <>Covers {parsed.period}</> : 'Per-section analysis of your finances'}
              {formattedDate && <> · generated {formattedDate}</>}
            </p>
          </div>
        </div>
        <button
          onClick={run}
          disabled={generate.isPending}
          className="flex-shrink-0 px-3 py-1.5 text-xs font-medium rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {generate.isPending ? 'Analysing…' : insight ? 'Refresh' : 'Generate'}
        </button>
      </div>

      {generate.isPending && (
        <div className="flex items-center gap-2 text-sm text-indigo-600 py-6 justify-center">
          <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
          </svg>
          Analysing your finances for the selected period…
        </div>
      )}

      {!generate.isPending && parsed && parsed.sections.length > 0 && (
        <div className="grid gap-2.5 sm:grid-cols-2">
          {parsed.sections.map((sec) => (
            <div key={sec.key} className="rounded-xl border border-gray-100 bg-gray-50/60 p-3">
              <div className="flex items-center gap-1.5 mb-1">
                <span>{sec.icon}</span>
                <h4 className="text-sm font-semibold text-gray-800">{sec.key}</h4>
              </div>
              <p className="text-[13px] text-gray-600 leading-relaxed whitespace-pre-wrap">{sec.body}</p>
            </div>
          ))}
        </div>
      )}

      {/* Fallback for a free-form insight with no section headings. */}
      {!generate.isPending && parsed && parsed.sections.length === 0 && (
        <div className="text-sm text-gray-700 leading-relaxed whitespace-pre-wrap">
          {parsed.preamble || 'The model returned no analysis sections — hit Refresh to regenerate.'}
        </div>
      )}

      {!generate.isPending && !insight && !isLoading && (
        <p className="text-sm text-gray-500 italic py-4">
          Generate a per-section overview — Transactions, Balances, Stocks, Budget and Reports — scoped to the
          date range selected in the header.
        </p>
      )}

      {generate.error && (
        <p className="mt-3 text-xs text-red-500">
          {((generate.error as { response?: { data?: { error?: string } } }).response?.data?.error) ??
            (generate.error as Error).message}
        </p>
      )}

      <p className="mt-3 text-xs text-gray-400">
        Runs through your configured AI gateway · reflects the header date range and your latest balances,
        budgets and live stock prices
      </p>
    </div>
  )
}
