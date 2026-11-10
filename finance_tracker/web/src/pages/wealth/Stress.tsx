import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurk, monthLabel } from '../../lib/format'
import type { Shock, StressReport, StressResult } from '../../lib/types'
import { cleanShock, lasts, LEVELS, shockEmpty, STRESS_ICONS } from '../../lib/stress'
import { AskCFO, Card, ErrorBox, Loading, NumberInput, Toggle } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'

// ── Stress test ─────────────────────────────────────────────────────
//
// "What if a crisis hit next month?" Each ready-made crisis — a lost job,
// illness, a crash, a rate rise, 2022's prices, a big bill — is played out
// on the cash forecast by the server, so the page, Ask CFO and the MCP tool
// give the same answer. Build your own reruns it with your numbers.

const toneText = { good: 'text-good', warn: 'text-warn', bad: 'text-bad' } as const

/** The one figure a tile leads with. */
function headline(r: StressResult): string {
  switch (r.level) {
    case 'short': return `${eur(r.short)} short`
    case 'sell': return `Sell ${eur(r.sold_total)}`
    // Cash holds up: the story is what the drop takes, if there is one.
    default: return r.market_hit > 0 ? `Loses ${eur(r.market_hit)}` : `Lowest ${eur(r.lowest)}`
  }
}

function useDebounced<T>(v: T, ms = 400): T {
  const [d, setD] = useState(v)
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms)
    return () => clearTimeout(t)
  }, [v, ms])
  return d
}

export function StressView() {
  const { data, isLoading, error } = useQuery({ queryKey: ['stress'], queryFn: () => api.get<StressReport>('/wealth/stress') })
  const [sel, setSel] = useState('job_loss')
  const [shock, setShock] = useState<Shock>({ months: 6 })
  const sent = useDebounced(cleanShock(shock))
  const custom = useQuery({
    queryKey: ['stress', 'custom', sent],
    queryFn: () => api.post<StressReport>('/wealth/stress', sent),
    enabled: !shockEmpty(sent),
    placeholderData: (prev) => prev,
  })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />

  const mine = !shockEmpty(sent) ? custom.data?.custom : undefined
  const shown = sel === 'custom' ? mine : data.scenarios.find((s) => s.id === sel)
  const set = (p: Partial<Shock>) => { setShock((x) => ({ ...x, ...p })); setSel('custom') }
  const rw = data.runway
  const reserveNet = data.reserves.reduce((a, r) => a + r.net, 0)

  return (
    <div className="space-y-4">
      {/* The plain question first: no income at all, how long? */}
      <section className="hero p-4 sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-sm text-ink2">With no income at all, your money lasts</div>
            <div className="grad-text mt-1 text-4xl font-bold tracking-tight">{rw ? `${rw.reserve_months.toFixed(1)} months` : '—'}</div>
            <div className="mt-1 text-sm text-ink2">
              {rw ? <>{rw.cash_months.toFixed(1)} on cash alone, the rest by selling investments · a month without pay costs <b className="tnum">{eur(rw.burn)}</b> with saving stopped</>
                : 'Not enough history yet to know what a usual month costs.'}
            </div>
          </div>
          <AskCFO q="Look at my stress test (get_stress_test). Which crisis would hurt me most, and what are the two or three things I should do now to be ready for it?" />
        </div>
        <div className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><div className="text-xs text-muted">Cash</div><div className="font-semibold tnum">{eur(data.cash)}</div></div>
          {data.reserves.map((r) => (
            <div key={r.key}><div className="text-xs text-muted">{r.label}{r.net !== r.value && ' (after tax)'}</div><div className="font-semibold tnum">{eur(r.net)}</div></div>
          ))}
          {!data.reserves.length && <div><div className="text-xs text-muted">Reserves to sell</div><div className="font-semibold tnum">{eur(reserveNet)}</div></div>}
        </div>
        {(data.locked > 0 || data.real_assets > 0) && (
          <p className="mt-3 text-xs text-muted">
            Not counted: {[data.locked > 0 && `${eur(data.locked)} in the II pillar (locked until retirement)`, data.real_assets > 0 && `${eur(data.real_assets)} in property and the car (can't pay next month's bills)`].filter(Boolean).join(' and ')}.
          </p>
        )}
      </section>

      {/* Every crisis at a glance; tap one for the months. */}
      <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-4">
        {[...data.scenarios, ...(mine ? [mine] : [])].map((r) => {
          const lv = LEVELS[r.level]
          return (
            <button key={r.id} onClick={() => setSel(r.id)} aria-pressed={sel === r.id}
              className={clsx('card min-h-[44px] p-3 text-left transition', sel === r.id ? 'ring-2 ring-accent' : 'hover:bg-sunken/50')}>
              <div className="flex items-center gap-2">
                <Icon name={STRESS_ICONS[r.id] ?? 'alert'} size={16} className="shrink-0 text-ink2" />
                <span className="min-w-0 flex-1 truncate text-sm font-semibold">{r.name}</span>
                <span className={clsx('shrink-0 text-xs font-medium', toneText[lv.tone])}>{lv.label}</span>
              </div>
              <div className={clsx('mt-1.5 text-lg font-bold tnum', toneText[lv.tone])}>{headline(r)}</div>
              <div className="text-xs text-muted">{r.cost > 0 ? <>costs <span className="tnum">{eur(r.cost)}</span> of net worth</> : 'no lasting cost'}</div>
            </button>
          )
        })}
      </div>

      {shown ? <Detail r={shown} rep={data} /> : sel === 'custom' && (
        <Card><div className="text-sm text-muted">{custom.isFetching ? 'Working it out…' : custom.error ? <ErrorBox error={custom.error} /> : 'Set at least one change below to see your crisis.'}</div></Card>
      )}

      <Builder shock={shock} set={set} reset={() => { setShock({ months: 6 }); setSel('job_loss') }} rep={data} error={custom.error} />

      <ul className="space-y-1 px-1 text-xs text-muted">{data.notes.map((n) => <li key={n}>· {n}</li>)}</ul>
    </div>
  )
}

function Detail({ r, rep }: { r: StressResult; rep: StressReport }) {
  const lv = LEVELS[r.level]
  const rows = (r.months ?? []).map((m) => ({ ...m, sold: m.sold || undefined }))
  const lo = Math.min(0, ...rows.map((m) => Math.min(m.end, m.base)))
  const anySold = rows.some((m) => m.sold)
  return (
    <Card icon={STRESS_ICONS[r.id] ?? 'alert'} color={`var(--hue-${lv.tone})`}
      title={<span className="flex flex-wrap items-baseline gap-x-2">{r.name}<span className={clsx('text-xs font-medium', toneText[lv.tone])}>{lv.label}</span></span>}
      action={<AskCFO q={`Run my "${r.name}" stress test (get_stress_test${r.id === 'custom' ? ' with my shock: ' + JSON.stringify(r.shock) : ''}). How would I get through it, and what should I change now?`} />}>
      {r.blurb && <p className="text-sm text-ink2">{r.blurb}</p>}
      <ul className={clsx('space-y-1 text-sm', r.blurb && 'mt-2')}>
        {r.summary.map((s, i) => <li key={i} className={clsx('flex gap-2', i === 0 && clsx('font-medium', toneText[lv.tone]))}><span aria-hidden>·</span><span>{s}</span></li>)}
      </ul>

      <div className="mt-3 h-56 sm:h-64">
        <ResponsiveContainer>
          <ComposedChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="month" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
            <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={[lo * 1.1, 'auto']} />
            <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
              <TooltipBox title={monthLabel(label as string, true)} rows={[
                { color: 'var(--s1)', label: 'At the plan', value: eur(payload[0].payload.base) },
                { color: 'var(--s3)', label: r.name, value: eur(payload[0].payload.end), bold: true },
                ...(payload[0].payload.sold ? [{ color: 'var(--s5)', label: 'Sold that month', value: eur(payload[0].payload.sold) }] : []),
              ]} />) : null} />
            <ReferenceLine y={0} stroke="rgb(var(--bad))" strokeOpacity={0.6} />
            {rep.floor > 0 && <ReferenceLine y={rep.floor} stroke="rgb(var(--warn))" strokeDasharray="4 3" />}
            {anySold && <Bar dataKey="sold" fill="var(--s5)" radius={[3, 3, 0, 0]} isAnimationActive={false} />}
            <Line dataKey="base" stroke="var(--s1)" strokeWidth={2} dot={false} strokeOpacity={0.55} isAnimationActive={false} />
            <Line dataKey="end" stroke="var(--s3)" strokeWidth={2.5} dot={false} isAnimationActive={false} />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'Cash at the plan' }, { color: 'var(--s3)', label: 'In this crisis' }, ...(anySold ? [{ color: 'var(--s5)', label: 'Sold to cover' }] : []), { color: 'rgb(var(--warn))', label: `Emergency target ${eurk(rep.floor)}` }]} /></div>

      <div className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <div><div className="text-xs text-muted">Lasts</div><div className="font-semibold">{lasts(r.shock, rep.horizon)}</div></div>
        <div><div className="text-xs text-muted">Worst month</div><div className={clsx('font-semibold tnum', r.monthly_gap < 0 && 'text-bad')}>{r.monthly_gap < 0 ? '−' : '+'}{eur(Math.abs(r.monthly_gap))}</div></div>
        <div><div className="text-xs text-muted">Net worth in {rep.horizon} months</div><div className="font-semibold tnum">{eur(r.net_worth_end)}</div></div>
        <div><div className="text-xs text-muted">At the plan</div><div className="font-semibold tnum text-ink2">{eur(r.base_net_worth_end)}</div></div>
      </div>

      {(r.sold?.length || r.levers?.length) ? (
        <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          {!!r.sold?.length && (
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-muted">What you'd sell</div>
              <ul className="mt-1.5 space-y-1 text-sm">
                {r.sold.map((s) => (
                  <li key={s.key} className="flex items-baseline gap-2">
                    <span className="flex-1">{s.label}{s.net !== s.amount && <span className="text-xs text-muted"> · brings {eur(s.net)} after tax</span>}</span>
                    <span className="tnum font-medium">{eur(s.amount)}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {!!r.levers?.length && (
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-muted">What would help</div>
              <ul className="mt-1.5 space-y-1 text-sm">
                {r.levers.map((l) => (
                  <li key={l.label} className="flex items-baseline gap-2">
                    <span className="flex-1">{l.label}</span>
                    <span className="tnum font-medium text-good">+{eur(l.amount)}{l.monthly && <span className="text-xs font-normal text-muted">/mo</span>}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      ) : null}
    </Card>
  )
}

type NumKey = { [K in keyof Shock]-?: Shock[K] extends number | undefined ? K : never }[keyof Shock]
const FIELDS: { key: NumKey; label: string; unit: string; hint?: string; integer?: boolean }[] = [
  { key: 'months', label: 'Lasts', unit: 'months', hint: '0 = the whole period', integer: true },
  { key: 'income_pct', label: 'Pay', unit: '%', hint: '−100 = no pay' },
  { key: 'benefit', label: 'Still coming in', unit: '€/mo', hint: 'benefit or sick pay' },
  { key: 'everyday_pct', label: 'Everyday spending', unit: '%' },
  { key: 'fixed_pct', label: 'Obligations', unit: '%' },
  { key: 'one_off', label: 'Unexpected bill', unit: '€', hint: 'next month' },
  { key: 'rate_pp', label: 'Euribor', unit: 'points' },
  { key: 'stocks_pct', label: 'Stocks', unit: '%' },
  { key: 'crypto_pct', label: 'Crypto', unit: '%' },
  { key: 'pension_pct', label: 'Pension funds', unit: '%' },
  { key: 'property_pct', label: 'Property', unit: '%' },
]

function Builder({ shock, set, reset, rep, error }: { shock: Shock; set: (p: Partial<Shock>) => void; reset: () => void; rep: StressReport; error: unknown }) {
  return (
    <Card icon="spark" color="var(--s3)" title="Build your own" action={<button className="btn-ghost h-8 px-2 text-xs" onClick={reset}>Reset</button>}>
      <div className="text-xs text-muted">Start from</div>
      <div className="mt-1 flex flex-wrap gap-1.5">
        {rep.scenarios.map((s) => <button key={s.id} className="chip hover:bg-sunken" onClick={() => set({ ...Object.fromEntries(FIELDS.map((f) => [f.key, undefined])), pause_saving: false, pause_goals: false, ...s.shock })}>{s.name}</button>)}
      </div>
      <div className="mt-3 grid grid-cols-2 gap-x-3 gap-y-2.5 sm:grid-cols-3 lg:grid-cols-4">
        {FIELDS.map((f) => (
          <label key={f.key} className="block min-w-0">
            <span className="text-xs text-muted">{f.label} <span className="text-muted/80">({f.unit})</span></span>
            <NumberInput className="input tnum mt-1 h-9 w-full" value={shock[f.key]} integer={f.integer} onChange={(v) => set({ [f.key]: v })}
              placeholder={f.hint ?? '0'} aria-label={`${f.label}, ${f.unit}`} />
          </label>
        ))}
      </div>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:gap-6">
        <Toggle checked={!!shock.pause_saving} onChange={(v) => set({ pause_saving: v })} label="Pause saving and investing" />
        <Toggle checked={!!shock.pause_goals} onChange={(v) => set({ pause_goals: v })} label="Put off wish-list goals" />
      </div>
      {!!error && <div className="mt-3"><ErrorBox error={error} /></div>}
      <p className="mt-3 text-xs text-muted">Starts next month. Changes to pay and spending last as long as the crisis; market drops happen at once and a rate rise stays.</p>
    </Card>
  )
}
