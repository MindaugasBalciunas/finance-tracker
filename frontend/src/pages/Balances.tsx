import { useState } from 'react'
import { useBalances, useCreateBalance, useUpdateBalance, useDeleteBalance, useLatestBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import BalanceForm from '../components/forms/BalanceForm'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import StatCard from '../components/ui/StatCard'
import { formatEuro, formatDate } from '../utils/format'
import { useBtcEur } from '../hooks/useBtcPrice'
import type { Balance, CreateBalanceInput } from '../types'

export default function Balances() {
  const [showForm, setShowForm] = useState(false)
  const [editingBalance, setEditingBalance] = useState<Balance | null>(null)

  const { price: liveBtcPrice } = useBtcEur()
  const { data: latest } = useLatestBalance(liveBtcPrice)
  const storedBtcPrice = latest?.btc_price ?? 0
  const btcPrice: number | null = liveBtcPrice ?? (storedBtcPrice > 0 ? storedBtcPrice : null)
  const { data: balances, isLoading } = useBalances({}, btcPrice)
  const { data: trend } = useBalanceTrend()
  const { data: allocations } = useAccountAllocation()
  const createMutation = useCreateBalance()
  const updateMutation = useUpdateBalance()
  const deleteMutation = useDeleteBalance()

  const handleCreate = async (input: CreateBalanceInput) => {
    await createMutation.mutateAsync(input)
    setShowForm(false)
  }

  const handleUpdate = async (input: CreateBalanceInput) => {
    if (!editingBalance) return
    await updateMutation.mutateAsync({ id: editingBalance.id, input })
    setEditingBalance(null)
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
        <p className="text-sm text-gray-500">Track your net worth across all accounts</p>
        <button
          onClick={() => setShowForm(true)}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
        >
          + Add
        </button>
      </div>

      {/* Edit form modal */}
      {editingBalance && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Edit Snapshot — {formatDate(editingBalance.date)}</h3>
            <BalanceForm
              key={editingBalance.id}
              onSubmit={handleUpdate}
              onCancel={() => setEditingBalance(null)}
              isSubmitting={updateMutation.isPending}
              defaultValues={{
                date: editingBalance.date.slice(0, 10),
                seb: editingBalance.seb,
                swed: editingBalance.swed,
                swed_etf: editingBalance.swed_etf,
                seb_pen: editingBalance.seb_pen,
                luminor: editingBalance.luminor,
                art: editingBalance.art,
                cash: editingBalance.cash,
                rev_m: editingBalance.rev_m,
                rev_r: editingBalance.rev_r,
                r_btc: editingBalance.r_btc,
                m_btc: editingBalance.m_btc,
                rev_stocks: editingBalance.rev_stocks,
              }}
            />
          </div>
        </div>
      )}

      {/* Add form modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-lg mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Balance Snapshot</h3>
            <BalanceForm
              onSubmit={handleCreate}
              onCancel={() => setShowForm(false)}
              isSubmitting={createMutation.isPending}
              defaultValues={latest ? {
                seb: latest.seb,
                swed: latest.swed,
                swed_etf: latest.swed_etf,
                seb_pen: latest.seb_pen,
                luminor: latest.luminor,
                art: latest.art,
                cash: latest.cash,
                rev_m: latest.rev_m,
                rev_r: latest.rev_r,
                r_btc: latest.r_btc,
                m_btc: latest.m_btc,
                rev_stocks: latest.rev_stocks,
              } : undefined}
            />
          </div>
        </div>
      )}

      {/* Latest stats */}
      {latest && (
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4">
          <StatCard title="Total Net Worth" value={formatEuro(latest.total)} color="blue" />
          <StatCard
            title="Free Cash"
            value={formatEuro(latest.seb + latest.swed + latest.luminor + latest.cash + latest.rev_m + latest.rev_r)}
            subtitle="Banks + Cash + Revolut"
            color="green"
          />
          <StatCard
            title="Investments"
            value={formatEuro(latest.swed_etf + latest.rev_stocks)}
            subtitle="ETF + Revolut Stocks"
            color="blue"
          />
          <StatCard
            title="Pensions"
            value={formatEuro(latest.seb_pen + latest.art)}
            subtitle="2nd + 3rd Pillar"
            color="purple"
          />
          <StatCard
            title="Crypto"
            value={formatEuro(
              btcPrice != null
                ? (latest.r_btc + latest.m_btc) * btcPrice
                : (latest.r_btc_eur ?? 0) + (latest.m_btc_eur ?? 0)
            )}
            subtitle={btcPrice != null
              ? `${(latest.r_btc + latest.m_btc).toFixed(8)} BTC · €${btcPrice.toLocaleString()} /BTC`
              : 'Revolut R & M BTC'}
            color="yellow"
          />
        </div>
      )}

      {/* Charts row */}
      {trend && allocations && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <h3 className="text-base font-semibold text-gray-900 mb-4">Net Worth Trend</h3>
            <BalanceTrendChart trend={trend} btcPrice={btcPrice} />
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
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
          <table className="min-w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-3 py-2 font-semibold text-gray-600">Date</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Total</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Seb</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Swedbank</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Swedbank ETF</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">SEB 2nd pillar pension</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Luminor</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Artea 3rd pillar pension</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Cash</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut M account</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut R account</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut R account BTC</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut M account BTC</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut M account stocks</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {balances?.map((b) => {
                return (
                  <tr key={b.id} className="hover:bg-gray-50">
                    <td className="px-3 py-2 text-gray-700 font-medium">{formatDate(b.date)}</td>
                    <td className="px-3 py-2 text-right font-bold text-blue-700">{formatEuro(b.total)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.seb)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed_etf)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.seb_pen)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.luminor)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.art)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.cash)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_m)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_r)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(btcPrice != null ? b.r_btc * btcPrice : (b.r_btc_eur ?? 0))}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(btcPrice != null ? b.m_btc * btcPrice : (b.m_btc_eur ?? 0))}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_stocks)}</td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button onClick={() => setEditingBalance(b)} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
                        <button onClick={() => handleDelete(b.id)} className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
                      </div>
                    </td>
                  </tr>
                )
              })}
              {!balances?.length && (
                <tr>
                  <td colSpan={15} className="px-4 py-12 text-center text-gray-400">
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
