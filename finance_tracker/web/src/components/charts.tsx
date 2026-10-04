import { ReactNode } from 'react'
import { Area, AreaChart, ResponsiveContainer } from 'recharts'
import { eurc, eurk } from '../lib/format'

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
