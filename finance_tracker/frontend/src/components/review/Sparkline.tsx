import { ACCENT, MUTED } from './colors'

// A 12-point trend in the de-emphasis gray with the current point accented.
// Decorative context for a stat tile — the tile's number is the reading.
export default function Sparkline({ values, accent = ACCENT, height = 28 }: { values: number[]; accent?: string; height?: number }) {
  if (values.length < 2) return null
  const w = 100
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const pad = 3
  const pts = values.map((v, i) => [
    (i / (values.length - 1)) * w,
    pad + (1 - (v - min) / span) * (height - pad * 2),
  ])
  const [lx, ly] = pts[pts.length - 1]
  return (
    <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className="w-full overflow-visible" style={{ height }} aria-hidden>
      <polyline
        points={pts.map((p) => p.join(',')).join(' ')}
        fill="none"
        stroke={MUTED}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
      <circle cx={lx} cy={ly} r={4} fill={accent} stroke="#fff" strokeWidth={2} vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
