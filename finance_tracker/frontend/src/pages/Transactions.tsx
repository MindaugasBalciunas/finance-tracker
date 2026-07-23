import { useState, useEffect } from 'react'
import {
  useTransactions,
  useCreateTransaction,
  useUpdateTransaction,
  useDeleteTransaction,
} from '../hooks/useTransactions'
import TransactionForm from '../components/forms/TransactionForm'
import Badge from '../components/ui/Badge'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro, formatDate } from '../utils/format'
import type { Transaction, TransactionFilter, TransactionType, Category, CreateTransactionInput, AccountKey } from '../types'
import { ACCOUNT_LABELS } from '../types'
import { CATEGORIES } from '../constants/categories'
import { useDateRange } from '../context/DateRangeContext'

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

export default function Transactions() {
  const { dateRange } = useDateRange()
  const [showForm, setShowForm] = useState(false)
  const [editingTx, setEditingTx] = useState<Transaction | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [filter, setFilter] = useState<TransactionFilter>({ page: 1, page_size: 20, ...dateRange })

  // Sync global date range into local filter
  useEffect(() => {
    setFilter((f) => ({ ...f, date_from: dateRange.date_from, date_to: dateRange.date_to, page: 1 }))
  }, [dateRange])

  const { data, isLoading } = useTransactions(filter)
  const createMutation = useCreateTransaction()
  const updateMutation = useUpdateTransaction()
  const deleteMutation = useDeleteTransaction()

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
      <div className="flex items-center justify-between">
        <p className="text-sm text-gray-500">
          {data ? `${data.total} records` : 'All expenses, income and investments'}
        </p>
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
        <button
          onClick={() => { setFilter({ page: 1, page_size: 20, ...dateRange }) }}
          className="text-sm text-gray-500 hover:text-gray-800 underline"
        >
          Clear
        </button>
      </div>

      {/* Transaction list */}
      {isLoading ? (
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
                      {tx.labels.split(',').map((l) => (
                        <span key={l} className="inline-block text-[10px] font-medium bg-indigo-50 text-indigo-600 rounded px-1.5 py-0.5 mr-1">{l}</span>
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
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Date</th>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Type</th>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Category</th>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Comment</th>
                  <th className="text-left px-4 py-3 font-semibold text-gray-600">Account</th>
                  <th className="text-right px-4 py-3 font-semibold text-gray-600">Amount</th>
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
                          {tx.labels.split(',').map((l) => (
                            <span key={l} className="inline-block text-[10px] font-medium bg-indigo-50 text-indigo-600 rounded px-1.5 py-0.5 mr-1">{l}</span>
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
