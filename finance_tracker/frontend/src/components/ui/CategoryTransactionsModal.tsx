import { useEffect, useMemo, useState } from 'react'
import { useAllTransactions } from '../../hooks/useTransactions'
import LoadingSpinner from './LoadingSpinner'
import { formatEuro } from '../../utils/format'
import { txLabels } from '../../utils/labels'
import type { Category, TransactionType } from '../../types'
import type { DateRange } from './DateRangeFilter'

interface Props {
  category?: Category
  label?: string
  title?: string
  type: TransactionType
  dateRange: DateRange
  // Show only transactions WITHOUT any label (the API can't express this, so
  // the type+range fetch is filtered client-side).
  unlabeled?: boolean
  onClose: () => void
}

// Sentinel for the synthetic "no label" chip — cannot collide with real
// labels, which are normalized lowercase words.
const NO_LABEL = '__none__'

const TYPE_STYLES: Record<TransactionType, { badge: string; amount: string; sign: string }> = {
  expense: { badge: 'bg-red-100 text-red-700', amount: 'text-red-600', sign: '-' },
  income: { badge: 'bg-green-100 text-green-700', amount: 'text-green-600', sign: '+' },
  investment: { badge: 'bg-blue-100 text-blue-700', amount: 'text-blue-600', sign: '' },
}

export default function CategoryTransactionsModal({ category, label, title, type, dateRange, unlabeled, onClose }: Props) {
  const { data, isLoading } = useAllTransactions({
    ...(category ? { category } : {}),
    ...(label ? { label } : {}),
    type,
    ...dateRange,
  })

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const fetched = data?.data ?? []
  const allTransactions = useMemo(
    () => (unlabeled ? fetched.filter((tx) => txLabels(tx).length === 0) : fetched),
    [fetched, unlabeled]
  )
  const [activeLabel, setActiveLabel] = useState<string | null>(null)

  // Sums per label across the fetched set — a transaction with several labels
  // counts toward each (labels are overlapping views, not splits). Unlabeled
  // money gets its own synthetic chip so the set fully decomposes.
  const { labelSums, noLabelSum } = useMemo(() => {
    const sums: Record<string, number> = {}
    let noLabelSum = 0
    for (const tx of allTransactions) {
      const labels = txLabels(tx)
      if (labels.length === 0) noLabelSum += tx.amount.value
      for (const l of labels) {
        sums[l] = (sums[l] ?? 0) + tx.amount.value
      }
    }
    return { labelSums: Object.entries(sums).sort((a, b) => b[1] - a[1]), noLabelSum }
  }, [allTransactions])

  const transactions = activeLabel
    ? activeLabel === NO_LABEL
      ? allTransactions.filter((tx) => txLabels(tx).length === 0)
      : allTransactions.filter((tx) => txLabels(tx).includes(activeLabel))
    : allTransactions
  const total = transactions.reduce((s, tx) => s + tx.amount.value, 0)
  const styles = TYPE_STYLES[type]

  // Month → sum profile of the visible set: a compact trend strip that makes
  // every label/category click yield a shape, not just a list.
  const monthProfile = useMemo(() => {
    const sums: Record<string, number> = {}
    for (const tx of transactions) {
      const mk = tx.date.slice(0, 7)
      sums[mk] = (sums[mk] ?? 0) + tx.amount.value
    }
    const months = Object.keys(sums).sort()
    return months.map((m) => ({ month: m, sum: sums[m] }))
  }, [transactions])
  // Only the last 24 months fit visually; older history is summarized in text.
  const profileWindow = monthProfile.slice(-24)
  const profileMax = Math.max(...profileWindow.map((m) => m.sum), 1)

  const period = dateRange.date_from || dateRange.date_to
    ? `${dateRange.date_from ?? '…'} → ${dateRange.date_to ?? 'today'}`
    : 'All time'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-6"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={`${title ?? category ?? label ?? 'Unlabeled'} transactions`}
    >
      <div
        className="bg-white rounded-xl shadow-xl w-full max-w-3xl max-h-[90vh] sm:max-h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between px-4 sm:px-6 py-4 border-b border-gray-200">
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-lg font-semibold text-gray-900">{title ?? category ?? label ?? 'Unlabeled'}</h3>
              <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${styles.badge}`}>
                {type}
              </span>
            </div>
            <p className="text-xs text-gray-400 mt-0.5">{period}</p>
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="text-gray-400 hover:text-gray-600 text-xl leading-none p-1 -m-1"
          >
            ×
          </button>
        </div>

        {!unlabeled && (labelSums.length > 0 || noLabelSum > 0) && (
          <div className="flex flex-wrap gap-1.5 px-4 sm:px-6 py-2.5 border-b border-gray-100">
            {labelSums.map(([l, sum]) => (
              <button
                key={l}
                onClick={() => setActiveLabel(activeLabel === l ? null : l)}
                className={`inline-flex items-center gap-1 text-xs font-medium rounded-full px-2.5 py-1 border transition-colors ${
                  activeLabel === l
                    ? 'bg-indigo-600 text-white border-indigo-600'
                    : 'bg-indigo-50 text-indigo-700 border-indigo-100 hover:bg-indigo-100'
                }`}
              >
                {l}
                <span className={activeLabel === l ? 'text-indigo-200' : 'text-indigo-400'}>{formatEuro(sum)}</span>
              </button>
            ))}
            {noLabelSum > 0 && labelSums.length > 0 && (
              <button
                onClick={() => setActiveLabel(activeLabel === NO_LABEL ? null : NO_LABEL)}
                className={`inline-flex items-center gap-1 text-xs font-medium rounded-full px-2.5 py-1 border transition-colors ${
                  activeLabel === NO_LABEL
                    ? 'bg-gray-600 text-white border-gray-600'
                    : 'bg-gray-50 text-gray-500 border-gray-200 hover:bg-gray-100'
                }`}
              >
                no label
                <span className={activeLabel === NO_LABEL ? 'text-gray-300' : 'text-gray-400'}>{formatEuro(noLabelSum)}</span>
              </button>
            )}
            {activeLabel && (
              <button onClick={() => setActiveLabel(null)} className="text-xs text-gray-400 hover:text-gray-600 px-1">clear</button>
            )}
          </div>
        )}

        {/* Month-by-month shape of the visible set */}
        {!isLoading && profileWindow.length >= 2 && (
          <div className="px-4 sm:px-6 py-2.5 border-b border-gray-100">
            <div className="flex items-end gap-[3px] h-12">
              {profileWindow.map(({ month, sum }) => (
                <div
                  key={month}
                  className="flex-1 bg-indigo-200 hover:bg-indigo-400 rounded-sm transition-colors"
                  title={`${month}: ${formatEuro(sum)}`}
                  style={{ height: `${Math.max((sum / profileMax) * 100, 4)}%` }}
                />
              ))}
            </div>
            <div className="flex justify-between text-[10px] text-gray-400 mt-0.5">
              <span>{profileWindow[0].month}</span>
              <span>
                avg {formatEuro(profileWindow.reduce((s, m) => s + m.sum, 0) / profileWindow.length)}/mo
                {monthProfile.length > profileWindow.length ? ` · last 24 of ${monthProfile.length} months` : ''}
              </span>
              <span>{profileWindow[profileWindow.length - 1].month}</span>
            </div>
          </div>
        )}

        {isLoading ? (
          <div className="py-12">
            <LoadingSpinner />
          </div>
        ) : transactions.length === 0 ? (
          <p className="text-center text-gray-400 py-12 text-sm">
            No transactions in this period.
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 sm:px-6 py-3 bg-gray-50 border-b border-gray-100 text-sm">
              <span className="text-gray-500 whitespace-nowrap">
                {transactions.length} transaction{transactions.length === 1 ? '' : 's'}
              </span>
              <span className="text-gray-500 whitespace-nowrap">
                Avg <span className="font-medium text-gray-700">{formatEuro(total / transactions.length)}</span>
              </span>
              <span className={`font-semibold whitespace-nowrap ${styles.amount}`}>
                {styles.sign}{formatEuro(total)}
              </span>
            </div>
            <ul className="overflow-y-auto divide-y divide-gray-100">
              {transactions.map((tx) => (
                <li key={tx.id} className="px-4 sm:px-6 py-2.5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm text-gray-700 break-words">
                        {tx.comment || '—'}
                      </p>
                      <p className="text-xs text-gray-400 mt-0.5 tabular-nums">
                        {tx.date.slice(0, 10)}
                        {!category && <span className="ml-1.5 inline-block text-[10px] font-medium bg-gray-100 text-gray-500 rounded px-1.5 py-0.5">{tx.category}</span>}
                        {tx.labels && tx.labels.split(',').map((l) => (
                          <span key={l} className="ml-1 inline-block text-[10px] font-medium bg-indigo-50 text-indigo-600 rounded px-1.5 py-0.5">{l}</span>
                        ))}
                      </p>
                    </div>
                    <span className={`text-sm font-medium whitespace-nowrap flex-shrink-0 ${styles.amount}`}>
                      {styles.sign}{formatEuro(tx.amount.value)}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </div>
  )
}
