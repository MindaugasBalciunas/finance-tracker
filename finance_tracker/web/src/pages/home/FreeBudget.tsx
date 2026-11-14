import clsx from 'clsx'
import type { CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import type { FreeBudget, FreePart } from '../../lib/types'
import { eur, pct } from '../../lib/format'
import { hue } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'

const NEEDS = 'var(--s1)'
const WANTS = 'var(--s7)'

/** The month's free budget: the spending lines you choose how to use —
 *  fixed obligations and saving left out — with how much of it is
 *  wants. The figures come from the plan engine (plan.Free), so they match
 *  the lines on Plan; a want is a line with any non-essential category. */
export function FreeBudgetCard({ f, month, unreviewed = 0 }: { f: FreeBudget; month: string; unreviewed?: number }) {
  if (f.lines.length === 0) return null
  const over = f.left < 0
  // Each part is as wide as its budget, or its spending when over, so the bar stays to scale.
  const total = Math.max(Math.max(f.needs.budgeted, f.needs.spent) + Math.max(f.wants.budgeted, f.wants.spent), 1)
  const wantsShare = f.budgeted > 0 ? f.wants.budgeted / f.budgeted : 0
  return (
    <section className="card" style={hue(over ? 'var(--hue-bad)' : 'var(--hue-good)')}>
      <Link to="/plan" className="block px-4 py-3 hover:bg-sunken/30" title="Your spending lines on Plan">
        <div className="flex items-center gap-3">
          <IconTile name="pie" color={NEEDS} size={32} />
          <div className="min-w-0 flex-1">
            <div className="text-xs text-ink2">Free budget in {month}</div>
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className={clsx('text-xl font-semibold tracking-tight tnum', over ? 'text-bad' : 'text-good')}>{eur(over ? -f.left : f.left)}</span>
              <span className="text-xs text-muted">{over ? 'over' : 'left'} of <b className="tnum text-ink2">{eur(f.budgeted)}</b> · {eur(f.spent)} spent</span>
            </div>
          </div>
          <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
        </div>
        {/* Budget split into needs and wants; the solid part of each is what is spent. */}
        <div className="mt-2.5 flex h-2.5 w-full gap-px overflow-hidden rounded-full bg-sunken" role="img"
          aria-label={`Needs ${eur(f.needs.spent)} of ${eur(f.needs.budgeted)}, wants ${eur(f.wants.spent)} of ${eur(f.wants.budgeted)}`}>
          <Segment p={f.needs} total={total} color={NEEDS} />
          <Segment p={f.wants} total={total} color={WANTS} />
        </div>
        <div className="mt-2 space-y-1.5 text-sm">
          <Row label="Needs" color={NEEDS} p={f.needs} />
          <Row label="Wants" note={f.wants.budgeted > 0 ? `${pct(wantsShare)} of the budget` : undefined} color={WANTS} p={f.wants} />
        </div>
        <div className="mt-1.5 text-xs text-muted">
          {f.wants.lines.length > 0 && <>Wants: {f.wants.lines.join(', ')}. </>}
          Fixed lines and saving are not included.
          {unreviewed > 0.005 && <> {eur(unreviewed)} in the inbox is not counted yet.</>}
        </div>
      </Link>
    </section>
  )
}

function Segment({ p, total, color }: { p: FreePart; total: number; color: string }) {
  const width = Math.max(p.budgeted, p.spent) / total
  if (width <= 0) return null
  const used = p.budgeted > 0 ? Math.min(1, p.spent / p.budgeted) : 1
  const over = p.spent > p.budgeted
  const fill = over ? 'rgb(var(--bad))' : color
  return (
    <div className="flex h-full" style={{ width: `${width * 100}%` }} title={`${eur(p.spent)} of ${eur(p.budgeted)}`}>
      <div className="h-full" style={{ width: `${used * 100}%`, background: fill }} />
      <div className="h-full flex-1" style={faded(color)} />
    </div>
  )
}

const faded = (c: string): CSSProperties => ({ background: `color-mix(in oklab, ${c} 28%, transparent)` })

function Row({ label, note, color, p }: { label: string; note?: string; color: string; p: FreePart }) {
  const over = p.left < 0
  return (
    <div>
      <div className="flex items-baseline gap-2">
        <span className="h-2 w-2 shrink-0 self-center rounded-sm" style={{ background: color }} />
        <span className="min-w-0 flex-1">{label}</span>
        <span className={clsx('whitespace-nowrap tnum', over ? 'text-bad' : 'text-ink')}>{over ? `${eur(-p.left)} over` : `${eur(p.left)} left`}</span>
      </div>
      <div className="pl-4 text-xs text-muted"><span className="tnum">{eur(p.spent)}</span> of <span className="tnum">{eur(p.budgeted)}</span> spent{note && ` · ${note}`}</div>
    </div>
  )
}
