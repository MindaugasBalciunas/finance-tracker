import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import type { BudgetInput, BudgetLineStatus, BudgetStatusReport, Transaction } from '../../types'
import { formatEuro } from '../../utils/format'
import SpendingLine, { RowActions } from './SpendingLine'
import { defaultFundStart, monthLabel } from './budgetUi'

interface Props {
  report: BudgetStatusReport
  month: string
  isCurrentMonth: boolean
  daysLeft: number | null
  monthTxs: Transaction[]
  incomeSourceNote: string
  onEditIncome: () => void
  onView: (l: BudgetLineStatus) => void
  onViewCategory: (category: string) => void
  onEdit: (l: BudgetLineStatus) => void
  onDelete: (l: BudgetLineStatus) => void
  onAdd: (prefill: BudgetInput | null) => void
  onApply: (l: BudgetLineStatus | null, input: BudgetInput, note: string) => void
  applying: boolean
}

// Stacked bar showing how the month's income base is allocated.
function AllocationBar({ incomeBase, fixed, investments, setAside, spent, remaining }: {
  incomeBase: number
  fixed: number
  investments: number
  setAside: number
  spent: number
  remaining: number
}) {
  const total = Math.max(incomeBase, fixed + investments + setAside + spent)
  if (total <= 0) return null
  const w = (v: number) => `${Math.max((v / total) * 100, 0)}%`
  const SEGMENTS = [
    { key: 'Fixed', value: fixed, cls: 'bg-slate-500' },
    { key: 'Investments', value: investments, cls: 'bg-blue-500' },
    { key: 'Into funds', value: setAside, cls: 'bg-teal-500' },
    { key: 'Spent', value: spent, cls: 'bg-orange-400' },
    { key: 'Left', value: Math.max(remaining, 0), cls: 'bg-green-500' },
  ].filter((s) => s.key !== 'Into funds' || s.value > 0)
  return (
    <div className="mt-4">
      <div className="flex h-5 rounded-lg overflow-hidden bg-gray-100">
        {SEGMENTS.filter((s) => s.value > 0).map((s) => (
          <div key={s.key} className={s.cls} style={{ width: w(s.value) }} title={`${s.key}: ${formatEuro(s.value)}`} />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-1 mt-2">
        {SEGMENTS.map((s) => (
          <span key={s.key} className="inline-flex items-center gap-1.5 text-xs">
            <span className={`w-2.5 h-2.5 rounded-sm ${s.cls}`} />
            <span className="text-gray-500">{s.key}</span>
            <span className="font-semibold text-gray-800">{formatEuro(s.key === 'Left' ? remaining : s.value)}</span>
          </span>
        ))}
      </div>
    </div>
  )
}

const PLAN_COLORS = ['#f97316', '#eab308', '#14b8a6', '#a855f7', '#ec4899', '#06b6d4', '#84cc16', '#f43f5e']

// Donut of the month's plan: fixed, investments, each spending line's
// monthly share, and whatever income stays unallocated.
function PlanPie({ report }: { report: BudgetStatusReport }) {
  const slices: { name: string; value: number; color: string }[] = []
  if (report.fixed_planned > 0) slices.push({ name: 'Fixed obligations', value: report.fixed_planned, color: '#475569' })
  if (report.investment_planned > 0) slices.push({ name: 'Investments', value: report.investment_planned, color: '#3b82f6' })
  report.lines.filter((l) => l.kind === 'spending').forEach((l, i) => {
    slices.push({ name: l.fund ? `${l.name} (fund)` : l.name, value: l.monthly_share, color: PLAN_COLORS[i % PLAN_COLORS.length] })
  })
  if (report.income_base != null) {
    const allocated = slices.reduce((s, x) => s + x.value, 0)
    const free = report.income_base - allocated
    if (free > 0.5) slices.push({ name: 'Unallocated', value: free, color: '#22c55e' })
    // A plan that exceeds income must be visible, not silently normalized.
    if (free < -0.5) slices.push({ name: 'Over income', value: -free, color: '#ef4444' })
  }
  if (slices.length === 0) return null
  const total = slices.reduce((s, x) => s + x.value, 0)

  return (
    <div>
      <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Plan composition (per month)</p>
      <ResponsiveContainer width="100%" height={215}>
        <PieChart>
          <Pie
            data={slices}
            cx="50%" cy="50%"
            innerRadius={44} outerRadius={72}
            dataKey="value" nameKey="name"
            label={({ percent }) => (percent >= 0.06 ? `${(percent * 100).toFixed(0)}%` : '')}
            labelLine={false}
            isAnimationActive={false}
          >
            {slices.map((sl, i) => (
              <Cell key={i} fill={sl.color} />
            ))}
          </Pie>
          <Tooltip formatter={(v: number, name: string) => [formatEuro(v), name]} contentStyle={{ fontSize: 11, borderRadius: 6 }} />
        </PieChart>
      </ResponsiveContainer>
      <div className="grid grid-cols-2 gap-x-4 gap-y-0.5 xl:grid-cols-1 mt-1">
        {slices.map((sl) => (
          <div key={sl.name} className="flex items-center justify-between text-xs gap-2">
            <span className="inline-flex items-center gap-1.5 text-gray-500 min-w-0">
              <span className="w-2.5 h-2.5 rounded-sm flex-shrink-0" style={{ backgroundColor: sl.color }} />
              <span className="truncate">{sl.name}</span>
            </span>
            <span className="text-gray-700 font-medium whitespace-nowrap">{formatEuro(sl.value)} <span className="text-gray-400 font-normal">{((sl.value / total) * 100).toFixed(0)}%</span></span>
          </div>
        ))}
      </div>
    </div>
  )
}

// "Do the amounts I set actually fit my income?": income − fixed −
// investment targets − every spending line's monthly share (a yearly line
// counts a twelfth, a fund its monthly contribution).
function PlanCheckCard({ report, daysLeft }: { report: BudgetStatusReport; daysLeft: number | null }) {
  if (report.income_base == null) return null
  const planned = report.fixed_planned + report.investment_planned + report.spending_planned
  const unallocated = report.income_base - planned
  const fits = unallocated >= 0
  // What monthly caps still allow this month (yearly lines and funds are
  // judged over their own horizon, so they don't add to it).
  const limitLeft = report.lines
    .filter((l) => l.kind === 'spending' && !l.fund && l.period === 'monthly')
    .reduce((s, l) => s + Math.max(l.remaining, 0), 0)
  const safe = report.safe_to_spend ?? 0
  const looseLimits = limitLeft > Math.max(safe, 0) + 0.5

  const row = (label: string, value: number, cls = 'text-gray-700') => (
    <div className="flex items-baseline justify-between text-sm gap-3">
      <span className="text-gray-500">{label}</span>
      <span className={`font-medium tabular-nums whitespace-nowrap ${cls}`}>{formatEuro(value)}</span>
    </div>
  )

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-3">
        <h3 className="text-base font-semibold text-gray-900">Does the plan fit your income?</h3>
        <span className={`text-sm font-semibold ${fits ? 'text-green-600' : 'text-red-600'}`}>
          {fits ? `✓ Fits — ${formatEuro(unallocated)} unallocated` : `⚠ Over income by ${formatEuro(-unallocated)}`}
        </span>
      </div>
      <div className="grid sm:grid-cols-2 gap-x-8 gap-y-1">
        <div className="space-y-1">
          {row('Income base', report.income_base)}
          {row('− Fixed obligations', -report.fixed_planned, 'text-slate-600')}
          {row('− Investment targets', -report.investment_planned, 'text-blue-600')}
          {row('− Spending lines (monthly share)', -report.spending_planned, 'text-orange-600')}
          <div className="border-t border-gray-100 pt-1">
            {row('= Month end, if every line is used in full', unallocated, fits ? 'text-green-600' : 'text-red-600')}
          </div>
        </div>
        <div className="space-y-1 sm:border-l sm:border-gray-100 sm:pl-8">
          {row('Still allowed by monthly limits', limitLeft, 'text-orange-600')}
          {row('Actually affordable (safe to spend)', Math.max(safe, 0), safe >= 0 ? 'text-green-600' : 'text-red-600')}
          <p className={`text-xs pt-1 ${looseLimits ? 'text-yellow-700' : 'text-gray-400'}`}>
            {looseLimits
              ? `Your limits allow ${formatEuro(limitLeft - Math.max(safe, 0))} more than income covers — spending to every limit ends the month ${fits ? 'thinner than planned' : 'in the red'}.`
              : 'Your limits are within what income covers — spend to the limit and the month still balances.'}
            {daysLeft != null && limitLeft > 0 && <> {' '}(~{formatEuro(limitLeft / daysLeft)}/day within limits)</>}
          </p>
        </div>
      </div>
    </div>
  )
}

function StatusLines({ title, hint, lines, done, doneText, pendingText, color, onView, onEdit, onDelete }: {
  title: string
  hint: string
  lines: BudgetLineStatus[]
  done: (l: BudgetLineStatus) => boolean
  doneText: string
  pendingText: (outstanding: number) => string
  color: string
  onView: (l: BudgetLineStatus) => void
  onEdit: (l: BudgetLineStatus) => void
  onDelete: (l: BudgetLineStatus) => void
}) {
  if (lines.length === 0) return null
  const outstanding = lines.reduce((s, l) => s + Math.max(l.budgeted - l.spent, 0), 0)
  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-1">
        <h3 className="text-base font-semibold text-gray-900">{title}</h3>
        <span className="text-sm">
          {outstanding > 0.5
            ? <span className={`font-semibold ${color}`}>{pendingText(outstanding)}</span>
            : <span className="font-semibold text-green-600">{doneText}</span>}
        </span>
      </div>
      <p className="text-xs text-gray-400 mb-3">{hint}</p>
      <div className="space-y-3">
        {lines.map((l) => {
          const ok = done(l)
          const pct = l.budgeted > 0 ? Math.min((l.spent / l.budgeted) * 100, 100) : 0
          return (
            <div key={l.id}>
              <div className="flex items-center justify-between gap-3 mb-1">
                <button onClick={() => onView(l)} className="text-sm font-medium text-gray-800 min-w-0 truncate text-left hover:text-blue-700">{l.name}</button>
                <div className="flex items-center gap-2 flex-shrink-0">
                  <span className={`text-sm font-semibold whitespace-nowrap ${ok ? 'text-green-600' : 'text-gray-600'}`}>
                    {formatEuro(l.spent)} / {formatEuro(l.budgeted)}
                  </span>
                  <RowActions onEdit={() => onEdit(l)} onDelete={() => onDelete(l)} />
                </div>
              </div>
              <div className="h-2.5 bg-gray-100 rounded-full overflow-hidden">
                <div className={`h-full rounded-full ${ok ? 'bg-green-500' : 'bg-slate-400'}`} style={{ width: `${pct}%` }} />
              </div>
              <p className="text-xs text-gray-400 mt-1">{l.label ? `label: ${l.label}` : l.category}</p>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default function MonthView(p: Props) {
  const { report, month, isCurrentMonth, daysLeft } = p
  const spending = report.lines.filter((l) => l.kind === 'spending')
  // Funds and yearly lines first: they're the long-horizon picture.
  const ordered = [...spending].sort((a, b) => Number(b.fund) - Number(a.fund) || Number(b.period === 'yearly') - Number(a.period === 'yearly'))
  const safe = report.safe_to_spend
  const spentOutsideFunds = report.discretionary_spent - report.fund_spent

  const totalSpent = p.monthTxs.filter((t) => t.type === 'expense').reduce((s, t) => s + t.amount.value, 0)
  const totalIncome = p.monthTxs.filter((t) => t.type === 'income').reduce((s, t) => s + t.amount.value, 0)
  const pct = totalIncome > 0 ? (totalSpent / totalIncome) * 100 : null

  return (
    <>
      <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
        <div className="grid grid-cols-1 xl:grid-cols-[1fr,320px] gap-6">
          <div>
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <p className="text-sm font-medium text-gray-500">Safe to spend {isCurrentMonth ? 'this month' : `in ${monthLabel(month)}`}</p>
              {daysLeft != null && safe != null && safe > 0 && (
                <p className="text-sm text-gray-500">
                  ≈ <span className="font-bold text-green-600">{formatEuro(safe / daysLeft)}</span>/day for the next {daysLeft} day{daysLeft === 1 ? '' : 's'}
                </p>
              )}
            </div>
            <p className={`text-3xl sm:text-4xl font-bold mt-1 ${safe != null && safe >= 0 ? 'text-green-600' : 'text-red-600'}`}>
              {safe != null ? formatEuro(safe) : '—'}
            </p>
            {report.income_base != null && safe != null && (
              <AllocationBar
                incomeBase={report.income_base}
                fixed={report.fixed_planned}
                investments={report.investment_planned}
                setAside={report.fund_contributions}
                spent={spentOutsideFunds}
                remaining={safe}
              />
            )}
            {report.fund_spent > 0.5 && (
              <p className="mt-2 text-xs text-teal-700">
                {formatEuro(report.fund_spent)} paid from funds this month — set aside earlier, so it doesn't count against this month.
              </p>
            )}
            <p className="mt-3 text-xs text-gray-500">
              This month: spent <span className={`font-semibold ${pct != null && pct > 100 ? 'text-red-600' : 'text-gray-700'}`}>{formatEuro(totalSpent)}</span>
              {' '}· income received <span className="font-semibold text-gray-700">{formatEuro(totalIncome)}</span>
              {pct != null && <span className={pct > 100 ? 'text-red-500 font-medium' : 'text-gray-400'}> ({pct.toFixed(0)}% of income)</span>}
            </p>
            <p className="mt-1 text-xs text-gray-500">
              Income base <span className="font-semibold text-gray-700">{report.income_base != null ? formatEuro(report.income_base) : '—'}</span>{' '}
              <span className="text-gray-400">({p.incomeSourceNote})</span>
              <button onClick={p.onEditIncome} className="ml-1.5 text-blue-600 hover:text-blue-800 font-medium">✎ edit</button>
            </p>
          </div>
          <PlanPie report={report} />
        </div>
      </div>

      <PlanCheckCard report={report} daysLeft={daysLeft} />

      {/* Desktop: spending as the main 2/3 column, fixed + investments as a
          status sidebar. Mobile: status cards first. */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-4 sm:gap-6 items-start">
        <div className="space-y-4 sm:space-y-6 xl:order-2">
          <StatusLines
            title="Fixed obligations" hint="Known monthly payments — tracked as paid / pending"
            lines={report.lines.filter((l) => l.kind === 'fixed')}
            done={(l) => l.spent >= l.budgeted * 0.95}
            doneText="✓ All paid" pendingText={(o) => `${formatEuro(o)} still outstanding`} color="text-yellow-600"
            onView={p.onView} onEdit={p.onEdit} onDelete={p.onDelete}
          />
          <StatusLines
            title="Investment targets" hint="Monthly contributions you aim to reach"
            lines={report.lines.filter((l) => l.kind === 'investment')}
            done={(l) => l.spent >= l.budgeted}
            doneText="✓ All targets reached" pendingText={(o) => `${formatEuro(o)} left to invest`} color="text-blue-700"
            onView={p.onView} onEdit={p.onEdit} onDelete={p.onDelete}
          />
        </div>

        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6 xl:order-1 xl:col-span-2">
          <div className="flex items-center justify-between mb-1 gap-2">
            <h3 className="text-base font-semibold text-gray-900">Spending</h3>
            <button
              onClick={() => p.onAdd(null)}
              className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
            >
              + Add budget
            </button>
          </div>
          <p className="text-xs text-gray-400 mb-3">
            Monthly caps for steady spending, yearly amounts or funds for costs that come in bursts — a fund carries whatever you don't spend.
          </p>
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-x-10">
            <div>
              {ordered.length === 0 && (
                <p className="text-sm text-gray-400 py-2">No spending lines yet — add one, or pick a category from the unbudgeted list.</p>
              )}
              <div className="space-y-4">
                {ordered.map((l) => (
                  <SpendingLine
                    key={l.id}
                    line={l}
                    daysLeft={daysLeft}
                    onView={() => p.onView(l)}
                    onEdit={() => p.onEdit(l)}
                    onDelete={() => p.onDelete(l)}
                    onApply={(input, note) => p.onApply(l, input, note)}
                    applying={p.applying}
                  />
                ))}
              </div>
            </div>

            {report.unbudgeted.length > 0 && (
              <div className="mt-4 pt-4 border-t border-gray-100 xl:mt-0 xl:pt-0 xl:border-t-0 xl:border-l xl:border-gray-100 xl:pl-10">
                <p className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Not covered by any budget</p>
                <p className="text-[11px] text-gray-400 mb-2">This month · since January</p>
                <div className="space-y-2">
                  {report.unbudgeted.map((u) => {
                    const s = u.suggestion
                    const asFund = s?.lumpy ?? false
                    return (
                      <div key={u.category} className="text-sm">
                        <div className="flex items-center justify-between gap-3">
                          <button onClick={() => p.onViewCategory(u.category)} className="text-gray-600 text-left hover:text-blue-700 truncate">{u.category}</button>
                          <span className="whitespace-nowrap">
                            <span className="font-medium text-gray-800">{formatEuro(u.spent)}</span>
                            <span className="text-xs text-gray-400"> · {formatEuro(u.year_spent)}</span>
                          </span>
                        </div>
                        <div className="flex flex-wrap gap-x-3 text-xs">
                          {s && (
                            <button
                              disabled={p.applying}
                              onClick={() => p.onApply(null, asFund
                                ? { name: u.category, kind: 'spending', category: u.category, amount: s.amount, period: 'monthly', fund: true, start_month: defaultFundStart() }
                                : { name: u.category, kind: 'spending', category: u.category, amount: s.amount, period: 'monthly' },
                                asFund
                                  ? `${u.category} fund created: ${formatEuro(s.amount)}/mo from ${defaultFundStart()}.`
                                  : `${u.category} limit created: ${formatEuro(s.amount)}/mo.`)}
                              className={`font-medium disabled:opacity-50 ${asFund ? 'text-teal-700 hover:text-teal-900' : 'text-blue-600 hover:text-blue-800'}`}
                            >
                              + {asFund ? 'fund' : 'limit'} {formatEuro(s.amount)}/mo
                            </button>
                          )}
                          <button
                            onClick={() => p.onAdd({ name: u.category, kind: 'spending', category: u.category, amount: s?.amount ?? Math.ceil(Math.max(u.spent, 1) / 50) * 50, period: 'monthly', fund: asFund, start_month: asFund ? defaultFundStart() : '' })}
                            className="text-gray-400 hover:text-gray-700"
                          >
                            custom…
                          </button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  )
}
