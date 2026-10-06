import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, CartesianGrid, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurk, monthLabel, pct } from '../../lib/format'
import { Card, Loading } from '../../components/ui'
import { axisProps, gridProps, TooltipBox } from '../../components/charts'

// ── FI ──────────────────────────────────────────────────────────────

export function FI() {
  const { data: f, isLoading } = useQuery({ queryKey: ['fi'], queryFn: () => api.get<any>('/insights/fi') })
  if (isLoading || !f) return <Loading />
  return (
    <div className="space-y-4">
      <section className="card p-4">
        <div className="text-sm text-ink2">Progress to financial independence</div>
        <div className="mt-1 flex items-baseline gap-3">
          <div className="text-3xl font-semibold">{pct(f.progress, 1)}</div>
          <div className="text-sm text-muted">{eur(f.investable)} of {eur(f.target)}</div>
        </div>
        <div className="mt-3 h-2 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(f.progress, 1) * 100}%` }} /></div>
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><div className="text-xs text-muted">At current pace</div><div className="font-semibold">{f.years_to_fi >= 0 ? `${f.years_to_fi} years` : 'not reached'}</div>{f.fi_date && <div className="text-xs text-muted">{monthLabel(f.fi_date, true)}{f.fi_age ? ` · age ${Math.round(f.fi_age)}` : ''}</div>}</div>
          <div><div className="text-xs text-muted">Saving now</div><div className="font-semibold">{eur(f.monthly_invested)}/mo</div></div>
          <div><div className="text-xs text-muted">Needed for target age {f.target_age || '—'}</div><div className="font-semibold">{f.required_monthly ? `${eur(f.required_monthly)}/mo` : 'set it in Plan'}</div></div>
          <div><div className="text-xs text-muted">Assumptions</div><div className="font-semibold">{f.withdrawal_rate}% draw · {f.expected_return}% real</div></div>
        </div>
        <div className="mt-2 text-xs text-muted">Target = {f.spend_source} ({eur(f.annual_spend)}/yr) ÷ {f.withdrawal_rate}%. Investable = brokers, pension, crypto and cash above the emergency fund — the house is not counted.</div>
      </section>
      <Card title="Projection">
        <div className="h-64">
          <ResponsiveContainer>
            <AreaChart data={f.projection} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="year" {...axisProps} />
              <YAxis {...axisProps} tickFormatter={eurk} width={52} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={label} rows={[{ color: 'var(--s1)', label: 'Investable', value: eur(payload[0].payload.investable), bold: true }, { label: 'Target', value: eur(payload[0].payload.target) }]} /> : null} />
              <ReferenceLine y={f.target} stroke="var(--s2)" label={{ value: 'FI target', fill: 'var(--chart-text)', fontSize: 11, position: 'insideTopLeft' }} />
              <Area dataKey="investable" stroke="var(--s1)" strokeWidth={2} fill="var(--s1)" fillOpacity={0.15} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </Card>
      <Card title="Emergency fund">
        <div className="grid grid-cols-3 gap-3 text-sm">
          <div><div className="text-xs text-muted">Cash</div><div className="font-semibold">{eur(f.emergency.cash)}</div></div>
          <div><div className="text-xs text-muted">Essential spend</div><div className="font-semibold">{eur(f.emergency.monthly_essential)}/mo</div></div>
          <div><div className="text-xs text-muted">Covers</div><div className={clsx('font-semibold', f.emergency.months < f.emergency.target_months ? 'text-warn' : 'text-good')}>{f.emergency.months} months</div><div className="text-xs text-muted">target {f.emergency.target_months}</div></div>
        </div>
      </Card>
    </div>
  )
}
