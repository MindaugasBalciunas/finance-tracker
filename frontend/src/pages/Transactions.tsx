import { useState } from 'react'
import {
  useTransactions,
  useCreateTransaction,
  useDeleteTransaction,
} from '../hooks/useTransactions'
import TransactionForm from '../components/forms/TransactionForm'
import Badge from '../components/ui/Badge'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro, formatDate } from '../utils/format'
import type { TransactionFilter, TransactionType, Category, CreateTransactionInput } from '../types'

export default function Transactions() {
  const [showForm, setShowForm] = useState(false)
  const [filter, setFilter] = useState<TransactionFilter>({ page: 1, page_size: 20 })

  const { data, isLoading } = useTransactions(filter)
  const createMutation = useCreateTransaction()
  const deleteMutation = useDeleteTransaction()

  const handleCreate = async (input: CreateTransactionInput) => {
    await createMutation.mutateAsync(input)
    setShowForm(false)
  }

  const handleDelete = async (id: number) => {
    if (confirm('Delete this transaction?')) {
      await deleteMutation.mutateAsync(id)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Transactions</h2>
          <p className="text-sm text-gray-500 mt-1">
            {data ? `${data.total} records` : 'All expenses, income and investments'}
          </p>
        </div>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
        >
          + Add Transaction
        </button>
      </div>

      {/* Add form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Transaction</h3>
            <TransactionForm
              onSubmit={handleCreate}
              onCancel={() => setShowForm(false)}
              isSubmitting={createMutation.isPending}
            />
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="bg-white rounded-xl border border-gray-200 p-4 flex flex-wrap gap-3">
        <input
          type="date"
          placeholder="From"
          value={filter.date_from ?? ''}
          onChange={(e) => setFilter((f) => ({ ...f, date_from: e.target.value || undefined, page: 1 }))}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        />
        <input
          type="date"
          placeholder="To"
          value={filter.date_to ?? ''}
          onChange={(e) => setFilter((f) => ({ ...f, date_to: e.target.value || undefined, page: 1 }))}
          className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
        />
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
          {['Food','Kids','Kids(food)','Health','Finance','Investment','Entertainment','House expense','Credit','Car','Income','Clothes','Kids school','Kids (Entertainment)','Divorce'].map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        <button
          onClick={() => setFilter({ page: 1, page_size: 20 })}
          className="text-sm text-gray-500 hover:text-gray-800 underline"
        >
          Clear
        </button>
      </div>

      {/* Table */}
      {isLoading ? (
        <LoadingSpinner />
      ) : (
        <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Date</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Type</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Category</th>
                <th className="text-left px-4 py-3 font-semibold text-gray-600">Comment</th>
                <th className="text-right px-4 py-3 font-semibold text-gray-600">Amount</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data?.data.map((tx) => (
                <tr key={tx.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3 text-gray-700">{formatDate(tx.date)}</td>
                  <td className="px-4 py-3"><Badge type={tx.type} /></td>
                  <td className="px-4 py-3 text-gray-600">{tx.category}</td>
                  <td className="px-4 py-3 text-gray-500 max-w-xs truncate">{tx.comment || '—'}</td>
                  <td className={`px-4 py-3 text-right font-semibold ${tx.type === 'expense' ? 'text-red-600' : tx.type === 'income' ? 'text-green-600' : 'text-blue-600'}`}>
                    {tx.type === 'expense' ? '-' : '+'}{formatEuro(tx.amount)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => handleDelete(tx.id)}
                      className="text-gray-400 hover:text-red-600 text-xs"
                    >
                      ✕
                    </button>
                  </td>
                </tr>
              ))}
              {!data?.data.length && (
                <tr>
                  <td colSpan={6} className="px-4 py-12 text-center text-gray-400">
                    No transactions found. Add one above.
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          {/* Pagination */}
          {data && data.total_pages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 bg-gray-50">
              <p className="text-sm text-gray-500">
                Page {data.page} of {data.total_pages} ({data.total} records)
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setFilter((f) => ({ ...f, page: Math.max(1, (f.page ?? 1) - 1) }))}
                  disabled={data.page <= 1}
                  className="px-3 py-1 text-sm border border-gray-300 rounded disabled:opacity-40 hover:bg-gray-100"
                >
                  Previous
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
        </div>
      )}
    </div>
  )
}
