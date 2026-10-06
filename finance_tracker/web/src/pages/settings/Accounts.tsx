import { useState } from 'react'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useAccounts, useRefresh } from '../../lib/hooks'
import { GROUPS } from '../../lib/categories'
import { eurc, shortDate } from '../../lib/format'
import type { Account } from '../../lib/types'
import { Card, Loading, Toggle, useToast } from '../../components/ui'
import { IconTile } from '../../components/Icon'
import { accountIcon, brandColor } from '../../lib/brand'

// ── accounts ────────────────────────────────────────────────────────

/** Hide closed or unused accounts (Luminor…): they leave Update balances,
 *  the "paid from" pickers and the Wealth lists; history stays intact. */
export function AccountsVisibility() {
  const { data: accounts, isLoading } = useAccounts()
  const refresh = useRefresh()
  const toast = useToast()
  const [busy, setBusy] = useState<string | null>(null)
  if (isLoading || !accounts) return <Loading />
  const set = async (a: Account, shown: boolean) => {
    setBusy(a.id)
    try {
      await api.put(`/accounts/${a.id}`, { ...a, archived: !shown })
      refresh()
      toast(shown ? `${a.name} is back` : `${a.name} hidden`, 'good')
    } catch (e: any) { toast(e?.message ?? 'Failed', 'bad') } finally { setBusy(null) }
  }
  const groups = GROUPS.map((g) => ({ ...g, items: accounts.filter((a) => a.group === g.id) })).filter((g) => g.items.length)
  const other = accounts.filter((a) => !GROUPS.some((g) => g.id === a.group))
  if (other.length) groups.push({ id: 'other', name: 'Other', slot: 0, items: other })
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted">Hidden accounts stop appearing when you update balances or log a transaction, and drop out of the Wealth lists. Their past balances and transactions stay in every total and chart.</p>
      {groups.map((g) => (
        <Card key={g.id} pad={false} title={g.name}>
          <div className="divide-y divide-line border-t border-line">
            {g.items.map((a) => (
              <div key={a.id} className={clsx('flex items-center gap-3 px-4 py-2.5', a.archived && 'opacity-60')}>
                <IconTile name={accountIcon(a)} color={brandColor(a) ?? `var(--s${g.slot || 1})`} size={32} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{a.name}</div>
                  <div className="text-xs text-muted">{a.balance != null ? eurc(a.balance) : 'no balance'}{a.balance_date ? ` · ${shortDate(a.balance_date)}` : ''}</div>
                </div>
                <Toggle checked={!a.archived} onChange={(v) => busy !== a.id && set(a, v)} label={<span className="w-12 text-xs text-muted">{a.archived ? 'Hidden' : 'Shown'}</span>} />
              </div>
            ))}
          </div>
        </Card>
      ))}
    </div>
  )
}
