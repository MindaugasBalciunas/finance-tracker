import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  useTransactions,
  useAllTransactions,
  useCreateTransaction,
  useUpdateTransaction,
  useDeleteTransaction,
} from '../hooks/useTransactions'
import TransactionForm from '../components/forms/TransactionForm'
import Badge from '../components/ui/Badge'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import { formatEuro, formatDate } from '../utils/format'
import { txLabels } from '../utils/labels'
import type { Transaction, TransactionFilter, TransactionType, Category, CreateTransactionInput, AccountKey } from '../types'
import { ACCOUNT_LABELS } from '../types'
import { CATEGORIES } from '../constants/categories'
import { useDateRange } from '../context/DateRangeContext'
import { useLabels } from '../hooks/useBudgets'

function label(key: string) {
  return ACCOUNT_LABELS[key as AccountKey] ?? key
}

function formatAccount(tx: Transaction): string {
  const debit = tx.debit_account
  const credit = tx.credit_account
  // New-style rows
  if (debit && credit) return `${label(debit)} → ${label(credit)}`
  if (debit) return label(debit)
  if (credit) return label(credit)
  // Legacy rows
  if (tx.source_account) return label(tx.source_account)
  return '—'
}

function SortableTh({ label, col, filter, onSort, align = 'left' }: {
  label: string
  col: 'date' | 'amount' | 'comment'
  filter: TransactionFilter
  onSort: (col: 'date' | 'amount' | 'comment') => void
  align?: 'left' | 'right'
}) {
  const active = (filter.sort ?? 'date') === col
  const dir = filter.dir ?? (col === 'comment' ? 'asc' : 'desc')
  return (
    <th className={`text-${align} px-4 py-3 font-semibold text-gray-600`}>
      <button
        onClick={() => onSort(col)}
        className={`inline-flex items-center gap-0.5 hover:text-gray-900 ${active ? 'text-gray-900' : ''}`}
        title={`Sort by ${label.toLowerCase()}`}
      >
        {label}
        <span className="w-3 text-center text-xs">{active ? (dir === 'asc' ? '↑' : '↓') : ''}</span>
      </button>
    </th>
  )
}

export default function Transactions() {
  const { dateRange, setCustomRange } = useDateRange()
  const [searchParams, setSearchParams] = useSearchParams()
  const [showForm, setShowForm] = useState(false)
  const [editingTx, setEditingTx] = useState<Transaction | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [filter, setFilter] = useState<TransactionFilter>(() => ({
    page: 1,
    page_size: 20,
    ...dateRange,
    // ?label=… deep-links straight to a filtered list — every label chip in
    // the app links here.
    label: searchParams.get('label') ?? undefined,
  }))
  const [searchDraft, setSearchDraft] = useState('')

  // Adopt an explicit period from the URL (label links carry one) so a deep
  // link always opens exactly the period the source page showed, even when
  // the shared date-range state isn't available (fresh tab, copied URL).
  useEffect(() => {
    const from = searchParams.get('date_from') ?? undefined
    const to = searchParams.get('date_to') ?? undefined
    if (!from && !to) return
    if (from !== dateRange.date_from || to !== dateRange.date_to) {
      setCustomRange({ date_from: from, date_to: to })
    }
    const next = new URLSearchParams(searchParams)
    next.delete('date_from')
    next.delete('date_to')
    setSearchParams(next, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams])

  // Sync global date range into local filter
  useEffect(() => {
    setFilter((f) => ({ ...f, date_from: dateRange.date_from, date_to: dateRange.date_to, page: 1 }))
  }, [dateRange])

  // Column sorting: first click applies the column's natural direction
  // (newest/biggest first, comments A→Z), second click flips it.
  const sortBy = (col: 'date' | 'amount' | 'comment') => {
    setFilter((f) => {
      const natural = col === 'comment' ? 'asc' : 'desc'
      const current = f.sort ?? 'date'
      const currentDir = f.dir ?? (current === 'comment' ? 'asc' : 'desc')
      const dir = current === col ? (currentDir === 'asc' ? 'desc' : 'asc') : natural
      return { ...f, sort: col, dir, page: 1 }
    })
  }

  // URL → filter (in-page navigation to ?label=…) and filter → URL. The
  // equality guards make the two effects converge instead of looping.
  useEffect(() => {
    const urlLabel = searchParams.get('label') ?? undefined
    const urlMode = searchParams.get('label_mode') === 'all' ? ('all' as const) : undefined
    setFilter((f) => (f.label === urlLabel && f.label_mode === urlMode ? f : { ...f, label: urlLabel, label_mode: urlMode, page: 1 }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams])
  useEffect(() => {
    const urlMode = searchParams.get('label_mode') === 'all' ? 'all' : undefined
    if ((searchParams.get('label') ?? undefined) === filter.label && urlMode === filter.label_mode) return
    const next = new URLSearchParams(searchParams)
    if (filter.label) next.set('label', filter.label)
    else next.delete('label')
    if (filter.label && filter.label_mode === 'all') next.set('label_mode', 'all')
    else next.delete('label_mode')
    setSearchParams(next, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter.label, filter.label_mode])

  // Debounced comment search
  useEffect(() => {
    const t = setTimeout(() => {
      setFilter((f) => ({ ...f, search: searchDraft.trim() || undefined, page: 1 }))
    }, 350)
    return () => clearTimeout(t)
  }, [searchDraft])

  const { data, isLoading, isError, error, refetch } = useTransactions(filter)
  // Scoped to the selected category (when set), so the dropdown only offers
  // labels that can actually match — 78 flat labels don't fit a phone screen.
  const { data: allLabels = [] } = useLabels(filter.category as string | undefined)
  // Full matching set for the header totals — only fetched while a label
  // filter is active (the paginated list can't sum across pages).
  const { data: labelMatches } = useAllTransactions(
    {
      label: filter.label,
      label_mode: filter.label_mode,
      type: filter.type,
      category: filter.category,
      search: filter.search,
      date_from: filter.date_from,
      date_to: filter.date_to,
    },
    !!filter.label
  )
  const labelTotals = (() => {
    if (!filter.label || !labelMatches) return null
    let expenses = 0
    let income = 0
    let investments = 0
    for (const tx of labelMatches.data) {
      if (tx.type === 'expense') expenses += tx.amount.value
      else if (tx.type === 'income') income += tx.amount.value
      else investments += tx.amount.value
    }
    return { expenses, income, investments, count: labelMatches.data.length }
  })()
  const createMutation = useCreateTransaction()
  const updateMutation = useUpdateTransaction()
  const deleteMutation = useDeleteTransaction()

  // The active label filter is a set (comma list in the URL/API); chips and
  // dropdown picks toggle membership.
  const labelList = (filter.label ?? '').split(',').map((s) => s.trim()).filter(Boolean)
  const toggleLabelFilter = (l: string) => {
    setFilter((f) => {
      const set = (f.label ?? '').split(',').map((s) => s.trim()).filter(Boolean)
      const next = set.includes(l) ? set.filter((x) => x !== l) : [...set, l]
      return {
        ...f,
        label: next.length ? next.join(',') : undefined,
        label_mode: next.length > 1 ? f.label_mode : undefined,
        page: 1,
      }
    })
  }
  const clearLabelFilter = () => setFilter((f) => ({ ...f, label: undefined, label_mode: undefined, page: 1 }))

  const handleCreate = async (input: CreateTransactionInput) => {
    try {
      await createMutation.mutateAsync(input)
      setShowForm(false)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  const handleUpdate = async (input: CreateTransactionInput) => {
    if (!editingTx) return
    try {
      await updateMutation.mutateAsync({ id: editingTx.id, input })
      setEditingTx(null)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (confirm('Delete this transaction?')) {
      await deleteMutation.mutateAsync(id)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        {filter.label && labelTotals ? (
          <p className="text-sm text-gray-500 min-w-0">
            {labelList.map((l, i) => (
              <span key={l}>
                {i > 0 && <span className="text-gray-400 text-xs mx-0.5">{filter.label_mode === 'all' ? '&' : 'or'}</span>}
                <span className="inline-block text-xs font-medium bg-indigo-600 text-white rounded px-1.5 py-0.5 mr-1">{l}</span>
              </span>
            ))}
            {labelTotals.count} tx
            {labelTotals.expenses > 0 && <> · <span className="text-red-600 font-semibold">-{formatEuro(labelTotals.expenses)}</span></>}
            {labelTotals.income > 0 && <> · <span className="text-green-600 font-semibold">+{formatEuro(labelTotals.income)}</span></>}
            {labelTotals.investments > 0 && <> · <span className="text-blue-600 font-semibold">{formatEuro(labelTotals.investments)} invested</span></>}
            <button onClick={clearLabelFilter} className="ml-2 text-xs text-gray-400 hover:text-gray-600 underline">clear</button>
          </p>
        ) : (
          <p className="text-sm text-gray-500">
            {data ? `${data.total} records` : 'All expenses, income and investments'}
          </p>
        )}
        <button
          onClick={() => { setShowForm(true); setFormError(null) }}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
        >
          + Add
        </button>
      </div>

      {/* Add form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Transaction</h3>
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <TransactionForm
              onSubmit={handleCreate}
              onCancel={() => { setShowForm(false); setFormError(null) }}
              isSubmitting={createMutation.isPending}
            />
          </div>
        </div>
      )}

      {/* Edit form modal */}
      {editingTx && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Edit Transaction</h3>
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <TransactionForm
              key={editingTx.id}
              onSubmit={handleUpdate}
              onCancel={() => { setEditingTx(null); setFormError(null) }}
              isSubmitting={updateMutation.isPending}
              defaultValues={{
                date: editingTx.date.slice(0, 10),
                type: editingTx.type,
                amount: editingTx.amount.value,
                category: editingTx.category,
                comment: editingTx.comment,
                labels: editingTx.labels || '',
                debit_account: editingTx.debit_account || '',
                credit_account: editingTx.credit_account || '',
              }}
            />
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="bg-white rounded-xl border border-gray-200 p-4 flex flex-wrap gap-3 items-center">
        <div className="relative flex-1 min-w-40">
          <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400 text-sm">🔍</span>
          <input
            type="search"
            value={searchDraft}
            onChange={(e) => setSearchDraft(e.target.value)}
            placeholder="Search comments…"
            className="w-full border border-gray-300 rounded-lg pl-8 pr-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <select
          value={filter.type ?? ''}
          onChange={(e) => setFilter((f) => ({ ...f, type: (e.target.value as TransactionType) || undefined, page: 1 }))}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        >
          <option value="">All types</option>
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="investment">Investment</option>
        </select>
        <select
          value={filter.category ?? ''}
          onChange={(e) => setFilter((f) => ({ ...f, category: (e.target.value as Category) || undefined, page: 1 }))}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        >
          <option value="">All categories</option>
          {CATEGORIES.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        {/* Multi-label filter: picking adds a chip, chips toggle off; with
            two or more labels the any/all switch picks union vs intersection. */}
        <select
          value=""
          onChange={(e) => { if (e.target.value) toggleLabelFilter(e.target.value) }}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        >
          <option value="">{labelList.length ? '+ Add label' : 'All labels'}</option>
          {allLabels.filter((l) => !labelList.includes(l)).map((l) => (
            <option key={l} value={l}>{l}</option>
          ))}
        </select>
        {labelList.map((l) => (
          <button
            key={l}
            onClick={() => toggleLabelFilter(l)}
            title="Remove from filter"
            className="inline-flex items-center gap-1 text-xs font-medium bg-indigo-600 text-white rounded-lg px-2 py-1.5 hover:bg-indigo-700"
          >
            {l} <span className="text-indigo-200">×</span>
          </button>
        ))}
        {labelList.length > 1 && (
          <div className="grid grid-cols-2 gap-0.5 bg-gray-100 rounded-lg p-0.5 text-xs">
            <button
              onClick={() => setFilter((f) => ({ ...f, label_mode: undefined, page: 1 }))}
              title="Rows carrying any of the labels"
              className={`px-2 py-1 rounded-md font-medium ${filter.label_mode !== 'all' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
            >
              any
            </button>
            <button
              onClick={() => setFilter((f) => ({ ...f, label_mode: 'all', page: 1 }))}
              title="Only rows carrying every label"
              className={`px-2 py-1 rounded-md font-medium ${filter.label_mode === 'all' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
            >
              all
            </button>
          </div>
        )}
        <button
          onClick={() => { setSearchDraft(''); setFilter({ page: 1, page_size: 20, ...dateRange }) }}
          className="text-sm text-gray-500 hover:text-gray-800 underline"
        >
          Clear
        </button>
      </div>

      {/* Transaction list */}
      {isError ? (
        <QueryError error={error} onRetry={() => refetch()} />
      ) : isLoading ? (
        <LoadingSpinner />
      ) : (
        <>
          {/* Mobile card list */}
          <div className="sm:hidden space-y-2">
            {data?.data.map((tx) => (
              <div key={tx.id} className="bg-white rounded-xl border border-gray-200 px-4 py-3 flex items-start justify-between gap-3">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-0.5">
                    <Badge type={tx.type} />
                    <span className="text-xs text-gray-400">{formatDate(tx.date)}</span>
                  </div>
                  <p className="text-sm font-medium text-gray-800 truncate">{tx.category}</p>
                  {tx.comment && <p className="text-xs text-gray-400 truncate">{tx.comment}</p>}
                  {tx.labels && (
                    <p className="mt-0.5">
                      {txLabels(tx).map((l) => (
                        <button
                          key={l}
                          onClick={() => toggleLabelFilter(l)}
                          className={`inline-block text-[10px] font-medium rounded px-1.5 py-0.5 mr-1 transition-colors ${
                            filter.label === l ? 'bg-indigo-600 text-white' : 'bg-indigo-50 text-indigo-600 hover:bg-indigo-100'
                          }`}
                        >
                          {l}
                        </button>
                      ))}
                    </p>
                  )}
                </div>
                <div className="flex flex-col items-end gap-1 shrink-0">
                  <span className={`text-sm font-bold ${tx.type === 'expense' ? 'text-red-600' : tx.type === 'income' ? 'text-green-600' : 'text-blue-600'}`}>
                    {tx.type === 'expense' ? '-' : '+'}{formatEuro(tx.amount.value)}
                  </span>
                  <div className="flex gap-1">
                    <button onClick={() => { setEditingTx(tx); setFormError(null) }} className="p-2 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
                    <button onClick={() => handleDelete(tx.id)} className="p-2 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
                  </div>
                </div>
              </div>
            ))}
            {!data?.data.length && (
              <div className="bg-white rounded-xl border border-gray-200 px-4 py-12 text-center text-gray-400 text-sm">
                No transactions found. Add one above.
              </div>
            )}
          </div>

          {/* Desktop table */}
          <div className="hidden sm:block bg-white rounded-xl border border-gray-200 overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">ID</th>
                  <SortableTh label="Date" col="date" filter={filter} onSort={sortBy} />
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Type</th>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Category</th>
                  <SortableTh label="Comment" col="comment" filter={filter} onSort={sortBy} />
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Account</th>
                  <SortableTh label="Amount" col="amount" align="right" filter={filter} onSort={sortBy} />
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {data?.data.map((tx) => (
                  <tr key={tx.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-gray-400 text-xs">{tx.id}</td>
                    <td className="px-4 py-3 text-gray-700">{formatDate(tx.date)}</td>
                    <td className="px-4 py-3"><Badge type={tx.type} /></td>
                    <td className="px-4 py-3 text-gray-600">{tx.category}</td>
                    <td className="px-4 py-3 text-gray-500">
                      {tx.comment || '—'}
                      {tx.labels && (
                        <span className="block mt-0.5">
                          {txLabels(tx).map((l) => (
                            <button
                              key={l}
                              onClick={() => toggleLabelFilter(l)}
                              className={`inline-block text-[10px] font-medium rounded px-1.5 py-0.5 mr-1 transition-colors ${
                                filter.label === l ? 'bg-indigo-600 text-white' : 'bg-indigo-50 text-indigo-600 hover:bg-indigo-100'
                              }`}
                            >
                              {l}
                            </button>
                          ))}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-gray-400 text-xs">{formatAccount(tx)}</td>
                    <td className={`px-4 py-3 text-right font-semibold ${tx.type === 'expense' ? 'text-red-600' : tx.type === 'income' ? 'text-green-600' : 'text-blue-600'}`}>
                      {tx.type === 'expense' ? '-' : '+'}{formatEuro(tx.amount.value)}
                    </td>
                    <td className="px-2 py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button onClick={() => { setEditingTx(tx); setFormError(null) }} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
                        <button onClick={() => handleDelete(tx.id)} className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
                      </div>
                    </td>
                  </tr>
                ))}
                {!data?.data.length && (
                  <tr>
                    <td colSpan={8} className="px-4 py-12 text-center text-gray-400">
                      No transactions found. Add one above.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {data && data.total_pages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-white rounded-xl border mt-0 sm:border-t sm:mt-0 sm:rounded-none sm:bg-gray-50">
              <p className="text-sm text-gray-500">
                Page {data.page} of {data.total_pages} ({data.total} records)
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setFilter((f) => ({ ...f, page: Math.max(1, (f.page ?? 1) - 1) }))}
                  disabled={data.page <= 1}
                  className="px-3 py-1 text-sm border border-gray-300 rounded disabled:opacity-40 hover:bg-gray-100"
                >
                  Prev
                </button>
                <button
                  onClick={() => setFilter((f) => ({ ...f, page: (f.page ?? 1) + 1 }))}
                  disabled={data.page >= data.total_pages}
                  className="px-3 py-1 text-sm border border-gray-300 rounded disabled:opacity-40 hover:bg-gray-100"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
