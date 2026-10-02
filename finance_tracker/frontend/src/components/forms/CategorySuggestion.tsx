import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { transactionsApi } from '../../api/transactions'
import type { TransactionType } from '../../types'

interface Props {
  type: TransactionType
  comment: string
  amount: number
  /** The category already chosen — no point suggesting what is already set. */
  current: string
  onPick: (category: string) => void
}

/**
 * "7 similar transactions are usually Food" — the category the user's own
 * history gives, offered rather than applied.
 *
 * Shared by the transaction form and the bank review queue. The bank
 * importer asks the same question server-side at sync time, so this is the
 * second look: it re-asks against the comment as it stands now, which is what
 * makes it useful right after the user fixes a mangled description.
 */
export default function CategorySuggestion({ type, comment, amount, current, onPick }: Props) {
  const [debounced, setDebounced] = useState({ comment, amount })
  useEffect(() => {
    const t = setTimeout(() => setDebounced({ comment, amount }), 400)
    return () => clearTimeout(t)
  }, [comment, amount])

  const { data: suggestion } = useQuery({
    queryKey: ['suggest-category', type, debounced.comment, debounced.amount],
    queryFn: () =>
      transactionsApi.suggestCategory({
        type,
        comment: debounced.comment,
        amount: debounced.amount,
      }),
    enabled: debounced.comment.trim().length >= 3 || debounced.amount > 0,
    staleTime: 30_000,
  })

  if (!suggestion?.category || suggestion.category === current) return null

  return (
    <p className="text-xs text-gray-600 bg-blue-50 border border-blue-100 rounded-lg px-3 py-2">
      💡 {suggestion.matches} similar transaction{suggestion.matches === 1 ? '' : 's'}{' '}
      ({suggestion.basis === 'amount' ? 'same amount' : 'matching comment'}) are usually{' '}
      <span className="font-semibold text-gray-800">{suggestion.category}</span>
      <button
        type="button"
        onClick={() => onPick(suggestion.category)}
        className="ml-2 text-blue-600 hover:text-blue-800 font-medium"
      >
        Use it
      </button>
    </p>
  )
}
