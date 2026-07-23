import { useState } from 'react'
import { useAssets, useAssetSummary, useCreateAsset, useUpdateAsset, useDeleteAsset } from '../hooks/useAssets'
import AssetForm from '../components/forms/AssetForm'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatDate, formatEuro, formatPercent, gainColor } from '../utils/format'
import type { Asset, AssetType, CreateAssetInput } from '../types'
import { ASSET_TYPE_LABELS, ACCOUNT_LABELS } from '../types'
import type { AccountKey } from '../types'

const TYPE_ICONS: Record<AssetType, string> = {
  vehicle: '🚗',
  real_estate: '🏠',
  solar: '☀️',
  other: '📦',
}

const TYPE_BADGE: Record<AssetType, string> = {
  vehicle: 'bg-blue-100 text-blue-700',
  real_estate: 'bg-emerald-100 text-emerald-700',
  solar: 'bg-amber-100 text-amber-700',
  other: 'bg-gray-100 text-gray-600',
}

function accountLabel(key?: string): string {
  if (!key) return ''
  return ACCOUNT_LABELS[key as AccountKey] ?? key
}

function AssetCard({ asset, onEdit, onDelete }: { asset: Asset; onEdit: () => void; onDelete: () => void }) {
  const appreciation = asset.purchase_price > 0
    ? ((asset.current_value - asset.purchase_price) / asset.purchase_price) * 100
    : 0
  const hasLoan = asset.loan_remaining > 0

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-5 flex flex-col gap-3" data-testid="asset-card">
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-2xl shrink-0">{TYPE_ICONS[asset.type]}</span>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-gray-900 truncate" title={asset.name}>{asset.name}</h3>
            <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium mt-0.5 ${TYPE_BADGE[asset.type]}`}>
              {ASSET_TYPE_LABELS[asset.type]}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button onClick={onEdit} aria-label={`Edit ${asset.name}`}
            className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
          <button onClick={onDelete} aria-label={`Delete ${asset.name}`}
            className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 text-sm">
        <div>
          <p className="text-xs text-gray-500">Purchased</p>
          <p className="font-medium text-gray-800">{formatEuro(asset.purchase_price)}</p>
          <p className="text-xs text-gray-400">{asset.purchase_date ? formatDate(asset.purchase_date) : '—'}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Current value</p>
          <p className="font-medium text-gray-800">{formatEuro(asset.current_value)}</p>
          <p className="text-xs text-gray-400">
            {asset.valuation_date ? `valued ${formatDate(asset.valuation_date)}` : 'at cost'}
            {appreciation !== 0 && (
              <span className={`ml-1 font-medium ${gainColor(appreciation)}`}>{formatPercent(appreciation, 1)}</span>
            )}
          </p>
        </div>
      </div>

      {hasLoan ? (
        <div className="bg-orange-50 border border-orange-100 rounded-lg p-3 text-sm">
          <div className="flex items-baseline justify-between">
            <p className="text-xs font-semibold text-orange-700 uppercase tracking-wide">Loan outstanding</p>
            <p className="font-bold text-orange-700">{formatEuro(asset.loan_remaining)}</p>
          </div>
          <p className="text-xs text-orange-600/80 mt-1">
            {asset.loan_remaining_date && <>as of {formatDate(asset.loan_remaining_date)} · </>}
            {asset.loan_rate && <>{asset.loan_rate} · </>}
            {asset.loan_account && <>from {accountLabel(asset.loan_account)}</>}
          </p>
        </div>
      ) : (
        <div className="bg-green-50 border border-green-100 rounded-lg px-3 py-2 text-xs text-green-700 font-medium">
          ✓ Owned outright{asset.loan_paid_off_date && <> — paid off {formatDate(asset.loan_paid_off_date)}</>}
        </div>
      )}

      <div className="flex items-baseline justify-between border-t border-gray-100 pt-2">
        <p className="text-xs text-gray-500">Net equity</p>
        <p className="text-lg font-bold text-gray-900">{formatEuro(asset.equity)}</p>
      </div>

      {asset.notes && <p className="text-xs text-gray-400">{asset.notes}</p>}
    </div>
  )
}

export default function Assets() {
  const [showForm, setShowForm] = useState(false)
  const [editingAsset, setEditingAsset] = useState<Asset | null>(null)
  const [formError, setFormError] = useState<string | null>(null)

  const { data: assets, isLoading: assetsLoading } = useAssets()
  const { data: summary, isLoading: summaryLoading } = useAssetSummary()
  const createMutation = useCreateAsset()
  const updateMutation = useUpdateAsset()
  const deleteMutation = useDeleteAsset()

  const handleCreate = async (input: CreateAssetInput) => {
    try {
      await createMutation.mutateAsync(input)
      setShowForm(false)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }
  const handleUpdate = async (input: CreateAssetInput) => {
    if (!editingAsset) return
    try {
      await updateMutation.mutateAsync({ id: editingAsset.id, input })
      setEditingAsset(null)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }
  const handleDelete = async (asset: Asset) => {
    if (confirm(`Delete asset "${asset.name}"?`)) await deleteMutation.mutateAsync(asset.id)
  }

  if (assetsLoading || summaryLoading) return <LoadingSpinner message="Loading assets…" />

  const valueGain = (summary?.total_value ?? 0) - (summary?.total_purchase_price ?? 0)

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-gray-500">Physical assets — property, vehicles and installations with their loans and equity</p>
        <button
          onClick={() => { setShowForm(true); setFormError(null) }}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 shrink-0 ml-3"
        >
          + Add
        </button>
      </div>

      {/* Create modal */}
      {showForm && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-6">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-xl mx-4 my-auto">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">New Asset</h3>
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <AssetForm onSubmit={handleCreate} onCancel={() => { setShowForm(false); setFormError(null) }} isSubmitting={createMutation.isPending} />
          </div>
        </div>
      )}

      {/* Edit modal */}
      {editingAsset && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-6">
          <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-xl mx-4 my-auto">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Edit Asset — {editingAsset.name}</h3>
            {formError && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{formError}</p>}
            <AssetForm
              key={editingAsset.id}
              onSubmit={handleUpdate}
              onCancel={() => { setEditingAsset(null); setFormError(null) }}
              isSubmitting={updateMutation.isPending}
              defaultValues={{
                name: editingAsset.name,
                type: editingAsset.type,
                purchase_date: editingAsset.purchase_date?.slice(0, 10) ?? '',
                purchase_price: editingAsset.purchase_price,
                current_value: editingAsset.current_value,
                valuation_date: editingAsset.valuation_date?.slice(0, 10) ?? '',
                notes: editingAsset.notes,
                loan_remaining: editingAsset.loan_remaining,
                loan_remaining_date: editingAsset.loan_remaining_date?.slice(0, 10) ?? '',
                loan_rate: editingAsset.loan_rate ?? '',
                loan_account: editingAsset.loan_account ?? '',
                loan_paid_off_date: editingAsset.loan_paid_off_date?.slice(0, 10) ?? '',
              }}
            />
          </div>
        </div>
      )}

      {/* Summary cards */}
      {summary && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Total value</p>
            <p className="text-xl font-bold text-gray-900">{formatEuro(summary.total_value)}</p>
            <p className="text-xs text-gray-400 mt-1">{summary.count} asset{summary.count !== 1 ? 's' : ''}</p>
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Loans outstanding</p>
            <p className={`text-xl font-bold ${summary.total_loans > 0 ? 'text-orange-600' : 'text-gray-900'}`}>
              {formatEuro(summary.total_loans)}
            </p>
            <p className="text-xs text-gray-400 mt-1">Remaining debt</p>
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Net equity</p>
            <p className="text-xl font-bold text-gray-900">{formatEuro(summary.net_equity)}</p>
            <p className="text-xs text-gray-400 mt-1">Value − loans</p>
          </div>
          <div className="bg-white rounded-xl border border-gray-200 p-4">
            <p className="text-xs text-gray-500 mb-1">Value gain</p>
            <p className={`text-xl font-bold ${gainColor(valueGain)}`}>
              {valueGain >= 0 ? '+' : ''}{formatEuro(valueGain)}
            </p>
            <p className="text-xs text-gray-400 mt-1">vs. purchase price</p>
          </div>
        </div>
      )}

      {/* Asset cards */}
      {assets && assets.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
          {assets.map((a) => (
            <AssetCard key={a.id} asset={a} onEdit={() => { setEditingAsset(a); setFormError(null) }} onDelete={() => handleDelete(a)} />
          ))}
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-gray-200 px-4 py-12 text-center text-gray-400">
          No assets yet. Add your first asset above.
        </div>
      )}
    </div>
  )
}
