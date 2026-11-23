import { useNavigate } from 'react-router-dom'
import { useOverview } from '../../lib/hooks'
import { eur, eurk, pct } from '../../lib/format'
import { CountUp, Stat } from '../../components/ui'
import { coverMonths } from '../wealth/Emergency'

/** The slow numbers that say whether wealth is being built — savings rate,
 *  emergency fund, independence, invested. They lived on Home; they change
 *  month to month, not day to day, so they open Insights instead. */
export function ProgressTiles() {
  const { data: o } = useOverview()
  const nav = useNavigate()
  if (!o) return null
  const avg = o.avg12
  const e = o.emergency_plan
  const months = e ? coverMonths(e) : o.emergency.months
  const target = e ? e.target_months : o.emergency.target_months
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Stat icon="piggy" color="var(--s6)" label="Savings rate · usual month" value={<CountUp value={o.usual_rate ?? avg.savings_rate} format={pct} />}
        sub={`${eur(Math.round(o.usual_saved ?? avg.saved))}/mo · 12-mo avg ${pct(avg.savings_rate)}`} tone={(o.usual_rate ?? avg.savings_rate) < 0 ? 'bad' : undefined} onClick={() => nav('/insights/cashflow')} />
      {/* The same figure the plan holds the wish list to: after what leaves before payday. */}
      <Stat icon="umbrella" color="var(--s3)" label="Emergency fund" value={<CountUp value={months} format={(v) => `${v.toFixed(1)} mo`} />}
        sub={e ? `${eur(e.available)} after what leaves before payday · target ${target} mo` : `${eur(o.emergency.cash)} · target ${target} mo`}
        tone={(e ? e.gap > 0 : months < target) ? 'warn' : undefined} onClick={() => nav('/wealth/fi')} />
      <Stat icon="trend" color="var(--s7)" label="Financial independence" value={<CountUp value={o.fi_progress} format={(v) => pct(v, 1)} />} sub={o.years_to_fi >= 0 ? `${o.years_to_fi.toFixed(1)} years at this pace` : 'set a target in Wealth → Independence'} onClick={() => nav('/wealth/fi')} />
      <Stat icon="trend" color="var(--s2)" label="Invested · 12 mo" value={<CountUp value={avg.invested * 12} format={eurk} />} sub={`${eur(avg.invested)}/month · net of money taken out, incl. loan principal`} onClick={() => nav('/insights/cashflow')} />
    </div>
  )
}
