import { useEffect, useRef } from 'react'
import type { BudgetLineStatus, BudgetMonthCell, BudgetStatusReport } from '../../types'
import { formatEuro } from '../../utils/format'
import { compactEuro, shortMonth } from './budgetUi'

// Cell colour = how much of that month's share was used. Fund rows are
// coloured by the fund's balance instead: a big trip month is fine as long
// as the fund stays above zero.
function cellClass(cell: BudgetMonthCell, line?: BudgetLineStatus): string {
  if (line?.fund) {
    if (cell.fund_end == null) return 'text-gray-300'
    return cell.fund_end < -0.5 ? 'bg-red-50 text-red-700' : 'bg-teal-50 text-teal-800'
  }
  if (cell.budget <= 0) return cell.spent > 0.5 ? 'text-gray-700' : 'text-gray-300'
  const r = cell.spent / cell.budget
  if (cell.spent < 0.5) return 'text-gray-300'
  if (line?.kind === 'fixed' || line?.kind === 'investment') {
    return r >= 0.95 ? 'bg-green-50 text-green-800' : 'bg-yellow-50 text-yellow-800'
  }
  if (r > 1) return 'bg-red-50 text-red-700'
  if (r > 0.8) return 'bg-yellow-50 text-yellow-800'
  return 'bg-green-50 text-green-800'
}

function Row({ name, sub, cells, line, months, current }: {
  name: string
  sub?: string
  cells: BudgetMonthCell[]
  line?: BudgetLineStatus
  months: string[]
  current: string
}) {
  const spent = cells.reduce((s, c) => s + c.spent, 0)
  const budget = cells.reduce((s, c) => s + c.budget, 0)
  const over = line?.kind === 'spending' && !line.fund && budget > 0 && spent > budget
  return (
    <tr className="border-t border-gray-100">
      <th scope="row" className="sticky left-0 z-10 bg-white text-left font-medium text-gray-700 py-1.5 pr-3 min-w-[110px] max-w-[140px] shadow-[4px_0_4px_-4px_rgba(0,0,0,0.12)]">
        <span className="block truncate">{name}</span>
        {sub && <span className="block text-[10px] font-normal text-gray-400 truncate">{sub}</span>}
      </th>
      {months.map((m, i) => {
        const c = cells[i]
        return (
          <td
            key={m}
            className={`text-right tabular-nums px-1.5 py-1.5 rounded ${c ? cellClass(c, line) : ''} ${m === current ? 'font-semibold' : ''}`}
            title={c ? `${shortMonth(m)}: spent ${formatEuro(c.spent)}${c.budget > 0 ? ` of ${formatEuro(c.budget)}` : ''}${c.fund_end != null ? ` · fund ${formatEuro(c.fund_end)}` : ''}` : ''}
          >
            {c && c.spent >= 0.5 ? compactEuro(c.spent) : '·'}
            {line?.fund && c?.fund_end != null && (
              <span className="block text-[10px] leading-tight opacity-80">{compactEuro(c.fund_end)}</span>
            )}
          </td>
        )
      })}
      <td className={`text-right tabular-nums pl-3 py-1.5 font-semibold whitespace-nowrap ${over ? 'text-red-600' : 'text-gray-800'}`}>
        {compactEuro(spent)}
        {budget > 0 && <span className="block text-[10px] font-normal text-gray-400">of {compactEuro(budget)}</span>}
      </td>
    </tr>
  )
}

export default function YearGrid({ report }: { report: BudgetStatusReport }) {
  const months = report.months
  // On a phone only ~5 months fit: open on the recent end, scroll back for more.
  const scroller = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollLeft = el.scrollWidth
  }, [report.month])
  const groups: { title: string; lines: BudgetLineStatus[] }[] = [
    { title: 'Spending', lines: report.lines.filter((l) => l.kind === 'spending') },
    { title: 'Fixed obligations', lines: report.lines.filter((l) => l.kind === 'fixed') },
    { title: 'Investments', lines: report.lines.filter((l) => l.kind === 'investment') },
  ]
  // Spending total per month (incl. unbudgeted) against the monthly shares.
  const spendTotals: BudgetMonthCell[] = months.map((m, i) => {
    const lines = report.lines.filter((l) => l.kind === 'spending')
    return {
      month: m,
      budget: lines.reduce((s, l) => s + (l.history[i]?.budget ?? 0), 0),
      spent: lines.reduce((s, l) => s + (l.history[i]?.spent ?? 0), 0) +
        report.unbudgeted.reduce((s, u) => s + (u.history[i]?.spent ?? 0), 0),
    }
  })

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2 mb-1">
        <h3 className="text-base font-semibold text-gray-900">Last 12 months</h3>
        <span className="text-[11px] text-gray-400">
          <span className="inline-block w-2.5 h-2.5 rounded-sm bg-green-50 border border-green-200 align-middle" /> within
          {' '}<span className="inline-block w-2.5 h-2.5 rounded-sm bg-yellow-50 border border-yellow-200 align-middle ml-2" /> &gt;80%
          {' '}<span className="inline-block w-2.5 h-2.5 rounded-sm bg-red-50 border border-red-200 align-middle ml-2" /> over
          {' '}<span className="inline-block w-2.5 h-2.5 rounded-sm bg-teal-50 border border-teal-200 align-middle ml-2" /> fund balance
        </span>
      </div>
      <p className="text-xs text-gray-400 mb-3">
        Each cell is what was spent; fund rows show the balance left underneath. Totals compare 12 months of spending with 12 months of budget.
      </p>
      <div ref={scroller} className="overflow-x-auto">
        <table className="text-xs w-full border-separate border-spacing-y-0.5">
          <thead>
            <tr>
              <th className="sticky left-0 z-10 bg-white" />
              {months.map((m) => (
                <th key={m} className={`text-right font-medium px-1.5 pb-1 ${m === report.month ? 'text-blue-700' : 'text-gray-400'}`}>{shortMonth(m)}</th>
              ))}
              <th className="text-right font-medium text-gray-400 pl-3 pb-1">12 mo</th>
            </tr>
          </thead>
          <tbody>
            {groups.filter((g) => g.lines.length > 0).map((g) => (
              <GroupRows key={g.title} title={g.title} months={months}>
                {g.lines.map((l) => (
                  <Row key={l.id} name={l.name} sub={l.fund ? `fund · ${formatEuro(l.monthly_share)}/mo` : l.period === 'yearly' ? `yearly · ${formatEuro(l.amount)}` : undefined}
                    cells={l.history} line={l} months={months} current={report.month} />
                ))}
              </GroupRows>
            ))}
            {report.unbudgeted.length > 0 && (
              <GroupRows title="Not covered by any budget" months={months}>
                {report.unbudgeted.map((u) => (
                  <Row key={u.category} name={u.category} cells={u.history} months={months} current={report.month} />
                ))}
              </GroupRows>
            )}
            <tr className="border-t-2 border-gray-200">
              <td colSpan={months.length + 2} className="pt-2" />
            </tr>
            <Row name="All spending" sub="vs. sum of monthly shares" cells={spendTotals} months={months} current={report.month} />
          </tbody>
        </table>
      </div>
    </div>
  )
}

function GroupRows({ title, months, children }: { title: string; months: string[]; children: React.ReactNode }) {
  return (
    <>
      <tr>
        {/* Only the first cell is sticky: a full-width sticky cell can't keep
            its text in view once the table scrolls. */}
        <th scope="rowgroup" className="sticky left-0 z-10 bg-white text-left pt-3 pb-0.5 text-[10px] font-semibold uppercase tracking-wide text-gray-400 whitespace-nowrap">{title}</th>
        <td colSpan={months.length + 1} />
      </tr>
      {children}
    </>
  )
}
