import { useState } from 'react'
import type { BankConnection, SyncResult } from '../../api/banking'
import { ACCOUNT_LABELS, type AccountKey } from '../../types'
import { useDisconnectBank, useMapBankAccount, useSyncBankAccount } from '../../hooks/useBanking'

const statusStyle: Record<BankConnection['status'], string> = {
  authorized: 'bg-emerald-100 text-emerald-700',
  pending: 'bg-amber-100 text-amber-700',
  expired: 'bg-red-100 text-red-700',
  revoked: 'bg-gray-100 text-gray-500',
}

// Consent runs out on a bank-set schedule (usually 90–180 days) and there is no
// background job watching it — so the card says how long is left rather than
// letting a sync fail one day with no explanation.
function expiryNote(days: number): { text: string; className: string } {
  if (days <= 0) return { text: 'Consent expired — reconnect to sync', className: 'text-red-600' }
  if (days <= 14) return { text: `Consent ends in ${days} days`, className: 'text-amber-600' }
  return { text: `Consent ends in ${days} days`, className: 'text-gray-400' }
}

function syncSummary(r: SyncResult): string {
  const bits = [`${r.fetched} fetched`, `${r.staged_new} new to review`]
  if (r.unchanged) bits.push(`${r.unchanged} already seen`)
  // auto_skipped counts one thing only: rows the bank has not booked yet.
  // "skipped" alone reads like something went wrong, when the honest answer
  // is "today's card payments are still reserved — they arrive tomorrow".
  if (r.auto_skipped) bits.push(`${r.auto_skipped} still pending at the bank`)
  return bits.join(' · ')
}

export default function BankConnectionCard({
  conn,
  validAccountKeys,
  onReconnect,
}: {
  conn: BankConnection
  validAccountKeys: string[]
  onReconnect: (aspspName: string, country: string) => void
}) {
  const sync = useSyncBankAccount()
  const mapAccount = useMapBankAccount()
  const disconnect = useDisconnectBank()
  const [results, setResults] = useState<Record<number, string>>({})
  const [errors, setErrors] = useState<Record<number, string>>({})

  const expiry = expiryNote(conn.days_until_expiry)

  const runSync = async (linkId: number, days?: number) => {
    setErrors((e) => ({ ...e, [linkId]: '' }))
    try {
      const res = await sync.mutateAsync({ id: linkId, days })
      setResults((r) => ({ ...r, [linkId]: syncSummary(res) }))
    } catch (err) {
      setErrors((e) => ({ ...e, [linkId]: err instanceof Error ? err.message : 'Sync failed' }))
    }
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="font-semibold text-gray-900 truncate">{conn.aspsp_name}</h3>
            <span className={`px-2 py-0.5 text-xs font-semibold rounded-full ${statusStyle[conn.status]}`}>
              {conn.status}
            </span>
          </div>
          <p className={`text-xs mt-0.5 ${expiry.className}`}>{expiry.text}</p>
        </div>
        <button
          onClick={async () => {
            if (!confirm(`Disconnect ${conn.aspsp_name}? Transactions already added stay; rows still waiting for review are removed.`)) return
            await disconnect.mutateAsync(conn.id)
          }}
          className="text-xs text-gray-400 hover:text-red-600 shrink-0"
        >
          Disconnect
        </button>
      </div>

      {conn.last_error && (
        <p className="mt-2 text-xs text-red-600 bg-red-50 rounded-lg px-3 py-2">{conn.last_error}</p>
      )}

      {(conn.status !== 'authorized' || conn.days_until_expiry <= 14) && (
        <button
          onClick={() => onReconnect(conn.aspsp_name, conn.aspsp_country)}
          className="mt-2 px-3 py-1.5 text-sm rounded-lg border border-indigo-200 text-indigo-700 hover:bg-indigo-50"
        >
          Reconnect
        </button>
      )}

      <div className="mt-3 space-y-3">
        {conn.accounts.length === 0 && (
          <p className="text-xs text-gray-400">No accounts on this connection.</p>
        )}
        {conn.accounts.map((acc) => (
          <div key={acc.id} className="border border-gray-100 rounded-xl p-3">
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <p className="text-sm font-medium text-gray-800 truncate">
                  {acc.display_name || acc.iban || `Account ${acc.id}`}
                </p>
                <p className="text-xs text-gray-400 truncate">{acc.iban}</p>
              </div>
              {acc.pending > 0 && (
                <span className="shrink-0 px-2 py-0.5 text-xs font-semibold rounded-full bg-indigo-100 text-indigo-700">
                  {acc.pending} to review
                </span>
              )}
            </div>

            <div className="mt-2 flex flex-wrap items-center gap-2">
              <select
                value={acc.account_key}
                onChange={(e) => mapAccount.mutate({ id: acc.id, accountKey: e.target.value })}
                className="text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
                aria-label="Which account this is"
              >
                <option value="">Don't sync</option>
                {validAccountKeys.map((k) => (
                  <option key={k} value={k}>
                    {ACCOUNT_LABELS[k as AccountKey] ?? k}
                  </option>
                ))}
              </select>

              {/* Deliberately two buttons: the cautious 7-day look is the one
                  to press first against a real bank, before pulling 90 days
                  of history across six weeks of existing CSV rows. */}
              <button
                onClick={() => runSync(acc.id, 7)}
                disabled={!acc.account_key || sync.isPending}
                className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-700 hover:bg-gray-50 disabled:opacity-40"
              >
                Last 7 days
              </button>
              <button
                onClick={() => runSync(acc.id)}
                disabled={!acc.account_key || sync.isPending}
                className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-40"
              >
                {sync.isPending ? 'Syncing…' : 'Sync now'}
              </button>
            </div>

            {!acc.account_key && (
              <p className="mt-1.5 text-xs text-gray-400">
                Pick which of your accounts this is before syncing.
              </p>
            )}
            {acc.last_synced_at && (
              <p className="mt-1.5 text-xs text-gray-400">
                Last synced {new Date(acc.last_synced_at).toLocaleString('lt-LT')}
              </p>
            )}
            {results[acc.id] && <p className="mt-1.5 text-xs text-emerald-600">{results[acc.id]}</p>}
            {errors[acc.id] && <p className="mt-1.5 text-xs text-red-600">{errors[acc.id]}</p>}
          </div>
        ))}
      </div>
    </div>
  )
}
