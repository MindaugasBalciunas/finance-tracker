import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, CartesianGrid, ComposedChart, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { usePeriod, useRefresh } from '../../lib/hooks'
import { eur, eurk, pct, shortDate, todayISO } from '../../lib/format'
import { RANGES, rangeLabel, rangeTick } from '../../lib/periods'
import { Card, Delta, Empty, ErrorBox, Field, Loading, NumberInput, Segmented, Sheet, useToast } from '../../components/ui'
import { axisProps, Donut, foldSlices, gridProps, Legend, TooltipBox, type Slice } from '../../components/charts'
import { Icon } from '../../components/Icon'
import { brandColor } from '../../lib/brand'

// ── investments ─────────────────────────────────────────────────────

export function Investments() {
  const { data: p, isLoading, error } = useQuery({ queryKey: ['portfolio'], queryFn: () => api.get<any>('/portfolio'), staleTime: 300_000 })
  const { data: trades } = useQuery({ queryKey: ['trades'], queryFn: () => api.get<any[]>('/trades') })
  const { data: sc } = useQuery({ queryKey: ['scenarios'], queryFn: () => api.get<any>('/portfolio/scenarios'), staleTime: 3_600_000 })
  const [range, setRange] = usePeriod('stocks', '1y', RANGES.map((r) => r.value))
  const { data: perf, isLoading: perfLoading } = useQuery({ queryKey: ['portfolio-history', range], queryFn: () => api.get<any[]>('/portfolio/history', { range }), staleTime: 600_000 })
  const [sort, setSort] = usePeriod('stocks-sort', 'value', ['value', 'gain', 'day'])
  const [split, setSplit] = usePeriod('stocks-split', 'positions', ['positions', 'brokers', 'currency'])
  const [pos, setPos] = useState<any>(null)
  const [trade, setTrade] = useState<any>(null)
  const [showTrades, setShowTrades] = useState(false)
  const [mode, setMode] = usePeriod('stocks-chart', 'worth', ['worth', 'gain'])
  if (isLoading) return <Loading label="Fetching live prices…" />
  if (error) return <ErrorBox error={error} />
  const hs: any[] = p.holdings
  // One colour per position (largest first), shared by the donut and the rows.
  const color: Record<string, string> = {}
  ;[...hs].sort((a, b) => (b.value_eur ?? b.cost_eur ?? 0) - (a.value_eur ?? a.cost_eur ?? 0)).forEach((h, i) => { color[h.ticker] = i < 7 ? `var(--s${i + 1})` : 'var(--s-other)' })
  const val = (h: any) => h.value_eur ?? h.cost_eur ?? 0
  const gainEUR = (h: any) => (h.value_eur != null && h.cost_eur != null ? h.value_eur - h.cost_eur : null)
  const upside: Record<string, number> = {}
  // Only real analyst coverage; ETFs fall back to their 52-week range, which isn't a target.
  for (const x of sc?.positions ?? []) if (x.analysts > 0 && x.mean_eur && x.value_eur) upside[x.ticker] = x.mean_eur / x.value_eur - 1
  const sorted = [...hs].sort((a, b) => sort === 'gain' ? (b.gain_pct ?? -9) - (a.gain_pct ?? -9) : sort === 'day' ? (b.day_pct ?? -9) - (a.day_pct ?? -9) : val(b) - val(a))
  const slices: Slice[] = split === 'positions'
    ? foldSlices(hs.map((h) => ({ key: h.ticker, label: h.ticker, value: val(h), color: color[h.ticker] })))
    : (() => {
      const m = new Map<string, number>()
      for (const h of hs) { const k = split === 'brokers' ? (h.account_id || 'other') : h.currency; m.set(k, (m.get(k) ?? 0) + val(h)) }
      return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([k, v], i) => ({ key: k, label: split === 'brokers' ? brokerName(k) : k, value: v, color: split === 'brokers' ? (brandColor({ institution: brokerName(k), name: k, kind: 'brokerage' }) ?? `var(--s${i + 1})`) : `var(--s${i + 1})` }))
    })()
  const gainPct = p.cost_eur ? p.gain_eur / p.cost_eur : 0
  const dayPct = p.value_eur - p.day_change_eur > 0 ? p.day_change_eur / (p.value_eur - p.day_change_eur) : 0
  const last = perf?.[perf.length - 1], first = perf?.[0]
  return (
    <div className="space-y-4">
      {/* Headline + money in vs worth now */}
      <section className="card p-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-sm text-ink2">Stocks & ETFs</div>
            <div className="text-3xl font-semibold tracking-tight">{eur(p.value_eur)}</div>
            <div className="mt-0.5 flex flex-wrap gap-x-3 text-sm text-muted">
              <span>Today <Delta value={p.day_change_eur} /> <span className="tnum">{p.day_change_eur ? `(${dayPct >= 0 ? '+' : ''}${pct(dayPct, 2)})` : ''}</span></span>
              <span>Unrealised <Delta value={p.gain_eur} /> <span className="tnum">({gainPct >= 0 ? '+' : ''}{pct(gainPct, 1)})</span></span>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4 text-sm sm:text-right">
            <div><div className="text-xs text-muted">Cost basis</div><div className="font-semibold tnum">{eur(p.cost_eur)}</div></div>
            <div><div className="text-xs text-muted">Realised</div><div className={clsx('font-semibold tnum', p.realized_eur >= 0 ? 'text-good' : 'text-bad')}>{eur(p.realized_eur)}</div></div>
            <div><div className="text-xs text-muted">Positions</div><div className="font-semibold tnum">{hs.length}</div></div>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-muted">Worth vs money in {last && first ? <>· {rangeLabel(range)} <Delta value={(last.value_eur - last.cost_eur) - (first.value_eur - first.cost_eur)} /></> : null}</span>
          <div className="flex flex-wrap items-center gap-2">
            <Segmented size="sm" value={mode} onChange={setMode} options={[{ value: 'worth', label: 'Worth' }, { value: 'gain', label: 'Gain' }]} />
            <Segmented size="sm" value={range} onChange={setRange} options={RANGES} />
          </div>
        </div>
        <div className="mt-2 h-56 sm:h-64">
          {perfLoading ? <Loading label="Replaying trades…" /> : !perf?.length ? <div className="grid h-full place-items-center text-sm text-muted">No trades in this period</div> : (
            <ResponsiveContainer>
              <ComposedChart data={mode === 'gain' ? perf.map((x: any) => ({ ...x, gain: x.value_eur - x.cost_eur, up: Math.max(0, x.value_eur - x.cost_eur), down: Math.min(0, x.value_eur - x.cost_eur) })) : perf} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                <defs><linearGradient id="stk" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--s1)" stopOpacity={0.28} /><stop offset="100%" stopColor="var(--s1)" stopOpacity={0.02} /></linearGradient></defs>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={['auto', 'auto']} />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={shortDate(label)} rows={[
                    { color: 'var(--s1)', label: 'Worth', value: eur(payload[0].payload.value_eur), bold: true },
                    { color: 'var(--s2)', label: 'Money in (cost)', value: eur(payload[0].payload.cost_eur) },
                    { label: 'Gain', value: <span className={payload[0].payload.value_eur >= payload[0].payload.cost_eur ? 'text-good' : 'text-bad'}>{eur(payload[0].payload.value_eur - payload[0].payload.cost_eur)}</span> },
                  ]} />) : null} />
                {mode === 'gain' ? <>
                  <ReferenceLine y={0} stroke="var(--chart-axis)" />
                  <Area type="monotone" dataKey="up" name="Gain" stroke="rgb(var(--good))" strokeWidth={0} fill="rgb(var(--good))" fillOpacity={0.18} isAnimationActive={false} />
                  <Area type="monotone" dataKey="down" name="Loss" stroke="rgb(var(--bad))" strokeWidth={0} fill="rgb(var(--bad))" fillOpacity={0.18} isAnimationActive={false} />
                  <Line type="monotone" dataKey="gain" name="Gain" stroke="rgb(var(--ink))" strokeWidth={2} dot={false} isAnimationActive={false} />
                </> : <>
                  <Area type="monotone" dataKey="value_eur" name="Worth" stroke="var(--s1)" strokeWidth={2} fill="url(#stk)" isAnimationActive={false} />
                  <Line type="stepAfter" dataKey="cost_eur" name="Money in" stroke="var(--s2)" strokeWidth={2} dot={false} isAnimationActive={false} />
                </>}
              </ComposedChart>
            </ResponsiveContainer>
          )}
        </div>
        <div className="mt-2">{mode === 'gain'
          ? <Legend items={[{ color: 'rgb(var(--ink))', label: 'Unrealised gain (worth − money in)', value: last ? eur(last.value_eur - last.cost_eur) : undefined }]} />
          : <Legend items={[{ color: 'var(--s1)', label: 'Worth', value: last ? eurk(last.value_eur) : undefined }, { color: 'var(--s2)', label: 'Money in (cost of open positions)', value: last ? eurk(last.cost_eur) : undefined }]} />}</div>
        <div className="mt-1 text-[11px] text-muted">From your trades and daily/weekly closes at today's exchange rates; fund accounts without trades (Revolut ETF, Swedbank funds) are on Net worth.</div>
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
        <Card icon="pie" color="var(--s1)" title="Allocation" action={<Segmented size="sm" value={split} onChange={setSplit} options={[{ value: 'positions', label: 'Positions' }, { value: 'brokers', label: 'Brokers' }, { value: 'currency', label: 'Currency' }]} />}>
          <Donut slices={slices} center={eurk(p.value_eur)} sub="market value" height={180} stacked />
        </Card>

        <Card pad={false} icon="trend" color="var(--s7)" title="Positions" action={
          <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => setTrade({ action: 'buy', date: todayISO(), currency: 'USD', account_id: 'ibkr' })}><Icon name="plus" size={14} />Trade</button>}>
          <div className="flex items-center justify-between gap-2 px-4 pb-2.5">
            <span className="text-xs text-muted">Sort by</span>
            <Segmented size="sm" value={sort} onChange={setSort} options={[{ value: 'value', label: 'Value' }, { value: 'gain', label: 'Gain' }, { value: 'day', label: 'Today' }]} />
          </div>
          {!hs.length ? <Empty title="No open positions" /> : (
            <div className="divide-y divide-line border-t border-line">
              {sorted.map((h) => {
                const g = gainEUR(h)
                const up = upside[h.ticker]
                return (
                  <button key={h.ticker} onClick={() => setPos(h)} className="block w-full px-4 py-3 text-left hover:bg-sunken/50">
                    <div className="flex items-center gap-3">
                      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl text-[10px] font-bold tracking-tight"
                        style={{ background: `color-mix(in oklab, ${color[h.ticker]} var(--tint), transparent)`, color: `color-mix(in oklab, ${color[h.ticker]} 80%, rgb(var(--ink)))` }}>{h.ticker.replace(/\..*$/, '').slice(0, 5)}</span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-baseline gap-1.5"><span className="text-sm font-semibold">{h.ticker}</span><span className="truncate text-xs text-muted">{h.name ?? brokerName(h.account_id)}</span></div>
                        <div className="truncate text-xs text-muted tnum">{+h.shares.toFixed(4)} × {h.price != null ? h.price.toFixed(2) : '—'} {h.currency} · avg {h.avg_cost.toFixed(2)}</div>
                      </div>
                      <div className="text-right">
                        <div className="tnum text-sm font-semibold">{eur(val(h))}</div>
                        <div className="flex items-center justify-end gap-1.5 text-xs tnum">
                          {h.day_pct != null && <span className={h.day_pct >= 0 ? 'text-good' : 'text-bad'}>{h.day_pct >= 0 ? '▲' : '▼'}{pct(Math.abs(h.day_pct), 1)}</span>}
                          {g != null ? <span className={clsx('rounded px-1', g >= 0 ? 'bg-good/10 text-good' : 'bg-bad/10 text-bad')}>{g >= 0 ? '+' : '−'}{eurk(Math.abs(g))} · {pct(Math.abs(h.gain_pct ?? 0), 0)}</span> : <span className="text-muted">no quote</span>}
                        </div>
                      </div>
                    </div>
                    <div className="mt-2 grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 pl-12">
                      <div className="h-1.5 rounded-full bg-sunken"><div className="h-full rounded-full" style={{ width: `${Math.max(2, h.weight * 100)}%`, background: color[h.ticker] }} /></div>
                      <span className="w-24 text-right text-[11px] text-muted tnum">{pct(h.weight, 1)} of book</span>
                      {h.week52_high > h.week52_low && h.price != null && <Range52 lo={h.week52_low} hi={h.week52_high} price={h.price} avg={h.avg_cost} />}
                      {h.week52_high > h.week52_low && h.price != null && <span className="w-24 text-right text-[11px] text-muted">{up != null ? <span className={up >= 0 ? 'text-good' : 'text-bad'}>target {up >= 0 ? '+' : '−'}{pct(Math.abs(up), 0)}</span> : '52-week range'}</span>}
                    </div>
                  </button>
                )
              })}
            </div>
          )}
        </Card>
      </div>

      <ScenarioCard />

      {p.closed?.length > 0 && (
        <Card pad={false} icon="check" color="var(--s6)" title="Closed positions" action={<span className="text-xs text-muted">realised {eur(p.realized_eur)}</span>}>
          <div className="divide-y divide-line border-t border-line">
            {p.closed.map((h: any) => (
              <div key={h.ticker} className="flex items-center justify-between px-4 py-2 text-sm">
                <span><b>{h.ticker}</b> <span className="text-xs text-muted">{brokerName(h.account_id)} · first bought {shortDate(h.first_buy)}</span></span>
                <span className={clsx('tnum', h.realized >= 0 ? 'text-good' : 'text-bad')}>{h.realized >= 0 ? '+' : '−'}{Math.abs(h.realized).toFixed(2)} {h.currency}</span>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card pad={false} icon="list" color="var(--s2)" title={`Trades (${trades?.length ?? 0})`} action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowTrades(!showTrades)}>{showTrades ? 'Hide' : 'Show all'}</button>}>
        <div className="divide-y divide-line border-t border-line">
          {[...(trades ?? [])].reverse().slice(0, showTrades ? undefined : 6).map((t) => (
            <button key={t.id} onClick={() => setTrade(t)} className="flex w-full items-center gap-3 px-4 py-2 text-left text-sm hover:bg-sunken/50">
              <span className={clsx('w-10 shrink-0 rounded-md py-0.5 text-center text-[10px] font-semibold uppercase', t.action === 'buy' ? 'bg-good/10 text-good' : 'bg-bad/10 text-bad')}>{t.action}</span>
              <span className="min-w-0 flex-1 truncate"><b>{t.ticker}</b> <span className="text-xs text-muted">{shortDate(t.date)} · {brokerName(t.account_id)}</span></span>
              <span className="text-right tnum"><span className="text-xs text-muted">{+t.shares.toFixed(4)} × {t.price}</span> <b>{(t.shares * t.price).toFixed(2)} {t.currency}</b></span>
            </button>
          ))}
        </div>
      </Card>
      {pos && <PositionSheet h={pos} onClose={() => setPos(null)} />}
      {trade && <TradeEditor t={trade} onClose={() => setTrade(null)} />}
    </div>
  )
}

const brokerName = (id?: string) => ({ ibkr: 'IBKR', revolut_stocks: 'Revolut', revolut_etf: 'Revolut', swed_etf: 'Swedbank' } as Record<string, string>)[id ?? ''] ?? (id || '—')

/** 52-week range: where today's price sits, with your average cost marked. */
function Range52({ lo, hi, price, avg }: { lo: number; hi: number; price: number; avg: number }) {
  const at = (v: number) => `${Math.min(100, Math.max(0, ((v - lo) / (hi - lo)) * 100))}%`
  return (
    <div className="relative h-3" title={`52-week ${lo.toFixed(2)} – ${hi.toFixed(2)} · price ${price.toFixed(2)} · your avg ${avg.toFixed(2)}`}>
      <div className="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 rounded-full bg-sunken" />
      <div className="absolute top-0 h-3 w-0.5 -translate-x-1/2 rounded bg-axis" style={{ left: at(avg) }} />
      <div className="absolute top-1/2 h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface bg-accent" style={{ left: at(price) }} />
    </div>
  )
}

/** The book at analyst low / mean / high targets (52-week range for ETFs). */
function ScenarioCard() {
  const { data: s, isLoading } = useQuery({ queryKey: ['scenarios'], queryFn: () => api.get<any>('/portfolio/scenarios'), staleTime: 3_600_000 })
  if (isLoading) return <Card icon="target" color="var(--s4)" title="Scenarios"><Loading label="Fetching targets…" /></Card>
  if (!s?.value_eur) return null
  // Honest labels: analyst targets where Yahoo has coverage, otherwise each
  // holding's 52-week low / middle / high.
  const analysts = s.covered > 0
  const basis = analysts ? 'targets' : '52-week'
  const rows = [{ label: `Bear (${analysts ? 'low targets' : '52-week low'})`, v: s.low_eur }, { label: `Base (${analysts ? 'mean targets' : '52-week middle'})`, v: s.mean_eur }, { label: `Bull (${analysts ? 'high targets' : '52-week high'})`, v: s.high_eur }]
  return (
    <Card icon="target" color="var(--s4)" title={analysts ? 'Where analysts see it' : 'Range scenarios'}
      action={<span className="text-xs text-muted">{analysts ? `${pct(s.covered)} of the book has analyst targets` : 'analyst targets unavailable'}</span>}>
      <div className="grid grid-cols-3 gap-3 text-sm">
        {rows.map((x) => (
          <div key={x.label}>
            <div className="text-xs text-muted">{x.label}</div>
            <div className="font-semibold tnum">{eur(x.v)}</div>
            <div className={clsx('text-xs tnum', x.v >= s.value_eur ? 'text-good' : 'text-bad')}>{x.v >= s.value_eur ? '+' : '−'}{pct(Math.abs(x.v / s.value_eur - 1))}</div>
          </div>
        ))}
      </div>
      <div className="mt-2 text-xs text-muted">Today {eur(s.value_eur)}. {analysts
        ? 'Positions without targets use their 52-week range. Targets are opinions, not forecasts.'
        : `Yahoo isn't returning analyst targets right now, so each holding is valued at its 52-week low, middle and high — a range, not a ${basis === '52-week' ? 'forecast' : 'target'}.`}</div>
    </Card>
  )
}

function PositionSheet({ h, onClose }: { h: any; onClose: () => void }) {
  const [range, setRange] = usePeriod('position', '1y', ['1mo', '6mo', 'ytd', '1y', '5y'])
  const { data: hist } = useQuery({ queryKey: ['hist', h.ticker, range], queryFn: () => api.get<any[]>(`/market/history/${h.ticker}`, { range }) })
  const { data: an } = useQuery({ queryKey: ['analyst', h.ticker], queryFn: () => api.get<any>(`/market/analyst/${h.ticker}`), retry: false })
  return (
    <Sheet open onClose={onClose} title={h.ticker} wide>
      <div className="mb-3 grid grid-cols-3 gap-3 text-sm">
        <div><div className="text-xs text-muted">Value</div><div className="font-semibold">{h.value_eur != null ? eur(h.value_eur) : '—'}</div></div>
        <div><div className="text-xs text-muted">Gain</div><div className={clsx('font-semibold', (h.gain ?? 0) >= 0 ? 'text-good' : 'text-bad')}>{h.gain != null ? `${h.gain.toFixed(0)} ${h.currency}` : '—'}</div></div>
        <div><div className="text-xs text-muted">52-week</div><div className="font-semibold tnum">{h.week52_low ? `${h.week52_low.toFixed(0)}–${h.week52_high.toFixed(0)}` : '—'}</div></div>
      </div>
      <Segmented value={range} onChange={setRange} size="sm" options={[{ value: '1mo', label: '1M' }, { value: '6mo', label: '6M' }, { value: 'ytd', label: 'YTD' }, { value: '1y', label: '1Y' }, { value: '5y', label: '5Y' }]} />
      <div className="mt-2 h-52">
        <ResponsiveContainer>
          <LineChart data={hist ?? []} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="date" {...axisProps} minTickGap={40} tickFormatter={(d) => d.slice(2, 7)} />
            <YAxis {...axisProps} domain={['auto', 'auto']} width={48} />
            <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={label} rows={[{ label: 'Close', value: `${(payload[0].value as number).toFixed(2)} ${h.currency}`, bold: true }]} /> : null} />
            <ReferenceLine y={h.avg_cost} stroke="var(--s2)" strokeDasharray="0" label={{ value: 'your avg', fill: 'var(--chart-text)', fontSize: 10, position: 'insideTopLeft' }} />
            <Line type="monotone" dataKey="close" stroke="var(--s1)" strokeWidth={2} dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>
      {an && (
        <div className="mt-3 rounded-xl bg-sunken p-3 text-sm">
          <div className="section-title mb-1">Analyst targets {an.num_analysts ? `(${an.num_analysts})` : '(from 52-week range)'}</div>
          <div className="grid grid-cols-3 gap-2 tnum">
            <div><div className="text-xs text-muted">Low</div>{an.target_low?.toFixed(2)}</div>
            <div><div className="text-xs text-muted">Mean</div>{an.target_mean?.toFixed(2)}</div>
            <div><div className="text-xs text-muted">High</div>{an.target_high?.toFixed(2)}</div>
          </div>
          {an.recommendation && <div className="mt-1 text-xs text-muted">Consensus: {an.recommendation}</div>}
        </div>
      )}
    </Sheet>
  )
}

function TradeEditor({ t: init, onClose }: { t: any; onClose: () => void }) {
  const [t, setT] = useState(init)
  const refresh = useRefresh()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => (t.id ? api.put(`/trades/${t.id}`, t) : api.post('/trades', t)),
    onSuccess: () => { refresh(); toast('Saved', 'good'); onClose() },
  })
  const del = useMutation({ mutationFn: () => api.del(`/trades/${t.id}`), onSuccess: () => { refresh(); onClose() } })
  return (
    <Sheet open onClose={onClose} title={t.id ? 'Edit trade' : 'New trade'} footer={<>
      {t.id && <button className="btn-danger mr-auto" onClick={() => confirm('Delete this trade?') && del.mutate()}>Delete</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()}>Save</button>
    </>}>
      <div className="space-y-4">
        <Segmented value={t.action} onChange={(a) => setT({ ...t, action: a })} options={[{ value: 'buy', label: 'Buy' }, { value: 'sell', label: 'Sell' }]} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="Ticker"><input className="input uppercase" value={t.ticker ?? ''} onChange={(e) => setT({ ...t, ticker: e.target.value.toUpperCase() })} /></Field>
          <Field label="Date"><input type="date" className="input" value={t.date} onChange={(e) => setT({ ...t, date: e.target.value })} /></Field>
          <Field label="Shares"><NumberInput value={t.shares} onChange={(v) => setT({ ...t, shares: v })} /></Field>
          <Field label="Price per share"><NumberInput value={t.price} onChange={(v) => setT({ ...t, price: v })} /></Field>
          <Field label="Currency"><select className="input select-pad" value={t.currency} onChange={(e) => setT({ ...t, currency: e.target.value })}>{['USD', 'EUR', 'GBP'].map((c) => <option key={c}>{c}</option>)}</select></Field>
          <Field label="Account"><select className="input select-pad" value={t.account_id ?? ''} onChange={(e) => setT({ ...t, account_id: e.target.value })}><option value="ibkr">IBKR</option><option value="revolut_stocks">Revolut Stocks</option><option value="">—</option></select></Field>
        </div>
        <Field label="Notes"><input className="input" value={t.notes ?? ''} onChange={(e) => setT({ ...t, notes: e.target.value })} /></Field>
        <ErrorBox error={save.error} />
      </div>
    </Sheet>
  )
}
