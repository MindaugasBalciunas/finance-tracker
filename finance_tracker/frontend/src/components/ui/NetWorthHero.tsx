import { AreaChart, Area, YAxis, ResponsiveContainer } from 'recharts'
import type { Balance, BalanceTrend } from '../../types'
import { GROUPS as BALANCE_GROUPS, OTHER_COLOR, cryptoEur, cryptoSubtitle } from '../../utils/balanceGroups'
import { formatEuro } from '../../utils/format'

interface Props {
  balance: Balance
  btcPrice: number | null
  trend?: BalanceTrend
  change: number | null
  changePct: number | null
}

const GROUPS = BALANCE_GROUPS.map((g) => ({ key: g.key, fn: g.total, color: g.color }))

// One hero card replacing five stat tiles: total, period change, trend
// sparkline, and a composition bar showing where the money sits.
export default function NetWorthHero({ balance, btcPrice, trend, change, changePct }: Props) {
  const total = balance.total
  const parts = GROUPS
    .map((g) => ({ ...g, value: g.fn(balance) }))
    .filter((p) => p.value > 0.5)
  const grouped = parts.reduce((s, p) => s + p.value, 0)
  if (total - grouped > 0.5) {
    parts.push({ key: 'Other', fn: () => 0, color: OTHER_COLOR, value: total - grouped })
  }

  const spark = (trend?.dates ?? []).map((d, i) => ({ d, v: trend!.totals[i] ?? 0 }))
  const sparkUp = spark.length > 1 && spark[spark.length - 1].v >= spark[0].v
  const sparkColor = sparkUp ? '#16a34a' : '#dc2626'

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-gray-500">Net Worth</p>
          <p className="text-3xl sm:text-4xl font-bold text-gray-900 mt-1">{formatEuro(total)}</p>
          {change != null && (
            <p className={`text-sm font-semibold mt-1 ${change >= 0 ? 'text-green-600' : 'text-red-600'}`}>
              {change >= 0 ? '▲' : '▼'} {formatEuro(Math.abs(change))}
              {changePct != null && ` · ${changePct >= 0 ? '+' : ''}${changePct.toFixed(1)}%`}
              <span className="text-gray-400 font-normal"> in period</span>
            </p>
          )}
        </div>
        {spark.length > 1 && (
          <div className="w-full sm:w-72 h-16 flex-shrink-0">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={spark} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="nw-spark" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={sparkColor} stopOpacity={0.25} />
                    <stop offset="100%" stopColor={sparkColor} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <YAxis hide domain={['dataMin', 'dataMax']} />
                <Area
                  type="monotone"
                  dataKey="v"
                  stroke={sparkColor}
                  strokeWidth={2}
                  fill="url(#nw-spark)"
                  dot={false}
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>

      {total > 0 && parts.length > 0 && (
        <div className="mt-4">
          <div className="flex h-3 rounded-full overflow-hidden bg-gray-100">
            {parts.map((p) => (
              <div
                key={p.key}
                style={{ width: `${(p.value / total) * 100}%`, backgroundColor: p.color }}
                title={`${p.key}: ${formatEuro(p.value)}`}
              />
            ))}
          </div>
          <div className="grid grid-cols-2 sm:flex sm:flex-wrap gap-x-6 gap-y-1.5 mt-3">
            {parts.map((p) => (
              <div key={p.key} className="flex flex-wrap items-baseline gap-x-1.5 text-xs min-w-0">
                <span className="w-2.5 h-2.5 rounded-sm flex-shrink-0 self-center" style={{ backgroundColor: p.color }} />
                <span className="text-gray-500">{p.key}</span>
                <span className="font-semibold text-gray-800">{formatEuro(p.value)}</span>
                <span className="text-gray-400">{((p.value / total) * 100).toFixed(0)}%</span>
              </div>
            ))}
          </div>
          {cryptoEur(balance) > 0.5 && (
            <p className="text-xs text-gray-400 mt-2">Crypto: {cryptoSubtitle(balance, btcPrice)}</p>
          )}
        </div>
      )}
    </div>
  )
}
