import { useEffect, useMemo, useState } from 'react'
import { useAllTransactions } from '../../hooks/useTransactions'
import LoadingSpinner from './LoadingSpinner'
import { formatEuro } from '../../utils/format'
import type { Category, TransactionType } from '../../types'
import type { DateRange } from './DateRangeFilter'

interface Props {
  category?: Category
  label?: string
  title?: string
  type: TransactionType
  dateRange: DateRange
  onClose: () => void
}

const TYPE_STYLES: Record<TransactionType, { badge: string; amount: string; sign: string }> = {
  expense: { badge: 'bg-red-100 text-red-700', amount: 'text-red-600', sign: '-' },
  income: { badge: 'bg-green-100 text-green-700', amount: 'text-green-600', sign: '+' },
  investment: { badge: 'bg-blue-100 text-blue-700', amount: 'text-blue-600', sign: '' },
}

export default function CategoryTransactionsModal({ category, label, title, type, dateRange, onClose }: Props) {
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

  const allTransactions = data?.data ?? []
  const [activeLabel, setActiveLabel] = useState<string | null>(null)

  // Sums per label across the fetched set — a transaction with several labels
  // counts toward each (labels are overlapping views, not splits).
  const labelSums = useMemo(() => {
    const sums: Record<string, number> = {}
    for (const tx of allTransactions) {
      for (const l of (tx.labels ?? '').split(',').filter(Boolean)) {
        sums[l] = (sums[l] ?? 0) + tx.amount.value
      }
    }
    return Object.entries(sums).sort((a, b) => b[1] - a[1])
  }, [allTransactions])

  const transactions = activeLabel
    ? allTransactions.filter((tx) => (tx.labels ?? '').split(',').includes(activeLabel))
    : allTransactions
  const total = transactions.reduce((s, tx) => s + tx.amount.value, 0)
  const styles = TYPE_STYLES[type]

  const period = dateRange.date_from || dateRange.date_to
    ? `${dateRange.date_from ?? '…'} → ${dateRange.date_to ?? 'today'}`
    : 'All time'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-6"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={`${title ?? category ?? label} transactions`}
    >
      <div
        className="bg-white rounded-xl shadow-xl w-full max-w-3xl max-h-[90vh] sm:max-h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between px-4 sm:px-6 py-4 border-b border-gray-200">
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-lg font-semibold text-gray-900">{title ?? category ?? label}</h3>
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

        {labelSums.length > 0 && (
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
            {activeLabel && (
              <button onClick={() => setActiveLabel(null)} className="text-xs text-gray-400 hover:text-gray-600 px-1">clear</button>
            )}
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
