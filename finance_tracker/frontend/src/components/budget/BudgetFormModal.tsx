import { useState } from 'react'
import { useLabels } from '../../hooks/useBudgets'
import { CATEGORIES } from '../../constants/categories'
import type { Budget, BudgetInput, BudgetKind, BudgetPeriod } from '../../types'
import { formatEuro } from '../../utils/format'
import { defaultFundStart, ym } from './budgetUi'

interface Props {
  budget: Budget | null
  prefill: BudgetInput | null
  error: string | null
  onSave: (input: BudgetInput) => void
  onClose: () => void
}

export default function BudgetFormModal({ budget, prefill, error, onSave, onClose }: Props) {
  const initial: BudgetInput = budget
    ? {
        name: budget.name, kind: budget.kind, label: budget.label, category: budget.category, amount: budget.amount,
        period: budget.period || 'monthly', fund: budget.fund, start_month: budget.start_month,
      }
    : prefill ?? { name: '', kind: 'spending', category: '', label: '', amount: 0, period: 'monthly', fund: false, start_month: '' }

  const [form, setForm] = useState<BudgetInput>(initial)
  // When an existing line's amount changes: from which month (history is
  // kept), or a correction for every month.
  const [amountFrom, setAmountFrom] = useState(ym(new Date()))
  const [correctAll, setCorrectAll] = useState(false)
  // Existing labels for autocomplete — a typo here ("lease" vs "leasing")
  // silently creates a budget that never matches anything.
  const { data: allLabels = [] } = useLabels()

  const isSpending = form.kind === 'spending'
  const period: BudgetPeriod = form.period ?? 'monthly'
  const amountChanged = budget != null && form.amount !== budget.amount
  const valid = form.name.trim() !== '' && form.amount > 0 &&
    ((form.label ?? '').trim() !== '' || (form.category ?? '') !== '')

  function submit() {
    const out: BudgetInput = { ...form }
    if (!isSpending) {
      out.fund = false
      out.period = 'monthly'
    }
    if (out.fund && !out.start_month) out.start_month = defaultFundStart()
    if (amountChanged) out.amount_from = correctAll ? 'all' : amountFrom
    onSave(out)
  }

  const amountPlaceholder = period === 'yearly' ? 'Yearly amount (€)' : form.fund ? 'Set aside per month (€)' : 'Monthly amount (€)'

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8" onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-md mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-gray-900 mb-3">{budget ? 'Edit Budget' : 'New Budget'}</h3>
        {error && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{error}</p>}
        <div className="space-y-3">
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder="Name (e.g. Food)"
            className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
          />
          <div className="grid grid-cols-3 gap-1 bg-gray-100 rounded-lg p-1">
            {(['spending', 'fixed', 'investment'] as BudgetKind[]).map((k) => (
              <button
                key={k}
                onClick={() => setForm({ ...form, kind: k })}
                className={`px-2 py-1.5 text-xs font-medium rounded-md capitalize ${form.kind === k ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
              >
                {k}
              </button>
            ))}
          </div>

          {isSpending && (
            <div className="rounded-lg border border-gray-200 p-3 space-y-2">
              <div className="grid grid-cols-2 gap-1 bg-gray-100 rounded-lg p-1">
                {(['monthly', 'yearly'] as BudgetPeriod[]).map((p) => (
                  <button
                    key={p}
                    onClick={() => setForm({ ...form, period: p })}
                    className={`px-2 py-1.5 text-xs font-medium rounded-md capitalize ${period === p ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
                  >
                    {p}
                  </button>
                ))}
              </div>
              <label className="flex items-start gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.fund ?? false}
                  onChange={(e) => setForm({ ...form, fund: e.target.checked, start_month: e.target.checked ? form.start_month || defaultFundStart() : form.start_month })}
                  className="mt-0.5"
                />
                <span>
                  <span className="block font-medium text-gray-800">Fund — carry unspent money over</span>
                  <span className="block text-[11px] text-gray-400">
                    For costs that come in bursts (holidays, health, gifts): the monthly share builds up, spending draws it down.
                  </span>
                </span>
              </label>
              {form.fund && (
                <label className="flex items-center justify-between gap-3 text-xs text-gray-500">
                  Accrues from
                  <input
                    type="month"
                    value={form.start_month || defaultFundStart()}
                    onChange={(e) => setForm({ ...form, start_month: e.target.value })}
                    className="border border-gray-300 rounded-lg px-2 py-1.5 text-sm"
                  />
                </label>
              )}
            </div>
          )}

          {/* Rule-style matching: category, labels, or both (both = the
              transaction must be in the category AND carry a label). */}
          <select
            value={form.category ?? ''}
            onChange={(e) => setForm({ ...form, category: e.target.value })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm bg-white"
          >
            <option value="">Any category</option>
            {CATEGORIES.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          <input
            type="text"
            value={form.label ?? ''}
            onChange={(e) => setForm({ ...form, label: e.target.value.toLowerCase() })}
            placeholder="Label(s) — e.g. restaurant, fast food (optional)"
            list="budget-label-options"
            autoComplete="off"
            className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
          />
          <datalist id="budget-label-options">
            {allLabels.map((l) => (
              <option key={l} value={l} />
            ))}
          </datalist>
          <p className="text-[11px] text-gray-400 -mt-1">
            Comma-separate for a label group (any of them counts). Set a category too and only
            transactions in that category carrying one of the labels count.
          </p>
          {(() => {
            const unknown = (form.label ?? '')
              .split(',')
              .map((l) => l.trim())
              .filter((l) => l !== '' && !allLabels.includes(l))
            return unknown.length > 0 ? (
              <p className="text-[11px] text-amber-600 -mt-1">
                “{unknown.join('”, “')}” do{unknown.length === 1 ? 'es' : ''}n't match any existing label yet — that part of the budget will stay at €0 until transactions carry it.
              </p>
            ) : null
          })()}
          <input
            type="number"
            inputMode="decimal"
            value={form.amount || ''}
            onChange={(e) => setForm({ ...form, amount: parseFloat(e.target.value) || 0 })}
            placeholder={amountPlaceholder}
            aria-label={amountPlaceholder}
            className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
          />
          {isSpending && form.amount > 0 && period === 'yearly' && (
            <p className="text-[11px] text-gray-400 -mt-1">= {formatEuro(form.amount / 12)} per month in the plan</p>
          )}

          {amountChanged && (
            <div className="rounded-lg bg-gray-50 p-3 space-y-2 text-sm">
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="radio" checked={!correctAll} onChange={() => setCorrectAll(false)} />
                <span className="text-gray-700">New amount from</span>
                <input
                  type="month"
                  value={amountFrom}
                  onChange={(e) => setAmountFrom(e.target.value)}
                  disabled={correctAll}
                  className="border border-gray-300 rounded-lg px-2 py-1 text-sm disabled:opacity-50"
                />
              </label>
              <p className="text-[11px] text-gray-400 pl-6 -mt-1">Earlier months keep {formatEuro(budget!.amount)}.</p>
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="radio" checked={correctAll} onChange={() => setCorrectAll(true)} />
                <span className="text-gray-700">Correct every month (the old amount was a mistake)</span>
              </label>
            </div>
          )}

          <div className="flex gap-2 pt-1">
            <button
              disabled={!valid}
              onClick={submit}
              className="flex-1 px-4 py-2.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40"
            >
              {budget ? 'Save' : 'Create'}
            </button>
            <button onClick={onClose} className="px-4 py-2.5 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200">
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
