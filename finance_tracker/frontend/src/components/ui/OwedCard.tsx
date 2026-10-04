import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { transactionsApi } from '../../api/transactions'
import { useMarkRepayment, useOwed } from '../../hooks/useSplits'
import { formatEuro } from '../../utils/format'

// Money other people owe you: shares you fronted (split parts marked
// "someone owes me this") minus what they paid back. Hidden when nobody owes
// anything.
export default function OwedCard() {
  const { data } = useOwed()
  const [settling, setSettling] = useState<string | null>(null)
  const open = (data ?? []).filter((p) => Math.abs(p.outstanding) >= 0.01)
  if (open.length === 0) return null
  const total = open.reduce((s, p) => s + p.outstanding, 0)

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex items-baseline justify-between mb-3">
        <h3 className="text-base font-semibold text-gray-900">Owed to you</h3>
        <span className="text-base font-bold text-gray-900">{formatEuro(total)}</span>
      </div>
      <ul className="divide-y divide-gray-100">
        {open.map((p) => (
          <li key={p.person} className="py-2">
            <div className="flex items-baseline justify-between gap-2">
              <span className="text-sm font-medium text-gray-800">{p.name}</span>
              <span className={`text-sm font-semibold ${p.outstanding > 0 ? 'text-gray-900' : 'text-green-600'}`}>
                {p.outstanding < 0 ? `overpaid ${formatEuro(-p.outstanding)}` : formatEuro(p.outstanding)}
              </span>
            </div>
            <p className="text-xs text-gray-400">
              Fronted {formatEuro(p.lent)} · paid back {formatEuro(p.repaid)}
            </p>
            {settling === p.person ? (
              <RepaymentPicker person={p.person} name={p.name} onDone={() => setSettling(null)} />
            ) : (
              <button onClick={() => setSettling(p.person)} className="mt-1 text-xs text-blue-600 hover:underline">
                Mark a received payment as repayment
              </button>
            )}
          </li>
        ))}
      </ul>
      <p className="text-xs text-gray-400 mt-2">
        Fronted shares and repayments count as transfers — not your spending, not your income.
      </p>
    </div>
  )
}

function RepaymentPicker({ person, name, onDone }: { person: string; name: string; onDone: () => void }) {
  const from = new Date(Date.now() - 120 * 86400_000).toISOString().slice(0, 10)
  const { data, isLoading } = useQuery({
    queryKey: ['transactions', 'repayment-candidates', from],
    queryFn: () => transactionsApi.list({ type: 'income', date_from: from, page: 1, page_size: 30, sort: 'date', dir: 'desc' }),
  })
  const mark = useMarkRepayment()
  const [error, setError] = useState<string | null>(null)
  const rows = (data?.data ?? []).filter((t) => !t.labels.split(',').some((l) => l.startsWith('owed-')))

  return (
    <div className="mt-2 border border-gray-200 rounded-lg p-2 space-y-1">
      <p className="text-xs text-gray-500">Which payment came from {name}?</p>
      {isLoading && <p className="text-xs text-gray-400">Loading…</p>}
      {!isLoading && rows.length === 0 && <p className="text-xs text-gray-400">No incoming payments in the last 120 days.</p>}
      <ul className="max-h-48 overflow-y-auto">
        {rows.map((t) => (
          <li key={t.id}>
            <button
              disabled={mark.isPending}
              onClick={() => mark.mutate({ id: t.id, person }, { onSuccess: onDone, onError: (e: any) => setError(e?.response?.data?.error ?? 'Could not mark it') })}
              className="w-full flex justify-between gap-2 text-left text-xs px-2 py-1.5 rounded hover:bg-blue-50"
            >
              <span className="truncate text-gray-700">{t.date.slice(0, 10)} · {t.comment || t.category}</span>
              <span className="font-semibold text-green-600 flex-shrink-0">+{formatEuro(t.amount.value)}</span>
            </button>
          </li>
        ))}
      </ul>
      {error && <p className="text-xs text-red-600">{error}</p>}
      <button onClick={onDone} className="text-xs text-gray-500 hover:underline">Cancel</button>
    </div>
  )
}
