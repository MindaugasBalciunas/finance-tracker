import { useMemo } from 'react'
import type { Balance } from '../../types'
import { withAddedAccounts } from '../../utils/balanceGroups'
import { useAccountGroupOf, useAccountName, useAddedAccounts } from '../../hooks/useAccounts'
import { formatEuro, formatDate } from '../../utils/format'

interface Props {
  // Snapshots inside the selected date range (any order).
  balances: Balance[]
}

// Compares the first and last snapshot of the selected range per account:
// where did money arrive, where did it leave. Diverging bars share one
// scale; green/red follows the app's gain/loss convention and every delta
// is written out, so the sign is never carried by color alone.
export default function AccountMovement({ balances }: Props) {
  const added = useAddedAccounts()
  const name = useAccountName()
  const groupOf = useAccountGroupOf()
  const data = useMemo(() => {
    const { groups, other } = withAddedAccounts(added, name, groupOf)
    const ALL_ACCOUNTS = [...groups.flatMap((g) => g.accounts), ...other]
    if (balances.length < 2) return null
    const sorted = [...balances].sort((a, b) => a.date.localeCompare(b.date))
    const first = sorted[0]
    const last = sorted[sorted.length - 1]
    const rows = ALL_ACCOUNTS
      .map((a) => ({ key: a.key, label: a.label, delta: a.value(last) - a.value(first) }))
      .filter((r) => Math.abs(r.delta) > 0.5)
      .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta))
    const maxDelta = Math.max(...rows.map((r) => Math.abs(r.delta)), 1)
    return { first, last, rows, maxDelta, totalDelta: last.total - first.total }
  }, [balances, added, name, groupOf])

  if (!data) return null
  const { first, last, rows, maxDelta, totalDelta } = data
  const up = totalDelta >= 0

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-baseline justify-between mb-1">
        <h3 className="text-base font-semibold text-gray-900">Movement in period</h3>
        <span className={`text-sm font-bold ${up ? 'text-green-600' : 'text-red-600'}`}>
          {up ? '▲' : '▼'} {formatEuro(Math.abs(totalDelta))}
        </span>
      </div>
      <p className="text-xs text-gray-400 mb-3">
        {formatDate(first.date)} → {formatDate(last.date)}
      </p>

      {rows.length === 0 ? (
        <p className="text-sm text-gray-400 py-4 text-center">No account moved in this period.</p>
      ) : (
        <div className="space-y-1.5">
          {rows.map((r) => {
            const positive = r.delta >= 0
            const width = (Math.abs(r.delta) / maxDelta) * 100
            return (
              <div key={r.key} className="flex items-center gap-2 text-xs">
                <span className="w-24 sm:w-28 flex-shrink-0 text-gray-500 truncate">{r.label}</span>
                {/* Diverging bar around a center axis */}
                <div className="flex-1 flex items-center h-3">
                  <div className="w-1/2 flex justify-end">
                    {!positive && (
                      <div className="h-1.5 rounded-l-full bg-red-500" style={{ width: `${Math.max(width, 2)}%`, minWidth: 2 }} />
                    )}
                  </div>
                  <div className="w-px h-3 bg-gray-200 flex-shrink-0" />
                  <div className="w-1/2">
                    {positive && (
                      <div className="h-1.5 rounded-r-full bg-green-500" style={{ width: `${Math.max(width, 2)}%`, minWidth: 2 }} />
                    )}
                  </div>
                </div>
                <span className={`w-20 flex-shrink-0 text-right tabular-nums font-medium ${positive ? 'text-green-700' : 'text-red-600'}`}>
                  {positive ? '+' : '−'}{formatEuro(Math.abs(r.delta))}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
