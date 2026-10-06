import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { usePeriod } from '../../lib/hooks'
import { catColor, catIcon, useCats } from '../../lib/categories'
import { eur, eurk, pct, shortDate } from '../../lib/format'
import type { Tx } from '../../lib/types'
import { Card, Loading } from '../../components/ui'
import { axisProps, Donut, foldSlices, gridProps, Legend, ShareBar, TooltipBox } from '../../components/charts'
import { TxRow, useTxEditor } from '../../components/TxEditor'
import { Icon, IconTile } from '../../components/Icon'

// ── spending ────────────────────────────────────────────────────────

const PRESETS = [{ value: 'month', label: 'This month' }, { value: 'last_month', label: 'Last month' }, { value: '3m', label: '3 months' }, { value: '6m', label: '6 months' }, { value: 'ytd', label: 'This year' }, { value: '12m', label: '12 months' }, { value: 'last_year', label: 'Last year' }]

export function Spending() {
  const [preset, setPreset] = usePeriod('spending', '12m', PRESETS.map((p) => p.value))
  const [open, setOpen] = useState<string | null>(null)
  const cats = useCats()
  const editor = useTxEditor()
  const { data, isLoading } = useQuery({ queryKey: ['breakdown', preset], queryFn: () => api.get<any>('/insights/breakdown', { preset }) })
  if (isLoading || !data) return <Loading />
  const total = data.categories.reduce((a: number, c: any) => a + c.total, 0)
  const max = Math.max(...data.categories.map((c: any) => c.total), 1)
  return (
    <div className="space-y-4">
      <div className="no-scrollbar overflow-y-hidden -mx-4 flex gap-1.5 overflow-x-auto px-4 sm:mx-0 sm:px-0">
        {PRESETS.map((p) => <button key={p.value} className={preset === p.value ? 'chip-on' : 'chip'} onClick={() => setPreset(p.value)}>{p.label}</button>)}
      </div>
      <PaceCard />
      <div className="text-sm text-muted">{shortDate(data.from)} – {shortDate(data.to)} · <b className="text-ink">{eur(total)}</b> spent, compared with the same length before</div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card pad={false} icon="bag" color="var(--s2)" title="By category" className="lg:col-span-2">
          <div className="divide-y divide-line border-t border-line">
            {data.categories.map((c: any) => (
              <div key={c.category}>
                <button className="w-full px-4 py-2.5 text-left hover:bg-sunken/40" onClick={() => setOpen(open === c.category ? null : c.category)}>
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="flex items-center gap-2 text-sm font-medium"><IconTile name={catIcon(c.category)[0]} color={catIcon(c.category)[1]} size={26} />{cats.name(c.category)}</span>
                    <span className="tnum text-sm"><b>{eur(c.total)}</b> <span className="text-xs text-muted">{pct(c.share)}</span></span>
                  </div>
                  <div className="mt-1.5 flex items-center gap-3">
                    <ShareBar value={c.total} max={max} />
                    <span className={clsx('w-16 shrink-0 text-right text-xs tnum', c.change > 0.1 ? 'text-bad' : c.change < -0.1 ? 'text-good' : 'text-muted')}>
                      {c.previous > 0 ? `${c.change > 0 ? '+' : ''}${pct(c.change)}` : 'new'}
                    </span>
                  </div>
                  <div className="mt-0.5 text-xs text-muted">{eur(c.monthly_avg)}/mo{c.top_merchant ? ` · mostly ${c.top_merchant}` : ''}</div>
                </button>
                {open === c.category && c.children?.length > 0 && (
                  <div className="bg-sunken/40 px-4 py-2">
                    {c.children.map((ch: any) => (
                      <a key={ch.category} href={`#/ledger?category=${ch.category}&from=${data.from}&to=${data.to}`} className="flex items-center justify-between py-1 text-sm">
                        <span className="text-ink2">{cats.name(ch.category)}</span>
                        <span className="tnum">{eur(ch.total)} <span className="text-xs text-muted">{ch.previous > 0 ? `${ch.change > 0 ? '+' : ''}${pct(ch.change)}` : ''}</span></span>
                      </a>
                    ))}
                    <a href={`#/ledger?category=${c.category}&from=${data.from}&to=${data.to}`} className="mt-1 inline-flex items-center gap-1 text-xs text-accent">Transactions<Icon name="chevronR" size={12} /></a>
                  </div>
                )}
              </div>
            ))}
          </div>
        </Card>
        <div className="space-y-4">
          <Card icon="pie" color="var(--s2)" title="Where it went">
            <Donut slices={foldSlices(data.categories.map((c: any) => ({ key: c.category, label: cats.name(c.category), value: c.total, color: catColor(c.category) })))} sub="spent" />
          </Card>
          <Card pad={false} icon="utensils" color="var(--s4)" title="Top merchants">
            <div className="divide-y divide-line border-t border-line">
              {data.merchants.slice(0, 12).map((m: any) => (
                <a key={m.name} href={`#/ledger?merchant=${encodeURIComponent(m.name)}&from=${data.from}&to=${data.to}`} className="flex items-center justify-between px-4 py-2 text-sm hover:bg-sunken/40">
                  <span className="min-w-0 truncate">{m.name} <span className="text-xs text-muted">×{m.count}</span></span>
                  <span className="tnum">{eur(m.amount)}</span>
                </a>
              ))}
            </div>
          </Card>
          {data.tags?.length > 0 && (
            <Card pad={false} title="By tag" action={<span className="text-xs text-muted">people · trips · properties</span>}>
              <div className="divide-y divide-line border-t border-line">
                {data.tags.slice(0, 12).map((t: any) => (
                  <a key={t.name} href={`#/ledger?tag=${encodeURIComponent(t.name)}&from=${data.from}&to=${data.to}`} className="flex items-center justify-between px-4 py-2 text-sm hover:bg-sunken/40">
                    <span className="min-w-0 truncate">#{t.name} <span className="text-xs text-muted">×{t.count}</span></span>
                    <span className="tnum">{eur(t.amount)}</span>
                  </a>
                ))}
              </div>
            </Card>
          )}
          <Card pad={false} title="Largest expenses">
            <div className="divide-y divide-line border-t border-line">
              {data.largest.slice(0, 8).map((t: Tx) => <TxRow key={t.id} t={t} showDate onClick={() => editor.open(t)} />)}
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}

function PaceCard() {
  const { data } = useQuery({ queryKey: ['pace'], queryFn: () => api.get<{ day: number; current?: number; last_month: number; typical: number }[]>('/insights/pace') })
  if (!data) return null
  const today = data.filter((p) => p.current != null).pop()
  const ahead = today ? today.current! - today.typical : 0
  return (
    <Card title="This month so far" action={today && <span className={clsx('text-sm tnum', ahead > 0 ? 'text-bad' : 'text-good')}>{ahead > 0 ? `${eur(ahead)} ahead of` : `${eur(-ahead)} below`} a typical month</span>}>
      <div className="h-48">
        <ResponsiveContainer>
          <LineChart data={data} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="day" {...axisProps} interval={4} />
            <YAxis {...axisProps} tickFormatter={eurk} width={48} />
            <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
              <TooltipBox title={`Day ${label}`} rows={payload.filter((p: any) => p.value != null).map((p: any) => ({ color: p.stroke, label: p.name, value: eur(p.value) }))} />) : null} />
            <Line dataKey="typical" name="Typical (6-month avg)" stroke="var(--s-other)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Line dataKey="last_month" name="Last month" stroke="var(--s2)" strokeWidth={2} dot={false} isAnimationActive={false} />
            <Line dataKey="current" name="This month" stroke="var(--s1)" strokeWidth={2.5} dot={false} isAnimationActive={false} connectNulls={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-2"><Legend items={[{ color: 'var(--s1)', label: 'This month' }, { color: 'var(--s2)', label: 'Last month' }, { color: 'var(--s-other)', label: 'Typical (6-month avg)' }]} /></div>
    </Card>
  )
}
