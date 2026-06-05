import { useForm, Controller } from 'react-hook-form'
import type { CreateStockTradeInput, StockAction, StockSource } from '../../types'
import DateInput from '../ui/DateInput'

interface Props {
  onSubmit: (data: CreateStockTradeInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateStockTradeInput>
}

export default function StockTradeForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const today = new Date().toISOString().slice(0, 10)
  const { register, handleSubmit, control, formState: { errors } } = useForm<CreateStockTradeInput>({
    defaultValues: { action: 'buy', currency: 'USD', source: 'Revolut', date: today, ...defaultValues },
  })

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
          <label className="block text-sm font-medium text-gray-700 mb-1">Action</label>
          <select
            {...register('action', { required: true })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {(['buy', 'sell'] as StockAction[]).map((a) => (
              <option key={a} value={a}>{a.toUpperCase()}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Ticker</label>
          <input
            type="text"
            placeholder="e.g. MSFT"
            {...register('ticker', { required: 'Ticker is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm uppercase focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.ticker && <p className="text-xs text-red-600 mt-1">{errors.ticker.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Broker</label>
          <select
            {...register('source', { required: true })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {(['Revolut', 'IBKR'] as StockSource[]).map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Currency</label>
          <select
            {...register('currency')}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="USD">USD</option>
            <option value="EUR">EUR</option>
          </select>
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Shares</label>
          <input
            type="number"
            step="0.0001"
            min="0.0001"
            {...register('shares', { required: 'Required', valueAsNumber: true, min: { value: 0.0001, message: 'Must be > 0' } })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.shares && <p className="text-xs text-red-600 mt-1">{errors.shares.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Price per share</label>
          <input
            type="number"
            step="0.0001"
            min="0.0001"
            {...register('price_per_share', { required: 'Required', valueAsNumber: true, min: { value: 0.0001, message: 'Must be > 0' } })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.price_per_share && <p className="text-xs text-red-600 mt-1">{errors.price_per_share.message}</p>}
        </div>
      </div>

      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Notes</label>
        <input
          type="text"
          {...register('notes')}
          placeholder="Optional"
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
          {isSubmitting ? 'Saving...' : 'Save Trade'}
        </button>
      </div>
    </form>
  )
}
