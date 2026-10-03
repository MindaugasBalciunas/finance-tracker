import { useMemo, useState } from 'react'
import { useAllTransactions, useTransactionSummary } from '../hooks/useTransactions'
import { useBudgets, useCreateBudget, useUpdateBudget, useDeleteBudget, useApplyLabel, useReapplyRules, useBudgetSettings, useSaveBudgetSettings, useBudgetStatus } from '../hooks/useBudgets'
import { medianMonthlyIncome } from '../utils/budget'
import { ltNetSalary } from '../utils/ltSalary'
import LoadingSpinner from '../components/ui/LoadingSpinner'
import CategoryTransactionsModal from '../components/ui/CategoryTransactionsModal'
import LabelRulesModal from '../components/ui/LabelRulesModal'
import MonthView from '../components/budget/MonthView'
import YearGrid from '../components/budget/YearGrid'
import TripsView from '../components/budget/TripsView'
import BudgetFormModal from '../components/budget/BudgetFormModal'
import { monthLabel, monthRange, shiftMonth, ym } from '../components/budget/budgetUi'
import { formatEuro } from '../utils/format'
import type { Budget, BudgetInput, BudgetLineStatus, BudgetSettings, IncomeMode } from '../types'

// Suggested setup matching this database: loan + alimony as labeled fixed
// costs, VWCE and Artea as investment targets.
const SUGGESTED = [
  { budget: { name: 'Loan payments', kind: 'fixed', label: 'loan', amount: 1285 }, rule: { label: 'loan', category: 'Finance', comment_match: 'loan' } },
  { budget: { name: 'Alimony', kind: 'fixed', label: 'alimony', amount: 1000 }, rule: { label: 'alimony', category: 'Kids', comment_match: 'alim' } },
  { budget: { name: 'VWCE / ETF', kind: 'investment', category: 'Stocks & ETF', amount: 1000 }, rule: null },
  { budget: { name: 'Artea 3rd pillar', kind: 'investment', category: 'Pension', amount: 200 }, rule: null },
] as const

type Tab = 'month' | 'year' | 'trips'
const TABS: { id: Tab; label: string }[] = [
  { id: 'month', label: 'Month' },
  { id: 'year', label: '12 months' },
  { id: 'trips', label: 'Trips' },
]

export default function Budget() {
  const [month, setMonth] = useState(() => ym(new Date()))
  const [tab, setTab] = useState<Tab>('month')
  const [rulesOpen, setRulesOpen] = useState(false)
  const range = monthRange(month)

  const { data: budgets, isLoading: budgetsLoading } = useBudgets()
  const { data: report } = useBudgetStatus(month)
  const { data: monthTxs } = useAllTransactions(range)
  const { data: allTimeSummary } = useTransactionSummary({})
  const { data: settings } = useBudgetSettings()
  const createMutation = useCreateBudget()
  const updateMutation = useUpdateBudget()
  const deleteMutation = useDeleteBudget()
  const applyLabel = useApplyLabel()
  const reapplyRules = useReapplyRules()

  const [editing, setEditing] = useState<Budget | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [settingUp, setSettingUp] = useState(false)
  const [prefill, setPrefill] = useState<BudgetInput | null>(null)
  const [viewing, setViewing] = useState<{ title: string; type: 'expense' | 'investment'; category?: string; label?: string } | null>(null)
  const [showIncomeSettings, setShowIncomeSettings] = useState(false)

  const byId = useMemo(() => new Map((budgets ?? []).map((b) => [b.id, b])), [budgets])

  function viewLine(l: BudgetLineStatus) {
    setViewing({
      title: l.name,
      type: l.kind === 'investment' ? 'investment' : 'expense',
      category: l.label ? undefined : l.category,
      label: l.label || undefined,
    })
  }

  function editLine(l: BudgetLineStatus) {
    const b = byId.get(l.id)
    if (!b) return
    setEditing(b)
    setPrefill(null)
    setShowForm(true)
    setFormError(null)
  }

  async function handleReapply() {
    setNotice(null)
    setFormError(null)
    try {
      const res = await reapplyRules.mutateAsync()
      setNotice(`Rules re-applied: ${res.relabeled} transaction${res.relabeled === 1 ? '' : 's'} newly labeled (${res.rules} rule${res.rules === 1 ? '' : 's'}).`)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  // One-tap suggestion: update the line (or create one for an unbudgeted
  // category) and say what changed.
  async function applySuggestion(l: BudgetLineStatus | null, input: BudgetInput, note: string) {
    setNotice(null)
    setFormError(null)
    try {
      if (l) await updateMutation.mutateAsync({ id: l.id, input })
      else await createMutation.mutateAsync(input)
      setNotice(note)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  const medianBase = useMemo(
    () => medianMonthlyIncome(allTimeSummary?.by_month ?? [], new Date()),
    [allTimeSummary]
  )

  const incomeSourceNote = useMemo(() => {
    if (settings?.income_mode === 'manual' && settings.manual_income > 0) return 'projected (manual)'
    if (settings?.income_mode === 'gross' && settings.gross_salary > 0) return `from ${formatEuro(settings.gross_salary)} gross − LT tax`
    return 'median month'
  }, [settings])

  const isCurrentMonth = month === ym(new Date())

  // Daily allowance for the rest of the current month.
  const daysLeft = (() => {
    if (!isCurrentMonth) return null
    const now = new Date()
    const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate()
    return daysInMonth - now.getDate() + 1
  })()

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
      setPrefill(null)
    } catch (err) {
      setFormError((err as Error).message)
    }
  }

  async function handleDelete(l: BudgetLineStatus) {
    if (confirm(`Delete the ${l.name} budget?`)) {
      await deleteMutation.mutateAsync(l.id)
    }
  }

  if (budgetsLoading) return <LoadingSpinner />

  const hasBudgets = (budgets ?? []).some((b) => b.kind !== 'trip')
  const vacationFund = report?.lines.find((l) => l.fund && (l.category === 'Vacation' || (l.label ?? '').split(',').includes('vacation')))

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Budget</h2>
          <p className="text-sm text-gray-500 mt-1">Monthly limits, yearly amounts and funds that carry over</p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <button
            onClick={() => setRulesOpen(true)}
            title="View and delete label rules"
            className="px-2.5 py-1.5 text-xs text-gray-500 hover:text-gray-800 font-medium rounded-md hover:bg-gray-100 whitespace-nowrap"
          >
            ⚡ Rules
          </button>
          {hasBudgets && (
            <button
              onClick={handleReapply}
              disabled={reapplyRules.isPending}
              title="Run all label rules over every transaction"
              className="px-2.5 py-1.5 text-xs text-gray-500 hover:text-gray-800 font-medium rounded-md hover:bg-gray-100 disabled:opacity-50 whitespace-nowrap"
            >
              {reapplyRules.isPending ? 'Re-labeling…' : '↻ Re-apply rules'}
            </button>
          )}
          {tab !== 'trips' && (
            <div className="flex items-center gap-1 bg-gray-100 rounded-lg p-1">
              <button onClick={() => setMonth(shiftMonth(month, -1))} aria-label="Previous month" className="px-2.5 py-1.5 text-sm text-gray-600 hover:text-gray-900 rounded-md">‹</button>
              <span className="px-2 text-sm font-medium text-gray-800 whitespace-nowrap">{monthLabel(month)}</span>
              <button
                onClick={() => setMonth(shiftMonth(month, 1))}
                disabled={isCurrentMonth}
                aria-label="Next month"
                className="px-2.5 py-1.5 text-sm text-gray-600 hover:text-gray-900 rounded-md disabled:opacity-30"
              >
                ›
              </button>
            </div>
          )}
        </div>
      </div>

      {hasBudgets && (
        <div className="grid grid-cols-3 gap-1 bg-gray-100 rounded-lg p-1 max-w-sm" role="tablist">
          {TABS.map((t) => (
            <button
              key={t.id}
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => setTab(t.id)}
              className={`px-3 py-1.5 text-sm font-medium rounded-md ${tab === t.id ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-800'}`}
            >
              {t.label}
            </button>
          ))}
        </div>
      )}

      {formError && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{formError}</p>
      )}
      {notice && (
        <p className="text-sm text-green-700 bg-green-50 border border-green-200 rounded-lg px-3 py-2">{notice}</p>
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
      ) : tab === 'trips' ? (
        <TripsView vacationFund={vacationFund} />
      ) : !report ? (
        <LoadingSpinner />
      ) : tab === 'year' ? (
        <YearGrid report={report} />
      ) : (
        <MonthView
          report={report}
          month={month}
          isCurrentMonth={isCurrentMonth}
          daysLeft={daysLeft}
          monthTxs={monthTxs?.data ?? []}
          incomeSourceNote={incomeSourceNote}
          onEditIncome={() => setShowIncomeSettings(true)}
          onView={viewLine}
          onViewCategory={(category) => setViewing({ title: category, type: 'expense', category })}
          onEdit={editLine}
          onDelete={handleDelete}
          onAdd={(pf) => { setEditing(null); setPrefill(pf); setShowForm(true); setFormError(null) }}
          onApply={applySuggestion}
          applying={createMutation.isPending || updateMutation.isPending}
        />
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

      {viewing && (
        <CategoryTransactionsModal
          title={`${viewing.title} — ${monthLabel(month)}`}
          category={viewing.category as never}
          label={viewing.label}
          type={viewing.type}
          dateRange={range}
          onClose={() => setViewing(null)}
        />
      )}

      {showIncomeSettings && (
        <IncomeSettingsModal
          settings={settings ?? null}
          medianBase={medianBase}
          onClose={() => setShowIncomeSettings(false)}
        />
      )}

      {rulesOpen && <LabelRulesModal onClose={() => setRulesOpen(false)} />}
    </div>
  )
}

function IncomeSettingsModal({ settings, medianBase, onClose }: {
  settings: BudgetSettings | null
  medianBase: number | null
  onClose: () => void
}) {
  const save = useSaveBudgetSettings()
  const [mode, setMode] = useState<IncomeMode>(settings?.income_mode ?? 'median')
  const [manual, setManual] = useState(settings?.manual_income || 0)
  const [gross, setGross] = useState(settings?.gross_salary || 0)
  const [deductions, setDeductions] = useState(settings?.monthly_deductions || 0)
  const [error, setError] = useState<string | null>(null)

  const breakdown = gross > 0 ? ltNetSalary(gross, deductions) : null

  const valid = mode === 'median' || (mode === 'manual' ? manual > 0 : gross > 0)

  async function handleSave() {
    setError(null)
    try {
      await save.mutateAsync({
        income_mode: mode,
        manual_income: manual,
        gross_salary: gross,
        monthly_deductions: deductions,
      })
      onClose()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 overflow-y-auto py-8" onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl p-6 w-full max-w-md mx-4" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-gray-900 mb-1">Income Base</h3>
        <p className="text-xs text-gray-400 mb-4">The monthly income "Safe to spend" is calculated from</p>
        {error && <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-3">{error}</p>}

        <div className="space-y-3">
          <label className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer ${mode === 'median' ? 'border-blue-400 bg-blue-50/50' : 'border-gray-200'}`}>
            <input type="radio" checked={mode === 'median'} onChange={() => setMode('median')} className="mt-0.5" />
            <span>
              <span className="block text-sm font-medium text-gray-800">Automatic — median month</span>
              <span className="block text-xs text-gray-400 mt-0.5">
                From your history{medianBase != null ? `: currently ${formatEuro(medianBase)}` : ''}. Includes all income types.
              </span>
            </span>
          </label>

          <label className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer ${mode === 'gross' ? 'border-blue-400 bg-blue-50/50' : 'border-gray-200'}`}>
            <input type="radio" checked={mode === 'gross'} onChange={() => setMode('gross')} className="mt-0.5" />
            <span className="flex-1">
              <span className="block text-sm font-medium text-gray-800">Projected salary — from gross (LT tax)</span>
              <span className="block text-xs text-gray-400 mt-0.5 mb-2">Bruto salary, net computed with 2026 LT employee taxes</span>
              {mode === 'gross' && (
                <span className="block space-y-2">
                  <input
                    type="number" inputMode="decimal"
                    value={gross || ''}
                    onChange={(e) => setGross(parseFloat(e.target.value) || 0)}
                    placeholder="Gross salary € (e.g. 8400)"
                    className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm"
                  />
                  <input
                    type="number" inputMode="decimal"
                    value={deductions || ''}
                    onChange={(e) => setDeductions(parseFloat(e.target.value) || 0)}
                    placeholder="Fixed monthly deductions € (e.g. 30 parking)"
                    className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm"
                  />
                  {breakdown && (
                    <span className="block text-xs text-gray-500 bg-gray-50 rounded-lg p-2.5 space-y-0.5">
                      <span className="flex justify-between"><span>Sodra (19.5%)</span><span>−{formatEuro(breakdown.sodra)}</span></span>
                      <span className="flex justify-between"><span>GPM (20%{breakdown.npd > 0 ? `, NPD ${formatEuro(breakdown.npd)}` : ''})</span><span>−{formatEuro(breakdown.gpm)}</span></span>
                      {deductions > 0 && <span className="flex justify-between"><span>Deductions</span><span>−{formatEuro(deductions)}</span></span>}
                      <span className="flex justify-between font-semibold text-gray-800 border-t border-gray-200 pt-1 mt-1">
                        <span>Net income base</span><span>{formatEuro(breakdown.netAfterDeductions)}</span>
                      </span>
                    </span>
                  )}
                </span>
              )}
            </span>
          </label>

          <label className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer ${mode === 'manual' ? 'border-blue-400 bg-blue-50/50' : 'border-gray-200'}`}>
            <input type="radio" checked={mode === 'manual'} onChange={() => setMode('manual')} className="mt-0.5" />
            <span className="flex-1">
              <span className="block text-sm font-medium text-gray-800">Manual — fixed net amount</span>
              {mode === 'manual' && (
                <input
                  type="number" inputMode="decimal"
                  value={manual || ''}
                  onChange={(e) => setManual(parseFloat(e.target.value) || 0)}
                  placeholder="Net income € / month"
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm mt-2"
                />
              )}
            </span>
          </label>

          <div className="flex gap-2 pt-1">
            <button
              disabled={!valid || save.isPending}
              onClick={handleSave}
              className="flex-1 px-4 py-2.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40"
            >
              {save.isPending ? 'Saving…' : 'Save'}
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
