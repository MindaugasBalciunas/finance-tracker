// Savings rate as a meter: the fill is this month, the tick the 6-month
// average. A negative month shows an empty track and says so in words.
export default function SavingsMeter({ rate, avg, prev }: { rate?: number; avg?: number; prev?: number }) {
  if (rate == null) return null
  const clamp = (v: number) => Math.max(0, Math.min(100, v))
  const tone = rate < 0 ? 'bg-red-500' : rate < (avg ?? 0) ? 'bg-amber-500' : 'bg-green-600'
  const track = rate < 0 ? 'bg-red-100' : rate < (avg ?? 0) ? 'bg-amber-100' : 'bg-green-100'
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-sm font-semibold text-gray-900">Savings rate</h3>
        <span className={`text-2xl font-semibold ${rate < 0 ? 'text-red-600' : 'text-gray-900'}`}>{rate.toFixed(1)}%</span>
      </div>
      <div className={`relative mt-3 h-3 rounded-full ${track}`} role="meter" aria-valuenow={rate} aria-valuemin={0} aria-valuemax={100} aria-label="Savings rate">
        <div className={`h-3 rounded-full ${tone}`} style={{ width: `${clamp(rate)}%` }} />
        {avg != null && (
          <div className="absolute -top-1 h-5 w-0.5 bg-gray-800 rounded" style={{ left: `calc(${clamp(avg)}% - 1px)` }} title={`6-month average ${avg.toFixed(1)}%`} />
        )}
      </div>
      <div className="flex justify-between mt-1.5 text-xs text-gray-400">
        <span>{rate < 0 ? '⚠ spent more than came in' : '0%'}</span>
        <span>
          <span className="inline-block w-0.5 h-2.5 bg-gray-800 align-middle mr-1" />
          6-mo avg {avg != null ? `${avg.toFixed(1)}%` : '—'}
          {prev != null && <> · last month {prev.toFixed(1)}%</>}
        </span>
      </div>
    </div>
  )
}
