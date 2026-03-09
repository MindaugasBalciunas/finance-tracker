import { useForm } from 'react-hook-form'
import type { CreateBalanceInput } from '../../types'

const ACCOUNTS: { key: keyof CreateBalanceInput; label: string }[] = [
  { key: 'seb', label: 'SEB' },
  { key: 'swed', label: 'Swedbank' },
  { key: 'swed_etf', label: 'Swed ETF' },
  { key: 'swed_pen', label: 'Swed Pension' },
  { key: 'luminor', label: 'Luminor' },
  { key: 'art', label: 'Art' },
  { key: 'cash', label: 'Cash' },
  { key: 'rev_m', label: 'Revolut M' },
  { key: 'rev_r', label: 'Revolut R' },
  { key: 'r_btc', label: 'R BTC' },
  { key: 'm_btc', label: 'M BTC' },
  { key: 'rev_stocks', label: 'Rev Stocks' },
]

interface Props {
  onSubmit: (data: CreateBalanceInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateBalanceInput>
}

export default function BalanceForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const { register, handleSubmit, formState: { errors } } = useForm<CreateBalanceInput>({
    defaultValues: { ...defaultValues },
  })

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Date</label>
        <input
          type="date"
          {...register('date', { required: 'Date is required' })}
          className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        {errors.date && <p className="text-xs text-red-600 mt-1">{errors.date.message}</p>}
      </div>

      <p className="text-xs text-gray-500">
        Enter balances for each account. Leave blank for zero. Total is auto-calculated.
      </p>

      <div className="grid grid-cols-2 gap-3">
        {ACCOUNTS.map(({ key, label }) => (
          <div key={key}>
            <label className="block text-xs font-medium text-gray-600 mb-0.5">{label} (€)</label>
            <input
              type="number"
              step="0.01"
              min="0"
              {...register(key, { valueAsNumber: true })}
              placeholder="0.00"
              className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
        ))}
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
