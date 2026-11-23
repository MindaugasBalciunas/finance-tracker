import clsx from 'clsx'
import { useOverview } from '../../lib/hooks'
import { eur, shortDate } from '../../lib/format'
import { Card } from '../../components/ui'

/** The emergency fund the plan goes by: what an emergency could use — the
 *  bank (and the Cash account, unless left out) less what leaves before
 *  payday and the wish list's set-asides — against months of essentials.
 *  The payday checklist funds the wish list only above this line. */
/** Months covered, rounded down while short of the target: 2.95 months
 *  showed "3.0 of 3 months" beside "€148 short". */
export function coverMonths(e: { available: number; monthly_essential: number; months: number; gap: number }) {
  if (e.monthly_essential <= 0) return e.months
  const m = e.available / e.monthly_essential
  return e.gap > 0 ? Math.floor(m * 10) / 10 : Math.round(m * 10) / 10
}

export function EmergencyFund() {
  const { data: o } = useOverview()
  const e = o?.emergency_plan
  if (!e || e.monthly_essential <= 0) return null
  const short = e.gap > 0
  const scale = Math.max(e.target * 1.5, e.available, 1)
  const rows: [string, number, string?][] = [
    ['In the bank', e.bank],
    ...(e.cash > 0 ? [[e.counts_cash ? 'Cash account (notes and coins)' : 'Cash account — not counted', e.counts_cash ? e.cash : 0, e.counts_cash ? undefined : eur(e.cash)] as [string, number, string?]] : []),
    ...(e.committed > 0 ? [[`Leaving before payday${e.payday ? ` (${shortDate(e.payday)})` : ''}`, -e.committed] as [string, number]] : []),
    ...(e.set_aside > 0 ? [['Set aside for the wish list', -e.set_aside] as [string, number]] : []),
  ]
  return (
    <Card icon="umbrella" color={short ? 'var(--hue-warn)' : 'var(--hue-good)'} title="Emergency fund"
      action={<span className={clsx('text-xs font-medium', short ? 'text-warn' : 'text-good')}>{coverMonths(e).toFixed(1)} of {e.target_months} months</span>}>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)] md:items-center">
        <div>
          <div className="flex flex-wrap items-baseline justify-between gap-x-3">
            <div><span className={clsx('text-2xl font-semibold tnum', e.available < 0 && 'text-bad')}>{eur(e.available)}</span> <span className="text-sm text-muted">an emergency could use</span></div>
            <div className={clsx('text-sm font-medium', short ? 'text-warn' : 'text-good')}>{short ? `${eur(e.gap)} short` : `${eur(e.spare)} above the floor`}</div>
          </div>
          <div className="relative mt-2 h-3 rounded-full bg-sunken" role="meter" aria-label="Emergency fund" aria-valuenow={Math.max(0, Math.min(100, Math.round((e.available / Math.max(e.target, 1)) * 100)))} aria-valuemin={0} aria-valuemax={100}>
            <div className={clsx('h-full rounded-full', short ? 'bg-warn' : 'bg-good')} style={{ width: `${Math.max(0, Math.min(1, e.available / scale)) * 100}%` }} />
            <div className="absolute -top-1 h-5 w-0.5 rounded bg-ink" style={{ left: `${(e.target / scale) * 100}%` }} title={`${e.target_months} months: ${eur(e.target)}`} />
          </div>
          <div className="mt-1 flex justify-between text-xs text-muted">
            <span>{e.target_months} months of essentials ({eur(e.monthly_essential)}/mo) = <b className="text-ink2 tnum">{eur(e.target)}</b></span>
          </div>
          <p className={clsx('mt-2 text-xs', short ? 'text-warn' : 'text-muted')}>
            {e.available < 0 ? <>What leaves before payday is {eur(-e.available)} more than you hold — top up before the wish list gets anything.</>
              : short ? <>The payday checklist keeps last month's leftovers here before the wish list gets any.</> : <>The wish list can take up to {eur(e.spare)} without going under the floor.</>}
          </p>
        </div>
        <ul className="divide-y divide-line rounded-xl border border-line text-sm">
          {rows.map(([l, v, note]) => (
            <li key={l} className="flex justify-between gap-3 px-3 py-1.5"><span className="text-ink2">{l}</span><span className={clsx('tnum', v < 0 && 'text-ink2')}>{note ?? (v < 0 ? `−${eur(-v)}` : eur(v))}</span></li>
          ))}
          <li className="flex justify-between gap-3 bg-sunken/50 px-3 py-1.5 font-medium"><span>Emergency fund</span><span className="tnum">{eur(e.available)}</span></li>
        </ul>
      </div>
    </Card>
  )
}

