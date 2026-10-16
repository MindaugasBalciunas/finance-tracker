import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { CartesianGrid, ComposedChart, Line, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { addMonths, eur, eurk, monthLabel, thisMonth } from '../../lib/format'
import { AskCFO, Card, ErrorBox, Loading, NumberInput, Segmented, useToast } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'

type Item = { id?: number; label: string; kind: 'bill' | 'goal' | 'purchase'; month?: string; amount: number }
type Month = { month: string; income: number; fixed: number; saving: number; everyday: number; one_offs: number; dated?: Item[] | null; net: number; end: number }
type Forecast = {
  start: string; cash: number; floor: number; months: Month[]; lowest: number; lowest_on: string; below_floor?: string; negative?: string; invested: number; unscheduled?: Item[] | null
  assumed: { income: number; gross?: number; fixed: number; saving: number; everyday: number; one_offs: number }
}
type Comparison = { baseline: Forecast; scenario?: Forecast; summary?: string[] }
type Scenario = { gross?: number; saving?: number; everyday_pct?: number; from?: string; purchases?: Item[]; skip_goals?: number[] }

const PCT = [{ value: '-20', label: '−20%' }, { value: '-10', label: '−10%' }, { value: '0', label: 'as usual' }, { value: '10', label: '+10%' }]

/** The year ahead: cash month by month at the current plan, and a what-if
 *  beside it — a new salary, a different saving amount, leaner months, a
 *  purchase, a goal left out. The same arithmetic both ways, done on the
 *  server, so the app and Ask CFO agree. */
export function ForecastView() {
  const [sc, setSc] = useState<Scenario>({})
  const [buy, setBuy] = useState<{ label: string; month: string; amount?: number }>({ label: '', month: addMonths(thisMonth(), 2) })
  const active = !!(sc.gross || sc.saving !== undefined || sc.everyday_pct || sc.purchases?.length || sc.skip_goals?.length)
  const { data, isLoading, error } = useQuery({
    queryKey: ['forecast', active ? sc : null],
    queryFn: () => (active ? api.post<Comparison>('/forecast', sc) : api.get<Comparison>('/forecast')),
    placeholderData: (prev) => prev,
  })
  const qc = useQueryClient()
  const toast = useToast()
  const months = useMemo(() => Array.from({ length: 12 }, (_, i) => addMonths(thisMonth(), i)), [])
  if (isLoading && !data) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  const b = data.baseline
  const s = data.scenario
  const show = s ?? b
  const goals = [...b.months.flatMap((m) => (m.dated ?? []).filter((x) => x.kind === 'goal').map((x) => ({ ...x, month: m.month })))]
  const rows = b.months.map((m, i) => ({ month: m.month, base: m.end, alt: s?.months[i]?.end }))
  const lo = Math.min(0, ...rows.map((r) => Math.min(r.base, r.alt ?? r.base)))
  const set = (p: Partial<Scenario>) => setSc((x) => ({ ...x, ...p }))
  const applySalary = async () => {
    try {
      await api.put('/plan/settings', { ...(await api.get<any>('/plan/settings')).settings, income_mode: 'gross', gross_salary: sc.gross })
      qc.invalidateQueries()
      setSc((x) => ({ ...x, gross: undefined }))
      toast(`Plan income now from ${eur(sc.gross!)} gross`, 'good')
    } catch (e) { toast((e as Error).message, 'bad') }
  }

  const verdict = show.negative
    ? { tone: 'text-bad', icon: 'alert', text: <>Cash runs out in <b>{monthLabel(show.negative, true)}</b>{show.below_floor && show.below_floor !== show.negative && <> — below your {eur(show.floor)} emergency target from {monthLabel(show.below_floor, true)}</>}.</> }
    : show.below_floor
      ? { tone: 'text-warn', icon: 'alert', text: <>Cash drops below your <b className="tnum">{eur(show.floor)}</b> emergency target in <b>{monthLabel(show.below_floor, true)}</b>; lowest <b className="tnum">{eur(show.lowest)}</b> in {monthLabel(show.lowest_on)}.</> }
      : { tone: 'text-good', icon: 'check', text: <>Cash stays above your <b className="tnum">{eur(show.floor)}</b> emergency target all year — lowest <b className="tnum">{eur(show.lowest)}</b> in {monthLabel(show.lowest_on)}.</> }

  return (
    <div className="space-y-4">
      <section className="card p-4">
        <div className="text-xs text-ink2">{s ? 'With your what-if' : 'At the current plan'} · cash in checking, savings and cash</div>
        <div className={clsx('mt-1 flex items-start gap-2 text-sm font-medium', verdict.tone)}><Icon name={verdict.icon} size={18} className="mt-0.5 shrink-0" /><span>{verdict.text}</span></div>
        {data.summary?.length ? <ul className="mt-2 space-y-0.5 text-xs text-ink2">{data.summary.map((x) => <li key={x}>· {x}</li>)}</ul> : null}
        <div className="mt-3 h-56 sm:h-64">
          <ResponsiveContainer>
            <ComposedChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="month" {...axisProps} tickFormatter={(m) => monthLabel(m).split(' ')[0]} interval={0} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={[lo * 1.1, 'auto']} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                <TooltipBox title={monthLabel(label as string, true)} rows={[{ color: 'var(--s1)', label: 'Current plan', value: eur(payload[0].payload.base) },
                  ...(payload[0].payload.alt != null ? [{ color: 'var(--s3)', label: 'What-if', value: eur(payload[0].payload.alt), bold: true }] : [])]} />) : null} />
              <ReferenceLine y={0} stroke="rgb(var(--bad))" strokeOpacity={0.6} />
              {b.floor > 0 && <ReferenceLine y={b.floor} stroke="rgb(var(--warn))" strokeDasharray="4 3" label={{ value: `emergency target ${eurk(b.floor)}`, position: 'insideTopLeft', fontSize: 10, fill: 'rgb(var(--warn))' }} />}
              <Line dataKey="base" stroke="var(--s1)" strokeWidth={2} dot={{ r: 2.5 }} isAnimationActive={false} strokeOpacity={s ? 0.5 : 1} />
              {s && <Line dataKey="alt" stroke="var(--s3)" strokeWidth={2.5} dot={{ r: 3 }} isAnimationActive={false} />}
            </ComposedChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'Current plan' }, ...(s ? [{ color: 'var(--s3)', label: 'What-if' }] : []), { color: 'rgb(var(--warn))', label: 'Emergency target' }]} /></div>
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card icon="spark" color="var(--s3)" title="What if…" action={active && <button className="btn-ghost h-8 px-2 text-xs" onClick={() => setSc({})}>Reset</button>}>
          <div className="space-y-3 text-sm">
            <label className="block">
              <span className="text-xs text-muted">Gross salary per month{b.assumed.gross ? ` (now ${eur(b.assumed.gross)})` : ''}</span>
              <div className="mt-1 flex items-center gap-2">
                <NumberInput className="input tnum w-32" value={sc.gross} onChange={(v) => set({ gross: v })} placeholder={b.assumed.gross ? String(b.assumed.gross) : 'e.g. 10000'} aria-label="Gross salary" />
                <select className="input h-9 w-auto text-xs" value={sc.from ?? ''} onChange={(e) => set({ from: e.target.value || undefined })} aria-label="From month">
                  <option value="">from now</option>
                  {months.slice(1).map((m) => <option key={m} value={m}>from {monthLabel(m)}</option>)}
                </select>
                {!!sc.gross && <button className="chip shrink-0 hover:bg-sunken" onClick={applySalary}>Use in plan</button>}
              </div>
            </label>
            <label className="block">
              <span className="text-xs text-muted">Saving & investing per month (now {eur(b.assumed.saving)})</span>
              <NumberInput className="input tnum mt-1 w-32" value={sc.saving} onChange={(v) => set({ saving: v })} placeholder={String(Math.round(b.assumed.saving))} aria-label="Saving per month" />
            </label>
            <div>
              <span className="text-xs text-muted">Everyday spending (usually {eur(b.assumed.everyday)}/month)</span>
              <div className="mt-1"><Segmented size="sm" value={String(sc.everyday_pct ?? 0)} onChange={(v) => set({ everyday_pct: Number(v) || undefined })} options={PCT} /></div>
            </div>
            <div>
              <span className="text-xs text-muted">A purchase</span>
              <div className="mt-1 flex flex-wrap items-center gap-2">
                <input className="input h-9 w-36" value={buy.label} onChange={(e) => setBuy({ ...buy, label: e.target.value })} placeholder="What" aria-label="Purchase" />
                <select className="input h-9 w-auto text-xs" value={buy.month} onChange={(e) => setBuy({ ...buy, month: e.target.value })} aria-label="Purchase month">
                  {months.map((m) => <option key={m} value={m}>{monthLabel(m)}</option>)}
                </select>
                <NumberInput className="input tnum h-9 w-24" value={buy.amount} onChange={(v) => setBuy({ ...buy, amount: v })} placeholder="€" aria-label="Purchase amount" />
                <button className="chip hover:bg-sunken" disabled={!buy.amount} onClick={() => { set({ purchases: [...(sc.purchases ?? []), { label: buy.label || 'Purchase', kind: 'purchase', month: buy.month, amount: buy.amount! }] }); setBuy({ label: '', month: buy.month }) }}>Add</button>
              </div>
              {(sc.purchases ?? []).map((p, i) => (
                <div key={i} className="mt-1 flex items-center gap-2 text-xs text-ink2">
                  <span className="flex-1">{p.label} · {monthLabel(p.month!)} · <span className="tnum">{eur(p.amount)}</span></span>
                  <button className="text-muted" aria-label={`Remove ${p.label}`} onClick={() => set({ purchases: sc.purchases!.filter((_, j) => j !== i) })}><Icon name="x" size={12} /></button>
                </div>
              ))}
            </div>
            {goals.length > 0 && (
              <div>
                <span className="text-xs text-muted">Wish-list goals in this year</span>
                <div className="mt-1 flex flex-wrap gap-1.5">
                  {goals.map((g) => {
                    const off = sc.skip_goals?.includes(g.id!)
                    return <button key={g.id} className={off ? 'chip line-through opacity-60' : 'chip-on'} aria-pressed={!off}
                      onClick={() => set({ skip_goals: off ? sc.skip_goals!.filter((x) => x !== g.id) : [...(sc.skip_goals ?? []), g.id!] })}>{g.label} · {monthLabel(g.month!).split(' ')[0]} · {eur(g.amount)}</button>
                  })}
                </div>
              </div>
            )}
          </div>
        </Card>

        <Card icon="list" color="var(--s1)" title="Assumed every month">
          <div className="space-y-1.5 text-sm">
            {[['Income', b.assumed.gross ? `net of ${eur(b.assumed.gross)} gross` : 'the plan\'s income base', b.assumed.income, ''],
              ['Fixed obligations', 'alimony, loan', -b.assumed.fixed, ''],
              ['Saving & investing', 'leaves cash for investments', -b.assumed.saving, ''],
              ['Everyday spending', 'a usual month, payments under €250', -b.assumed.everyday, ''],
              ['One-offs', 'average of the past year, beyond dated ones', -b.assumed.one_offs, '']].map(([l, sub, v]) => (
              <div key={l as string} className="flex items-baseline gap-2">
                <span className="min-w-0 flex-1">{l}<span className="block text-xs text-muted sm:inline sm:pl-1.5">{sub}</span></span>
                <span className={clsx('tnum', (v as number) < 0 ? '' : 'text-good')}>{(v as number) < 0 ? '−' : '+'}{eur(Math.abs(v as number))}</span>
              </div>
            ))}
            <div className="flex items-baseline gap-2 border-t border-line pt-1.5 font-semibold">
              <span className="flex-1">A usual month</span>
              {(() => { const n = b.assumed.income - b.assumed.fixed - b.assumed.saving - b.assumed.everyday - b.assumed.one_offs; return <span className={clsx('tnum', n < 0 ? 'text-bad' : 'text-good')}>{n < 0 ? '−' : '+'}{eur(Math.abs(n))}</span> })()}
            </div>
            <p className="pt-1 text-xs text-muted">Not included: bonuses, side income and investing beyond the plan — add them as a what-if. Tested on the past year, the month-to-month trend held; single big moves (a bonus, a €10k top-up) are what it can't know.</p>
            {(b.unscheduled ?? []).length > 0 && <p className="text-xs text-muted">Goals with no date this year: {b.unscheduled!.map((g) => `${g.label} (${eur(g.amount)})`).join(', ')}.</p>}
          </div>
        </Card>
      </div>

      <Card pad={false} title="Month by month" action={<AskCFO q="Look at my 12-month cash forecast (get_forecast). What is the smallest change that keeps cash above my emergency target all year?" label="Ask CFO" />}>
        <div className="overflow-x-auto">
          <table className="w-full text-xs tnum sm:text-sm [&_td]:px-2 [&_td:first-child]:pl-4 [&_td:last-child]:pr-4 [&_th]:px-2 [&_th:first-child]:pl-4 [&_th:last-child]:pr-4">
            <thead className="text-xs text-muted"><tr className="border-y border-line">
              <th className="py-2 text-left font-medium">Month</th><th className="py-2 text-right font-medium">In</th><th className="py-2 text-right font-medium">Out</th>
              <th className="hidden py-2 text-left font-medium sm:table-cell">Dated</th><th className="py-2 text-right font-medium">Cash</th>{s && <th className="py-2 text-right font-medium">What-if</th>}
            </tr></thead>
            <tbody className="divide-y divide-line">
              {b.months.map((m, i) => {
                const out = m.fixed + m.saving + m.everyday + m.one_offs + (m.dated ?? []).reduce((a, x) => a + x.amount, 0)
                const alt = s?.months[i]
                const tone = (v: number, f: number) => (v < 0 ? 'text-bad' : v < f ? 'text-warn' : '')
                return (
                  <tr key={m.month}>
                    <td className="whitespace-nowrap py-1.5 font-medium">{monthLabel(m.month)}{(m.dated ?? []).length > 0 && <span className="text-warn sm:hidden" title={(m.dated ?? []).map((x) => `${x.label} ${eur(x.amount)}`).join(', ')}> •</span>}</td>
                    <td className="py-1.5 text-right text-good">{eur(m.income)}</td>
                    <td className="py-1.5 text-right">{eur(out)}</td>
                    <td className="hidden max-w-[10rem] truncate py-1.5 text-muted sm:table-cell">{(m.dated ?? []).map((x) => `${x.label} ${eurk(x.amount)}`).join(', ')}</td>
                    <td className={clsx('py-1.5 text-right font-medium', tone(m.end, b.floor))}>{eur(m.end)}</td>
                    {s && <td className={clsx('py-1.5 text-right font-semibold', tone(alt!.end, b.floor))}>{eur(alt!.end)}</td>}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
