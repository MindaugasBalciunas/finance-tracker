import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { transactionsApi } from '../../api/transactions'
import type { TransactionType } from '../../types'

interface Props {
  type: TransactionType
  comment: string
  /** Comma string of labels already on the draft. */
  current: string
  onApply: (labels: string[]) => void
}

/**
 * "You labelled this groceries, barbora last time" — the label counterpart of
 * CategorySuggestion.
 *
 * Label rules only cover what the user bothered to write a rule for. The rest
 * of the pattern lives in the history and nowhere else, so without this the
 * same merchant gets labelled by hand over and over. Offered as one tap, not
 * applied: the form never writes labels the user has not seen.
 */
export default function LabelSuggestion({ type, comment, current, onApply }: Props) {
  const [debounced, setDebounced] = useState(comment)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(comment), 400)
    return () => clearTimeout(t)
  }, [comment])

  const { data } = useQuery({
    queryKey: ['suggest-labels', type, debounced],
    queryFn: () => transactionsApi.suggestLabels({ type, comment: debounced }),
    enabled: debounced.trim().length >= 3,
    staleTime: 30_000,
  })

  const already = current
    .split(',')
    .map((l) => l.trim().toLowerCase())
    .filter(Boolean)
  const missing = (data?.labels ?? []).filter((l) => !already.includes(l))
  if (missing.length === 0) return null

  return (
    <p className="mt-1.5 text-xs text-gray-600 bg-blue-50 border border-blue-100 rounded-lg px-2.5 py-2">
      🏷️ {data?.matches} similar transaction{data?.matches === 1 ? '' : 's'} carry{' '}
      {missing.map((l) => (
        <span key={l} className="inline-block font-medium bg-white border border-blue-200 rounded px-1.5 py-0.5 mr-1">
          {l}
        </span>
      ))}
      <button
        type="button"
        onClick={() => onApply(missing)}
        className="ml-1 text-blue-600 hover:text-blue-800 font-medium"
      >
        Use {missing.length === 1 ? 'it' : 'them'}
      </button>
    </p>
  )
}
