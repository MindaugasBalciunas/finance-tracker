import clsx from 'clsx'
import type { CSSProperties, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { FreeMoney } from '../../lib/types'
import { eur, pct } from '../../lib/format'
import { hue } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'
import { ledgerLink } from '../../lib/links'

const ESSENTIAL = 'var(--s1)'
const OPTIONAL = 'var(--s7)'
const INBOX = 'var(--s-other)'
const FUNDS = 'var(--s3)'

/** The month from the salary's side: what is free once bills and saving are
 *  planned, what was spent of it — essential or optional, by each payment's
 *  own category — and what is left, of which some is kept in funds and the
 *  rest is Left to spend (the same figure as the card above it). All figures
 *  come from the plan engine (plan.FreeMoneyOf). */
export function FreeMoneyCard({ f, ym, month, link = true }: { f: FreeMoney; ym: string; month: string; link?: boolean }) {
  if (f.income <= 0 && f.spent <= 0) return null
  const over = f.left_to_spend < 0
  const refunds = f.refunds ?? 0
  // Money a fund saved in earlier months and paid out now adds to the month.
  const fromEarlier = Math.max(-f.in_funds, 0)
  const total = Math.max(f.free + fromEarlier, f.essential + f.optional + f.unreviewed + Math.max(f.in_funds, 0) + Math.max(f.left_to_spend, 0), 1)
  const share = (v: number) => (f.spent > 0 ? pct(v / f.spent) : '')
  const segs = [
    { key: 'essential', v: f.essential, color: ESSENTIAL, label: 'Essential' },
    { key: 'optional', v: f.optional, color: OPTIONAL, label: 'Optional' },
    { key: 'inbox', v: f.unreviewed, color: INBOX, label: 'Not reviewed' },
    { key: 'funds', v: f.in_funds, color: FUNDS, label: 'Kept in funds', faded: true },
  ].filter((s) => s.v > 0.005)
  // Essential and Optional open the Ledger on those payments for the month.
  const need = (n: 'essential' | 'optional') => ledgerLink({ month: ym, need: n })
  const head = <>
    <div className="flex items-center gap-3">
      <IconTile name="wallet" color={ESSENTIAL} size={32} />
      <div className="min-w-0 flex-1">
        <div className="text-xs text-ink2">Free money · {month}</div>
        <div className="flex flex-wrap items-baseline gap-x-1.5">
          <span className="text-xl font-semibold tracking-tight tnum">{eur(f.free)}</span>
          <span className="text-sm text-ink2">from income after fixed costs and saving</span>
        </div>
      </div>
      {link && <Icon name="chevronR" size={16} className="shrink-0 text-muted" />}
    </div>
    <div className="mt-1 text-xs text-muted tnum">{eur(f.income)} income − {eur(f.fixed)} fixed − {eur(f.saving)} saving</div>
    <div className="mt-2.5 flex h-2.5 w-full gap-px overflow-hidden rounded-full bg-line" role="img"
      aria-label={`Spent ${eur(f.spent)}: essential ${eur(f.essential)}, optional ${eur(f.optional)}; ${f.in_funds >= 0 ? `kept in funds ${eur(f.in_funds)}` : `${eur(fromEarlier)} from funds saved earlier`}; ${f.left_to_spend < 0 ? `${eur(-f.left_to_spend)} over` : `left to spend ${eur(f.left_to_spend)}`}`}>
      {segs.map((s) => <div key={s.key} className="h-full" style={{ width: `${(s.v / total) * 100}%`, ...(s.faded ? faded(s.color) : { background: s.color }) }} />)}
    </div>
  </>
  const rows = <>
    <div className="space-y-1 text-sm">
      <Row dot={ESSENTIAL} label="Essential" note={share(f.essential)} v={f.essential} to={need('essential')} />
      <Row dot={OPTIONAL} label="Optional" note={share(f.optional)} v={f.optional} to={need('optional')} />
      {refunds > 0.005 && <Row label="Refunds" note="money back" v={refunds} sign="−" />}
      {f.unreviewed > 0.005 && <Row dot={INBOX} label="Not reviewed yet" note="in the bank inbox" v={f.unreviewed} />}
      <Row label="Spent" v={f.spent} strong rule />
      {f.in_funds > 0.005 && <Row dot={FUNDS} dotFaded label="Kept in funds" note="for lumpy costs" v={f.in_funds} />}
      {fromEarlier > 0.005 && <Row dot={FUNDS} label="From funds saved earlier" note="adds to this month" v={fromEarlier} sign="+" />}
      <Row label={over ? 'Over' : 'Left to spend'} v={Math.abs(f.left_to_spend)} strong className={over ? 'text-bad' : 'text-good'} />
    </div>
  </>
  return (
    <section className="card" style={hue(over ? 'var(--hue-bad)' : 'var(--hue-good)')}>
      {link
        ? <Link to="/plan" className="block px-4 pt-3 pb-2 hover:bg-sunken/30" title="The month on Plan">{head}</Link>
        : <div className="px-4 pt-3 pb-2">{head}</div>}
      <div className="px-4 pb-3">{rows}</div>
    </section>
  )
}

const faded = (c: string): CSSProperties => ({ background: `color-mix(in oklab, ${c} 40%, transparent)` })

function Row({ dot, dotFaded, label, note, v, sign, strong, rule, className, to }: { dot?: string; dotFaded?: boolean; label: ReactNode; note?: string; v: number; sign?: '+' | '−'; strong?: boolean; rule?: boolean; className?: string; to?: string }) {
  const inner = <>
    <span className="h-2 w-2 shrink-0 self-center rounded-full" style={dot ? (dotFaded ? faded(dot) : { background: dot }) : undefined} />
    <span className="min-w-0 flex-1">{label}{note && <span className="pl-1.5 text-xs font-normal text-muted">{note}</span>}</span>
    <span className="tnum">{sign}{eur(v)}</span>
  </>
  const cls = clsx('flex items-baseline gap-2', rule && 'border-t border-line pt-1', strong && 'font-semibold', className)
  // A row that opens its transactions: a full-width tap target with a chevron.
  return to
    ? <Link to={to} className={clsx(cls, '-mx-2 min-h-10 items-center rounded-lg px-2 hover:bg-sunken/50')} title={`See the ${String(label).toLowerCase()} payments`}>
        {inner}<Icon name="chevronR" size={14} className="shrink-0 self-center text-muted" />
      </Link>
    : <div className={cls}>{inner}</div>
}
