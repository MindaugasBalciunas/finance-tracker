import { useEffect, useMemo, useRef, useState } from 'react'
import clsx from 'clsx'
import { todayISO } from '../lib/format'
import { Icon } from './Icon'

const DAYS = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su']
const iso = (y: number, m: number, d: number) => `${y}-${String(m + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`

/** A date field whose calendar starts on Monday. The browser's own date
 *  picker takes the first weekday from the browser's locale (Sunday in US
 *  English) and a page can't change it — so the calendar is drawn here.
 *  value and onChange use YYYY-MM-DD; "" means no date. */
export function DateInput({ value, onChange, min, max, placeholder = 'Pick a date', className = 'input', clearable = false, ...rest }: {
  value: string; onChange: (v: string) => void; min?: string; max?: string; placeholder?: string; className?: string; clearable?: boolean
  'aria-label'?: string; id?: string
}) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  // Placed against the screen, not the field: kept inside its edges and
  // opened upwards when there is no room below (inside sheets, which clip).
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)
  const place = () => {
    const r = box.current?.getBoundingClientRect()
    if (!r) return
    const w = Math.min(288, window.innerWidth - 16), h = 360
    const left = Math.max(8, Math.min(r.left, window.innerWidth - w - 8))
    const top = r.bottom + 4 + h > window.innerHeight && r.top - h - 4 > 8 ? r.top - h - 4 : r.bottom + 4
    setPos({ top, left })
  }
  const start = value || todayISO()
  const [view, setView] = useState({ y: Number(start.slice(0, 4)), m: Number(start.slice(5, 7)) - 1 })
  useEffect(() => {
    if (!open) return
    const v = value || todayISO()
    setView({ y: Number(v.slice(0, 4)), m: Number(v.slice(5, 7)) - 1 })
    const away = (e: MouseEvent | TouchEvent) => { if (!box.current?.contains(e.target as Node) && !panel.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.stopPropagation(); setOpen(false) } }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    document.addEventListener('mousedown', away)
    document.addEventListener('touchstart', away)
    document.addEventListener('keydown', esc, true)
    return () => { window.removeEventListener('resize', place); window.removeEventListener('scroll', place, true); document.removeEventListener('mousedown', away); document.removeEventListener('touchstart', away); document.removeEventListener('keydown', esc, true) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
  // Monday-first grid: blanks before the 1st, then every day of the month.
  const cells = useMemo(() => {
    const first = new Date(view.y, view.m, 1).getDay() // 0 = Sunday
    const lead = (first + 6) % 7
    const n = new Date(view.y, view.m + 1, 0).getDate()
    return [...Array(lead).fill(null), ...Array.from({ length: n }, (_, i) => i + 1)] as (number | null)[]
  }, [view])
  const step = (k: number) => setView((v) => { const d = new Date(v.y, v.m + k, 1); return { y: d.getFullYear(), m: d.getMonth() } })
  const pick = (d: string) => { onChange(d); setOpen(false) }
  const today = todayISO()
  const label = value ? new Date(value + 'T00:00:00').toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' }) : ''
  const out = (d: string) => (!!min && d < min) || (!!max && d > max)
  return (
    <div ref={box} className="relative min-w-0">
      <button type="button" className={clsx(className, 'flex w-full items-center gap-2 text-left')} onClick={() => setOpen(!open)} aria-haspopup="dialog" aria-expanded={open} {...rest}>
        <Icon name="calendar" size={15} className="shrink-0 text-muted" />
        <span className={clsx('min-w-0 flex-1 truncate', !value && 'text-muted')}>{label || placeholder}</span>
      </button>
      {open && (
        <div ref={panel} role="dialog" aria-label="Choose a date" className="fixed z-[80] w-72 max-w-[calc(100vw-1rem)] rounded-2xl border border-line bg-raised p-3 shadow-xl"
          style={pos ? { top: pos.top, left: pos.left } : { visibility: 'hidden' }}>
          <div className="mb-2 flex items-center justify-between">
            <button type="button" className="btn-ghost h-8 w-8 px-0" onClick={() => step(-1)} aria-label="Previous month"><Icon name="chevronL" size={16} /></button>
            <span className="text-sm font-semibold">{new Date(view.y, view.m, 1).toLocaleDateString('en-GB', { month: 'long', year: 'numeric' })}</span>
            <button type="button" className="btn-ghost h-8 w-8 px-0" onClick={() => step(1)} aria-label="Next month"><Icon name="chevronR" size={16} /></button>
          </div>
          <div className="grid grid-cols-7 gap-0.5 text-center text-[11px] text-muted">
            {DAYS.map((d, i) => <span key={d} className={clsx('py-1', i >= 5 && 'text-ink2')}>{d}</span>)}
          </div>
          <div className="grid grid-cols-7 gap-0.5">
            {cells.map((d, i) => {
              if (d == null) return <span key={`b${i}`} />
              const s = iso(view.y, view.m, d)
              const sel = s === value
              return (
                <button key={s} type="button" disabled={out(s)} onClick={() => pick(s)} aria-pressed={sel} aria-label={new Date(s + 'T00:00:00').toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}
                  className={clsx('h-9 rounded-lg text-sm tnum transition disabled:opacity-30',
                    sel ? 'bg-accent font-semibold text-white' : 'hover:bg-sunken',
                    !sel && s === today && 'ring-1 ring-accent text-accent',
                    !sel && i % 7 >= 5 && 'text-ink2')}>
                  {d}
                </button>
              )
            })}
          </div>
          <div className="mt-2 flex items-center justify-between border-t border-line pt-2 text-xs">
            <button type="button" className="text-accent disabled:opacity-40" disabled={out(today)} onClick={() => pick(today)}>Today</button>
            {clearable && value && <button type="button" className="text-muted" onClick={() => pick('')}>Clear</button>}
          </div>
        </div>
      )}
    </div>
  )
}
