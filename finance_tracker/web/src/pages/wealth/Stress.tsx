import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurk, monthLabel } from '../../lib/format'
import type { Shock, StressReport, StressResult } from '../../lib/types'
import { cleanShock, lasts, LEVEL_ORDER, LEVELS, shockEmpty, STRESS_ICONS } from '../../lib/stress'
import { AskCFO, Card, ErrorBox, Loading, NumberInput, Toggle } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'

// ── Stress test ─────────────────────────────────────────────────────
//
// "What if a crisis hit next month?" Each ready-made crisis — a lost job,
// illness, a crash, a rate rise, 2022's prices, a big bill — is played out
// on the cash forecast by the server, so the page, Ask CFO and the MCP tool
// give the same answer. The page answers in three steps: how many crises
// you get through, one line per crisis (tap for the months), and your own.

const toneText = { good: 'text-good', warn: 'text-warn', bad: 'text-bad' } as const
const tonePill = { good: 'bg-good/12 text-good', warn: 'bg-warn/15 text-warn', bad: 'bg-bad/12 text-bad' } as const
const levelBar = { ok: 'bg-good', tight: 'bg-warn/45', pause: 'bg-warn/70', sell: 'bg-warn', short: 'bg-bad' } as const

const whole = (v: number) => eur(Math.round(v))

/** One plain line under a crisis's name: what it makes you do. */
function outcome(r: StressResult): string {
  switch (r.level) {
    case 'short': return `${whole(r.short)} short at worst`
    case 'sell': return `Sell ${whole(r.sold_total)} of investments`
    case 'pause': return `Invest ${whole(r.saving_cut)} less, sell nothing`
    case 'tight': return 'Dips below your emergency fund'
    default: return 'Cash stays above your emergency fund'
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

const NO_SHOCK: Shock = { months: 6 }

export function StressView() {
  const { data, isLoading, error } = useQuery({ queryKey: ['stress'], queryFn: () => api.get<StressReport>('/wealth/stress') })
  const [open, setOpen] = useState<string | null>(null)
  const [building, setBuilding] = useState(false)
  const [shock, setShock] = useState<Shock>(NO_SHOCK)
  const builder = useRef<HTMLDivElement>(null)
  const sent = useDebounced(cleanShock(shock))
  const custom = useQuery({
    queryKey: ['stress', 'custom', sent],
    queryFn: () => api.post<StressReport>('/wealth/stress', sent),
    enabled: building && !shockEmpty(sent),
    placeholderData: (prev) => prev,
  })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />

  // "Change the numbers" on a crisis: its shock becomes the start of yours.
  const tweak = (r: StressResult) => {
    setShock({ ...cleanShock(r.shock), months: r.shock.months ?? 0 }) // 0 = all along
    setBuilding(true)
    requestAnimationFrame(() => {
      builder.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
      document.getElementById('stress-first')?.focus({ preventScroll: true })
    })
  }
  const groups = data.groups.map((g) => ({ ...g, items: data.scenarios.filter((r) => r.group === g.id) })).filter((g) => g.items.length > 0)

  return (
    <div className="space-y-4">
      <Verdict rep={data} />

      {groups.map((g) => (
        <section key={g.id} aria-label={g.name}>
          <h3 className="mb-1.5 px-1 text-xs font-semibold uppercase tracking-wide text-muted">{g.name}</h3>
          <div className="card divide-y divide-line overflow-hidden">
            {g.items.map((r) => (
              <Crisis key={r.id} r={r} rep={data} open={open === r.id} toggle={() => setOpen(open === r.id ? null : r.id)} tweak={() => tweak(r)} />
            ))}
          </div>
        </section>
      ))}

      <div ref={builder} className="scroll-mt-20">
        <Builder rep={data} building={building} start={() => setBuilding(true)} close={() => { setBuilding(false); setShock(NO_SHOCK) }}
          shock={shock} set={(p) => setShock((x) => ({ ...x, ...p }))} result={!shockEmpty(sent) ? custom.data?.custom : undefined}
          fetching={custom.isFetching} error={custom.error} />
      </div>

      <HowItWorks rep={data} />
    </div>
  )
}

/** The answer first: how many crises you get through, and how long the money lasts with no pay. */
function Verdict({ rep }: { rep: StressReport }) {
  const all = rep.scenarios
  const count = (l: string) => all.filter((r) => r.level === l).length
  const short = count('short'), sell = count('sell')
  const head = short > 0 ? <>You'd run out of money in <b className="text-bad">{short} of {all.length}</b> crises</>
    : sell > 0 ? <>You'd get through all {all.length} crises — <b className="text-warn">{sell}</b> of them by selling investments</>
    : <>You'd get through all {all.length} crises without selling anything</>
  const rw = rep.runway
  const lowTarget = !!rw && !!rep.floor_months && !!rep.target_months && rep.floor_months < rep.target_months - 0.25
  return (
    <section className="hero p-4 sm:p-6">
      <div className="flex items-start justify-between gap-3">
        <h2 className="min-w-0 text-lg font-semibold leading-snug sm:text-xl">{head}</h2>
        <AskCFO label="" q="Look at my stress test (get_stress_test). Which crisis would hurt me most, and what are the two or three things I should do now to be ready for it?" />
      </div>
      <div className="mt-3 flex h-2.5 overflow-hidden rounded-full bg-sunken" aria-hidden>
        {LEVEL_ORDER.map((l) => count(l) > 0 && <div key={l} className={levelBar[l]} style={{ width: `${(count(l) / all.length) * 100}%` }} />)}
      </div>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink2">
        {LEVEL_ORDER.map((l) => count(l) > 0 && (
          <li key={l} className="flex items-center gap-1.5"><span className={clsx('h-2 w-2 rounded-full', levelBar[l])} />{LEVELS[l].label} <b className="tnum">{count(l)}</b></li>
        ))}
      </ul>

      <div className="mt-5 border-t border-line pt-4">
        <div className="text-sm text-ink2">With no pay at all, your money lasts</div>
        {rw ? (
          <>
            <div className="grad-text mt-0.5 text-3xl font-bold tracking-tight">{rw.reserve_months.toFixed(1)} months</div>
            <div className="mt-2 flex h-2 overflow-hidden rounded-full bg-sunken" aria-hidden>
              <div className="bg-accent" style={{ width: `${(rw.cash_months / Math.max(rw.reserve_months, 0.01)) * 100}%` }} />
              <div className="bg-accent/35 flex-1" />
            </div>
            <div className="mt-1.5 text-xs text-ink2">
              <span className="tnum">{rw.cash_months.toFixed(1)}</span> on cash, <span className="tnum">{Math.max(0, rw.reserve_months - rw.cash_months).toFixed(1)}</span> more by selling what you hold · a month costs <b className="tnum">{eur(rw.burn)}</b>
            </div>
          </>
        ) : <div className="mt-1 text-sm text-muted">— Not enough history yet to know what a usual month costs.</div>}
      </div>

      {/* The target is set on essentials; say when it covers far less of a
          real month than its name suggests. */}
      {lowTarget && (
        <div className="mt-4 flex gap-2 rounded-xl bg-warn/10 p-3 text-sm text-ink">
          <Icon name="alert" size={16} className="mt-0.5 shrink-0 text-warn" />
          <span>
            Your <b className="tnum">{eur(rep.floor)}</b> emergency fund covers <b>{rep.floor_months!.toFixed(1)} months</b> of real spending, not {rep.target_months}.
            {' '}{rep.target_months} months would take <b className="tnum">{eur(rw!.burn * rep.target_months!)}</b>.
          </span>
        </div>
      )}
    </section>
  )
}

/** One crisis as a row; tapped, it opens in place. */
function Crisis({ r, rep, open, toggle, tweak }: { r: StressResult; rep: StressReport; open: boolean; toggle: () => void; tweak: () => void }) {
  const lv = LEVELS[r.level]
  const panel = `stress-${r.id}`
  return (
    <div>
      <button onClick={toggle} aria-expanded={open} aria-controls={panel}
        className={clsx('flex min-h-[56px] w-full items-center gap-3 px-3 py-2.5 text-left transition sm:px-4', open ? 'bg-sunken/60' : 'hover:bg-sunken/40')}>
        <Icon name={STRESS_ICONS[r.id] ?? 'alert'} size={18} className="shrink-0 text-ink2" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">{r.name}</span>
          <span className={clsx('block truncate text-xs tnum', r.level === 'ok' ? 'text-muted' : toneText[lv.tone])}>{outcome(r)}</span>
        </span>
        <span className={clsx('shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold', tonePill[lv.tone])}>{lv.label}</span>
        <Icon name="chevronD" size={16} className={clsx('shrink-0 text-muted transition', open && 'rotate-180')} />
      </button>
      {open && (
        <div id={panel} className="border-t border-line px-3 pb-4 pt-3 sm:px-4">
          <Story r={r} rep={rep} />
          <div className="mt-4 flex flex-wrap gap-2">
            <button className="btn-ghost h-10 px-3 text-sm" onClick={tweak}><Icon name="edit" size={15} />Change the numbers</button>
            <AskCFO q={`Run my "${r.name}" stress test (get_stress_test). How would I get through it, and what should I change now?`} />
          </div>
        </div>
      )}
    </div>
  )
}

/** What happens in one crisis: the plain answer, what it costs, the months, what to do. */
function Story({ r, rep }: { r: StressResult; rep: StressReport }) {
  const lv = LEVELS[r.level]
  // The server's last line restates the two cost figures shown below; the
  // first, what happens, always stays.
  const lines = (r.cost > 0 || r.paper_loss > 0) && r.summary.length > 1 ? r.summary.slice(0, -1) : r.summary
  const rows = (r.months ?? []).map((m) => ({ ...m, sold: m.sold || undefined }))
  const lo = Math.min(0, ...rows.map((m) => Math.min(m.end, m.base)))
  const anySold = rows.some((m) => m.sold)
  return (
    <div>
      {r.blurb && <p className="text-xs text-muted">{r.blurb} Lasts {lasts(r.shock, rep.horizon)}.</p>}
      {lines.length > 0 && <p className={clsx('text-sm font-medium', r.blurb && 'mt-1.5', toneText[lv.tone])}>{lines[0]}</p>}
      {lines.slice(1).map((s) => <p key={s} className="mt-1 text-sm text-ink2">{s}</p>)}

      <div className="mt-3 grid grid-cols-2 gap-2">
        <div className="rounded-xl bg-sunken/70 p-3">
          <div className="text-xs text-muted">Lost for good</div>
          <div className={clsx('text-lg font-bold tnum', r.cost > 0 ? 'text-bad' : 'text-good')}>{whole(r.cost)}</div>
          <div className="text-[11px] leading-tight text-muted">pay missed, extra costs, selling low</div>
        </div>
        <div className="rounded-xl bg-sunken/70 p-3">
          <div className="text-xs text-muted">Temporary drop</div>
          <div className="text-lg font-bold tnum text-ink2">{whole(r.paper_loss)}</div>
          <div className="text-[11px] leading-tight text-muted">back if prices recover</div>
        </div>
      </div>

      {rows.length > 0 && (
        <>
          <div className="mt-4 text-xs font-semibold uppercase tracking-wide text-muted">Your cash, month by month</div>
          <div className="mt-1 h-48 sm:h-60">
            <ResponsiveContainer>
              <ComposedChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="month" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={24} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={[lo * 1.1, 'auto']} />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={monthLabel(label as string, true)} rows={[
                    { color: 'var(--s1)', label: 'Your plan', value: eur(payload[0].payload.base) },
                    { color: 'var(--s3)', label: 'In this crisis', value: eur(payload[0].payload.end), bold: true },
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
          <div className="mt-1"><Legend items={[{ color: 'var(--s1)', label: 'Your plan' }, { color: 'var(--s3)', label: 'In this crisis' }, ...(anySold ? [{ color: 'var(--s5)', label: 'Sold' }] : []), ...(rep.floor > 0 ? [{ color: 'rgb(var(--warn))', label: `Emergency fund ${eurk(rep.floor)}` }] : [])]} /></div>
        </>
      )}

      {(r.sold?.length || r.levers?.length) ? (
        <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
          {!!r.sold?.length && (
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-muted">You'd sell</div>
              <ul className="mt-1.5 space-y-1 text-sm">
                {r.sold.map((s) => (
                  <li key={s.key} className="flex items-baseline gap-2">
                    <span className="flex-1">{s.label}{s.net !== s.amount && <span className="text-xs text-muted"> · {whole(s.net)} after tax</span>}</span>
                    <span className="tnum font-medium">{whole(s.amount)}</span>
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
                    <span className="tnum font-medium text-good">+{whole(l.amount)}{l.monthly && <span className="text-xs font-normal text-muted">/mo</span>}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      ) : null}
    </div>
  )
}

type NumKey = { [K in keyof Shock]-?: Shock[K] extends number | undefined ? K : never }[keyof Shock]
type Field = { key: NumKey; label: string; unit: string; hint?: string; integer?: boolean }
// The four that make most crises; the rest wait behind "More".
const MAIN: Field[] = [
  { key: 'months', label: 'Lasts', unit: 'months', hint: '0 = all along', integer: true },
  { key: 'income_pct', label: 'Pay change', unit: '%', hint: '−100 = no pay' },
  { key: 'one_off', label: 'Unexpected bill', unit: '€', hint: 'next month' },
  { key: 'stocks_pct', label: 'Stocks', unit: '%', hint: '−30 = fall by 30%' },
]
const MORE: Field[] = [
  { key: 'benefit', label: 'Still coming in', unit: '€/mo', hint: 'benefit, sick pay' },
  { key: 'everyday_pct', label: 'Everyday spending', unit: '%' },
  { key: 'fixed_pct', label: 'Bills and obligations', unit: '%' },
  { key: 'rate_pp', label: 'Euribor', unit: 'points', hint: '2 = +2 points' },
  { key: 'crypto_pct', label: 'Crypto', unit: '%' },
  { key: 'pension_pct', label: 'Pension funds', unit: '%' },
  { key: 'property_pct', label: 'Property', unit: '%' },
]

function Builder({ rep, building, start, close, shock, set, result, fetching, error }: {
  rep: StressReport; building: boolean; start: () => void; close: () => void
  shock: Shock; set: (p: Partial<Shock>) => void; result?: StressResult; fetching: boolean; error: unknown
}) {
  const extra = MORE.some((f) => !!shock[f.key]) || !!shock.pause_saving || !!shock.pause_goals
  const [more, setMore] = useState(false)
  // Closed and opened again, it starts short.
  useEffect(() => { if (!building) setMore(false) }, [building])
  if (!building) {
    return (
      <Card icon="spark" color="var(--s3)" title="Try your own crisis">
        <p className="text-sm text-ink2">Change your pay, a bill, the markets or rates and see what it does to your money.</p>
        <button className="btn-primary mt-3 h-10 px-4 text-sm" onClick={start}>Start</button>
      </Card>
    )
  }
  const field = (f: Field) => (
    <label key={f.key} className="block min-w-0">
      <span className="text-xs text-muted">{f.label} <span className="text-muted/80">({f.unit})</span></span>
      <NumberInput id={f.key === MORE[0].key ? 'stress-more' : f.key === MAIN[0].key ? 'stress-first' : undefined} className="input tnum mt-1 h-10 w-full" value={shock[f.key]} integer={f.integer} onChange={(v) => set({ [f.key]: v })}
        placeholder={f.hint ?? '0'} aria-label={`${f.label}, ${f.unit}`} />
    </label>
  )
  return (
    <Card icon="spark" color="var(--s3)" title="Try your own crisis" action={<button className="btn-ghost h-10 px-3 text-sm" onClick={close}>Close</button>}>
      <div className="grid grid-cols-2 gap-x-3 gap-y-2.5 sm:grid-cols-4">{MAIN.map(field)}</div>
      {more || extra ? (
        <>
          <div className="mt-2.5 grid grid-cols-2 gap-x-3 gap-y-2.5 sm:grid-cols-4">{MORE.map(field)}</div>
          <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:gap-6">
            <Toggle checked={!!shock.pause_saving} onChange={(v) => set({ pause_saving: v })} label="Pause investing" />
            <Toggle checked={!!shock.pause_goals} onChange={(v) => set({ pause_goals: v })} label="Put off wish-list goals" />
          </div>
        </>
      ) : (
        <button className="btn-ghost mt-2 h-10 px-2 text-sm text-accent" onClick={() => { setMore(true); requestAnimationFrame(() => document.getElementById('stress-more')?.focus()) }}>More options<Icon name="chevronD" size={14} /></button>
      )}

      <div className="mt-4 border-t border-line pt-3" aria-live="polite">
        {error ? <ErrorBox error={error} />
          : result ? (
            <>
              <div className="mb-2 flex items-center gap-2">
                <span className="text-sm font-semibold">Your crisis</span>
                <span className={clsx('rounded-full px-2 py-0.5 text-xs font-semibold', tonePill[LEVELS[result.level].tone])}>{LEVELS[result.level].label}</span>
                {fetching && <span className="text-xs text-muted">updating…</span>}
              </div>
              <Story r={result} rep={rep} />
            </>
          ) : <p className="text-sm text-muted">{fetching ? 'Working it out…' : 'Fill in at least one change to see what happens.'}</p>}
      </div>
    </Card>
  )
}

/** The fine print, folded away: what counts as a reserve and how the model works. */
function HowItWorks({ rep }: { rep: StressReport }) {
  return (
    <details className="card group p-4 text-sm">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-2 font-medium text-ink2">
        How this works<Icon name="chevronD" size={16} className="text-muted transition group-open:rotate-180" />
      </summary>
      <div className="mt-3 space-y-3 text-xs text-ink2">
        <p>
          <b>Counted:</b> Cash <span className="tnum">{eur(rep.cash)}</span>
          {rep.reserves.map((r) => <span key={r.key}> · {r.label} <span className="tnum">{eur(r.net)}</span>{r.net !== r.value && ' (after tax)'}</span>)}.
          {' '}Crises sell investments first, then crypto, then the III pillar.
        </p>
        {(rep.locked > 0 || rep.real_assets > 0) && (
          <p><b>Not counted:</b> {[rep.locked > 0 && `${eur(rep.locked)} in the II pillar (locked until retirement)`, rep.real_assets > 0 && `${eur(rep.real_assets)} in property and the car (can't pay next month's bills)`].filter(Boolean).join(' and ')}.</p>
        )}
        <p><b>Outcomes, best to worst:</b> Fine — cash stays above your emergency fund. Tight — it dips below. Invest less — pausing investing covers it. Must sell — you'd sell investments. Runs out — even selling isn't enough.</p>
        <ul className="space-y-1">{rep.notes.map((n) => <li key={n}>· {n}</li>)}</ul>
      </div>
    </details>
  )
}
