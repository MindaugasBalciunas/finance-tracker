import { useId } from 'react'

/** The app mark: a rising line that ends in a bright point, on a tile that
 *  runs from the palette's blue to its violet — money that grows. */
export function LogoMark({ size = 28, className = '' }: { size?: number; className?: string }) {
  // Gradient ids must be unique per copy: a copy inside a hidden element
  // (the desktop sidebar on a phone) would otherwise blank every other one.
  const uid = useId().replace(/[^a-zA-Z0-9]/g, '')
  const tile = `ft-tile-${uid}`, shine = `ft-shine-${uid}`
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" className={className} aria-hidden>
      <defs>
        <linearGradient id={tile} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#3b8cf0" />
          <stop offset="100%" stopColor="#5b3fc4" />
        </linearGradient>
        <linearGradient id={shine} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#fff" stopOpacity=".28" />
          <stop offset="60%" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
      </defs>
      <rect width="64" height="64" rx="16" fill={`url(#${tile})`} />
      <rect width="64" height="64" rx="16" fill={`url(#${shine})`} />
      {/* bars: the ledger underneath */}
      <rect x="14" y="40" width="7" height="11" rx="2" fill="#fff" fillOpacity=".35" />
      <rect x="25" y="35" width="7" height="16" rx="2" fill="#fff" fillOpacity=".35" />
      <rect x="36" y="29" width="7" height="22" rx="2" fill="#fff" fillOpacity=".35" />
      {/* the line: net worth rising */}
      <path d="M12 36 L24 27 L33 31 L48 16" fill="none" stroke="#fff" strokeWidth="4.5" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="49" cy="15" r="5.5" fill="#ffd25a" stroke="#fff" strokeWidth="2.5" />
    </svg>
  )
}

export function Logo({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <span className="flex items-center gap-2.5">
      <LogoMark size={30} />
      {!collapsed && <span className="text-base font-semibold tracking-tight">Finance<span className="text-accent">.</span></span>}
    </span>
  )
}
