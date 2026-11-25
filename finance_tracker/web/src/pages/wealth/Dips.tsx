import { useContext, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import clsx from 'clsx'
import { CartesianGrid, ComposedChart, Line, ReferenceDot, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, pct, shortDate } from '../../lib/format'
import { amount, PLAN_WORDS, awayText, closeness, nextRung, orderCost, px, rungColor, SERIES, TONE, yearTicks, ZONE } from '../../lib/dips'
import type { DipPlanName, DipFund, DipFundSetting, DipLevel, DipPlan, DipPoint } from '../../lib/types'
import { DipToday, PlanCtx, SAFETY_NET } from './DipToday'
import { Card, Empty, ErrorBox, Field, Loading, NumberInput, Segmented, Sheet, useToast } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'

// ── Dip plan ────────────────────────────────────────────────────────
//
// A crash plan written down before the crash: for each ETF a ladder of
// limit prices where a cash reserve goes in, worked out by the server from
// daily prices (moving averages, unfilled gaps, the all-time high). The page
// answers, in order: where each fund stands now, the limit orders to have
// at the broker, the picture, and the rules the plan runs on. It never
// moves money — the orders are placed at the broker.

const toneText = { good: 'text-good', warn: 'text-warn', bad: 'text-bad', muted: 'text-muted' } as const
const tonePill = { good: 'bg-good/12 text-good', warn: 'bg-warn/15 text-warn', bad: 'bg-bad/12 text-bad', muted: 'bg-sunken text-ink2' } as const

/** The ETF plan, or the single-stock bets at Revolut, or bitcoin, on the same ladder. */
export function Dips({ plan = 'funds' }: { plan?: DipPlanName }) {
  return <PlanCtx.Provider value={plan}><DipsPage plan={plan} /></PlanCtx.Provider>
}

function DipsPage({ plan }: { plan: DipPlanName }) {
  const { data, isLoading, error } = useQuery({ queryKey: ['dips', plan], queryFn: () => api.get<DipPlan>('/market/dips', plan === 'funds' ? undefined : { plan }), staleTime: 10 * 60_000 })
  const [pick, setPick] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  if (isLoading) return <Loading label="Reading the markets…" />
  if (error || !data) return <ErrorBox error={error} />

  const funds = data.funds
  const fund = funds.find((f) => f.symbol === pick) ?? funds[0]
  const words = PLAN_WORDS[plan]
  const edit = <button className="btn-ghost h-10 px-3 text-sm" onClick={() => setEditing(true)}>{words.edit}</button>

  return (
    <div className="space-y-4">
      {funds.length === 0 ? (
        <Card title="Dip plan" icon="down" action={edit}>
          <Empty title="No funds on the plan" icon="down">Add the ETFs you buy and the cash you keep aside for falls.</Empty>
        </Card>
      ) : (
        <>
          {/* The chart first (owner's ask): the picture of where the price is
              against its averages, then what to do, then the detail. */}
          {fund && <FundTop f={fund} funds={funds} onPick={setPick} />}
          {/* Two independent columns, so a short card never leaves a hole
              beside a tall one: what to do and the ladder on the left, where
              each fund stands, the rules and the gaps on the right. */}
          <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
            <div className="space-y-4">
              <DipToday plan={data} onSetup={() => setEditing(true)} />
              {fund && !fund.error && <Ladder f={fund} />}
            </div>
            <div className="space-y-4">
              <Standing funds={funds} current={fund?.symbol} onPick={setPick} action={edit} title={words.stands} />
              {fund && !fund.error && <Gaps f={fund} />}
              <Rules plan={plan} />
            </div>
          </div>
        </>
      )}
      <SettingsSheet open={editing} onClose={() => setEditing(false)} funds={data.settings.funds} />
    </div>
  )
}

/** One row per fund: price, drop from the high, zone and the next rung. */
function Standing({ funds, current, onPick, action, title }: { funds: DipFund[]; current?: string; onPick: (s: string) => void; action: React.ReactNode; title: string }) {
  return (
    <Card title={title} icon="down" action={action} pad={false}>
      <ul className="divide-y divide-line">
        {funds.map((f) => {
          const z = ZONE[f.zone]
          const next = nextRung(f)
          return (
            <li key={f.symbol}>
              <button onClick={() => onPick(f.symbol)} aria-pressed={f.symbol === current}
                className={clsx('flex w-full items-center gap-3 px-4 py-3 text-left transition hover:bg-sunken/60', f.symbol === current && 'bg-sunken/60')}>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-ink">{f.symbol.split('.')[0]}</span>
                    {!f.error && <span className={clsx('rounded-full px-2 py-0.5 text-[11px] font-semibold', f.todo.kind === 'buy' ? tonePill.good : tonePill[z.tone])}>{f.todo.kind === 'buy' ? 'Buy now' : z.label}</span>}
                    {f.plan_broken && <span className="rounded-full bg-bad/12 px-2 py-0.5 text-[11px] font-semibold text-bad">Plan broken</span>}
                  </div>
                  <div className="truncate text-xs text-muted">
                    {f.error ? 'Prices unavailable right now' : f.todo.kind === 'buy' ? `Buy ${eur(f.todo.amount ?? 0)} now` : next ? `Next: ${px(next.price, f.currency)} · ${awayText(next)}` : 'Every rung reached'}
                  </div>
                </div>
                {!f.error && (
                  <div className="text-right">
                    <div className="tnum font-semibold text-ink">{px(f.price, f.currency)}</div>
                    <div className={clsx('tnum text-xs', f.drawdown < -0.1 ? 'text-bad' : f.drawdown < -0.03 ? 'text-warn' : 'text-muted')}>{pct(f.drawdown, 1)} from high</div>
                  </div>
                )}
              </button>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

const Dot = ({ c }: { c: string }) => <span className="mr-1.5 inline-block h-2.5 w-2.5 rounded-full align-[-1px]" style={{ background: c }} aria-hidden />

/** The plan's rules. The levels follow each day's prices, so there is no
 *  monthly check to do: the page is current whenever it is opened. */
function Rules({ plan }: { plan: DipPlanName }) {
  return (
    <Card title="The plan" icon="check">
      {plan === 'bets' && (
        <p className="mb-3 rounded-xl border border-warn/40 bg-warn/8 px-3 py-2 text-xs text-ink2">
          <b className="text-ink">Same ladder, riskier ground.</b> The rules were written for index funds, which have always come back. A single company can fall and stay down, so “plan broken” matters more here, and the bets’ reserve comes after the funds’ reserve.
        </p>
      )}
      {plan === 'crypto' && (
        <p className="mb-3 rounded-xl border border-warn/40 bg-warn/8 px-3 py-2 text-xs text-ink2">
          <b className="text-ink">Same ladder, wilder ground.</b> The rules were written for index funds. Bitcoin trades every day, so its 2- and 4-year averages count calendar days, and it has fallen more than 75% before: −44% has not been its floor. Its reserve comes after the funds’ and bets’ reserves.
        </p>
      )}
      <ul className="space-y-2 text-sm text-ink2">
        {plan === 'funds' && <li><b className="text-ink">Always:</b> the same amount every month into the world fund, falls or not.</li>}
        <li><Dot c={SERIES.sma200} /><b className="text-ink">−10% under the 200-day average:</b> nothing extra.</li>
        <li><Dot c={SERIES.sma2y} /><b className="text-ink">2-year average</b> (and the gap beside it): first third of the reserve.</li>
        <li><Dot c={SERIES.sma4y} /><b className="text-ink">4-year average</b> (and its gap): second third.</li>
        <li><Dot c={SERIES.deep} /><b className="text-ink">−44% from the high:</b> last third.</li>
      </ul>
      <p className="mt-3 text-xs text-ink2">The prices follow each day’s close. When a limit here differs from the one at your broker, move the order. The reserve is savings above the <Link to={SAFETY_NET} className="text-accent underline-offset-2 hover:underline">emergency fund</Link>, never the fund itself.</p>
      <p className="mt-2 text-xs text-muted">The plan stops fitting if prices break both averages and both gaps and stay under the 4-year average for over half a year (as in 2001). Then it is rewritten. These are your plan's rules, not advice.</p>
    </Card>
  )
}

/** Share counts to 2 decimals, a fraction of a coin to its first 4 significant digits (0.008781). */
const units = (v: number) => v >= 1 ? v.toLocaleString('en-GB', { maximumFractionDigits: 2 }) : v.toLocaleString('en-GB', { maximumSignificantDigits: 4 })

/** The fund picker and its chart, at the top of the page. */
function FundTop({ f, funds, onPick }: { f: DipFund; funds: DipFund[]; onPick: (s: string) => void }) {
  return (
    <div className="space-y-3">
      {funds.length > 1 && (
        <Segmented size="sm" value={f.symbol} onChange={onPick} options={funds.map((x) => ({ value: x.symbol, label: x.symbol.split(/[.-]/)[0] }))} />
      )}
      {f.error ? <ErrorBox error={new Error(f.error)} /> : <PriceChart f={f} />}
    </div>
  )
}

/** The limit orders to have at the broker, top rung first. */
function Ladder({ f }: { f: DipFund }) {
  const plan = useContext(PlanCtx)
  const z = ZONE[f.zone]
  return (
    <Card title={<span>{f.name || f.symbol} <span className="font-normal text-muted">· {f.symbol}</span></span>} icon="target">
      <div className="grid grid-cols-3 gap-2 text-center">
        <Figure label="Price" value={px(f.price, f.currency)} sub={shortDate(f.as_of)} />
        <Figure label="From high" value={pct(f.drawdown, 1)} sub={px(f.high, f.currency)} tone={f.drawdown < -0.1 ? 'bad' : f.drawdown < -0.03 ? 'warn' : undefined} />
        <Figure label="Reserve left" value={eur(f.left)} sub={`of ${eur(f.reserve)}`} />
      </div>
      <p className={clsx('mt-3 text-sm', f.todo.kind === 'buy' ? 'font-semibold text-good' : toneText[z.tone] === 'text-muted' ? 'text-ink2' : toneText[z.tone])}>{f.todo.kind === 'setup' ? z.say : f.todo.text}</p>
      {f.plan_broken && (
        <p className="mt-2 rounded-xl border border-bad/30 bg-bad/5 px-3 py-2 text-sm text-bad">{f.days_under_4y} trading days under the 4-year average: over half a year. The plan no longer fits — rewrite it before buying more.</p>
      )}
      <ol className="mt-3 divide-y divide-line">
        {f.levels.map((l) => <Rung key={l.key} l={l} f={f} />)}
      </ol>
      {f.note && <p className="mt-2 text-xs text-muted">{f.note}</p>}
      {f.reserve === 0 && <p className="mt-2 text-xs text-muted">Set a reserve under “{PLAN_WORDS[plan].edit}” to see the amounts and share counts for each rung.</p>}
    </Card>
  )
}

function Figure({ label, value, sub, tone }: { label: string; value: string; sub?: string; tone?: 'bad' | 'warn' }) {
  return (
    <div className="min-w-0 rounded-xl bg-sunken px-2 py-2">
      <div className="text-[11px] font-semibold uppercase tracking-wide text-muted">{label}</div>
      <div className={clsx('tnum truncate text-base font-bold', tone === 'bad' ? 'text-bad' : tone === 'warn' ? 'text-warn' : 'text-ink')}>{value}</div>
      {sub && <div className="tnum truncate text-[11px] text-muted">{sub}</div>}
    </div>
  )
}

const STATUS: Record<DipLevel['status'], { label: string; cls: string } | undefined> = {
  watch: undefined,
  order: { label: 'Limit order', cls: 'bg-sunken text-ink2' },
  buy: { label: 'Buy now', cls: 'bg-good/12 text-good' },
  bought: { label: 'Bought', cls: 'bg-accent/12 text-accent' },
  hold: { label: 'Hold: plan broken', cls: 'bg-bad/12 text-bad' },
}

function Rung({ l, f }: { l: DipLevel; f: DipFund }) {
  const currency = f.currency
  const money = l.tranche > 0
  const st = STATUS[l.status]
  return (
    <li className="py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2 text-sm font-semibold text-ink"><span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: rungColor(l) }} aria-hidden />{l.label}
            {st && <span className={clsx('rounded-full px-2 py-0.5 text-[11px] font-semibold', st.cls)}>{st.label}</span>}
          </div>
          <div className="text-xs text-ink2">{l.action}</div>
        </div>
        <div className="text-right">
          <div className={clsx('tnum text-lg font-bold', money ? TONE[closeness(l)].text : 'text-muted')}>{px(l.price, currency)}</div>
          <div className={clsx('tnum text-xs', l.reached ? 'font-semibold text-good' : 'text-muted')}>{awayText(l)}</div>
        </div>
      </div>
      <div className="mt-1 space-y-0.5 text-xs text-muted">
        {l.gap && (
          <div>Gap of {shortDate(l.gap.date)} at {px(l.gap.low, currency)}–{px(l.gap.high, currency)}{money ? ': the limit sits at its top, where a fall first meets buyers' : ''} · average {px(l.basis, currency)}</div>
        )}
        {money && l.status === 'buy' && (
          <div className="text-ink2">Buy {eur(l.tranche)} now: ≈ <b className="tnum text-ink">{Math.floor((l.tranche * f.fx) / f.price)}</b> shares at today’s {px(f.price, currency)}</div>
        )}
        {money && l.status === 'order' && (l.shares > 0
          ? <div className="text-ink2">Limit buy <b className="tnum text-ink">{l.shares}</b> × {px(l.price, currency)} = <span className="tnum">{amount(orderCost(l), currency)}</span> of the {eur(l.tranche)} third{l.exact && l.exact !== l.shares ? ` (${l.exact} with fractions)` : ''}</div>
          : <div>{eur(l.tranche)} buys less than one share at this price{l.exact ? ` (${l.exact} with fractions)` : ''}.</div>)}
        {l.status === 'bought' && <div className="text-ink2">Bought <b className="tnum text-ink">{l.bought_shares}</b> × {px(l.bought_price, currency)} on {shortDate(l.bought_on ?? '')}, from your trades. The plan asks again only in the next fall.</div>}
      </div>
    </li>
  )
}

/** Price against the three averages, with the money rungs as dashed lines. */
function PriceChart({ f }: { f: DipFund }) {
  const rungs = f.levels
  const lo = Math.min(...f.chart.map((p) => p.close), ...rungs.map((l) => l.price))
  const hi = Math.max(...f.chart.map((p) => p.close))
  const pad = (hi - lo) * 0.05
  // whole units for big prices; a €4 fund keeps its decimals (else €2, €2)
  const fmt = (v: number) => (hi >= 100 ? px(v, f.currency).replace(/\.\d+$/, '') : px(v, f.currency))
  return (
    <Card title={`${f.name || f.symbol} · three years against the averages`} icon="trend">
      <div className="h-80 lg:h-[28rem]">
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={f.chart} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="date" {...axisProps} ticks={yearTicks(f.chart.map((p) => p.date))} tickFormatter={(d: string) => d.slice(0, 4)} />
            <YAxis {...axisProps} width={56} domain={[Math.max(0, lo - pad), hi + pad]} tickFormatter={fmt} />
            <Tooltip content={<ChartTip currency={f.currency} />} />
            {rungs.map((l) => (
              <ReferenceLine key={l.key} y={l.price} stroke={rungColor(l)} strokeWidth={closeness(l) === 'far' ? 1 : 2} strokeOpacity={closeness(l) === 'far' ? 0.7 : 1} strokeDasharray="4 4"
                label={{ value: l.key === 't1' ? '1' : l.key === 't2' ? '2' : l.key === 't3' ? '3' : '−10%', position: 'insideLeft', fill: rungColor(l), fontSize: 10, fontWeight: 600 }} />
            ))}
            <Line dataKey="sma4y" stroke={SERIES.sma4y} dot={false} strokeWidth={1.5} isAnimationActive={false} connectNulls />
            <Line dataKey="sma2y" stroke={SERIES.sma2y} dot={false} strokeWidth={1.5} isAnimationActive={false} connectNulls />
            <Line dataKey="sma200" stroke={SERIES.sma200} dot={false} strokeWidth={1.5} isAnimationActive={false} connectNulls />
            <Line dataKey="close" stroke={SERIES.close} dot={false} strokeWidth={2} isAnimationActive={false} />
            {/* Your own buys, from the trades: a ringed dot at the price paid. */}
            {f.buys.map((b, i) => (
              <ReferenceDot key={i} x={b.chart_date} y={b.price} r={5} fill="var(--chart-surface)" stroke={SERIES.close} strokeWidth={2.5} ifOverflow="extendDomain" />
            ))}
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <Legend items={[{ color: SERIES.close, label: 'Price' }, { color: SERIES.sma200, label: '200-day' }, { color: SERIES.sma2y, label: '2-year' }, { color: SERIES.sma4y, label: '4-year' }, { color: SERIES.deep, label: '−44% from high' }]} />
      {f.buys.length > 0 && (
        <p className="mt-1 flex items-center gap-1.5 text-xs text-ink2">
          <span className="inline-block h-3 w-3 rounded-full border-[2.5px] bg-surface" style={{ borderColor: SERIES.close }} aria-hidden />
          Your buys: {f.buys.length === 1 ? '1 purchase' : `${f.buys.length} purchases`}, {units(f.buys.reduce((a, b) => a + b.shares, 0))} {f.symbol.startsWith('BTC') ? 'BTC' : 'shares'} at an average of {px(f.buys.reduce((a, b) => a + b.price * b.shares, 0) / Math.max(1e-9, f.buys.reduce((a, b) => a + b.shares, 0)), f.currency)}
        </p>
      )}
      <p className="mt-1 text-xs text-muted">Dashed lines are the limit prices, in their average’s colour: rungs 1–3 and the −10% line. Bolder once within 5%.</p>
    </Card>
  )
}

function ChartTip({ active, payload, currency }: { active?: boolean; payload?: { payload?: DipPoint }[]; currency: string }) {
  const p = payload?.[0]?.payload
  if (!active || !p) return null
  return (
    <TooltipBox title={shortDate(p.date)} rows={[
      { color: SERIES.close, label: 'Price', value: px(p.close, currency), bold: true },
      { color: SERIES.sma200, label: '200-day', value: px(p.sma200 || undefined, currency) },
      { color: SERIES.sma2y, label: '2-year', value: px(p.sma2y || undefined, currency) },
      { color: SERIES.sma4y, label: '4-year', value: px(p.sma4y || undefined, currency) },
    ]} />
  )
}

/** Unfilled gaps under the price: where prices jumped and never came back. */
function Gaps({ f }: { f: DipFund }) {
  return (
    <Card title="Unfilled gaps below" icon="filter">
      {f.gaps.length === 0 ? <p className="text-sm text-muted">No open gaps under today's price.</p> : (
        <ul className="divide-y divide-line text-sm">
          {f.gaps.map((g) => (
            <li key={g.date} className="flex items-center justify-between gap-3 py-2">
              <span className="text-ink2">{shortDate(g.date)}</span>
              <span className="tnum text-ink">{px(g.low, f.currency)} – {px(g.high, f.currency)}</span>
              <span className="tnum w-20 text-right text-xs text-muted">{pct(g.high / f.price - 1, 1)}</span>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 text-xs text-muted">A gap is a range the price jumped over and has not traded back into. Falls often stop there, which is why a rung beside one uses its top.</p>
    </Card>
  )
}

/** Funds, their price-history proxy and the reserve kept for each. */
function SettingsSheet({ open, onClose, funds }: { open: boolean; onClose: () => void; funds: DipFundSetting[] }) {
  const plan = useContext(PlanCtx)
  const qc = useQueryClient()
  const toast = useToast()
  const [rows, setRows] = useState<DipFundSetting[]>(funds)
  const [seen, setSeen] = useState(open)
  if (open !== seen) { // reopening starts from what is saved
    setSeen(open)
    if (open) setRows(funds)
  }
  const save = useMutation({
    mutationFn: () => api.put<{ funds: DipFundSetting[] }>(`/market/dips/settings${plan === 'funds' ? '' : `?plan=${plan}`}`, { funds: rows }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['dips'] })
      qc.invalidateQueries({ queryKey: ['overview'] }) // the bell reads reserves
      toast('Dip plan saved')
      onClose()
    },
  })
  const set = (i: number, patch: Partial<DipFundSetting>) => setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  return (
    <Sheet open={open} onClose={onClose} title="Funds & reserve" footer={
      <div className="flex gap-2">
        <button className="btn-ghost h-11 flex-1" onClick={onClose}>Cancel</button>
        <button className="btn-primary h-11 flex-1" disabled={save.isPending} onClick={() => save.mutate()}>Save</button>
      </div>
    }>
      <div className="space-y-4">
        <p className="text-sm text-ink2">Use the exchange listing you buy (VWCE.DE, VALL.AS). A young fund can borrow the history of an older one on the same index.</p>
        {rows.map((r, i) => (
          <div key={i} className="space-y-2 rounded-xl border border-line p-3">
            <div className="grid grid-cols-2 gap-2">
              <Field label="Ticker"><input className="input uppercase" value={r.symbol} onChange={(e) => set(i, { symbol: e.target.value })} /></Field>
              <Field label="Reserve €"><NumberInput value={r.reserve} onChange={(v) => set(i, { reserve: v ?? 0 })} inputMode="decimal" /></Field>
            </div>
            <Field label="Name"><input className="input" value={r.name} onChange={(e) => set(i, { name: e.target.value })} /></Field>
            <Field label="History from" hint="Optional: an older listing of the same index, for the averages."><input className="input uppercase placeholder:normal-case" value={r.proxy ?? ''} placeholder="e.g. VWCE.DE" onChange={(e) => set(i, { proxy: e.target.value })} /></Field>
            <button className="min-h-10 text-sm text-bad" onClick={() => setRows(rows.filter((_, j) => j !== i))}>Remove {r.symbol || 'fund'}</button>
          </div>
        ))}
        {rows.length < 8 && <button className="btn-ghost h-10 w-full" onClick={() => setRows([...rows, { symbol: '', name: '', reserve: 0 }])}>Add a fund</button>}
        <ErrorBox error={save.error} />
      </div>
    </Sheet>
  )
}
