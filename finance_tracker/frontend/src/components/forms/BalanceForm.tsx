import { useState } from 'react'
import { useForm } from 'react-hook-form'
import type { CreateBalanceInput } from '../../types'
import { useBtcPrice } from '../../hooks/useBtcPrice'
import { formatEuro } from '../../utils/format'

const EUR_ACCOUNTS: { key: keyof CreateBalanceInput; label: string }[] = [
  { key: 'seb',       label: 'Seb' },
  { key: 'swed',      label: 'Swedbank' },
  { key: 'swed_etf',  label: 'Swedbank ETF' },
  { key: 'seb_pen',  label: 'SEB 2nd pillar pension' },
  { key: 'luminor',   label: 'Luminor' },
  { key: 'art',       label: 'Artea 3rd pillar pension' },
  { key: 'cash',      label: 'Cash' },
  { key: 'rev_m',     label: 'Revolut M account' },
  { key: 'rev_r',     label: 'Revolut R account' },
  { key: 'rev_stocks',  label: 'Revolut M account stocks' },
  { key: 'ibkr_stocks', label: 'IBKR stocks' },
]

interface Props {
  onSubmit: (data: CreateBalanceInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateBalanceInput>
}

export default function BalanceForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const { data: btcPrice } = useBtcPrice()

  const initRBtc = defaultValues?.r_btc != null ? String(defaultValues.r_btc) : ''
  const initMBtc = defaultValues?.m_btc != null ? String(defaultValues.m_btc) : ''

  const [rBtc, setRBtc] = useState<string>(initRBtc)
  const [mBtc, setMBtc] = useState<string>(initMBtc)

  const { register, handleSubmit, formState: { errors } } = useForm<CreateBalanceInput>({
    defaultValues: { ...defaultValues },
  })

  const rBtcEur = btcPrice && rBtc ? parseFloat(rBtc) * btcPrice : null
  const mBtcEur = btcPrice && mBtc ? parseFloat(mBtc) * btcPrice : null

  const handleFormSubmit = (data: CreateBalanceInput) => {
    // valueAsNumber returns NaN for untouched empty inputs — fall back to defaultValues then 0
    for (const { key } of EUR_ACCOUNTS) {
      const v = data[key] as number
      if (typeof v !== 'number' || isNaN(v)) {
        (data as any)[key] = defaultValues?.[key] ?? 0
      }
    }
    data.r_btc = rBtc ? parseFloat(rBtc) : 0
    data.m_btc = mBtc ? parseFloat(mBtc) : 0
    data.btc_price = btcPrice ?? 0
    onSubmit(data)
  }

  return (
    <form onSubmit={handleSubmit(handleFormSubmit)} className="space-y-4">
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
        {EUR_ACCOUNTS.map(({ key, label }) => (
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

        {/* BTC fields — stored and edited in BTC units */}
        <div>
          <label className="block text-xs font-medium text-gray-600 mb-0.5">Revolut R BTC (₿)</label>
          <input
            type="number"
            step="0.00000001"
            min="0"
            value={rBtc}
            onChange={(e) => setRBtc(e.target.value)}
            placeholder="0.00000000"
            className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {rBtcEur != null && (
            <p className="text-xs text-gray-400 mt-0.5">≈ {formatEuro(rBtcEur)}</p>
          )}
        </div>

        <div>
          <label className="block text-xs font-medium text-gray-600 mb-0.5">Revolut M BTC (₿)</label>
          <input
            type="number"
            step="0.00000001"
            min="0"
            value={mBtc}
            onChange={(e) => setMBtc(e.target.value)}
            placeholder="0.00000000"
            className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {mBtcEur != null && (
            <p className="text-xs text-gray-400 mt-0.5">≈ {formatEuro(mBtcEur)}</p>
          )}
        </div>
      </div>

      {btcPrice && (
        <p className="text-xs text-gray-400">
          Live BTC price: €{btcPrice.toLocaleString()} /BTC
        </p>
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
