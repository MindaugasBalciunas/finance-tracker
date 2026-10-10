import { ReactNode, useState } from 'react'
import clsx from 'clsx'
import { Area, AreaChart, Cell, Pie, PieChart, ResponsiveContainer } from 'recharts'
import { eur, eurc, eurk } from '../lib/format'

// Shared chart chrome: recessive axes and hairline grid in the palette's
// chart tokens, a tooltip on every chart, HTML legends (identity is never
// colour alone), no animation (screenshots and slow phones render blank).

export const axisProps = {
  stroke: 'var(--chart-axis)',
  tick: { fill: 'var(--chart-text)', fontSize: 11 },
  tickLine: false,
  axisLine: false,
} as const

export const gridProps = { stroke: 'var(--chart-grid)', strokeDasharray: undefined, vertical: false } as const

export const moneyTick = (v: number) => eurk(v)

export function TooltipBox({ title, rows, footer }: { title: ReactNode; rows: { color?: string; label: ReactNode; value: ReactNode; bold?: boolean }[]; footer?: ReactNode }) {
  return (
    <div className="rounded-xl border border-line bg-raised px-3 py-2 text-xs shadow-lg min-w-[160px]">
      <div className="mb-1 font-semibold text-ink">{title}</div>
      {rows.map((r, i) => (
        <div key={i} className="flex items-center justify-between gap-4 py-0.5">
          <span className="flex items-center gap-1.5 text-ink2">
            {r.color && <span className="h-2 w-2 rounded-sm" style={{ background: r.color }} />}
            {r.label}
          </span>
          <span className={`tnum ${r.bold ? 'font-semibold text-ink' : 'text-ink'}`}>{r.value}</span>
        </div>
      ))}
      {footer && <div className="mt-1 border-t border-line pt-1 text-muted">{footer}</div>}
    </div>
  )
}

/** Recharts custom tooltip that lists the hovered payload in money. */
export function MoneyTooltip({ active, payload, label, labelFormat, total }: any) {
  if (!active || !payload?.length) return null
  const rows = payload
    .filter((p: any) => p.value != null && p.value !== 0)
    .map((p: any) => ({ color: p.color || p.fill || p.stroke, label: p.name, value: eurc(p.value) }))
  const sum = payload.reduce((a: number, p: any) => a + (Number(p.value) || 0), 0)
  return <TooltipBox title={labelFormat ? labelFormat(label) : label} rows={rows} footer={total ? <div className="flex justify-between"><span>Total</span><span className="tnum text-ink">{eurc(sum)}</span></div> : undefined} />
}

export function Legend({ items }: { items: { color: string; label: ReactNode; value?: ReactNode }[] }) {
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-ink2">
      {items.map((it, i) => (
        <span key={i} className="inline-flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: it.color }} />
          {it.label}
          {it.value != null && <span className="tnum text-ink">{it.value}</span>}
        </span>
      ))}
    </div>
  )
}

export function Sparkline({ data, height = 44, color = 'var(--s1)' }: { data: { value: number }[]; height?: number; color?: string }) {
  if (!data?.length) return null
  return (
    <div style={{ height }} className="w-full">
      <ResponsiveContainer>
        <AreaChart data={data} margin={{ top: 2, right: 0, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="spark" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.25} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area type="monotone" dataKey="value" stroke={color} strokeWidth={2} fill="url(#spark)" isAnimationActive={false} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

/** A horizontal share bar for ranked lists (one hue: magnitude, not identity). */
export function ShareBar({ value, max, color = 'rgb(var(--accent))' }: { value: number; max: number; color?: string }) {
  const w = max > 0 ? Math.max(2, (Math.abs(value) / max) * 100) : 0
  return (
    <div className="h-1.5 w-full rounded-full bg-sunken">
      <div className="h-full rounded-full" style={{ width: `${Math.min(w, 100)}%`, background: color }} />
    </div>
  )
}

export type Slice = { key: string; label: string; value: number; color: string }

/** Keep the largest slices (fixed colours stay with their entity) and fold
 *  the rest into one neutral "Other" — never more than max-1 hues. */
export function foldSlices(slices: Slice[], max = 8): Slice[] {
  const pos = slices.filter((s) => s.value > 0).sort((a, b) => b.value - a.value)
  if (pos.length <= max) return pos
  const keep = pos.slice(0, max - 1)
  const rest = pos.slice(max - 1).reduce((a, s) => a + s.value, 0)
  return [...keep, { key: 'other', label: 'Other', value: rest, color: 'var(--s-other)' }]
}

/** Donut with the total in the middle, a tooltip, and an HTML legend with shares. */
export function Donut({ slices, center, sub, height = 200, legend = true, stacked = false }: { slices: Slice[]; center?: ReactNode; sub?: ReactNode; height?: number; legend?: boolean; stacked?: boolean }) {
  // The numbers live on the donut itself: each sizeable slice carries its
  // share, and a tapped (or hovered) slice takes over the centre — no
  // floating tooltip to end up behind the ring. A tap is hover then click,
  // so a click selects (never toggles); tapping the hole goes back.
  const [active, setActive] = useState<number | null>(null)
  const total = slices.reduce((a, s) => a + s.value, 0)
  if (!total) return null
  const on = active != null ? slices[active] : null
  const label = ({ cx, cy, midAngle, innerRadius, outerRadius, percent }: any) => {
    if (percent < 0.07) return null
    const r = (innerRadius + outerRadius) / 2
    const a = (-midAngle * Math.PI) / 180
    return (
      <text x={cx + r * Math.cos(a)} y={cy + r * Math.sin(a)} textAnchor="middle" dominantBaseline="central" className="pointer-events-none"
        style={{ fontSize: 10, fontWeight: 600, fill: '#fff', paintOrder: 'stroke', stroke: 'rgba(0,0,0,.35)', strokeWidth: 2 }}>
        {Math.round(percent * 100)}%
      </text>
    )
  }
  return (
    <div className={`flex w-full flex-col items-center gap-3 ${stacked ? '' : 'sm:flex-row sm:items-center'}`}>
      <div className="relative mx-auto w-full max-w-[220px] shrink-0" style={{ height, minWidth: Math.min(height, 220) }} onMouseLeave={() => setActive(null)}
        onClick={(e) => { if (!(e.target as Element).closest('.recharts-sector')) setActive(null) }}>
        <ResponsiveContainer>
          <PieChart>
            <Pie data={slices} dataKey="value" nameKey="label" innerRadius="62%" outerRadius="92%" paddingAngle={slices.length > 1 ? 1.5 : 0}
              stroke="var(--chart-surface)" strokeWidth={2} isAnimationActive={false} startAngle={90} endAngle={-270}
              label={label} labelLine={false}>
              {slices.map((s, i) => <Cell key={s.key} fill={s.color} fillOpacity={active == null || active === i ? 1 : 0.45} className="cursor-pointer outline-none"
                onMouseEnter={() => setActive(i)} onClick={() => setActive(i)} />)}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center px-[22%] text-center">
          {on ? (<>
            <div className="max-w-full truncate text-[11px] text-ink2">{on.label}</div>
            <div className="text-base font-semibold tnum">{eurc(on.value)}</div>
            <div className="text-[11px] tnum text-muted">{Math.round((on.value / total) * 1000) / 10}%</div>
          </>) : (<>
            <div className="text-base font-semibold tnum">{center ?? eurk(total)}</div>
            {sub && <div className="text-[11px] text-muted">{sub}</div>}
          </>)}
        </div>
      </div>
      {legend && (
        <div className="grid w-full min-w-0 grid-cols-1 gap-y-1 text-xs">
          {slices.map((s) => (
            <div key={s.key} className="flex items-center gap-2">
              <span className="h-2.5 w-2.5 shrink-0 rounded-[3px]" style={{ background: s.color }} />
              <span className="min-w-0 flex-1 truncate text-ink2">{s.label}</span>
              <span className="tnum text-muted">{Math.round((s.value / total) * 100)}%</span>
              <span className="w-14 text-right tnum text-ink">{eurk(s.value)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/** Where the money sits, as on Home: each group is one column — its bar
 *  segment on top, its name and full amount right underneath, so labels line
 *  up with their segment. A column grows with its share but never narrower
 *  than its amount. Debt, when given, gets its own line. */
export function GroupBar({ parts, debt }: { parts: { id: string; name: string; slot: number | string; v: number }[]; debt?: number }) {
  const total = parts.reduce((t, g) => t + g.v, 0)
  if (!total) return null
  return (
    <>
      <div className="flex w-full gap-1">
        {parts.map((g, i) => (
          <div key={g.id} className="min-w-0" style={{ flex: `${g.v / total} 1 0%`, minWidth: 'max-content' }}>
            <div className={clsx('h-2', i === 0 && 'rounded-l-full', i === parts.length - 1 && 'rounded-r-full')} style={{ background: `var(--s${g.slot})` }} />
            <div className="mt-1.5 flex flex-col pr-1 sm:flex-row sm:items-baseline sm:gap-1.5">
              <span className="whitespace-nowrap text-[11px] text-ink2 sm:text-xs">{g.name}</span>
              <span className="whitespace-nowrap text-xs font-semibold tnum sm:text-sm">{eur(g.v)}</span>
            </div>
          </div>
        ))}
      </div>
      {!!debt && debt > 0 && (
        <div className="mt-2 flex items-baseline gap-1.5 text-xs sm:text-sm">
          <span className="h-2 w-2 shrink-0 self-center rounded-[3px] border border-axis" />
          <span className="text-ink2">Debt</span>
          <span className="font-semibold tnum text-bad">−{eur(debt)}</span>
        </div>
      )}
    </>
  )
}
