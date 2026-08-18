import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  useLabelStats, useLabelSuggestions, useRenameLabel, useDeleteLabel,
} from '../hooks/useBudgets'
import CategoryTransactionsModal from '../components/ui/CategoryTransactionsModal'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import LabelsNav, { BannerAlert, errText, type Banner } from '../components/ui/LabelsNav'
import { formatEuro } from '../utils/format'
import { FIXED_LABELS } from '../utils/labels'
import type { LabelStat, RelabelResult } from '../types'

type SortKey = 'label' | 'transactions' | 'amount' | 'rules' | 'budgets' | 'last_used'

// Labels management: every label's footprint, typo/merge suggestions, and
// rename/merge/delete that rewrite the whole database (transactions, rules,
// budget label lists). Rules and AI tagging live on their own pages now.
export default function Labels() {
  const { data: stats = [], isLoading, isError, error, refetch } = useLabelStats()
  const { data: suggestions = [] } = useLabelSuggestions()
  const renameLabel = useRenameLabel()
  const deleteLabel = useDeleteLabel()

  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<{ key: SortKey; dir: 1 | -1 }>({ key: 'transactions', dir: -1 })
  const [renaming, setRenaming] = useState<LabelStat | null>(null)
  const [deleting, setDeleting] = useState<LabelStat | null>(null)
  const [reviewing, setReviewing] = useState<string | null>(null)
  const [dismissed, setDismissed] = useState<Set<string>>(new Set())
  const [banner, setBanner] = useState<Banner>(null)

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase()
    const rows = q ? stats.filter((s) => s.label.includes(q)) : [...stats]
    rows.sort((a, b) => {
      const av = a[sort.key], bv = b[sort.key]
      const cmp = typeof av === 'string' ? String(av).localeCompare(String(bv)) : Number(av) - Number(bv)
      return cmp !== 0 ? cmp * sort.dir : a.label.localeCompare(b.label)
    })
    return rows
  }, [stats, search, sort])

  const toggleSort = (key: SortKey) =>
    setSort((s) => (s.key === key ? { key, dir: s.dir === 1 ? -1 : 1 } : { key, dir: key === 'label' ? 1 : -1 }))

  const liveSuggestions = suggestions.filter((s) => !dismissed.has(s.from + '→' + s.to))

  const touched = (r: RelabelResult) =>
    [
      `${r.transactions} transaction${r.transactions === 1 ? '' : 's'}`,
      r.rules > 0 ? `${r.rules} rule${r.rules === 1 ? '' : 's'}` : '',
      r.budgets > 0 ? `${r.budgets} budget${r.budgets === 1 ? '' : 's'}` : '',
    ].filter(Boolean).join(', ')

  const doMerge = async (from: string, to: string) => {
    try {
      const res = await renameLabel.mutateAsync({ from, to })
      setBanner({ kind: 'ok', text: `Merged “${from}” into “${to}” — ${touched(res)} updated.` })
    } catch (err) {
      setBanner({ kind: 'error', text: `Merge failed: ${errText(err)}` })
    }
  }

  const Th = ({ k, children, className = '' }: { k: SortKey; children: React.ReactNode; className?: string }) => (
    <th className={`px-3 py-2 ${className}`}>
      <button
        onClick={() => toggleSort(k)}
        className={`inline-flex items-center gap-0.5 uppercase tracking-wide text-[11px] ${sort.key === k ? 'text-gray-700 font-semibold' : 'text-gray-400'} hover:text-gray-700`}
      >
        {children}
        <span className="w-3 text-center">{sort.key === k ? (sort.dir === 1 ? '↑' : '↓') : ''}</span>
      </button>
    </th>
  )

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-5xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">🏷️ Labels</h1>
          <p className="text-xs text-gray-400">
            {stats.length} labels in use — rename, merge or delete them everywhere in one action
          </p>
        </div>
        <LabelsNav />
      </div>

      <BannerAlert banner={banner} onClose={() => setBanner(null)} />

      {liveSuggestions.length > 0 && (
        <div className="bg-amber-50 border border-amber-200 rounded-2xl p-4">
          <h2 className="text-sm font-semibold text-amber-900 mb-1">Possible duplicates &amp; typos</h2>
          <p className="text-xs text-amber-700/80 mb-3">
            Merging rewrites every transaction, rule and budget that uses the label. Nothing happens without your click.
          </p>
          <div className="space-y-2">
            {liveSuggestions.map((s) => {
              const key = s.from + '→' + s.to
              return (
                <div key={key} className="flex flex-wrap items-center gap-2 text-sm">
                  <span className="font-medium text-gray-800">
                    {s.from} <span className="text-gray-400 font-normal">({s.from_count})</span>
                    <span className="mx-1.5 text-amber-500">→</span>
                    {s.to} <span className="text-gray-400 font-normal">({s.to_count})</span>
                  </span>
                  <span className="text-[11px] uppercase tracking-wide text-amber-600 bg-amber-100 rounded px-1.5 py-0.5">
                    {s.reason}
                  </span>
                  <span className="flex items-center gap-1 ml-auto">
                    <button
                      onClick={() => doMerge(s.from, s.to)}
                      disabled={renameLabel.isPending}
                      className="px-2.5 py-1 rounded-lg bg-amber-600 text-white text-xs font-medium hover:bg-amber-700 disabled:opacity-50"
                    >
                      Merge
                    </button>
                    <button
                      onClick={() => doMerge(s.to, s.from)}
                      disabled={renameLabel.isPending || (FIXED_LABELS as readonly string[]).includes(s.to)}
                      title={`Merge the other way: ${s.to} → ${s.from}`}
                      className="px-2 py-1 rounded-lg border border-amber-300 text-amber-700 text-xs hover:bg-amber-100 disabled:opacity-40"
                    >
                      ⇄
                    </button>
                    <button
                      onClick={() => setDismissed(new Set([...dismissed, key]))}
                      className="px-2 py-1 text-xs text-amber-500 hover:text-amber-800"
                      title="Hide this suggestion"
                    >
                      Dismiss
                    </button>
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      )}

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm">
        <div className="p-3 border-b border-gray-100">
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search labels…"
            className="w-full sm:w-64 text-sm border border-gray-200 rounded-lg px-3 py-1.5 focus:outline-none focus:ring-2 focus:ring-indigo-200"
          />
        </div>
        {isError ? (
          <div className="p-3">
            <QueryError error={error} onRetry={() => refetch()} />
          </div>
        ) : isLoading ? (
          <LoadingSpinner />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left border-b border-gray-100">
                  <Th k="label" className="pl-4">Label</Th>
                  <Th k="transactions" className="text-right">Transactions</Th>
                  <Th k="amount" className="text-right hidden sm:table-cell">Volume</Th>
                  <Th k="rules" className="text-right hidden md:table-cell">Rules</Th>
                  <Th k="budgets" className="text-right hidden md:table-cell">Budgets</Th>
                  <Th k="last_used" className="hidden lg:table-cell">Last used</Th>
                  <th className="px-3 py-2 text-right text-[11px] uppercase tracking-wide text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((s) => (
                  <tr key={s.label} className="border-b border-gray-50 hover:bg-gray-50/60 group">
                    <td className="px-4 py-2">
                      <button
                        onClick={() => setReviewing(s.label)}
                        className="inline-flex items-center gap-1.5 text-indigo-700 hover:underline"
                        title={`Review the ${s.transactions} transactions labeled “${s.label}”`}
                      >
                        {s.label}
                        {s.fixed && (
                          <span
                            className="text-[10px] uppercase tracking-wide bg-gray-100 text-gray-500 rounded px-1 py-0.5"
                            title="Fixed-obligation label — used by insights and budgets, cannot be renamed or deleted"
                          >
                            🔒 fixed
                          </span>
                        )}
                      </button>
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums">{s.transactions}</td>
                    <td className="px-3 py-2 text-right tabular-nums text-gray-500 hidden sm:table-cell">
                      {formatEuro(s.amount)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-gray-500 hidden md:table-cell">
                      {s.rules || '—'}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-gray-500 hidden md:table-cell">
                      {s.budgets || '—'}
                    </td>
                    <td className="px-3 py-2 text-gray-400 hidden lg:table-cell">{s.last_used}</td>
                    <td className="px-3 py-2 text-right whitespace-nowrap">
                      <span className="sm:opacity-0 sm:group-hover:opacity-100 transition-opacity">
                        <Link
                          to={`/transactions?label=${encodeURIComponent(s.label)}`}
                          className="px-2 py-1 text-gray-400 hover:text-indigo-600"
                          title="Open in Transactions"
                        >
                          ↗
                        </Link>
                        {!s.fixed && (
                          <>
                            <button
                              onClick={() => setRenaming(s)}
                              className="px-2 py-1 text-gray-400 hover:text-indigo-600"
                              aria-label={`Rename label ${s.label}`}
                              title="Rename / merge"
                            >
                              ✎
                            </button>
                            <button
                              onClick={() => setDeleting(s)}
                              className="px-2 py-1 text-gray-400 hover:text-red-600"
                              aria-label={`Delete label ${s.label}`}
                              title="Delete everywhere"
                            >
                              🗑
                            </button>
                          </>
                        )}
                      </span>
                    </td>
                  </tr>
                ))}
                {visible.length === 0 && (
                  <tr>
                    <td colSpan={7} className="px-4 py-8 text-center text-gray-400 text-sm">
                      No labels match “{search}”.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {renaming && (
        <RenameModal
          stat={renaming}
          all={stats}
          busy={renameLabel.isPending}
          onClose={() => setRenaming(null)}
          onRename={async (to) => {
            try {
              const res = await renameLabel.mutateAsync({ from: renaming.label, to })
              const merged = stats.some((s) => s.label === to)
              setBanner({
                kind: 'ok',
                text: `${merged ? 'Merged' : 'Renamed'} “${renaming.label}” ${merged ? 'into' : 'to'} “${to}” — ${touched(res)} updated.`,
              })
              setRenaming(null)
            } catch (err) {
              setBanner({ kind: 'error', text: `Rename failed: ${errText(err)}` })
              setRenaming(null)
            }
          }}
        />
      )}

      {deleting && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
          onClick={(e) => { if (e.target === e.currentTarget) setDeleting(null) }}
        >
          <div className="bg-white rounded-2xl shadow-xl w-full max-w-sm p-5">
            <h2 className="font-semibold text-gray-900 mb-1">Delete “{deleting.label}”?</h2>
            <p className="text-sm text-gray-500 mb-4">
              Removes the label from {deleting.transactions} transaction{deleting.transactions === 1 ? '' : 's'}
              {deleting.rules > 0 && <>, deletes {deleting.rules} rule{deleting.rules === 1 ? '' : 's'}</>}
              {deleting.budgets > 0 && <>, and detaches it from {deleting.budgets} budget{deleting.budgets === 1 ? '' : 's'}
                (a budget left with no matcher at all is removed)</>}
              . The transactions themselves stay.
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setDeleting(null)}
                className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={async () => {
                  try {
                    const res = await deleteLabel.mutateAsync(deleting.label)
                    setBanner({ kind: 'ok', text: `Deleted “${deleting.label}” — ${touched(res)} updated.` })
                  } catch (err) {
                    setBanner({ kind: 'error', text: `Delete failed: ${errText(err)}` })
                  }
                  setDeleting(null)
                }}
                disabled={deleteLabel.isPending}
                className="px-3 py-1.5 text-sm rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:opacity-50"
              >
                {deleteLabel.isPending ? 'Deleting…' : 'Delete everywhere'}
              </button>
            </div>
          </div>
        </div>
      )}

      {reviewing && (
        <CategoryTransactionsModal
          label={reviewing}
          title={reviewing}
          dateRange={{}}
          onClose={() => setReviewing(null)}
        />
      )}
    </div>
  )
}

function RenameModal({ stat, all, busy, onClose, onRename }: {
  stat: LabelStat
  all: LabelStat[]
  busy: boolean
  onClose: () => void
  onRename: (to: string) => void
}) {
  const [to, setTo] = useState(stat.label)
  const target = to.trim().toLowerCase()
  const existing = target !== stat.label && all.find((s) => s.label === target)
  const valid = target !== '' && target !== stat.label && !target.includes(',')

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-sm p-5">
        <h2 className="font-semibold text-gray-900 mb-1">Rename “{stat.label}”</h2>
        <p className="text-sm text-gray-500 mb-3">
          Rewrites {stat.transactions} transaction{stat.transactions === 1 ? '' : 's'}
          {stat.rules > 0 && <>, {stat.rules} rule{stat.rules === 1 ? '' : 's'}</>}
          {stat.budgets > 0 && <>, {stat.budgets} budget{stat.budgets === 1 ? '' : 's'}</>}.
        </p>
        <input
          autoFocus
          value={to}
          onChange={(e) => setTo(e.target.value.toLowerCase())}
          onKeyDown={(e) => { if (e.key === 'Enter' && valid && !busy) onRename(target) }}
          list="all-label-options"
          className="w-full text-sm border border-gray-200 rounded-lg px-3 py-2 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        />
        <datalist id="all-label-options">
          {all.filter((s) => s.label !== stat.label).map((s) => <option key={s.label} value={s.label} />)}
        </datalist>
        {existing && (
          <p className="text-xs text-amber-600 mt-2">
            “{target}” already exists ({existing.transactions} transactions) — the two labels will be merged.
          </p>
        )}
        <div className="flex justify-end gap-2 mt-4">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            onClick={() => onRename(target)}
            disabled={!valid || busy}
            className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {busy ? 'Renaming…' : existing ? 'Merge labels' : 'Rename'}
          </button>
        </div>
      </div>
    </div>
  )
}
