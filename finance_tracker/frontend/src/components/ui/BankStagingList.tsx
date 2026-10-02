import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { StagedTx, StagedVerdict } from '../../api/banking'
import { ACCOUNT_LABELS, type AccountKey } from '../../types'
import { formatEuro } from '../../utils/format'
import {
  useCommitStaged,
  useDismissStaged,
  useRestoreStaged,
  useUndoCommit,
} from '../../hooks/useBanking'
import StagedReviewModal from './StagedReviewModal'

const verdictStyle: Record<StagedVerdict, { label: string; className: string }> = {
  new: { label: 'new', className: 'bg-emerald-100 text-emerald-700' },
  duplicate_exact: { label: 'already imported', className: 'bg-gray-100 text-gray-500' },
  duplicate_content: { label: 'looks like a duplicate', className: 'bg-amber-100 text-amber-800' },
  internal: { label: 'own transfer', className: 'bg-blue-100 text-blue-700' },
  needs_review: { label: 'needs a look', className: 'bg-purple-100 text-purple-700' },
}

function accountLabel(key: string): string {
  return ACCOUNT_LABELS[key as AccountKey] ?? key
}

function StagedCard({
  row,
  selected,
  onToggle,
  busy,
  onAdd,
  onDismiss,
  onRestore,
}: {
  row: StagedTx
  selected: boolean
  onToggle: () => void
  busy: boolean
  onAdd: () => void
  onDismiss: () => void
  onRestore: () => void
}) {
  // Reviewing one row opens the ordinary transaction form, the same one
  // "+ Add" and editing a saved row use.
  const [reviewing, setReviewing] = useState(false)
  const verdict = verdictStyle[row.verdict] ?? verdictStyle.needs_review
  const signed = row.type === 'income' ? row.amount : -row.amount

  return (
    <div
      className={`bg-white rounded-2xl border shadow-sm ${
        row.state === 'dismissed' ? 'border-gray-100 opacity-60' : 'border-gray-100'
      }`}
    >
      {row.verdict === 'duplicate_content' && row.state === 'staged' && (
        <div className="bg-amber-50 text-amber-800 text-xs px-4 py-2 rounded-t-2xl border-b border-amber-100">
          {row.verdict_note || 'Matches a transaction already in your ledger.'}{' '}
          {row.matched_tx_id && (
            <Link to="/transactions" className="underline font-medium">
              View transactions
            </Link>
          )}
        </div>
      )}

      <div className="p-3 sm:p-4">
        <div className="flex items-start gap-3">
          {row.state === 'staged' && (
            <input
              type="checkbox"
              checked={selected}
              onChange={onToggle}
              className="mt-1 w-4 h-4 shrink-0 accent-indigo-600"
              aria-label={`Select ${row.comment}`}
            />
          )}

          <button
            onClick={() => row.state === 'staged' && setReviewing(true)}
            disabled={row.state !== 'staged'}
            className="flex-1 min-w-0 text-left disabled:cursor-default"
          >
            <div className="flex items-baseline justify-between gap-2">
              <span className="text-sm font-medium text-gray-900 truncate">
                {row.comment || row.raw_payee || '(no description)'}
              </span>
              <span
                className={`text-sm font-semibold shrink-0 ${
                  signed < 0 ? 'text-gray-900' : 'text-emerald-600'
                }`}
              >
                {signed < 0 ? '' : '+'}
                {formatEuro(signed)}
              </span>
            </div>
            <div className="flex flex-wrap items-center gap-1.5 mt-1.5">
              <span className="text-xs text-gray-400">{row.date.slice(0, 10)}</span>
              <span className="px-1.5 py-0.5 text-xs rounded bg-gray-100 text-gray-600">
                {row.category || 'uncategorised'}
              </span>
              {(row.debit_account || row.credit_account) && (
                <span className="px-1.5 py-0.5 text-xs rounded bg-gray-100 text-gray-600">
                  {accountLabel(row.debit_account || row.credit_account)}
                </span>
              )}
              <span className={`px-1.5 py-0.5 text-xs rounded-full font-medium ${verdict.className}`}>
                {verdict.label}
              </span>
            </div>
          </button>
        </div>

        {/* One-by-one is the primary interaction, so the buttons live on the
            row — not only in a batch bar at the bottom of the screen.
            Add is only the loud primary on a row the server pre-ticked. On a
            likely duplicate it drops to an outline: the row the user most needs
            to stop and look at must not carry the most inviting button on the
            screen. Same action, same place — just not shouting. */}
        <div className="flex items-center gap-2 mt-3">
          {row.state === 'staged' ? (
            <>
              <button
                onClick={onAdd}
                disabled={busy}
                className={`flex-1 px-3 py-2 text-sm rounded-lg disabled:opacity-50 ${
                  row.preticked
                    ? 'bg-indigo-600 text-white hover:bg-indigo-700'
                    : 'border border-indigo-200 text-indigo-700 hover:bg-indigo-50'
                }`}
              >
                ✓ Add
              </button>
              <button
                onClick={() => setReviewing(true)}
                disabled={busy}
                className="px-3 py-2 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
              >
                ✎ Review
              </button>
              <button
                onClick={onDismiss}
                disabled={busy}
                className="px-3 py-2 text-sm rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-50"
              >
                ✗ Dismiss
              </button>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-between gap-2">
              <span className="text-xs text-gray-400">
                {row.state === 'imported' ? 'Added to your transactions' : 'Dismissed'}
              </span>
              {row.state === 'dismissed' && (
                <button onClick={onRestore} className="text-xs text-indigo-600 hover:underline">
                  Put back
                </button>
              )}
            </div>
          )}
        </div>

        {reviewing && <StagedReviewModal row={row} onClose={() => setReviewing(false)} />}
      </div>
    </div>
  )
}

// The account pickers come from the transaction form now, which offers every
// account the app knows — the same list "+ Add" offers. The server still
// validates the key on save, so there is nothing for this component to narrow.
export default function BankStagingList({ rows }: { rows: StagedTx[] }) {
  const commit = useCommitStaged()
  const dismiss = useDismissStaged()
  const restore = useRestoreStaged()
  const undo = useUndoCommit()
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [touched, setTouched] = useState(false)
  const [lastImported, setLastImported] = useState<number[]>([])
  const [note, setNote] = useState('')

  const stagedRows = useMemo(() => rows.filter((r) => r.state === 'staged'), [rows])

  // Until the user touches a checkbox, the selection is whatever the server
  // pre-ticked: new rows in, likely duplicates out. That rule lives in one
  // place (row.preticked) so the UI cannot drift from the backend's.
  const isSelected = (r: StagedTx) => (touched ? selected.has(r.id) : r.preticked)
  const selectedIds = stagedRows.filter(isSelected).map((r) => r.id)

  const toggle = (r: StagedTx) => {
    const base = touched ? selected : new Set(stagedRows.filter((x) => x.preticked).map((x) => x.id))
    const next = new Set(base)
    if (next.has(r.id)) next.delete(r.id)
    else next.add(r.id)
    setTouched(true)
    setSelected(next)
  }

  const runCommit = async (ids: number[]) => {
    setNote('')
    const res = await commit.mutateAsync(ids)
    setLastImported(res.imported_tx_ids)
    const parts = [`Added ${res.imported}`]
    if (res.skipped) parts.push(`skipped ${res.skipped}`)
    setNote([parts.join(' · '), ...(res.notes ?? []), res.balance_note].join(' — '))
    setSelected(new Set())
    setTouched(true)
  }

  if (rows.length === 0) {
    return (
      <p className="text-sm text-gray-400 text-center py-8">
        Nothing here yet.
      </p>
    )
  }

  const busy = commit.isPending || dismiss.isPending || restore.isPending

  return (
    <div className="space-y-2 pb-24">
      {note && (
        <div className="bg-emerald-50 border border-emerald-100 rounded-xl px-3 py-2 flex items-start justify-between gap-2">
          <p className="text-xs text-emerald-800">{note}</p>
          {lastImported.length > 0 && (
            <button
              onClick={async () => {
                await undo.mutateAsync(lastImported)
                setLastImported([])
                // The snapshot the commit cut is deliberately left alone:
                // balance history records what was observed, and undoing the
                // rows does not un-observe it. Say so rather than let the
                // totals quietly disagree with the ledger.
                setNote('Undone — the rows are back on the review list. The balance snapshot it added stays; remove it on the Balances page if you want the totals back.')
              }}
              disabled={undo.isPending}
              className="text-xs font-medium text-emerald-700 underline shrink-0 disabled:opacity-50"
            >
              Undo
            </button>
          )}
        </div>
      )}

      {commit.isError && (
        <p className="text-xs text-red-600">
          {commit.error instanceof Error ? commit.error.message : 'Could not add those'}
        </p>
      )}

      {rows.map((row) => (
        <StagedCard
          key={row.id}
          row={row}
          selected={isSelected(row)}
          onToggle={() => toggle(row)}
          busy={busy}
          onAdd={() => runCommit([row.id])}
          onDismiss={() => dismiss.mutate([row.id])}
          onRestore={() => restore.mutate([row.id])}
        />
      ))}

      {selectedIds.length > 0 && (
        <div className="fixed bottom-16 sm:bottom-4 left-0 right-0 px-4 z-30">
          <div className="max-w-2xl mx-auto bg-gray-900 text-white rounded-xl shadow-lg px-4 py-3 flex items-center justify-between gap-3">
            <span className="text-sm">{selectedIds.length} selected</span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  setTouched(true)
                  setSelected(new Set())
                }}
                className="text-xs text-gray-300 hover:text-white"
              >
                Clear
              </button>
              <button
                onClick={() => runCommit(selectedIds)}
                disabled={busy}
                className="px-3 py-1.5 text-sm rounded-lg bg-indigo-500 hover:bg-indigo-400 disabled:opacity-50"
              >
                {commit.isPending ? 'Adding…' : `Add ${selectedIds.length}`}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
