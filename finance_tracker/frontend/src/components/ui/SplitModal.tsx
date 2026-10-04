import { useState } from 'react'
import type { Transaction } from '../../types'
import { CATEGORIES_BY_TYPE } from '../../constants/categories'
import { useSplitTransaction, useUnsplitTransaction } from '../../hooks/useSplits'
import { formatEuro } from '../../utils/format'

type Row = { amount: string; category: string; labels: string; comment: string; owed: boolean; owedBy: string }

const toCents = (s: string) => Math.round((parseFloat(s.replace(',', '.')) || 0) * 100)

// Splits one transaction into parts. Part one stays the original row (same
// bank id, same comment) so dedup still recognises it; the rest are new rows
// linked back to it. A part can instead be money someone owes you — it is
// then filed as a transfer, not spending, until they pay it back.
export default function SplitModal({ tx, onClose }: { tx: Transaction; onClose: () => void }) {
  const total = Math.round(tx.amount.value * 100)
  const half = Math.floor(total / 2)
  const [rows, setRows] = useState<Row[]>([
    { amount: ((total - half) / 100).toFixed(2), category: tx.category, labels: tx.labels, comment: '', owed: false, owedBy: '' },
    { amount: (half / 100).toFixed(2), category: tx.category, labels: '', comment: '', owed: false, owedBy: '' },
  ])
  const [error, setError] = useState<string | null>(null)
  const [alreadySplit, setAlreadySplit] = useState(!!tx.split_of)
  const split = useSplitTransaction()
  const unsplit = useUnsplitTransaction()
  const categories = CATEGORIES_BY_TYPE[tx.type] ?? []

  const sum = rows.reduce((s, r) => s + toCents(r.amount), 0)
  const left = total - sum
  const set = (i: number, patch: Partial<Row>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))

  const submit = () => {
    setError(null)
    split.mutate({
      id: tx.id,
      parts: rows.map((r) => ({
        amount: toCents(r.amount) / 100,
        category: r.owed ? undefined : r.category,
        labels: r.labels,
        comment: r.comment || undefined,
        owed_by: r.owed ? r.owedBy : undefined,
      })),
    }, {
      onSuccess: onClose,
      onError: (e: any) => {
        const msg = e?.response?.data?.error ?? 'Could not split'
        setError(msg)
        if (/already split|already a part/.test(msg)) setAlreadySplit(true)
      },
    })
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
      <div className="bg-white rounded-xl shadow-xl p-5 sm:p-6 w-full max-w-lg mx-4">
        <h3 className="text-lg font-semibold text-gray-900">Split transaction</h3>
        <p className="text-sm text-gray-500 mt-0.5 mb-4">
          {tx.date.slice(0, 10)} · {tx.comment || tx.category} · <span className="font-semibold">{formatEuro(tx.amount.value)}</span>
        </p>

        {alreadySplit ? (
          <div className="space-y-3">
            <p className="text-sm text-gray-600">
              This transaction is already split. Undo the split to fold its parts back into one row, then split it again.
            </p>
            {error && <p className="text-xs text-red-600">{error}</p>}
            <div className="flex justify-end gap-2">
              <button onClick={onClose} className="px-4 py-2 text-sm border border-gray-300 rounded-lg">Close</button>
              <button
                onClick={() => unsplit.mutate(tx.id, { onSuccess: onClose, onError: (e: any) => setError(e?.response?.data?.error ?? 'Could not undo') })}
                disabled={unsplit.isPending}
                className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg disabled:opacity-50"
              >
                Undo split
              </button>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            {rows.map((r, i) => (
              <div key={i} className="border border-gray-200 rounded-lg p-3 space-y-2">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium text-gray-400 w-12">Part {i + 1}</span>
                  <input
                    inputMode="decimal"
                    value={r.amount}
                    onChange={(e) => set(i, { amount: e.target.value })}
                    className="w-24 border border-gray-300 rounded-lg px-2 py-1 text-sm text-right"
                    aria-label={`Amount of part ${i + 1}`}
                  />
                  <span className="text-sm text-gray-400">€</span>
                  {rows.length > 2 && (
                    <button onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))} className="ml-auto text-gray-400 hover:text-red-600 text-sm" aria-label="Remove part">✕</button>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-gray-600">
                  <input type="checkbox" checked={r.owed} onChange={(e) => set(i, { owed: e.target.checked })} />
                  Someone owes me this
                </label>
                {r.owed ? (
                  <input
                    value={r.owedBy}
                    onChange={(e) => set(i, { owedBy: e.target.value })}
                    placeholder="Who owes it, e.g. Tomas"
                    className="w-full border border-gray-300 rounded-lg px-2 py-1 text-sm"
                  />
                ) : (
                  <div className="grid grid-cols-2 gap-2">
                    <select value={r.category} onChange={(e) => set(i, { category: e.target.value })} className="border border-gray-300 rounded-lg px-2 py-1 text-sm" aria-label="Category">
                      {!categories.includes(r.category) && <option value={r.category}>{r.category}</option>}
                      {categories.map((c) => <option key={c} value={c}>{c}</option>)}
                    </select>
                    <input
                      value={r.labels}
                      onChange={(e) => set(i, { labels: e.target.value })}
                      placeholder="labels"
                      className="border border-gray-300 rounded-lg px-2 py-1 text-sm"
                    />
                  </div>
                )}
                {i > 0 && (
                  <input
                    value={r.comment}
                    onChange={(e) => set(i, { comment: e.target.value })}
                    placeholder={`Comment (default: ${tx.comment || '—'})`}
                    className="w-full border border-gray-200 rounded-lg px-2 py-1 text-xs"
                  />
                )}
              </div>
            ))}

            <div className="flex items-center justify-between text-sm">
              <button
                onClick={() => setRows((rs) => [...rs, { amount: left > 0 ? (left / 100).toFixed(2) : '', category: tx.category, labels: '', comment: '', owed: false, owedBy: '' }])}
                className="text-blue-600 hover:underline"
              >
                + Add part
              </button>
              <span className={left === 0 ? 'text-green-600' : 'text-amber-700'}>
                {left === 0 ? 'Adds up' : left > 0 ? `${formatEuro(left / 100)} left` : `${formatEuro(-left / 100)} too much`}
              </span>
            </div>

            {error && <p className="text-xs text-red-600">{error}</p>}
            <div className="flex justify-end gap-2 pt-1">
              <button onClick={onClose} className="px-4 py-2 text-sm border border-gray-300 rounded-lg">Cancel</button>
              <button
                onClick={submit}
                disabled={left !== 0 || split.isPending}
                className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg disabled:opacity-50"
              >
                Split
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
