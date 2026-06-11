import { useEffect, useState, useRef } from 'react'
import { useForm, useWatch, Controller } from 'react-hook-form'
import type { CreateTransactionInput, TransactionType, AccountKey } from '../../types'
import { ACCOUNT_LABELS } from '../../types'
import { CATEGORIES_BY_TYPE } from '../../constants/categories'
import DateInput from '../ui/DateInput'
import { useTransactionComments } from '../../hooks/useTransactions'

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
        </div>
      </div>

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
