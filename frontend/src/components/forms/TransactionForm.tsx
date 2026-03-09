import { useForm } from 'react-hook-form'
import type { CreateTransactionInput, TransactionType } from '../../types'

const SUGGESTED_CATEGORIES = [
  'Food', 'Kids', 'Kids(food)', 'Kids school', 'Kids (Entertainment)',
  'Health', 'Finance', 'Investment', 'Entertainment', 'House expense',
  'Credit', 'Car', 'Income', 'Clothes', 'Divorce',
]

interface Props {
  onSubmit: (data: CreateTransactionInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateTransactionInput>
}

export default function TransactionForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const { register, handleSubmit, formState: { errors } } = useForm<CreateTransactionInput>({
    defaultValues: { type: 'expense', ...defaultValues },
  })

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Date</label>
          <input
            type="date"
            {...register('date', { required: 'Date is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
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
          <input
            type="text"
            list="category-options"
            {...register('category', { required: 'Category is required' })}
            placeholder="Select or type a category..."
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <datalist id="category-options">
            {SUGGESTED_CATEGORIES.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
          {errors.category && <p className="text-xs text-red-600 mt-1">{errors.category.message}</p>}
        </div>
      </div>

      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Comment</label>
        <input
          type="text"
          {...register('comment')}
          placeholder="Optional description..."
          className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>

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
