import { useMemo, useState } from 'react'
import { useAllTransactions, useTransactionSummary } from '../hooks/useTransactions'
import { useBudgets, useCreateBudget, useUpdateBudget, useDeleteBudget, useApplyLabel } from '../hooks/useBudgets'
import { computeMonthPlan, medianMonthlyIncome } from '../utils/budget'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import { formatEuro } from '../utils/format'
import { CATEGORIES } from '../constants/categories'
import type { Budget, BudgetInput, BudgetKind } from '../types'

function ym(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
}

function monthRange(month: string): { date_from: string; date_to: string } {
  const [y, m] = month.split('-').map(Number)
  const last = new Date(y, m, 0).getDate()
  return { date_from: `${month}-01`, date_to: `${month}-${String(last).padStart(2, '0')}` }
}

function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(y, m - 1).toLocaleDateString('en', { month: 'long', year: 'numeric' })
}

function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  return ym(new Date(y, m - 1 + delta))
}

const KIND_INFO: Record<BudgetKind, { title: string; hint: string }> = {
  fixed: { title: 'Fixed obligations', hint: 'Known monthly payments — tracked as paid / pending' },
  investment: { title: 'Investment targets', hint: 'Monthly contributions you aim to reach' },
  spending: { title: 'Spending limits', hint: 'Caps for discretionary categories' },
}

// Suggested setup matching this database: loan + alimony as labeled fixed
// costs, VWCE and Artea as investment targets.
const SUGGESTED = [
  { budget: { name: 'Loan payments', kind: 'fixed', label: 'loan', amount: 1285 }, rule: { label: 'loan', category: 'Finance', comment_match: 'loan' } },
  { budget: { name: 'Alimony', kind: 'fixed', label: 'alimony', amount: 1000 }, rule: { label: 'alimony', category: 'Kids - General', comment_match: 'alim' } },
  { budget: { name: 'VWCE / ETF', kind: 'investment', category: 'Stocks & ETF', amount: 1000 }, rule: null },
  { budget: { name: 'Artea 3rd pillar', kind: 'investment', category: 'Pension', amount: 200 }, rule: null },
] as const

export default function Budget() {
  const [month, setMonth] = useState(() => ym(new Date()))
  const range = monthRange(month)

  const { data: budgets, isLoading: budgetsLoading } = useBudgets()
  const { data: monthTxs } = useAllTransactions(range)
  const { data: allTimeSummary } = useTransactionSummary({})
  const createMutation = useCreateBudget()
  const updateMutation = useUpdateBudget()
  const deleteMutation = useDeleteBudget()
  const applyLabel = useApplyLabel()

  const [editing, setEditing] = useState<Budget | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [settingUp, setSettingUp] = useState(false)
  const [prefill, setPrefill] = useState<BudgetInput | null>(null)

  const incomeBase = useMemo(
    () => medianMonthlyIncome(allTimeSummary?.by_month ?? [], new Date()),
    [allTimeSummary]
  )

  const plan = useMemo(
    () => computeMonthPlan(budgets ?? [], monthTxs?.data ?? [], incomeBase),
    [budgets, monthTxs, incomeBase]
  )

  const isCurrentMonth = month === ym(new Date())

  async function setupSuggested() {
    setSettingUp(true)
    try {
      for (const s of SUGGESTED) {
        if (s.rule) {
          await applyLabel.mutateAsync({ ...s.rule, create_rule: true })
        }
        await createMutation.mutateAsync(s.budget as BudgetInput)
      }
    } catch (err) {
      setFormError((err as Error).message)
    } finally {
      setSettingUp(false)
    }
  }

  async function handleSave(input: BudgetInput) {
    try {
      if (editing) {
        await updateMutation.mutateAsync({ id: editing.id, input })
      } else {
        await createMutation.mutateAsync(input)
      }
      setEditing(null)
      setShowForm(false)
      setFormError(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  async function handleDelete(id: number) {
    if (confirm('Delete this budget?')) {
      await deleteMutation.mutateAsync(id)
    }
  }

  if (budgetsLoading) return <LoadingSpinner />

  const hasBudgets = (budgets ?? []).length > 0

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Budget</h2>
          <p className="text-sm text-gray-500 mt-1">Fixed costs, investment targets and spending limits</p>
        </div>
        <div className="flex items-center gap-1 bg-gray-100 rounded-lg p-1">
          <button onClick={() => setMonth(shiftMonth(month, -1))} className="px-2.5 py-1.5 text-sm text-gray-600 hover:text-gray-900 rounded-md">‹</button>
          <span className="px-2 text-sm font-medium text-gray-800 whitespace-nowrap">{monthLabel(month)}</span>
          <button
            onClick={() => setMonth(shiftMonth(month, 1))}
            disabled={isCurrentMonth}
            className="px-2.5 py-1.5 text-sm text-gray-600 hover:text-gray-900 rounded-md disabled:opacity-30"
          >
            ›
          </button>
        </div>
      </div>

      {formError && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{formError}</p>
      )}

      {!hasBudgets ? (
        <div className="bg-white rounded-xl border border-dashed border-gray-300 p-10 text-center space-y-4">
          <p className="text-gray-600 font-medium">No budgets yet</p>
          <p className="text-sm text-gray-400 max-w-md mx-auto">
            Set up the suggested plan — loan payments and alimony as fixed costs (auto-labeled from your
            history), €1,000/mo VWCE and €200/mo Artea as investment targets — then add spending limits per category.
          </p>
          <div className="flex justify-center gap-3">
            <button
              onClick={setupSuggested}
              disabled={settingUp}
              className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
            >
              {settingUp ? 'Setting up…' : 'Set up suggested budgets'}
            </button>
            <button
              onClick={() => { setShowForm(true); setFormError(null) }}
              className="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
            >
              Add manually
            </button>
          </div>
        </div>
      ) : (
        <>
          {/* Safe to spend */}
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <p className="text-sm font-medium text-gray-500">Safe to spend {isCurrentMonth ? 'this month' : `in ${monthLabel(month)}`}</p>
            <p className={`text-3xl sm:text-4xl font-bold mt-1 ${plan.safeToSpend != null && plan.safeToSpend >= 0 ? 'text-green-600' : 'text-red-600'}`}>
              {plan.safeToSpend != null ? formatEuro(plan.safeToSpend) : '—'}
            </p>
            <div className="flex flex-wrap gap-x-5 gap-y-1 mt-3 text-xs text-gray-500">
              <span>Income base <span className="font-semibold text-gray-700">{plan.incomeBase != null ? formatEuro(plan.incomeBase) : '—'}</span> <span className="text-gray-400">(median month)</span></span>
              <span>− Fixed <span className="font-semibold text-gray-700">{formatEuro(plan.fixedPlanned)}</span></span>
              <span>− Investments <span className="font-semibold text-gray-700">{formatEuro(plan.investmentPlanned)}</span></span>
              <span>− Spent so far <span className="font-semibold text-gray-700">{formatEuro(plan.discretionarySpent)}</span></span>
            </div>
          </div>

          {/* Fixed obligations */}
          {plan.fixed.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
              <h3 className="text-base font-semibold text-gray-900 mb-1">{KIND_INFO.fixed.title}</h3>
              <p className="text-xs text-gray-400 mb-3">{KIND_INFO.fixed.hint}</p>
              <div className="space-y-2">
                {plan.fixed.map(({ budget, actual }) => {
                  const paid = actual >= budget.amount * 0.95
                  return (
                    <div key={budget.id} className="flex items-center justify-between gap-3 py-1.5">
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-gray-800">{budget.name}</p>
                        <p className="text-xs text-gray-400">
                          {budget.label ? `label: ${budget.label}` : budget.category} · planned {formatEuro(budget.amount)}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-semibold ${paid ? 'text-green-600' : 'text-gray-500'}`}>
                          {paid ? `✓ Paid ${formatEuro(actual)}` : actual > 0 ? `${formatEuro(actual)} of ${formatEuro(budget.amount)}` : 'Pending'}
                        </span>
                        <BudgetRowActions onEdit={() => { setEditing(budget); setShowForm(true); setFormError(null) }} onDelete={() => handleDelete(budget.id)} />
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* Investment targets */}
          {plan.investments.length > 0 && (
            <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
              <h3 className="text-base font-semibold text-gray-900 mb-1">{KIND_INFO.investment.title}</h3>
              <p className="text-xs text-gray-400 mb-3">{KIND_INFO.investment.hint}</p>
              <div className="space-y-3">
                {plan.investments.map(({ budget, actual }) => {
                  const pct = Math.min((actual / budget.amount) * 100, 100)
                  const reached = actual >= budget.amount
                  return (
                    <div key={budget.id}>
                      <div className="flex items-center justify-between gap-3 mb-1">
                        <p className="text-sm font-medium text-gray-800">{budget.name}</p>
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-semibold ${reached ? 'text-green-600' : 'text-blue-700'}`}>
                            {formatEuro(actual)} / {formatEuro(budget.amount)}
                          </span>
                          <BudgetRowActions onEdit={() => { setEditing(budget); setShowForm(true); setFormError(null) }} onDelete={() => handleDelete(budget.id)} />
                        </div>
                      </div>
                      <div className="h-2.5 bg-gray-100 rounded-full overflow-hidden">
                        <div className={`h-full rounded-full ${reached ? 'bg-green-500' : 'bg-blue-500'}`} style={{ width: `${pct}%` }} />
                      </div>
                      <p className="text-xs text-gray-400 mt-1">
                        {reached ? '✓ Target reached' : `${formatEuro(budget.amount - actual)} to go`}
                      </p>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* Spending limits */}
          <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
            <div className="flex items-center justify-between mb-1">
              <h3 className="text-base font-semibold text-gray-900">{KIND_INFO.spending.title}</h3>
              <button
                onClick={() => { setEditing(null); setShowForm(true); setFormError(null) }}
                className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
              >
                + Add budget
              </button>
            </div>
            <p className="text-xs text-gray-400 mb-3">{KIND_INFO.spending.hint}</p>
            {plan.spending.length === 0 && (
              <p className="text-sm text-gray-400 py-2">No spending limits yet — add one, or pick a category below.</p>
            )}
            <div className="space-y-3">
              {plan.spending.map(({ budget, actual }) => {
                const pct = Math.min((actual / budget.amount) * 100, 100)
                const over = actual > budget.amount
                const near = !over && actual > budget.amount * 0.8
                return (
                  <div key={budget.id}>
                    <div className="flex items-center justify-between gap-3 mb-1">
                      <p className="text-sm font-medium text-gray-800">{budget.name}</p>
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-semibold ${over ? 'text-red-600' : near ? 'text-yellow-600' : 'text-gray-700'}`}>
                          {formatEuro(actual)} / {formatEuro(budget.amount)}
                        </span>
                        <BudgetRowActions onEdit={() => { setEditing(budget); setShowForm(true); setFormError(null) }} onDelete={() => handleDelete(budget.id)} />
                      </div>
                    </div>
                    <div className="h-2.5 bg-gray-100 rounded-full overflow-hidden">
                      <div className={`h-full rounded-full ${over ? 'bg-red-500' : near ? 'bg-yellow-400' : 'bg-green-500'}`} style={{ width: `${pct}%` }} />
                    </div>
                    {over && <p className="text-xs text-red-500 mt-1">{formatEuro(actual - budget.amount)} over budget</p>}
                  </div>
                )
              })}
            </div>

            {/* Unbudgeted categories */}
            {plan.unbudgeted.length > 0 && (
              <div className="mt-4 pt-4 border-t border-gray-100">
                <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Unbudgeted spending this month</p>
                <div className="space-y-1">
                  {plan.unbudgeted.map(({ category, spent }) => (
                    <div key={category} className="flex items-center justify-between text-sm py-1">
                      <span className="text-gray-600">{category}</span>
                      <div className="flex items-center gap-3">
                        <span className="font-medium text-gray-800">{formatEuro(spent)}</span>
                        <button
                          onClick={() => {
                            setEditing(null)
                            setShowForm(true)
                            setFormError(null)
                            setPrefill({ name: category, kind: 'spending', category, amount: Math.ceil(spent / 50) * 50 })
                          }}
                          className="text-xs text-blue-600 hover:text-blue-800"
                        >
                          + limit
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </>
      )}

      {showForm && (
        <BudgetFormModal
          budget={editing}
          prefill={prefill}
          error={formError}
          onSave={handleSave}
          onClose={() => { setShowForm(false); setEditing(null); setFormError(null); setPrefill(null) }}
        />
      )}
    </div>
  )
}

function BudgetRowActions({ onEdit, onDelete }: { onEdit: () => void; onDelete: () => void }) {
  return (
    <div className="flex items-center">
      <button onClick={onEdit} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
      <button onClick={onDelete} className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
    </div>
  )
}

function BudgetFormModal({ budget, prefill, error, onSave, onClose }: {
  budget: Budget | null
  prefill: BudgetInput | null
  error: string | null
  onSave: (input: BudgetInput) => void
  onClose: () => void
}) {
  const initial: BudgetInput = budget
    ? { name: budget.name, kind: budget.kind, label: budget.label, category: budget.category, amount: budget.amount }
    : prefill ?? { name: '', kind: 'spending', category: '', label: '', amount: 0 }

  const [form, setForm] = useState<BudgetInput>(initial)
  const [matcher, setMatcher] = useState<'category' | 'label'>(initial.label ? 'label' : 'category')

  const valid = form.name.trim() !== '' && form.amount > 0 &&
    (matcher === 'label' ? (form.label ?? '').trim() !== '' : (form.category ?? '') !== '')

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
          <div className="grid grid-cols-2 gap-1 bg-gray-100 rounded-lg p-1">
            <button
              onClick={() => setMatcher('category')}
              className={`px-2 py-1.5 text-xs font-medium rounded-md ${matcher === 'category' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
            >
              Match by category
            </button>
            <button
              onClick={() => setMatcher('label')}
              className={`px-2 py-1.5 text-xs font-medium rounded-md ${matcher === 'label' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500'}`}
            >
              Match by label
            </button>
          </div>
          {matcher === 'category' ? (
            <select
              value={form.category ?? ''}
              onChange={(e) => setForm({ ...form, category: e.target.value, label: '' })}
              className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm bg-white"
            >
              <option value="">Select category…</option>
              {CATEGORIES.map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          ) : (
            <input
              type="text"
              value={form.label ?? ''}
              onChange={(e) => setForm({ ...form, label: e.target.value.toLowerCase(), category: '' })}
              placeholder="Label (e.g. loan)"
              className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
            />
          )}
          <input
            type="number"
            inputMode="decimal"
            value={form.amount || ''}
            onChange={(e) => setForm({ ...form, amount: parseFloat(e.target.value) || 0 })}
            placeholder="Monthly amount (€)"
            className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
          />
          <div className="flex gap-2 pt-1">
            <button
              disabled={!valid}
              onClick={() => onSave(form)}
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
