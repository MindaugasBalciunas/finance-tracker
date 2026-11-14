import clsx from 'clsx'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { FreeBudget, FreePart } from '../../lib/types'
import { eur, pct } from '../../lib/format'
import { hue } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'

const NEEDS = 'var(--s1)'
const WANTS = 'var(--s7)'

/** The month's spending budget: the spending lines you choose how to use —
 *  fixed obligations and saving left out — and what is left of it, as needs
 *  and wants side by side. The figures come from the plan engine
 *  (plan.Free), so they match the lines on Plan; a want is a line with any
 *  non-essential category. */
export function FreeBudgetCard({ f, month, unreviewed = 0, link = true }: { f: FreeBudget; month: string; unreviewed?: number; link?: boolean }) {
  if (f.lines.length === 0) return null
  const over = f.left < 0
  const used = f.budgeted > 0 ? Math.min(1, f.spent / f.budgeted) : f.spent > 0 ? 1 : 0
  const body: ReactNode = <>
        <div className="flex items-center gap-3">
          <IconTile name="pie" color={NEEDS} size={32} />
          <div className="min-w-0 flex-1">
            <div className="text-xs text-ink2">Spending budget · {month}</div>
            <div className="flex flex-wrap items-baseline gap-x-1.5">
              <span className={clsx('text-xl font-semibold tracking-tight tnum', over ? 'text-bad' : 'text-good')}>{eur(Math.abs(f.left))}</span>
              <span className="text-sm text-ink2">{over ? 'over the' : 'left of'} <span className="tnum">{eur(f.budgeted)}</span>{over && ' budget'}</span>
            </div>
          </div>
          {link && <Icon name="chevronR" size={16} className="shrink-0 text-muted" />}
        </div>
        <Bar used={used} over={over} color="rgb(var(--good))" className="mt-2.5" label={`${eur(f.spent)} of ${eur(f.budgeted)} spent`} />
        <div className="mt-1 text-xs text-muted"><span className="tnum">{eur(f.spent)}</span> spent{f.budgeted > 0 && f.wants.budgeted > 0 && <> · wants are {pct(f.wants.budgeted / f.budgeted)} of the budget</>}{unreviewed > 0.005 && <> · <span className="tnum">{eur(unreviewed)}</span> in the bank inbox, not in these lines yet</>}</div>
        <div className="mt-3 grid grid-cols-2 gap-2">
          <Part label="Needs" color={NEEDS} p={f.needs} />
          <Part label="Wants" color={WANTS} p={f.wants} />
        </div>
        {f.wants.lines.length > 0 && <div className="mt-2 truncate text-xs text-muted" title={f.wants.lines.join(', ')}>Wants: {f.wants.lines.join(', ')}</div>}
  </>
  // On Home the card opens Plan; on Plan it is already there.
  return (
    <section className="card" style={hue(over ? 'var(--hue-bad)' : 'var(--hue-good)')}>
      {link
        ? <Link to="/plan" className="block px-4 py-3 hover:bg-sunken/30" title="Your spending lines on Plan">{body}</Link>
        : <div className="px-4 py-3">{body}</div>}
    </section>
  )
}

/** Needs or wants: what is left, big, with its own bar — so the two never blur into one. */
function Part({ label, color, p }: { label: string; color: string; p: FreePart }) {
  const over = p.left < 0
  const used = p.budgeted > 0 ? Math.min(1, p.spent / p.budgeted) : p.spent > 0 ? 1 : 0
  return (
    <div className="min-w-0 rounded-xl bg-sunken/50 p-2.5">
      <div className="flex items-center gap-1.5 text-xs text-ink2">
        <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: color }} />
        <span className="font-medium">{label}</span>
      </div>
      <div className={clsx('mt-1 text-lg font-semibold leading-tight tnum', over && 'text-bad')}>
        {eur(Math.abs(p.left))} <span className="text-xs font-normal text-muted">{over ? 'over' : 'left'}</span>
      </div>
      <Bar used={used} over={over} color={color} className="mt-1.5" label={`${label}: ${eur(p.spent)} of ${eur(p.budgeted)} spent`} />
      <div className="mt-1 text-[11px] text-muted"><span className="tnum">{eur(p.spent)}</span> of <span className="tnum">{eur(p.budgeted)}</span></div>
    </div>
  )
}

function Bar({ used, over, color, className, label }: { used: number; over: boolean; color: string; className?: string; label: string }) {
  return (
    <div className={clsx('h-1.5 w-full overflow-hidden rounded-full bg-line', className)} role="img" aria-label={label}>
      <div className="h-full rounded-full" style={{ width: `${used * 100}%`, background: over ? 'rgb(var(--bad))' : color }} />
    </div>
  )
}
