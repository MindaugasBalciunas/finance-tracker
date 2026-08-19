import {
  LineChart,
  Line,
  BarChart,
  Bar,
  AreaChart,
  Area,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'

// ChatChart renders a chart the AI emitted as a fenced ```chart JSON block.
// parseChartSpec validates the untrusted JSON; the component renders whatever
// survives, coercing bad cells to 0 so a malformed row never throws mid-chat.

export type ChartType = 'line' | 'bar' | 'area' | 'pie'

export interface ChartSeries {
  key: string
  name?: string
  color?: string
}

export interface ChartSpec {
  type: ChartType
  title?: string
  x: string
  unit?: string
  series: ChartSeries[]
  data: Array<Record<string, unknown>>
}

const TYPES: ChartType[] = ['line', 'bar', 'area', 'pie']

// Categorical palette, shared visual language with components/charts.
const PALETTE = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#14b8a6']

export function parseChartSpec(raw: string): ChartSpec | null {
  let obj: unknown
  try {
    obj = JSON.parse(raw)
  } catch {
    return null
  }
  if (!obj || typeof obj !== 'object') return null
  const o = obj as Record<string, unknown>

  if (typeof o.type !== 'string' || !TYPES.includes(o.type as ChartType)) return null
  if (typeof o.x !== 'string' || o.x.trim() === '') return null
  if (!Array.isArray(o.data) || o.data.length === 0) return null
  if (!Array.isArray(o.series)) return null

  const series: ChartSeries[] = []
  for (const s of o.series) {
    if (!s || typeof s !== 'object') continue
    const sk = (s as Record<string, unknown>).key
    if (typeof sk !== 'string' || sk.trim() === '') continue
    const name = (s as Record<string, unknown>).name
    const color = (s as Record<string, unknown>).color
    series.push({
      key: sk,
      name: typeof name === 'string' ? name : undefined,
      color: typeof color === 'string' ? color : undefined,
    })
  }
  if (series.length === 0) return null

  return {
    type: o.type as ChartType,
    title: typeof o.title === 'string' ? o.title : undefined,
    x: o.x,
    unit: typeof o.unit === 'string' ? o.unit : undefined,
    series,
    data: o.data as Array<Record<string, unknown>>,
  }
}

function toNum(v: unknown): number {
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) ? n : 0
}

// Thousands-grouped whole number: 1234567 -> "1 234 567".
function group(n: number): string {
  const neg = n < 0
  const s = Math.abs(Math.round(n))
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
  return neg ? `-${s}` : s
}

// Full value with unit — used in tooltips.
function formatFull(value: number, unit?: string): string {
  const body = group(value)
  if (unit === '€') return `${body} €`
  if (unit === '$') return `$${body}`
  return unit ? `${body} ${unit}` : body
}

// Compact value with unit — used on crowded axis ticks.
function formatCompact(value: number, unit?: string): string {
  const abs = Math.abs(value)
  let body: string
  if (abs >= 1_000_000) body = `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`
  else if (abs >= 1_000) body = `${(value / 1_000).toFixed(1).replace(/\.0$/, '')}k`
  else body = group(value)
  if (unit === '€') return `${body} €`
  if (unit === '$') return `$${body}`
  return unit ? `${body} ${unit}` : body
}

const seriesColor = (s: ChartSeries, i: number): string => s.color || PALETTE[i % PALETTE.length]

export default function ChatChart({ spec }: { spec: ChartSpec }) {
  const { type, unit } = spec

  // Normalize rows: keep the x value, coerce every series cell to a finite number.
  const rows = spec.data.map((row) => {
    const out: Record<string, unknown> = { [spec.x]: row[spec.x] ?? '' }
    for (const s of spec.series) out[s.key] = toNum(row[s.key])
    return out
  })

  const TooltipContent = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
        {label != null && label !== '' && (
          <p className="font-semibold text-gray-700 mb-1">{String(label)}</p>
        )}
        {payload.map((p: any, i: number) => (
          <div key={i} className="flex justify-between gap-3">
            <span style={{ color: p.color || p.fill }}>{p.name}</span>
            <span className="font-medium text-gray-700">{formatFull(toNum(p.value), unit)}</span>
          </div>
        ))}
      </div>
    )
  }

  const angled = rows.length > 6
  const showLegend = spec.series.length > 1 || type === 'pie'

  const commonAxes = (
    <>
      <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
      <XAxis
        dataKey={spec.x}
        tick={{ fontSize: 10 }}
        angle={angled ? -30 : 0}
        textAnchor={angled ? 'end' : 'middle'}
        height={angled ? 48 : 24}
        interval={rows.length > 12 ? 'preserveStartEnd' : 0}
      />
      <YAxis tick={{ fontSize: 10 }} width={48} tickFormatter={(v) => formatCompact(toNum(v), unit)} />
      <Tooltip content={<TooltipContent />} />
      {showLegend && <Legend wrapperStyle={{ fontSize: 11 }} />}
    </>
  )

  let chart: React.ReactElement
  if (type === 'line') {
    chart = (
      <LineChart data={rows} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
        {commonAxes}
        {spec.series.map((s, i) => (
          <Line
            key={s.key}
            type="monotone"
            dataKey={s.key}
            name={s.name || s.key}
            stroke={seriesColor(s, i)}
            strokeWidth={2}
            dot={{ r: 2 }}
            activeDot={{ r: 4 }}
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    )
  } else if (type === 'bar') {
    chart = (
      <BarChart data={rows} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
        {commonAxes}
        {spec.series.map((s, i) => (
          <Bar
            key={s.key}
            dataKey={s.key}
            name={s.name || s.key}
            fill={seriesColor(s, i)}
            radius={[3, 3, 0, 0]}
            isAnimationActive={false}
          />
        ))}
      </BarChart>
    )
  } else if (type === 'area') {
    chart = (
      <AreaChart data={rows} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
        {commonAxes}
        {spec.series.map((s, i) => (
          <Area
            key={s.key}
            type="monotone"
            dataKey={s.key}
            name={s.name || s.key}
            stroke={seriesColor(s, i)}
            fill={seriesColor(s, i)}
            fillOpacity={0.2}
            strokeWidth={2}
            isAnimationActive={false}
          />
        ))}
      </AreaChart>
    )
  } else {
    // pie — single series: series[0].key is the value, x is the slice label.
    const valueKey = spec.series[0].key
    chart = (
      <PieChart margin={{ top: 4, right: 4, left: 4, bottom: 4 }}>
        <Pie
          data={rows}
          dataKey={valueKey}
          nameKey={spec.x}
          cx="50%"
          cy="50%"
          outerRadius={80}
          isAnimationActive={false}
        >
          {rows.map((_, i) => (
            <Cell key={i} fill={spec.series[0].color || PALETTE[i % PALETTE.length]} />
          ))}
        </Pie>
        <Tooltip content={<TooltipContent />} />
        {showLegend && <Legend wrapperStyle={{ fontSize: 11 }} />}
      </PieChart>
    )
  }

  return (
    <div className="w-full my-1 rounded-lg border border-gray-200 bg-white p-2">
      {spec.title && <p className="text-xs font-semibold text-gray-700 mb-1 px-1">{spec.title}</p>}
      <ResponsiveContainer width="100%" height={240}>
        {chart}
      </ResponsiveContainer>
    </div>
  )
}
