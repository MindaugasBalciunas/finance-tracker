import type { BudgetInput, BudgetLineStatus } from '../../types'
import { formatEuro } from '../../utils/format'
import { defaultFundStart, lineInput } from './budgetUi'

interface Props {
  line: BudgetLineStatus
  daysLeft: number | null
  onView: () => void
  onEdit: () => void
  onDelete: () => void
  // One-tap application of the engine's suggestion.
  onApply: (input: BudgetInput, note: string) => void
  applying: boolean
}

function RowActions({ onEdit, onDelete }: { onEdit: () => void; onDelete: () => void }) {
  return (
    <div className="flex items-center">
      <button onClick={onEdit} aria-label="Edit" className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50 transition-colors">✎</button>
      <button onClick={onDelete} aria-label="Delete" className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50 transition-colors">✕</button>
    </div>
  )
}

function PeriodBadge({ line }: { line: BudgetLineStatus }) {
  if (line.fund) return <span className="text-[10px] font-semibold uppercase tracking-wide text-teal-700 bg-teal-50 rounded px-1.5 py-0.5">fund</span>
  if (line.period === 'yearly') return <span className="text-[10px] font-semibold uppercase tracking-wide text-indigo-700 bg-indigo-50 rounded px-1.5 py-0.5">yearly</span>
  return null
}

function Suggestion({ line, onApply, applying }: Pick<Props, 'line' | 'onApply' | 'applying'>) {
  const s = line.suggestion
  if (!s) return null
  // A lumpy monthly cap is wrong most months — offer the fund instead.
  if (s.lumpy && !line.fund && line.period === 'monthly') {
    return (
      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-gray-500">
        <span>
          Comes in bursts: typical month {formatEuro(s.median)}, peak {formatEuro(s.max)}, average {formatEuro(s.mean)}.
        </span>
        <button
          disabled={applying}
          onClick={() => onApply(
            { ...lineInput(line), amount: s.amount, fund: true, start_month: defaultFundStart(), amount_from: 'all' },
            `${line.name} is now a ${formatEuro(s.amount)}/mo fund from ${defaultFundStart()}.`,
          )}
          className="font-medium text-teal-700 hover:text-teal-900 disabled:opacity-50"
        >
          Make it a {formatEuro(s.amount)}/mo fund →
        </button>
      </div>
    )
  }
  const unit = line.period === 'yearly' ? '/yr' : '/mo'
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-gray-500">
      <span>Suggested {formatEuro(s.amount)}{unit} <span className="text-gray-400">({s.basis})</span></span>
      <button
        disabled={applying}
        onClick={() => onApply({ ...lineInput(line), amount: s.amount }, `${line.name} set to ${formatEuro(s.amount)}${unit} from this month — earlier months keep their old limit.`)}
        className="font-medium text-blue-600 hover:text-blue-800 disabled:opacity-50"
      >
        Apply from this month →
      </button>
    </div>
  )
}

export default function SpendingLine({ line, daysLeft, onView, onEdit, onDelete, onApply, applying }: Props) {
  const header = (right: React.ReactNode, warn = false) => (
    <div className="flex items-center justify-between gap-3 mb-1">
      <button onClick={onView} className={`text-sm font-medium text-left hover:text-blue-700 min-w-0 inline-flex items-center gap-1.5 ${warn ? 'text-red-800' : 'text-gray-800'}`}>
        <span className="truncate">{warn && '⚠ '}{line.name}</span>
        <PeriodBadge line={line} />
      </button>
      <div className="flex items-center gap-2 flex-shrink-0">
        {right}
        <RowActions onEdit={onEdit} onDelete={onDelete} />
      </div>
    </div>
  )

  if (line.fund && line.fund_state) {
    const f = line.fund_state
    const negative = f.available < -0.5
    const pool = f.opening + f.contribution
    const usedPct = pool > 0 ? Math.min((f.spent / pool) * 100, 100) : f.spent > 0 ? 100 : 0
    return (
      <div className={negative ? 'bg-red-50 border border-red-200 rounded-lg p-3 -mx-1' : ''}>
        {header(
          <span className={`text-sm font-semibold ${negative ? 'text-red-600' : 'text-teal-700'}`}>
            {formatEuro(f.available)} <span className="text-xs font-normal text-gray-400">available</span>
          </span>,
          negative,
        )}
        <div className="h-2.5 bg-teal-50 rounded-full overflow-hidden">
          <div className={`h-full rounded-full ${negative ? 'bg-red-400' : 'bg-teal-500'}`} style={{ width: `${usedPct}%` }} />
        </div>
        <p className="text-xs mt-1 text-gray-400">
          Carried in <span className="font-medium text-gray-600">{formatEuro(f.opening)}</span>
          {' '}+ {formatEuro(f.contribution)} this month − spent <span className="font-medium text-gray-600">{formatEuro(f.spent)}</span>
          <span className="text-gray-300"> · </span>
          {formatEuro(line.monthly_share)}/mo since {f.start_month}
          {negative && <span className="text-red-600 font-medium"> · overdrawn — later months' savings pay it back</span>}
        </p>
        <Suggestion line={line} onApply={onApply} applying={applying} />
      </div>
    )
  }

  if (line.period === 'yearly' && line.year) {
    const y = line.year
    const pct = y.budget > 0 ? (y.spent / y.budget) * 100 : 0
    const over = y.spent > y.budget
    const aheadOfPace = y.spent > y.pace * 1.05
    const projectedOver = y.projected > y.budget * 1.02
    return (
      <div className={over ? 'bg-red-50 border border-red-200 rounded-lg p-3 -mx-1' : ''}>
        {header(
          <span className={`text-sm font-semibold ${over ? 'text-red-600' : aheadOfPace ? 'text-yellow-600' : 'text-gray-700'}`}>
            {formatEuro(y.spent)} / {formatEuro(y.budget)}
            <span className="ml-1.5 text-xs font-bold text-gray-400">{pct.toFixed(0)}%</span>
          </span>,
          over,
        )}
        <div className="relative h-2.5 bg-gray-100 rounded-full overflow-hidden">
          <div className={`h-full rounded-full ${over ? 'bg-red-400' : aheadOfPace ? 'bg-yellow-400' : 'bg-indigo-500'}`} style={{ width: `${Math.min(pct, 100)}%` }} />
          {/* Pace marker: where spending "should" be by today. */}
          <div className="absolute top-0 h-full w-0.5 bg-gray-700/60" style={{ left: `${Math.min(y.elapsed * 100, 100)}%` }} title={`Pace: ${formatEuro(y.pace)}`} />
        </div>
        <p className="text-xs mt-1 text-gray-400">
          {y.year} so far · pace {formatEuro(y.pace)} ·{' '}
          <span className={projectedOver ? 'text-red-600 font-medium' : 'text-gray-500'}>
            projected {formatEuro(y.projected)} by December
          </span>
          {line.month_spent > 0 && <> · {formatEuro(line.month_spent)} this month</>}
        </p>
        <Suggestion line={line} onApply={onApply} applying={applying} />
      </div>
    )
  }

  const usedPct = line.budgeted > 0 ? (line.spent / line.budgeted) * 100 : 0
  const over = line.spent > line.budgeted
  const near = !over && line.spent > line.budgeted * 0.8
  return (
    <div className={over ? 'bg-red-50 border border-red-200 rounded-lg p-3 -mx-1' : ''}>
      {header(
        <span className={`text-sm font-semibold ${over ? 'text-red-600' : near ? 'text-yellow-600' : 'text-gray-700'}`}>
          {formatEuro(line.spent)} / {formatEuro(line.budgeted)}
          <span className={`ml-1.5 text-xs font-bold ${over ? 'text-red-600' : near ? 'text-yellow-600' : 'text-gray-400'}`}>{usedPct.toFixed(0)}%</span>
        </span>,
        over,
      )}
      {over ? (
        // Full bar = actual spend; light part is the limit, dark red the overflow.
        <div className="flex h-2.5 rounded-full overflow-hidden bg-red-100">
          <div className="h-full bg-red-400" style={{ width: `${(line.budgeted / line.spent) * 100}%` }} />
          <div className="h-full bg-red-700" style={{ width: `${(1 - line.budgeted / line.spent) * 100}%` }} />
        </div>
      ) : (
        <div className="h-2.5 bg-gray-100 rounded-full overflow-hidden">
          <div className={`h-full rounded-full ${near ? 'bg-yellow-400' : 'bg-green-500'}`} style={{ width: `${Math.min(usedPct, 100)}%` }} />
        </div>
      )}
      <p className="text-xs mt-1 text-gray-400">
        Spent <span className="font-medium text-gray-600">{formatEuro(line.spent)}</span>
        {over
          ? <> · <span className="text-red-600 font-semibold">{formatEuro(line.spent - line.budgeted)} over (+{(usedPct - 100).toFixed(0)}%)</span></>
          : <> · <span className={`font-medium ${near ? 'text-yellow-600' : 'text-green-600'}`}>{formatEuro(line.remaining)} left</span>{daysLeft != null && ` · ${formatEuro(line.remaining / daysLeft)}/day`}</>}
      </p>
      <Suggestion line={line} onApply={onApply} applying={applying} />
    </div>
  )
}

export { RowActions }
