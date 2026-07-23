import { useEffect, useMemo, useState } from 'react'
import { useLabelRules, useDeleteRule } from '../../hooks/useBudgets'

interface Props {
  onClose: () => void
}

// All saved label rules, grouped by label, each deletable — the undo button
// for rules created from the transaction form (or anywhere else).
export default function LabelRulesModal({ onClose }: Props) {
  const { data: rules = [], isLoading } = useLabelRules()
  const deleteRule = useDeleteRule()
  const [confirmId, setConfirmId] = useState<number | null>(null)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const grouped = useMemo(() => {
    const g = new Map<string, typeof rules>()
    for (const r of rules) {
      const list = g.get(r.label) ?? []
      list.push(r)
      g.set(r.label, list)
    }
    return [...g.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [rules])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-3 sm:p-6 overflow-y-auto"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-md max-h-[85vh] overflow-y-auto">
        <div className="flex items-center justify-between px-4 sm:px-5 py-3 border-b border-gray-100 sticky top-0 bg-white rounded-t-2xl">
          <div>
            <h2 className="font-semibold text-gray-900">⚡ Label rules</h2>
            <p className="text-xs text-gray-400">{rules.length} rules — new transactions matching a pattern get the label</p>
          </div>
          <button onClick={onClose} className="p-1 text-gray-400 hover:text-gray-700">✕</button>
        </div>

        <div className="p-4 sm:p-5 space-y-3">
          {isLoading && <p className="text-sm text-gray-400">Loading…</p>}
          {grouped.map(([label, list]) => (
            <div key={label}>
              <p className="text-xs font-semibold text-indigo-600 mb-1">{label}</p>
              <div className="flex flex-wrap gap-1.5">
                {list.map((r) => (
                  <span
                    key={r.id}
                    className="inline-flex items-center gap-1.5 text-xs bg-gray-50 border border-gray-200 rounded-full px-2.5 py-1"
                  >
                    <span className="text-gray-700">
                      {r.comment_match ? <>“{r.comment_match}”</> : null}
                      {r.comment_match && r.category ? ' · ' : null}
                      {r.category ? <span className="text-gray-400">{r.category}</span> : null}
                    </span>
                    {confirmId === r.id ? (
                      <button
                        onClick={() => { deleteRule.mutate(r.id); setConfirmId(null) }}
                        className="text-red-600 font-semibold hover:text-red-800"
                      >
                        delete?
                      </button>
                    ) : (
                      <button
                        onClick={() => setConfirmId(r.id)}
                        className="text-gray-300 hover:text-red-500 leading-none"
                        aria-label={`Delete rule ${r.comment_match || r.category} for ${label}`}
                      >
                        ×
                      </button>
                    )}
                  </span>
                ))}
              </div>
            </div>
          ))}
          {!isLoading && rules.length === 0 && (
            <p className="text-sm text-gray-400">No rules yet. Add a label on a transaction and tap “Create rule”.</p>
          )}
          <p className="text-[11px] text-gray-400 pt-1">
            Deleting a rule stops future auto-labeling; already-applied labels stay on transactions.
          </p>
        </div>
      </div>
    </div>
  )
}
