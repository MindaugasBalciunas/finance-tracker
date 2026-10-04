import type { MonthReview } from '../../api/review'
import { formatEuro } from '../../utils/format'
import { buildHighlights, type Tone } from '../../utils/reviewHighlights'

const TONE_DOT: Record<Tone, string> = { good: 'bg-green-600', neutral: 'bg-gray-400', bad: 'bg-red-500' }
const TONE_LABEL: Record<Tone, string> = { good: 'good', neutral: 'note', bad: 'needs a look' }

// The month in one card: a verdict, the three numbers that make it, the
// savings rate against usual, and a few plain sentences on why.
export default function HighlightsCard({ r }: { r: MonthReview }) {
  const h = buildHighlights(r)
  const rate = r.savings_rate
  const avg = r.six_month_avg.savings_rate
  const clamp = (v: number) => Math.max(0, Math.min(100, v))
  const border = h.tone === 'bad' ? 'border-l-red-500' : h.tone === 'good' ? 'border-l-green-600' : 'border-l-gray-300'

  return (
    <div className={`bg-white rounded-xl border border-gray-200 border-l-4 ${border} p-4 sm:p-6`}>
      <p className="text-xs font-semibold uppercase tracking-wide text-gray-400">Highlights</p>
      <h2 className="text-lg sm:text-xl font-semibold text-gray-900 mt-1">{h.headline}</h2>
      <p className="text-sm text-gray-500 mt-1">
        In <span className="font-semibold text-gray-800">{formatEuro(r.income)}</span>
        {' · '}Out <span className="font-semibold text-gray-800">{formatEuro(r.spending)}</span>
        {r.net_worth && (
          <>
            {' · '}Net worth <span className="font-semibold text-gray-800">{formatEuro(r.net_worth.end)}</span>{' '}
            <span className={r.net_worth.change >= 0 ? 'text-green-700' : 'text-red-600'}>
              ({r.net_worth.change >= 0 ? '▲' : '▼'} {formatEuro(Math.abs(r.net_worth.change))})
            </span>
          </>
        )}
      </p>

      {rate != null && (
        <div className="mt-4">
          <div className="flex items-baseline justify-between text-xs text-gray-500">
            <span>Savings rate <span className={`font-semibold ${rate < 0 ? 'text-red-600' : 'text-gray-900'}`}>{rate.toFixed(1)}%</span></span>
            {avg != null && <span><span className="inline-block w-0.5 h-2.5 bg-gray-800 align-middle mr-1" />usual {avg.toFixed(1)}%</span>}
          </div>
          <div className={`relative mt-1.5 h-2.5 rounded-full ${rate < 0 ? 'bg-red-100' : 'bg-green-100'}`} role="meter" aria-valuenow={rate} aria-valuemin={0} aria-valuemax={100} aria-label="Savings rate">
            <div className={`h-2.5 rounded-full ${rate < (avg ?? 0) ? 'bg-amber-500' : 'bg-green-600'}`} style={{ width: `${clamp(rate)}%` }} />
            {avg != null && <div className="absolute -top-1 h-4.5 w-0.5 bg-gray-800 rounded" style={{ left: `calc(${clamp(avg)}% - 1px)`, height: 18 }} />}
          </div>
        </div>
      )}

      {h.bullets.length > 0 && (
        <ul className="mt-4 space-y-2.5">
          {h.bullets.map((b) => (
            <li key={b.text} className="flex items-start gap-2.5 text-sm text-gray-700 leading-snug">
              <span className="text-base leading-5 flex-shrink-0" aria-hidden>{b.icon}</span>
              <span className="flex-1">{b.text}</span>
              <span className={`mt-1.5 w-2 h-2 rounded-full flex-shrink-0 ${TONE_DOT[b.tone]}`} title={TONE_LABEL[b.tone]} aria-label={TONE_LABEL[b.tone]} />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
