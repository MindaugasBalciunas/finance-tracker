import { useState } from 'react'
import { useBalances, useCreateBalance, useDeleteBalance, useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import BalanceForm from '../components/forms/BalanceForm'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import StatCard from '../components/ui/StatCard'
import { formatEuro, formatDate } from '../utils/format'
import type { CreateBalanceInput } from '../types'

export default function Balances() {
  const [showForm, setShowForm] = useState(false)

  const { data: balances, isLoading } = useBalances()
  const { data: latest } = useLatestBalance()
  const { data: trend } = useBalanceTrend()
  const { data: allocations } = useAccountAllocation()
  const createMutation = useCreateBalance()
  const deleteMutation = useDeleteBalance()

  const handleCreate = async (input: CreateBalanceInput) => {
    await createMutation.mutateAsync(input)
    setShowForm(false)
  }

  const handleDelete = async (id: number) => {
    if (confirm('Delete this balance snapshot?')) {
      await deleteMutation.mutateAsync(id)
    }
  }

  if (isLoading) return <LoadingSpinner />

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Balances</h2>
          <p className="text-sm text-gray-500 mt-1">Track your net worth across all accounts</p>
        </div>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
        >
          + Add Snapshot
        </button>
      </div>

      {/* Add form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Balance Snapshot</h3>
            <BalanceForm
              onSubmit={handleCreate}
              onCancel={() => setShowForm(false)}
              isSubmitting={createMutation.isPending}
            />
          </div>
        </div>
      )}

      {/* Latest stats */}
      {latest && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <StatCard title="Total Net Worth" value={formatEuro(latest.total)} color="blue" />
          <StatCard title="SEB + Swed" value={formatEuro(latest.seb + latest.swed)} color="green" />
          <StatCard title="Investments (ETF + Pen)" value={formatEuro(latest.swed_etf + latest.swed_pen)} color="purple" />
          <StatCard title="Cash + Revolut" value={formatEuro(latest.cash + latest.rev_m + latest.rev_r)} color="yellow" />
        </div>
      )}

      {/* Charts row */}
      {trend && allocations && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Net Worth Trend</h3>
            <BalanceTrendChart trend={trend} />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Current Allocation</h3>
            <AllocationPieChart allocations={allocations} />
          </div>
        </div>
      )}

      {/* History table */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
          <h3 className="text-sm font-semibold text-gray-700">Snapshot History</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-4 py-2 font-semibold text-gray-600">Date</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Total</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">SEB</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Swed</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">ETF</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Pension</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Luminor</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Cash</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Rev M</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {balances?.map((b) => (
                <tr key={b.id} className="hover:bg-gray-50">
                  <td className="px-4 py-2 text-gray-700 font-medium">{formatDate(b.date)}</td>
                  <td className="px-3 py-2 text-right font-bold text-blue-700">{formatEuro(b.total)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.seb)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed_etf)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed_pen)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.luminor)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.cash)}</td>
                  <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_m)}</td>
                  <td className="px-3 py-2 text-right">
                    <button
                      onClick={() => handleDelete(b.id)}
                      className="text-gray-400 hover:text-red-600 text-xs"
                    >
                      ✕
                    </button>
                  </td>
                </tr>
              ))}
              {!balances?.length && (
                <tr>
                  <td colSpan={10} className="px-4 py-12 text-center text-gray-400">
                    No balance snapshots yet. Add one above.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
