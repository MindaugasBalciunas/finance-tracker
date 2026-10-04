import { ACCENT, MUTED } from './colors'

// A 12-point trend in the de-emphasis gray with the current point accented.
// Decorative context for a stat tile — the tile's number is the reading.
//
// The line's SVG stretches to the tile's width, which would squash a circle
// into an oval, so the end dot is an HTML element placed over the line's end.
export default function Sparkline({ values, accent = ACCENT, height = 28 }: { values: number[]; accent?: string; height?: number }) {
  if (values.length < 2) return null
  const w = 100
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const pad = 5
  const pts = values.map((v, i) => [
    (i / (values.length - 1)) * w,
    pad + (1 - (v - min) / span) * (height - pad * 2),
  ])
  const [lx, ly] = pts[pts.length - 1]
  return (
    <div className="relative" style={{ height }} aria-hidden>
      <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className="absolute inset-0 w-full h-full overflow-visible">
        <polyline
          points={pts.map((p) => p.join(',')).join(' ')}
          fill="none"
          stroke={MUTED}
          strokeWidth={1.5}
          strokeLinejoin="round"
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      <span
        className="absolute w-2.5 h-2.5 rounded-full ring-2 ring-white"
        style={{ backgroundColor: accent, left: `${lx}%`, top: ly, transform: 'translate(-50%, -50%)' }}
      />
    </div>
  )
}
