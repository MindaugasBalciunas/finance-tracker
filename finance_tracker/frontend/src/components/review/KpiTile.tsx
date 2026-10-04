import { formatEuro } from '../../utils/format'
import Sparkline from './Sparkline'

// Stat tile: label · value · signed change vs a named reference · trend.
// The change is colored by whether it is good news and always carries an
// arrow, so direction never rests on color alone.
export default function KpiTile({ label, value, previous, refLabel = 'vs last month', goodWhenUp, trend, accent, note }: {
  label: string
  value: number
  previous?: number
  refLabel?: string
  goodWhenUp: boolean
  trend?: number[]
  accent: string
  note?: string
}) {
  const d = previous == null ? null : value - previous
  const flat = d == null || Math.abs(d) < 0.5
  const good = d != null && (goodWhenUp ? d > 0 : d < 0)
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-3 sm:p-4 flex flex-col gap-1 min-w-0">
      <div className="flex items-center gap-1.5">
        <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: accent }} />
        <span className="text-xs font-medium text-gray-500 truncate">{label}</span>
      </div>
      <span className={`text-lg sm:text-xl font-semibold ${value < 0 ? 'text-red-600' : 'text-gray-900'}`}>{formatEuro(value)}</span>
      <span className="text-xs">
        {flat ? (
          <span className="text-gray-400">{d == null ? note ?? ' ' : `about the same ${refLabel.replace('vs ', 'as ')}`}</span>
        ) : (
          <span className={good ? 'text-green-700' : 'text-red-600'}>
            {d! > 0 ? '▲' : '▼'} {formatEuro(Math.abs(d!))} <span className="text-gray-400">{refLabel}</span>
          </span>
        )}
      </span>
      {trend && trend.length > 1 && <div className="mt-1"><Sparkline values={trend} accent={accent} /></div>}
    </div>
  )
}
