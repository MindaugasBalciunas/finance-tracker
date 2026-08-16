import { useForm, Controller } from 'react-hook-form'
import type { AssetType, CreateAssetInput } from '../../types'
import { ASSET_TYPE_LABELS, ACCOUNT_LABELS } from '../../types'
import { useLabels } from '../../hooks/useBudgets'
import DateInput from '../ui/DateInput'

interface Props {
  onSubmit: (data: CreateAssetInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateAssetInput>
}

// Optional number inputs yield NaN when left empty — strip them so the API
// receives either a valid number or no field at all.
function clean(data: CreateAssetInput): CreateAssetInput {
  return {
    ...data,
    current_value: Number.isFinite(data.current_value) ? data.current_value : undefined,
    loan_remaining: Number.isFinite(data.loan_remaining) ? data.loan_remaining : undefined,
    loan_margin: Number.isFinite(data.loan_margin) ? data.loan_margin : undefined,
    loan_base_rate: Number.isFinite(data.loan_base_rate) ? data.loan_base_rate : undefined,
    loan_monthly_payment: Number.isFinite(data.loan_monthly_payment) ? data.loan_monthly_payment : undefined,
  }
}

export default function AssetForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const { register, handleSubmit, control, formState: { errors } } = useForm<CreateAssetInput>({
    defaultValues: { type: 'other', ...defaultValues },
  })
  const { data: allLabels = [] } = useLabels()

  const inputCls = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'

  return (
    <form onSubmit={handleSubmit((data) => onSubmit(clean(data)))} className="space-y-4">
      <div className="grid grid-cols-3 gap-4">
        <div className="col-span-2">
          <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
          <input
            type="text"
            placeholder="e.g. Toyota RAV4 Style Hybrid"
            {...register('name', { required: 'Name is required' })}
            className={inputCls}
          />
          {errors.name && <p className="text-xs text-red-600 mt-1">{errors.name.message}</p>}
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
          <select {...register('type', { required: true })} className={inputCls}>
            {(Object.keys(ASSET_TYPE_LABELS) as AssetType[]).map((t) => (
              <option key={t} value={t}>{ASSET_TYPE_LABELS[t]}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Purchase date</label>
          <Controller
            name="purchase_date"
            control={control}
            render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Purchase price (€)</label>
          <input
            type="number"
            step="0.01"
            min="0.01"
            {...register('purchase_price', { required: 'Required', valueAsNumber: true, min: { value: 0.01, message: 'Must be > 0' } })}
            className={inputCls}
          />
          {errors.purchase_price && <p className="text-xs text-red-600 mt-1">{errors.purchase_price.message}</p>}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Current value (€)</label>
          <input
            type="number"
            step="0.01"
            min="0"
            placeholder="defaults to purchase price"
            {...register('current_value', { valueAsNumber: true, min: { value: 0, message: 'Must be ≥ 0' } })}
            className={inputCls}
          />
          {errors.current_value && <p className="text-xs text-red-600 mt-1">{errors.current_value.message}</p>}
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Valuation date</label>
          <Controller
            name="valuation_date"
            control={control}
            render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
          />
        </div>
      </div>

      <fieldset className="border border-gray-200 rounded-lg p-3 space-y-3">
        <legend className="text-xs font-semibold text-gray-500 uppercase tracking-wide px-1">Financing (optional)</legend>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Loan remaining (€)</label>
            <input
              type="number"
              step="0.01"
              min="0"
              {...register('loan_remaining', { valueAsNumber: true, min: { value: 0, message: 'Must be ≥ 0' } })}
              className={inputCls}
            />
            {errors.loan_remaining && <p className="text-xs text-red-600 mt-1">{errors.loan_remaining.message}</p>}
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Balance as of</label>
            <Controller
              name="loan_remaining_date"
              control={control}
              render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
            />
          </div>
        </div>
        {/* Interest split into the fixed bank margin and the variable base
            (EURIBOR) that resets on a known date — total rate = margin + base. */}
        <div className="grid grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Bank margin (% p.a.)</label>
            <input
              type="number" step="0.01" min="0" placeholder="1.30"
              {...register('loan_margin', { valueAsNumber: true, min: { value: 0, message: 'Must be ≥ 0' } })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">EURIBOR / base (% p.a.)</label>
            <input
              type="number" step="0.01" min="0" placeholder="2.10"
              {...register('loan_base_rate', { valueAsNumber: true, min: { value: 0, message: 'Must be ≥ 0' } })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Next rate reset</label>
            <Controller
              name="loan_rate_reset_date"
              control={control}
              render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
            />
          </div>
        </div>
        <div className="grid grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Monthly payment (€)</label>
            <input
              type="number" step="0.01" min="0"
              {...register('loan_monthly_payment', { valueAsNumber: true, min: { value: 0, message: 'Must be ≥ 0' } })}
              className={inputCls}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Paid from account</label>
            <select {...register('loan_account')} className={inputCls}>
              <option value="">—</option>
              {Object.entries(ACCOUNT_LABELS).map(([key, label]) => (
                <option key={key} value={key}>{label}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Paid off on</label>
            <Controller
              name="loan_paid_off_date"
              control={control}
              render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Payments label</label>
            <input
              type="text" placeholder="e.g. loan" list="asset-loan-labels" autoComplete="off"
              {...register('loan_label', { setValueAs: (v) => String(v ?? '').toLowerCase().trim() })}
              className={inputCls}
            />
            <p className="text-[11px] text-gray-400 mt-1">
              Transactions carrying this label count as this loan's payments — the actual amounts drive the
              balance estimate and projection.
            </p>
            <datalist id="asset-loan-labels">
              {allLabels.map((l) => <option key={l} value={l} />)}
            </datalist>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Rate note (free text)</label>
            <input type="text" placeholder="e.g. 6M EURIBOR + 1.3%" {...register('loan_rate')} className={inputCls} />
          </div>
        </div>
      </fieldset>

      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Notes</label>
        <textarea
          rows={2}
          {...register('notes')}
          placeholder="Optional"
          className={inputCls}
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
          {isSubmitting ? 'Saving...' : 'Save Asset'}
        </button>
      </div>
    </form>
  )
}
