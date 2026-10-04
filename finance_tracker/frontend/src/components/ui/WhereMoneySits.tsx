import { useMemo } from 'react'
import type { Balance } from '../../types'
import { OTHER_COLOR, withAddedAccounts } from '../../utils/balanceGroups'
import { useAddedAccounts } from '../../hooks/useAccounts'
import { formatEuro } from '../../utils/format'

interface Props {
  balance: Balance
}

// Decomposes the latest snapshot: a group-level composition bar on top, then
// each group broken down into accounts with magnitude bars on one shared
// scale. Every value is written out as text — the bars only carry magnitude,
// so the card stays readable regardless of color perception.
export default function WhereMoneySits({ balance }: Props) {
  const added = useAddedAccounts()
  const { groups, maxAccount, total } = useMemo(() => {
    const { groups: GROUPS, other: OTHER_ACCOUNTS } = withAddedAccounts(added)
    const groups = GROUPS.map((g) => ({
      key: g.key,
      color: g.color,
      total: g.total(balance),
      accounts: g.accounts
        .map((a) => ({ ...a, amount: a.value(balance) }))
        .filter((a) => Math.abs(a.amount) > 0.005)
        .sort((a, b) => b.amount - a.amount),
    })).filter((g) => g.total > 0.005)

    const otherAccounts = OTHER_ACCOUNTS
      .map((a) => ({ ...a, amount: a.value(balance) }))
      .filter((a) => Math.abs(a.amount) > 0.005)
    const grouped = groups.reduce((s, g) => s + g.total, 0)
    if (balance.total - grouped > 0.5) {
      groups.push({
        key: 'Other',
        color: OTHER_COLOR,
        total: balance.total - grouped,
        accounts: otherAccounts,
      })
    }

    const maxAccount = Math.max(...groups.flatMap((g) => g.accounts.map((a) => a.amount)), 1)
    return { groups, maxAccount, total: balance.total }
  }, [balance, added])

  if (total <= 0) return null

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-baseline justify-between mb-3">
        <h3 className="text-base font-semibold text-gray-900">Where your money sits</h3>
        <span className="text-sm font-bold text-gray-900">{formatEuro(total)}</span>
      </div>

      {/* Group composition bar — 2px gaps keep segments apart without color */}
      <div className="flex h-3 rounded-full overflow-hidden bg-gray-100 gap-0.5">
        {groups.map((g) => (
          <div
            key={g.key}
            className="h-full first:rounded-l-full last:rounded-r-full"
            style={{ width: `${(g.total / total) * 100}%`, backgroundColor: g.color }}
            title={`${g.key}: ${formatEuro(g.total)}`}
          />
        ))}
      </div>

      <div className="mt-4 space-y-4">
        {groups.map((g) => (
          <div key={g.key}>
            <div className="flex items-center justify-between text-sm">
              <span className="flex items-center gap-2 font-medium text-gray-700">
                <span className="w-2.5 h-2.5 rounded-sm flex-shrink-0" style={{ backgroundColor: g.color }} />
                {g.key}
              </span>
              <span className="text-gray-700">
                <span className="font-semibold">{formatEuro(g.total)}</span>
                <span className="text-gray-400 ml-1.5">{((g.total / total) * 100).toFixed(0)}%</span>
              </span>
            </div>
            {g.accounts.length > 0 && (
              <div className="mt-1.5 space-y-1">
                {g.accounts.map((a) => (
                  <div key={a.key} className="flex items-center gap-2 text-xs">
                    <span className="w-24 sm:w-28 flex-shrink-0 text-gray-500 truncate">{a.label}</span>
                    <div className="flex-1 h-1.5 rounded-full bg-gray-50">
                      <div
                        className="h-full rounded-full"
                        style={{
                          width: `${Math.max((a.amount / maxAccount) * 100, 1)}%`,
                          backgroundColor: g.color,
                          opacity: 0.75,
                        }}
                      />
                    </div>
                    <span className="w-20 flex-shrink-0 text-right text-gray-700 tabular-nums">{formatEuro(a.amount)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
