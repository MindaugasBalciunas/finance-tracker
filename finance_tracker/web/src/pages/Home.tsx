import { Link, useNavigate } from 'react-router-dom'
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { useOverview } from '../lib/hooks'
import { eur, eurk, monthLabel, pct, shortDate, signed } from '../lib/format'
import { Card, Delta, ErrorBox, Loading, Meter, Stat } from '../components/ui'
import { TooltipBox } from '../components/charts'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { Icon } from '../components/Icon'
import { useCats, groupName } from '../lib/categories'

export default function Home() {
  const { data: o, isLoading, error } = useOverview()
  const editor = useTxEditor()
  const nav = useNavigate()
  const cats = useCats()
  if (isLoading) return <Loading />
  if (error || !o) return <ErrorBox error={error} />

  const m = o.month
  const avg = o.avg12
  const spendPace = avg.spending > 0 ? m.spending / (avg.spending * Math.max(o.month_progress, 0.05)) : 0
  const groups = Object.entries(o.by_group).filter(([g]) => g !== 'debt').sort((a, b) => b[1] - a[1])
  const stale = Object.entries(o.stale ?? {})

  return (
    <div className="space-y-4">
      {/* Hero: net worth */}
      <section className="card overflow-hidden">
        <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-end sm:justify-between sm:p-6">
          <div>
            <div className="text-sm text-ink2">Net worth</div>
            <div className="mt-1 text-4xl font-semibold tracking-tight sm:text-5xl">{eur(o.net_worth)}</div>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
              <span className="text-muted">30 days <Delta value={o.net_worth_30d} /></span>
              <span className="text-muted">This year <Delta value={o.net_worth_ytd} /></span>
              <span className="text-muted">12 months <Delta value={o.net_worth_12m} /></span>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4 text-sm sm:text-right">
            <div><div className="text-muted">Liquid</div><div className="font-semibold tnum">{eur(o.liquid)}</div></div>
            <div><div className="text-muted">Debt</div><div className="font-semibold tnum">{eur(o.debt)}</div></div>
            <div><div className="text-muted">Asset equity</div><div className="font-semibold tnum">{eur((o.by_group.real_assets ?? 0) - o.debt)}</div></div>
          </div>
        </div>
        <div className="h-28 sm:h-36">
          <ResponsiveContainer>
            <AreaChart data={o.spark} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id="nw" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--s1)" stopOpacity={0.22} />
                  <stop offset="100%" stopColor="var(--s1)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="date" hide />
              <YAxis hide domain={['dataMin - 5000', 'dataMax + 5000']} />
              <Tooltip cursor={{ stroke: 'var(--chart-axis)' }} content={({ active, payload }) =>
                active && payload?.length ? <TooltipBox title={shortDate(payload[0].payload.date)} rows={[{ label: 'Net worth', value: eur(payload[0].value as number), bold: true }]} /> : null} />
              <Area type="monotone" dataKey="value" stroke="var(--s1)" strokeWidth={2} fill="url(#nw)" isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
        <div className="no-scrollbar flex gap-2 overflow-x-auto border-t border-line px-4 py-3 text-xs">
          {groups.map(([g, v]) => (
            <span key={g} className="chip"><span className="text-ink2">{groupName(g)}</span> <span className="tnum font-medium text-ink">{eurk(v)}</span></span>
          ))}
        </div>
      </section>

      {o.inbox_open > 0 && (
        <button onClick={() => nav('/ledger/inbox')} className="card flex w-full items-center gap-3 p-3.5 text-left hover:bg-sunken/50">
          <span className="grid h-9 w-9 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="inbox" /></span>
          <span className="flex-1 text-sm"><b>{o.inbox_open}</b> bank {o.inbox_open === 1 ? 'transaction waits' : 'transactions wait'} for review</span>
          <Icon name="chevronR" className="text-muted" />
        </button>
      )}

      {/* This month */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label={`Spent in ${monthLabel(m.period || o.date.slice(0, 7))}`} value={eur(m.spending)}
          sub={<>typical month {eur(avg.spending)} · {pct(o.month_progress)} through</>}
          tone={spendPace > 1.15 ? 'bad' : undefined} onClick={() => nav('/insights/spending')} />
        <Stat label="Saved this month" value={eur(m.saved)} sub={m.income > 0 ? `${pct(m.savings_rate)} of income · avg ${pct(avg.savings_rate)}` : 'no income booked yet'}
          tone={m.saved < 0 ? 'bad' : undefined} onClick={() => nav('/insights/cashflow')} />
        <Stat label="Safe to spend" value={eur(o.plan.safe_to_spend)} sub={`of ${eur(o.plan.income_base)} income base`}
          tone={o.plan.safe_to_spend < 0 ? 'bad' : 'good'} onClick={() => nav('/plan')} />
        <Stat label="Emergency fund" value={`${o.emergency.months.toFixed(1)} mo`} sub={`${eur(o.emergency.cash)} cash · target ${o.emergency.target_months} mo`}
          tone={o.emergency.months < o.emergency.target_months ? 'warn' : undefined} onClick={() => nav('/insights/fi')} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card title="Plan this month" action={<Link to="/plan" className="text-sm text-accent">Open plan</Link>}>
          <div className="mb-2 flex items-baseline justify-between text-sm">
            <span className="text-ink2">Spending budgets</span>
            <span className="tnum"><b>{eur(o.plan.spent)}</b> <span className="text-muted">of {eur(o.plan.budgeted)}</span></span>
          </div>
          <Meter value={o.plan.spent} max={o.plan.budgeted} pace={o.month_progress} />
          {(o.plan.over ?? []).length > 0 ? (
            <div className="mt-4 space-y-2">
              <div className="section-title">Over budget</div>
              {o.plan.over!.map((l) => (
                <div key={l.name} className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-1.5"><Icon name="alert" size={15} className="text-bad" />{l.name}</span>
                  <span className="tnum text-bad">{signed(-l.remaining).replace('+', '')} over</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="mt-3 flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every budget is on track</div>
          )}
          {(o.anomalies ?? []).length > 0 && (
            <div className="mt-4 space-y-1.5">
              <div className="section-title">Running hot</div>
              {o.anomalies!.slice(0, 3).map((a) => (
                <div key={a.category} className="flex items-center justify-between text-sm">
                  <span>{cats.name(a.category)}</span>
                  <span className="text-muted tnum">on pace for {eur(a.projected)} · usually {eur(a.typical)}</span>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card title="Coming up" action={<Link to="/insights/recurring" className="text-sm text-accent">All recurring</Link>}>
          {(o.upcoming ?? []).length === 0 ? (
            <div className="text-sm text-muted">No recurring charges expected in the next two weeks.</div>
          ) : (
            <div className="divide-y divide-line">
              {o.upcoming!.slice(0, 6).map((r) => (
                <div key={r.merchant} className="flex items-center justify-between py-2 text-sm">
                  <div className="min-w-0">
                    <div className="truncate font-medium">{r.merchant}</div>
                    <div className="text-xs text-muted">{shortDate(r.next)} · {cats.path(r.category)}</div>
                  </div>
                  <div className="tnum">{eur(r.amount)}</div>
                </div>
              ))}
            </div>
          )}
          <div className="mt-3 grid grid-cols-2 gap-3 border-t border-line pt-3 text-sm">
            <div><div className="text-muted text-xs">FI progress</div><div className="font-semibold">{pct(o.fi_progress, 1)}</div></div>
            <div><div className="text-muted text-xs">Years to FI at current pace</div><div className="font-semibold">{o.years_to_fi >= 0 ? o.years_to_fi.toFixed(1) : '—'}</div></div>
          </div>
        </Card>
      </div>

      {stale.length > 0 && (
        <Link to="/wealth?update=1" className="card flex items-center gap-3 p-3.5 hover:bg-sunken/50">
          <span className="grid h-9 w-9 place-items-center rounded-full bg-warn/15 text-warn"><Icon name="refresh" /></span>
          <span className="flex-1 text-sm">{stale.length} {stale.length === 1 ? 'balance is' : 'balances are'} over 45 days old — update them so net worth stays true.</span>
          <Icon name="chevronR" className="text-muted" />
        </Link>
      )}

      <Card pad={false} title="Recent" action={
        <div className="flex gap-1">
          <button className="btn-ghost h-8 px-2.5 text-xs" onClick={editor.scan}><Icon name="camera" size={16} />Scan</button>
          <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => editor.open()}><Icon name="plus" size={16} />Add</button>
        </div>}>
        <div className="divide-y divide-line pb-1">
          {(o.recent ?? []).map((t) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
        </div>
        <Link to="/ledger" className="block border-t border-line px-4 py-3 text-center text-sm text-accent">All transactions</Link>
      </Card>

      <button onClick={() => editor.open()} className="fixed bottom-20 right-4 z-20 grid h-14 w-14 place-items-center rounded-full bg-accent text-white shadow-lg sm:hidden" aria-label="Add transaction">
        <Icon name="plus" size={26} />
      </button>
    </div>
  )
}


