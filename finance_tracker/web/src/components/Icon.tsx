// Minimal stroke icon set (24px grid, 1.75 stroke).
const P: Record<string, string> = {
  home: 'M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
  list: 'M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01',
  target: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zm0-4a5 5 0 1 0 0-10 5 5 0 0 0 0 10zm0-4a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
  bank: 'M3 9.5 12 4l9 5.5M5 10v8M9.5 10v8M14.5 10v8M19 10v8M3 20h18',
  chart: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
  spark: 'M12 3.75l1.95 6.3 6.3 1.95-6.3 1.95L12 20.25l-1.95-6.3L3.75 12l6.3-1.95z',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zm7.4-3a7.4 7.4 0 0 0-.1-1.2l2-1.6-2-3.4-2.4 1a7 7 0 0 0-2-1.2L14.5 3h-5l-.4 2.6a7 7 0 0 0-2 1.2l-2.4-1-2 3.4 2 1.6a7.4 7.4 0 0 0 0 2.4l-2 1.6 2 3.4 2.4-1a7 7 0 0 0 2 1.2l.4 2.6h5l.4-2.6a7 7 0 0 0 2-1.2l2.4 1 2-3.4-2-1.6c.1-.4.1-.8.1-1.2z',
  // Categories and account kinds (coloured tiles).
  utensils: 'M7 3v8a2 2 0 0 0 2 2v8M11 3v8a2 2 0 0 1-2 2M9 3v7M17 21V3c-2.2 1.2-3 4-3 7 0 1.6 1 2.5 3 2.5',
  bolt: 'M13 2 4.5 13.5H11L10 22l8.5-11.5H12z',
  baby: 'M12 21a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM9.5 11.5h.01M14.5 11.5h.01M9.5 15a3.5 3.5 0 0 0 5 0M12 5c0-1.2.8-2 2-2',
  bag: 'M5 8h14l-1.2 13H6.2zM9 10V6.5a3 3 0 0 1 6 0V10',
  heart: 'M12 20s-7.5-4.6-7.5-10.2A4.3 4.3 0 0 1 12 7.3a4.3 4.3 0 0 1 7.5 2.5C19.5 15.4 12 20 12 20z',
  pulse: 'M3 12h4l2.2-5.5 4 11L15.5 12H21',
  car: 'M4.5 16.5v-5l2-5h11l2 5v5M4 11.5h16M4.5 16.5h15M6 16.5V19M18 16.5V19M8 14h.01M16 14h.01',
  gift: 'M4 11h16v10H4zM3 7h18v4H3zM12 7v14M12 7S10.5 3 8 3.5 7.5 7 12 7zM12 7s1.5-4 4-3.5S16.5 7 12 7z',
  ticket: 'M3 7h18v3a2 2 0 0 0 0 4v3H3v-3a2 2 0 0 0 0-4zM14.5 7v10',
  percent: 'M19 5 5 19M7 9.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM17 19.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  repeat: 'M17 2.5 20 5.5l-3 3M4 11.5v-2a4 4 0 0 1 4-4h12M7 21.5l-3-3 3-3M20 12.5v2a4 4 0 0 1-4 4H4',
  briefcase: 'M3 8h18v12H3zM8.5 8V5h7v3M3 13h18',
  key: 'M8 16.5a4.5 4.5 0 1 1 4.1-6.5H21v3h-2v2.5h-3V13h-3.9A4.5 4.5 0 0 1 8 16.5zM7 12h.01',
  shield: 'M12 3l7.5 3v6c0 4.8-3.3 7.8-7.5 9-4.2-1.2-7.5-4.2-7.5-9V6z',
  trend: 'M3 17l6-6 4 4 8-8M15 7h6v6',
  undo: 'M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3',
  coins: 'M15 11c3.3 0 6-1.3 6-3s-2.7-3-6-3-6 1.3-6 3 2.7 3 6 3zM9 8v4c0 1.7 2.7 3 6 3s6-1.3 6-3V8M3 13c0 1.7 2.7 3 6 3M3 13v4c0 1.7 2.7 3 6 3 1.3 0 2.5-.2 3.5-.6M3 13c0-1.4 1.8-2.5 4.4-2.9',
  swap: 'M7 4 3 8l4 4M3 8h14M17 20l4-4-4-4M21 16H7',
  dots: 'M5 12h.01M12 12h.01M19 12h.01',
  piggy: 'M4 4.5h16v14H4zM12 15a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM12 8v1.2M15.5 11.5h-1.2M6 18.5V20M18 18.5V20',
  umbrella: 'M12 3a9 9 0 0 1 9 9H3a9 9 0 0 1 9-9zM12 12v6.5a2 2 0 0 0 4 0',
  bitcoin: 'M8 5.5h6a3 3 0 0 1 0 6H8zM8 11.5h7a3 3 0 0 1 0 6H8zM8 5.5v12M10 3.5v2M13.5 3.5v2M10 17.5v2M13.5 17.5v2M6 5.5h2M6 17.5h2',
  card: 'M3 6h18v12H3zM3 10h18M7 15h3',
  calendar: 'M4 6h16v15H4zM4 10h16M8 3v4M16 3v4',
  pie: 'M12 3v9h9a9 9 0 1 1-9-9zM15 3.5A9 9 0 0 1 20.5 9H15z',
  sun2: 'M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  plus: 'M12 5v14M5 12h14',
  menu: 'M4 6.5h16M4 12h16M4 17.5h10',
  copy: 'M9 9h10a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V10a1 1 0 0 1 1-1zM5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1',
  share: 'M12 3v12M7.5 7.5 12 3l4.5 4.5M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-7',
  x: 'M6 6l12 12M18 6 6 18',
  check: 'M5 12.5l4.5 4.5L19 7',
  chevronR: 'M9 6l6 6-6 6',
  chevronL: 'M15 6l-6 6 6 6',
  chevronD: 'M6 9l6 6 6-6',
  search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zm9 3-4.3-4.3',
  camera: 'M4 8h3l2-3h6l2 3h3a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1zm8 10a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  refresh: 'M20 11a8 8 0 0 0-14.6-4.5M4 4v4h4M4 13a8 8 0 0 0 14.6 4.5M20 20v-4h-4',
  inbox: 'M3 13h5l2 3h4l2-3h5M5 5h14l2 8v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-6z',
  alert: 'M12 9v4m0 4h.01M10.3 3.9 2.4 18a2 2 0 0 0 1.7 3h15.8a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z',
  up: 'M7 14l5-5 5 5',
  down: 'M7 10l5 5 5-5',
  trash: 'M4 7h16M10 11v6M14 11v6M5 7l1 13h12l1-13M9 7V4h6v3',
  edit: 'M4 20h4L19 9l-4-4L4 16zM14 6l4 4',
  link: 'M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1',
  send: 'M4 12 20 4l-6 16-3-7z',
  image: 'M4 5h16v14H4zM4 16l5-5 4 4 3-3 4 4M15 9h.01',
  lock: 'M6 11h12v10H6zM8.5 11V7.5a3.5 3.5 0 0 1 7 0V11',
  fingerprint: 'M12 11v3a8 8 0 0 1-1.5 4.5M8 11a4 4 0 0 1 8 0v1.5M5 12a7 7 0 0 1 12.6-4.2M19 12v1a12 12 0 0 1-.8 4.3M12 4a8 8 0 0 0-2.5.4',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
  filter: 'M4 5h16l-6 8v5l-4 2v-7z',
  tag: 'M3 12V4h8l10 10-8 8zM7.5 8.5h.01',
  rule: 'M4 6h10M4 12h7M4 18h10M17 9l3 3-3 3',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  wallet: 'M4 7h15a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1zm0 0V6a2 2 0 0 1 2-2h11M16 13.5h.01',
  plane: 'M10 14 3 11l1.5-1.5 7 1 4-4.5c1-1 2.5-1.5 3-1s0 2-1 3l-4.5 4 1 7L12.5 21z',
  download: 'M12 4v11m0 0 4-4m-4 4-4-4M4 20h16',
  upload: 'M12 16V5m0 0 4 4m-4-4-4 4M4 20h16',
}
// Optical centring for drawings whose bounding box isn't centred in 24×24
// (measured with getBBox); keeps icons level with the text beside them.
const NUDGE: Record<string, string> = { plane: 'translate(1.15 -0.9)', heart: 'translate(0 -0.93)' }

export function Icon({ name, size = 20, className = '' }: { name: keyof typeof P | string; size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" className={`shrink-0 ${className}`} aria-hidden>
      <path d={P[name] ?? ''} transform={NUDGE[name]} />
    </svg>
  )
}

/** A coloured icon on a soft tint of the same colour. Colour carries
 *  identity (category, account, section); the label always sits beside it. */
export function IconTile({ name, color, size = 36, round = false, className = '' }: { name: string; color: string; size?: number; round?: boolean; className?: string }) {
  return (
    <span className={`grid shrink-0 place-items-center ${round ? 'rounded-full' : 'rounded-xl'} ${className}`}
      style={{ width: size, height: size, background: `color-mix(in oklab, ${color} var(--tint), transparent)`, color: `color-mix(in oklab, ${color} 82%, rgb(var(--ink)))` }}>
      <Icon name={name} size={Math.round(size * 0.52)} />
    </span>
  )
}
