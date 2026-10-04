import { useState } from 'react'
import { useBalances, useCreateBalance, useUpdateBalance, useDeleteBalance, useLatestBalance, useProjectedBalance, useBalanceTrend, useAccountAllocation } from '../hooks/useBalances'
import { freeCash, investments, pensions, cryptoEur, cryptoSubtitle } from '../utils/balanceGroups'
import { useDateRange } from '../context/DateRangeContext'
import BalanceForm from '../components/forms/BalanceForm'
import BalanceTrendChart from '../components/charts/BalanceTrendChart'
import AllocationPieChart from '../components/charts/AllocationPieChart'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import QueryError from '../components/ui/QueryError'
import StatCard from '../components/ui/StatCard'
import WhereMoneySits from '../components/ui/WhereMoneySits'
import AccountMovement from '../components/ui/AccountMovement'
import AccountsManager from '../components/ui/AccountsManager'
import { formatEuro, formatDate, formatTime } from '../utils/format'
import { useBtcEur } from '../hooks/useBtcPrice'
import { useAssetSummary } from '../hooks/useAssets'
import type { Balance, CreateBalanceInput } from '../types'

// Per-account rows for the mobile snapshot cards; mirrors the desktop table columns.
const ACCOUNT_ROWS: { label: string; value: (b: Balance) => number }[] = [
  { label: 'SEB', value: (b) => b.seb },
  { label: 'Swedbank', value: (b) => b.swed },
  { label: 'IBKR stocks', value: (b) => b.ibkr_stocks },
  { label: 'Swedbank ETF', value: (b) => b.swed_etf },
  { label: 'Revolut M', value: (b) => b.rev_m },
  { label: 'Cash', value: (b) => b.cash },
  { label: 'M BTC (€)', value: (b) => b.m_btc_eur ?? 0 },
  { label: 'Rev M stocks', value: (b) => b.rev_stocks },
  { label: 'SEB pension', value: (b) => b.seb_pen },
  { label: 'Artea pension', value: (b) => b.art },
  { label: 'Revolut R', value: (b) => b.rev_r },
  { label: 'R BTC (€)', value: (b) => b.r_btc_eur ?? 0 },
]

export default function Balances() {
  const [showForm, setShowForm] = useState(false)
  const [editingBalance, setEditingBalance] = useState<Balance | null>(null)
  const [formError, setFormError] = useState<string | null>(null)

  const { dateRange } = useDateRange()
  const { price: liveBtcPrice } = useBtcEur()
  const { data: latest } = useLatestBalance(liveBtcPrice)
  const { data: assetSummary } = useAssetSummary()
  const { data: projected } = useProjectedBalance(liveBtcPrice)
  const { data: allBalances, isLoading, isError, error, refetch } = useBalances({}, liveBtcPrice)
  // Auto-generated snapshots are internal projection caches — hide from history table
  const balances = allBalances?.filter(b => !b.is_auto)
  // The trend chart and the movement card follow the global date range;
  // the snapshot history below stays complete.
  const { data: trend } = useBalanceTrend(dateRange)
  const rangeBalances = balances?.filter(
    (b) =>
      (!dateRange.date_from || b.date.slice(0, 10) >= dateRange.date_from) &&
      (!dateRange.date_to || b.date.slice(0, 10) <= dateRange.date_to)
  )
  const { data: allocations } = useAccountAllocation()
  const createMutation = useCreateBalance()
  const updateMutation = useUpdateBalance()
  const deleteMutation = useDeleteBalance()

  const handleCreate = async (input: CreateBalanceInput) => {
    try {
      await createMutation.mutateAsync(input)
      setShowForm(false)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  const handleUpdate = async (input: CreateBalanceInput) => {
    if (!editingBalance) return
    try {
      await updateMutation.mutateAsync({ id: editingBalance.id, input })
      setEditingBalance(null)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (confirm('Delete this balance snapshot?')) {
      await deleteMutation.mutateAsync(id)
    }
  }

  if (isLoading) return <LoadingSpinner />
  if (isError) return <QueryError error={error} onRetry={() => refetch()} />

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <p className="text-sm text-gray-500">Track your net worth across all accounts</p>
        <button
          onClick={() => { setShowForm(true); setFormError(null) }}
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
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <BalanceForm
              key={editingBalance.id}
              onSubmit={handleUpdate}
              onCancel={() => { setEditingBalance(null); setFormError(null) }}
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
                ibkr_stocks: editingBalance.ibkr_stocks,
                extra: editingBalance.extra,
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
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <BalanceForm
              onSubmit={handleCreate}
              onCancel={() => { setShowForm(false); setFormError(null) }}
              isSubmitting={createMutation.isPending}
              defaultValues={projected ? {
                seb: projected.seb,
                swed: projected.swed,
                swed_etf: projected.swed_etf,
                seb_pen: projected.seb_pen,
                luminor: projected.luminor,
                art: projected.art,
                cash: projected.cash,
                rev_m: projected.rev_m,
                rev_r: projected.rev_r,
                r_btc: projected.r_btc,
                m_btc: projected.m_btc,
                rev_stocks: projected.rev_stocks,
                ibkr_stocks: projected.ibkr_stocks,
                extra: projected.extra,
              } : undefined}
            />
          </div>
        </div>
      )}

      {/* Latest stats */}
      {latest && (
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-3 sm:gap-4">
          <StatCard
            title="Total Net Worth"
            value={formatEuro(latest.total)}
            subtitle={assetSummary && assetSummary.count > 0
              ? `${formatEuro(latest.total + assetSummary.net_equity)} incl. assets − loans`
              : undefined}
            color="blue"
          />
          <StatCard
            title="Free Cash"
            value={formatEuro(freeCash(latest))}
            subtitle="Banks + Cash + Revolut"
            color="green"
          />
          <StatCard
            title="Investments"
            value={formatEuro(investments(latest))}
            subtitle="ETF + Revolut + IBKR"
            color="blue"
          />
          <StatCard
            title="Pensions"
            value={formatEuro(pensions(latest))}
            subtitle="2nd + 3rd Pillar"
            color="purple"
          />
          <StatCard
            title="Crypto"
            value={formatEuro(cryptoEur(latest))}
            subtitle={cryptoSubtitle(latest, liveBtcPrice)}
            color="yellow"
          />
        </div>
      )}

      {/* Net worth trend — the headline chart, full width */}
      {trend && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-4">Net Worth Trend</h3>
          <BalanceTrendChart trend={trend} btcPrice={liveBtcPrice} />
        </div>
      )}

      {/* Where the money sits + movement over the selected range */}
      {latest && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
          <WhereMoneySits balance={latest} />
          {rangeBalances && rangeBalances.length >= 2 ? (
            <AccountMovement balances={rangeBalances} />
          ) : (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6 flex items-center justify-center">
              <p className="text-sm text-gray-400">Not enough snapshots in the selected period to show movement.</p>
            </div>
          )}
        </div>
      )}

      <AccountsManager />

      {/* Current allocation by account */}
      {allocations && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900 mb-4">Current Allocation</h3>
          <AllocationPieChart allocations={allocations} />
        </div>
      )}

      {/* History table */}
      <div className="bg-white rounded-xl border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200 bg-gray-50">
          <h3 className="text-sm font-semibold text-gray-700">Snapshot History</h3>
        </div>

        {/* Mobile cards */}
        <div className="sm:hidden divide-y divide-gray-100">
          {balances?.map((b) => (
            <div key={b.id} className="px-4 py-3">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium text-gray-700">{formatDate(b.date)}</p>
                  <p className="text-xs text-gray-400">{formatTime(b.created_at)}</p>
                </div>
                <div className="flex items-center gap-1">
                  <span className="text-base font-bold text-blue-700 mr-1">{formatEuro(b.total)}</span>
                  <button
                    onClick={() => { setEditingBalance(b); setFormError(null) }}
                    className="p-2 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors"
                  >
                    ✎
                  </button>
                  <button
                    onClick={() => handleDelete(b.id)}
                    className="p-2 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors"
                  >
                    ✕
                  </button>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-x-6 gap-y-1 mt-2 text-xs">
                <div className="flex justify-between">
                  <span className="text-gray-400">Free cash</span>
                  <span className="font-medium text-green-700">{formatEuro(freeCash(b))}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-400">Investments</span>
                  <span className="font-medium text-blue-700">{formatEuro(investments(b))}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-400">Pensions</span>
                  <span className="font-medium text-purple-700">{formatEuro(pensions(b))}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-400">Crypto</span>
                  <span className="font-medium text-yellow-700">{formatEuro(cryptoEur(b))}</span>
                </div>
              </div>
              <details className="mt-2">
                <summary className="text-xs text-blue-600 cursor-pointer select-none py-1">
                  All accounts
                </summary>
                <div className="grid grid-cols-2 gap-x-6 gap-y-1 mt-1 text-xs">
                  {ACCOUNT_ROWS.filter((r) => r.value(b) !== 0).map((r) => (
                    <div key={r.label} className="flex justify-between">
                      <span className="text-gray-400">{r.label}</span>
                      <span className="text-gray-700">{formatEuro(r.value(b))}</span>
                    </div>
                  ))}
                </div>
              </details>
            </div>
          ))}
          {!balances?.length && (
            <p className="px-4 py-12 text-center text-gray-400 text-sm">
              No balance snapshots yet. Add one above.
            </p>
          )}
        </div>

        {/* Desktop table */}
        <div className="overflow-x-auto hidden sm:block">
          <table className="min-w-full text-sm">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className="text-left px-3 py-2 font-semibold text-gray-600">Date</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Total</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">SEB</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Swedbank</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">IBKR stocks</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Swedbank ETF</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut M</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Cash</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">M BTC (€)</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Rev M stocks</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">SEB pension</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Artea pension</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">Revolut R</th>
                <th className="text-right px-3 py-2 font-semibold text-gray-600">R BTC (€)</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {balances?.map((b) => {
                return (
                  <tr key={b.id} className="hover:bg-gray-50">
                    <td className="px-3 py-2">
                      <span className="text-gray-700 font-medium">{formatDate(b.date)}</span>
                      <span className="block text-xs text-gray-400 mt-0.5">{formatTime(b.created_at)}</span>
                    </td>
                    <td className="px-3 py-2 text-right font-bold text-blue-700">{formatEuro(b.total)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.seb)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.ibkr_stocks)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.swed_etf)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_m)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.cash)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.m_btc_eur ?? 0)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_stocks)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.seb_pen)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.art)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.rev_r)}</td>
                    <td className="px-3 py-2 text-right text-gray-600">{formatEuro(b.r_btc_eur ?? 0)}</td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button onClick={() => { setEditingBalance(b); setFormError(null) }} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
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
