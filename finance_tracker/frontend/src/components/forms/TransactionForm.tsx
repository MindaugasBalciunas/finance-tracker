import { useEffect, useState, useRef } from 'react'
import { useForm, useWatch, Controller } from 'react-hook-form'
import type { CreateTransactionInput, TransactionType, AccountKey } from '../../types'
import { ACCOUNT_LABELS } from '../../types'
import { CATEGORIES_BY_TYPE, CATEGORY_HINTS } from '../../constants/categories'
import DateInput from '../ui/DateInput'
import { useQuery } from '@tanstack/react-query'
import { transactionsApi } from '../../api/transactions'
import { useTransactionComments } from '../../hooks/useTransactions'
import { useLabels, useLabelRules } from '../../hooks/useBudgets'

interface Props {
  onSubmit: (data: CreateTransactionInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateTransactionInput>
}

const ALL_ACCOUNTS = Object.entries(ACCOUNT_LABELS) as [AccountKey, string][]

function AccountSelect({ label, name, register }: { label: string; name: 'debit_account' | 'credit_account'; register: any }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 mb-1">
        {label} <span className="text-gray-400 font-normal">(optional)</span>
      </label>
      <select
        {...register(name)}
        className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        <option value="">None</option>
        {ALL_ACCOUNTS.map(([key, label]) => (
          <option key={key} value={key}>{label}</option>
        ))}
      </select>
    </div>
  )
}

export default function TransactionForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const today = new Date().toISOString().slice(0, 10)
  const { register, handleSubmit, formState: { errors }, control, setValue, watch } = useForm<CreateTransactionInput>({
    defaultValues: { type: 'expense', date: today, ...defaultValues },
  })

  const selectedType = useWatch({ control, name: 'type' })
  const categories = CATEGORIES_BY_TYPE[selectedType] ?? []

  const { data: allComments = [] } = useTransactionComments()
  const commentValue = watch('comment') ?? ''
  const [showSuggestions, setShowSuggestions] = useState(false)
  const commentRef = useRef<HTMLDivElement>(null)

  const suggestions = commentValue.length > 0
    ? allComments.filter(c => c.toLowerCase().includes(commentValue.toLowerCase()) && c !== commentValue).slice(0, 8)
    : []

  // Label suggestions: existing labels as one-tap chips, plus a live preview
  // of labels the saved rules will apply automatically on save.
  const { data: allLabels = [] } = useLabels()
  const { data: labelRules = [] } = useLabelRules()
  const labelsValue = watch('labels') ?? ''
  const selectedCategory = watch('category') ?? ''
  const currentLabels = labelsValue.split(',').map((l) => l.trim().toLowerCase()).filter(Boolean)

  const autoLabels = labelRules
    .filter((r) => {
      const catOk = !r.category || r.category.toLowerCase() === String(selectedCategory).toLowerCase()
      const commentOk = !r.comment_match || commentValue.toLowerCase().includes(r.comment_match.toLowerCase())
      return (r.category !== '' || r.comment_match !== '') && catOk && commentOk
    })
    .map((r) => r.label)
    .filter((l, i, arr) => arr.indexOf(l) === i)

  const labelChips = allLabels
    .filter((l) => !currentLabels.includes(l) && !autoLabels.includes(l))
    .slice(0, 8)

  function addLabelChip(label: string) {
    const next = [...currentLabels, label].join(',')
    setValue('labels', next)
  }

  // Category suggestion from similar historical transactions (debounced).
  const amountValue = watch('amount')
  const [debounced, setDebounced] = useState({ comment: '', amount: 0 })
  useEffect(() => {
    const t = setTimeout(() => setDebounced({ comment: commentValue, amount: Number(amountValue) || 0 }), 400)
    return () => clearTimeout(t)
  }, [commentValue, amountValue])

  const { data: suggestion } = useQuery({
    queryKey: ['suggest-category', selectedType, debounced.comment, debounced.amount],
    queryFn: () => transactionsApi.suggestCategory({
      type: selectedType,
      comment: debounced.comment,
      amount: debounced.amount,
    }),
    enabled: debounced.comment.trim().length >= 3 || debounced.amount > 0,
    staleTime: 30_000,
  })

  useEffect(() => {
    if (!defaultValues?.category) {
      setValue('category', '' as any)
    }
  }, [selectedType, defaultValues?.category, setValue])

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Date</label>
          <Controller
            name="date"
            control={control}
            rules={{ required: 'Date is required' }}
            render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
          />
          {errors.date && <p className="text-xs text-red-600 mt-1">{errors.date.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
          <select
            {...register('type', { required: 'Type is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {(['expense', 'income', 'investment'] as TransactionType[]).map((t) => (
              <option key={t} value={t}>{t.charAt(0).toUpperCase() + t.slice(1)}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Amount (€)</label>
          <input
            type="number"
            step="0.01"
            min="0.01"
            {...register('amount', { required: 'Amount is required', valueAsNumber: true, min: { value: 0.01, message: 'Must be > 0' } })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.amount && <p className="text-xs text-red-600 mt-1">{errors.amount.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Category</label>
          <select
            {...register('category', { required: 'Category is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">Select a category...</option>
            {categories.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          {errors.category && <p className="text-xs text-red-600 mt-1">{errors.category.message}</p>}
          {selectedCategory && CATEGORY_HINTS[String(selectedCategory)] && (
            <p className="text-xs text-gray-400 mt-1">{CATEGORY_HINTS[String(selectedCategory)]}</p>
          )}
        </div>
      </div>

      {suggestion && suggestion.category && suggestion.category !== String(selectedCategory) && (
        <p className="text-xs text-gray-600 bg-blue-50 border border-blue-100 rounded-lg px-3 py-2 -mt-1">
          💡 {suggestion.matches} similar transaction{suggestion.matches === 1 ? '' : 's'}{' '}
          ({suggestion.basis === 'amount' ? 'same amount' : 'matching comment'}) are usually{' '}
          <span className="font-semibold text-gray-800">{suggestion.category}</span>
          <button
            type="button"
            onClick={() => setValue('category', suggestion.category as any)}
            className="ml-2 text-blue-600 hover:text-blue-800 font-medium"
          >
            Use it
          </button>
        </p>
      )}

      <div className="relative" ref={commentRef}>
        <label className="block text-sm font-medium text-gray-700 mb-1">Comment</label>
        <input
          type="text"
          {...register('comment')}
          placeholder="Optional description..."
          autoComplete="off"
          onFocus={() => setShowSuggestions(true)}
          onBlur={() => setTimeout(() => setShowSuggestions(false), 150)}
          className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        {showSuggestions && suggestions.length > 0 && (
          <ul className="absolute z-50 left-0 right-0 mt-1 bg-white border border-gray-200 rounded-lg shadow-lg overflow-hidden">
            {suggestions.map((s) => (
              <li
                key={s}
                onMouseDown={() => { setValue('comment', s); setShowSuggestions(false) }}
                className="px-3 py-2 text-sm text-gray-700 hover:bg-blue-50 cursor-pointer truncate"
              >
                {s}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Labels</label>
        <input
          type="text"
          {...register('labels')}
          placeholder="Optional tags, comma-separated (e.g. loan, fixed)"
          autoComplete="off"
          className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        {autoLabels.length > 0 && (
          <p className="text-xs text-gray-500 mt-1.5">
            <span className="text-green-600">⚡ auto:</span>{' '}
            {autoLabels.map((l) => (
              <span key={l} className="inline-block text-[11px] font-medium bg-green-50 text-green-700 border border-green-200 rounded px-1.5 py-0.5 mr-1">{l}</span>
            ))}
            <span className="text-gray-400">will be applied by your rules</span>
          </p>
        )}
        {labelChips.length > 0 && (
          <p className="mt-1.5">
            {labelChips.map((l) => (
              <button
                key={l}
                type="button"
                onClick={() => addLabelChip(l)}
                className="inline-block text-[11px] font-medium bg-indigo-50 text-indigo-600 border border-indigo-100 rounded px-1.5 py-0.5 mr-1 mb-1 hover:bg-indigo-100"
              >
                + {l}
              </button>
            ))}
          </p>
        )}
      </div>

      {/* Account fields — context-aware based on transaction type */}
      {selectedType === 'expense' && (
        <AccountSelect label="From Account" name="debit_account" register={register} />
      )}

      {selectedType === 'income' && (
        <AccountSelect label="To Account" name="credit_account" register={register} />
      )}

      {selectedType === 'investment' && (
        <div className="grid grid-cols-2 gap-4">
          <AccountSelect label="From Account" name="debit_account" register={register} />
          <AccountSelect label="To Account" name="credit_account" register={register} />
        </div>
      )}

      <div className="flex justify-end gap-3 pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={isSubmitting}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {isSubmitting ? 'Saving...' : 'Save'}
        </button>
      </div>
    </form>
  )
}
