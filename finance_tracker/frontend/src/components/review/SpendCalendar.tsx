import type { ReviewPoint } from '../../api/review'
import { formatEuro } from '../../utils/format'
import { SPEND_RAMP } from './colors'

const WEEKDAYS = ['M', 'T', 'W', 'T', 'F', 'S', 'S']

// Daily spending as a month calendar: one orange ramp, light → dark with the
// amount (quantile steps, so a single huge day doesn't wash out the rest).
// Every day's amount is in its tooltip and the busiest days are listed.
export default function SpendCalendar({ days }: { days: ReviewPoint[] }) {
  if (days.length === 0) return null
  const spent = days.filter((d) => d.value > 0).map((d) => d.value).sort((a, b) => a - b)
  const steps = SPEND_RAMP.length - 1
  const cuts = Array.from({ length: steps - 1 }, (_, i) => spent[Math.floor(((i + 1) / steps) * spent.length)] ?? Infinity)
  const shade = (v: number) => {
    if (v <= 0) return null
    let i = 0
    while (i < cuts.length && v > cuts[i]) i++
    return SPEND_RAMP[i + 1]
  }
  const first = new Date(days[0].date + 'T00:00:00Z')
  const lead = (first.getUTCDay() + 6) % 7 // Monday-first
  const total = days.reduce((s, d) => s + d.value, 0)
  const noSpend = days.filter((d) => d.value <= 0).length
  const top = [...days].sort((a, b) => b.value - a.value).slice(0, 3).filter((d) => d.value > 0)

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5">
      <div className="flex items-baseline justify-between gap-2 mb-3">
        <h3 className="text-sm font-semibold text-gray-900">Spending by day</h3>
        <span className="text-xs text-gray-500">{noSpend} no-spend day{noSpend === 1 ? '' : 's'} · avg {formatEuro(total / days.length)}/day</span>
      </div>
      <div className="grid grid-cols-7 gap-1 max-w-sm mx-auto">
        {WEEKDAYS.map((w, i) => <span key={i} className="text-[10px] text-gray-400 text-center">{w}</span>)}
        {Array.from({ length: lead }, (_, i) => <span key={`lead-${i}`} />)}
        {days.map((d) => {
          const bg = shade(d.value)
          const dark = bg === SPEND_RAMP[4] || bg === SPEND_RAMP[5]
          return (
            <div
              key={d.date}
              title={`${d.date}: ${formatEuro(d.value)}`}
              className={`aspect-square rounded-md flex items-center justify-center text-[11px] ${bg ? '' : 'bg-gray-50'} ${dark ? 'text-white' : 'text-gray-600'}`}
              style={bg ? { backgroundColor: bg } : undefined}
            >
              {Number(d.date.slice(8))}
            </div>
          )
        })}
      </div>
      <div className="flex items-center justify-center gap-1 mt-3 text-[10px] text-gray-400">
        <span>none</span>
        <span className="w-3 h-3 rounded-sm bg-gray-50 border border-gray-100" />
        {SPEND_RAMP.slice(1).map((c) => <span key={c} className="w-3 h-3 rounded-sm" style={{ backgroundColor: c }} />)}
        <span>more</span>
      </div>
      {top.length > 0 && (
        <p className="text-xs text-gray-500 mt-2 text-center">
          Biggest days: {top.map((d) => `${`${Number(d.date.slice(8))} ${['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'][Number(d.date.slice(5, 7)) - 1]}`} ${formatEuro(d.value)}`).join(' · ')}
        </p>
      )}
    </div>
  )
}
