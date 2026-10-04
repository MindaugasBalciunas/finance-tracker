import { useMemo, useState } from 'react'
import { useForm, Controller } from 'react-hook-form'
import type { AccountGroup, CreateBalanceInput } from '../../types'
import { ACCOUNT_GROUP_LABELS } from '../../types'
import { useBtcPrice } from '../../hooks/useBtcPrice'
import { formatEuro } from '../../utils/format'
import { GROUP_COLORS } from '../../utils/balanceGroups'
import { useBankSections, type SnapshotField } from '../../utils/accountLayout'
import DateInput from '../ui/DateInput'

interface Props {
  onSubmit: (data: CreateBalanceInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateBalanceInput>
}

const BUILTIN_EUR = ['seb', 'swed', 'swed_etf', 'seb_pen', 'luminor', 'art', 'cash', 'rev_m', 'rev_r', 'rev_stocks', 'ibkr_stocks'] as const

// Every account an existing snapshot holds, as editable text.
function initialValues(d?: Partial<CreateBalanceInput>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const k of [...BUILTIN_EUR, 'r_btc', 'm_btc'] as const) {
    const v = d?.[k]
    if (v != null && v !== 0) out[k] = String(v)
  }
  for (const [k, v] of Object.entries(d?.extra ?? {})) if (v) out[k] = String(v)
  return out
}

const num = (s: string | undefined) => {
  const v = parseFloat((s ?? '').replace(',', '.'))
  return isNaN(v) ? 0 : v
}

// A balance snapshot, grouped by bank. Each field carries its type's color
// (the same dots as the net-worth cards), each bank a live subtotal. Archived
// accounts and a closed, empty Luminor are not shown — their last value is
// carried forward untouched.
export default function BalanceForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const { data: btcPrice } = useBtcPrice()
  const sections = useBankSections()
  const [values, setValues] = useState<Record<string, string>>(() => initialValues(defaultValues))

  const today = new Date().toISOString().slice(0, 10)
  const { handleSubmit, control, formState: { errors } } = useForm<CreateBalanceInput>({
    defaultValues: { date: today, ...defaultValues },
  })

  const hidden = (f: SnapshotField) => f.archived || (f.key === 'luminor' && !num(values.luminor))
  const visible = useMemo(
    () => sections.map((s) => ({ ...s, fields: s.fields.filter((f) => !hidden(f)) })).filter((s) => s.fields.length),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sections],
  )
  const eur = (f: SnapshotField) => (f.kind === 'btc' ? num(values[f.key]) * (btcPrice ?? 0) : num(values[f.key]))
  const total = sections.flatMap((s) => s.fields).reduce((sum, f) => sum + eur(f), 0)
  const present = new Set(visible.flatMap((s) => s.fields.map((f) => f.group)))
  const types = (['cash', 'investments', 'pensions', 'crypto', 'other'] as AccountGroup[]).filter((g) => present.has(g))

  const submit = (data: CreateBalanceInput) => {
    const out: CreateBalanceInput = { date: data.date }
    for (const k of BUILTIN_EUR) (out as any)[k] = num(values[k])
    const extra: Record<string, number> = {}
    for (const f of sections.flatMap((s) => s.fields)) if (!f.builtin) extra[f.key] = num(values[f.key])
    out.extra = extra
    out.r_btc = num(values.r_btc)
    out.m_btc = num(values.m_btc)
    out.btc_price = btcPrice ?? 0
    onSubmit(out)
  }

  return (
    <form onSubmit={handleSubmit(submit)} className="space-y-4">
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

      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500">
        {types.map((g) => (
          <span key={g} className="flex items-center gap-1.5">
            <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: GROUP_COLORS[g] }} />
            {ACCOUNT_GROUP_LABELS[g]}
          </span>
        ))}
      </div>

      <div className="space-y-3 max-h-[55vh] overflow-y-auto pr-1 -mr-1">
        {visible.map((s) => {
          const subtotal = s.fields.reduce((sum, f) => sum + eur(f), 0)
          return (
            <fieldset key={s.institution || 'other'} className="border border-gray-200 rounded-lg p-3">
              <legend className="w-full flex items-baseline justify-between px-1 -mx-1">
                <span className="text-sm font-semibold text-gray-800">{s.institution || 'Other'}</span>
                <span className="text-xs text-gray-500">{formatEuro(subtotal)}</span>
              </legend>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-3 gap-y-2 mt-1">
                {s.fields.map((f) => (
                  <label key={f.key} className="block">
                    <span className="flex items-center gap-1.5 text-xs font-medium text-gray-600 mb-0.5">
                      <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: GROUP_COLORS[f.group] }} aria-hidden />
                      <span className="truncate">{f.label}</span>
                      <span className="text-gray-400 font-normal">{f.kind === 'btc' ? '₿' : '€'}</span>
                    </span>
                    <input
                      type="number"
                      inputMode="decimal"
                      step={f.kind === 'btc' ? '0.00000001' : '0.01'}
                      value={values[f.key] ?? ''}
                      onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                      placeholder={f.kind === 'btc' ? '0.00000000' : '0.00'}
                      aria-label={`${s.institution} ${f.label}`}
                      className="w-full border border-gray-300 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    />
                    {f.kind === 'btc' && btcPrice != null && num(values[f.key]) > 0 && (
                      <span className="block text-xs text-gray-400 mt-0.5">≈ {formatEuro(eur(f))}</span>
                    )}
                  </label>
                ))}
              </div>
            </fieldset>
          )
        })}
      </div>

      <div className="flex items-baseline justify-between border-t border-gray-100 pt-3">
        <span className="text-sm text-gray-600">Total</span>
        <span className="text-base font-semibold text-gray-900">{formatEuro(total)}</span>
      </div>
      {btcPrice != null && (
        <p className="text-xs text-gray-400 -mt-2">BTC at the live price €{btcPrice.toLocaleString()} /BTC</p>
      )}

      <div className="flex justify-end gap-3 pt-1">
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
