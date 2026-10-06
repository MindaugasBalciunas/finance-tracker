import { createContext, InputHTMLAttributes, ReactNode, useCallback, useContext, useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import { Icon, IconTile } from './Icon'
import { eur, parseNum, signed } from '../lib/format'

// The header wraps: the title keeps its line and a long action drops
// underneath on narrow screens instead of squeezing the title.
export function Card({ children, className = '', title, action, pad = true, icon, color = 'rgb(var(--accent))' }: { children: ReactNode; className?: string; title?: ReactNode; action?: ReactNode; pad?: boolean; icon?: string; color?: string }) {
  return (
    <section className={clsx('card', pad && 'p-4', className)}>
      {(title || action) && (
        <div className={clsx('flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5', pad ? 'mb-3' : 'px-4 pt-4 pb-2')}>
          {title && <h2 className="flex min-w-0 shrink-0 items-center gap-2 text-sm font-semibold text-ink">{icon && <IconTile name={icon} color={color} size={26} />}{title}</h2>}
          {action}
        </div>
      )}
      {children}
    </section>
  )
}

/** A KPI tile: label, headline figure, optional context line. */
export function Stat({ label, value, sub, tone, onClick, icon, color = 'rgb(var(--accent))' }: { label: ReactNode; value: ReactNode; sub?: ReactNode; tone?: 'good' | 'bad' | 'warn'; onClick?: () => void; icon?: string; color?: string }) {
  const C = onClick ? 'button' : 'div'
  return (
    <C onClick={onClick} className={clsx('card p-3.5 text-left min-w-0', onClick && 'hover:bg-sunken/50 transition')}>
      <div className="flex items-center gap-2 text-xs text-ink2 min-w-0">{icon && <IconTile name={icon} color={color} size={28} />}<span className="line-clamp-2 leading-tight">{label}</span></div>
      <div className={clsx('mt-1 text-xl font-semibold tracking-tight truncate', tone === 'good' && 'text-good', tone === 'bad' && 'text-bad', tone === 'warn' && 'text-warn')}>{value}</div>
      {sub && <div className="mt-0.5 text-xs text-muted truncate">{sub}</div>}
    </C>
  )
}

/** Change vs a reference. goodWhenUp=false for costs. Always carries an arrow, never colour alone. */
export function Delta({ value, goodWhenUp = true, format = signed, className = '' }: { value: number; goodWhenUp?: boolean; format?: (v: number) => string; className?: string }) {
  if (!value || Math.abs(value) < 0.5) return <span className={clsx('text-muted', className)}>±0</span>
  const up = value > 0
  const good = up === goodWhenUp
  return (
    <span className={clsx('inline-flex items-center gap-0.5 tnum', good ? 'text-good' : 'text-bad', className)}>
      <Icon name={up ? 'up' : 'down'} size={14} />
      {format(value)}
    </span>
  )
}

/** Budget meter: thin track, fill to spent/budget, overflow in the bad tone, pace tick optional. */
export function Meter({ value, max, pace, goal = false, className = '' }: { value: number; max: number; pace?: number; goal?: boolean; className?: string }) {
  const ratio = max > 0 ? value / max : value > 0 ? 1.01 : 0
  const over = ratio > 1
  return (
    <div className={clsx('relative h-1.5 rounded-full bg-sunken overflow-hidden', className)} role="meter" aria-valuenow={Math.round(ratio * 100)} aria-valuemin={0} aria-valuemax={100}>
      <div className={clsx('h-full rounded-full', goal ? (ratio >= 1 ? 'bg-good' : 'bg-accent') : over ? 'bg-bad' : ratio > 0.9 ? 'bg-warn' : 'bg-accent')} style={{ width: `${Math.min(ratio, 1) * 100}%` }} />
      {pace != null && pace > 0 && pace < 1 && <div className="absolute top-0 h-full w-0.5 bg-ink/40" style={{ left: `${pace * 100}%` }} />}
    </div>
  )
}

export function Money({ v, cents = false, className = '' }: { v: number; cents?: boolean; className?: string }) {
  return <span className={clsx('tnum', className)}>{cents ? new Intl.NumberFormat('en-GB', { style: 'currency', currency: 'EUR' }).format(v) : eur(v)}</span>
}

export function Spinner({ className = '' }: { className?: string }) {
  return <div className={clsx('h-5 w-5 animate-spin rounded-full border-2 border-line border-t-accent', className)} />
}

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted">
      <Spinner /> {label}
    </div>
  )
}

export function Empty({ title, children, icon = 'inbox' }: { title: string; children?: ReactNode; icon?: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
      <div className="rounded-full bg-sunken p-3 text-muted"><Icon name={icon} /></div>
      <div className="text-sm font-medium text-ink">{title}</div>
      {children && <div className="max-w-xs text-sm text-muted">{children}</div>}
    </div>
  )
}

export function ErrorBox({ error }: { error: unknown }) {
  if (!error) return null
  return <div className="rounded-xl border border-bad/30 bg-bad/5 px-3 py-2 text-sm text-bad">{(error as Error).message ?? String(error)}</div>
}

export function Segmented<T extends string>({ value, options, onChange, size = 'md' }: { value: T; options: { value: T; label: ReactNode }[]; onChange: (v: T) => void; size?: 'sm' | 'md' }) {
  return (
    <div className="no-scrollbar inline-flex rounded-xl bg-sunken p-0.5 max-w-full overflow-x-auto overflow-y-hidden">
      {options.map((o) => (
        <button key={o.value} onClick={() => onChange(o.value)}
          className={clsx('rounded-[10px] px-3 font-medium whitespace-nowrap transition', size === 'sm' ? 'h-7 text-xs' : 'h-8 text-sm',
            value === o.value ? 'bg-raised text-ink shadow-sm' : 'text-ink2 hover:text-ink')}>
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** Bottom sheet on phones, centred dialog on wider screens. */
export function Sheet({ open, onClose, title, children, footer, wide = false }: { open: boolean; onClose: () => void; title?: ReactNode; children: ReactNode; footer?: ReactNode; wide?: boolean }) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [open, onClose])
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[1px]" onClick={onClose} />
      <div className={clsx('relative flex max-h-[92dvh] w-full flex-col rounded-t-2xl sm:rounded-2xl bg-surface border border-line shadow-xl', wide ? 'sm:max-w-3xl' : 'sm:max-w-lg')}>
        <div className="flex items-center justify-between gap-2 border-b border-line px-4 h-14 shrink-0">
          <div className="text-base font-semibold truncate">{title}</div>
          <button className="btn-ghost h-9 w-9 px-0" onClick={onClose} aria-label="Close"><Icon name="x" /></button>
        </div>
        <div className="flex-1 overflow-y-auto px-4 py-4">{children}</div>
        {footer && <div className="shrink-0 border-t border-line px-4 pt-3 pb-safe-3 flex flex-wrap gap-2 justify-end">{footer}</div>}
      </div>
    </div>
  )
}

export function Field({ label, children, hint }: { label: ReactNode; children: ReactNode; hint?: ReactNode }) {
  return (
    <label className="block">
      <span className="label">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-xs text-muted">{hint}</span>}
    </label>
  )
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: ReactNode }) {
  return (
    <button type="button" onClick={() => onChange(!checked)} className="inline-flex items-center gap-2 text-left text-sm text-ink" role="switch" aria-checked={checked}>
      <span className={clsx('relative h-6 w-10 shrink-0 rounded-full transition', checked ? 'bg-accent' : 'bg-axis')}>
        <span className={clsx('absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition', checked ? 'left-[18px]' : 'left-0.5')} />
      </span>
      {label}
    </button>
  )
}

// ── toasts ──────────────────────────────────────────────────────────

type Toast = { id: number; text: string; tone?: 'good' | 'bad' }
const ToastCtx = createContext<(text: string, tone?: 'good' | 'bad') => void>(() => {})
export const useToast = () => useContext(ToastCtx)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const n = useRef(0)
  const push = useCallback((text: string, tone?: 'good' | 'bad') => {
    const id = ++n.current
    setToasts((t) => [...t, { id, text, tone }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000)
  }, [])
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-20 sm:bottom-6 z-[60] flex flex-col items-center gap-2 px-4">
        {toasts.map((t) => (
          <div key={t.id} className={clsx('pointer-events-auto max-w-md rounded-xl px-4 py-2.5 text-sm shadow-lg', t.tone === 'bad' ? 'bg-bad text-white' : 'bg-ink text-page')}>{t.text}</div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

export function PageHeader({ title, sub, actions }: { title: ReactNode; sub?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-4 flex items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight truncate">{title}</h1>
        {sub && <div className="text-sm text-muted truncate">{sub}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Tabs<T extends string>({ value, onChange, tabs }: { value: T; onChange: (v: T) => void; tabs: { value: T; label: ReactNode; badge?: number }[] }) {
  return (
    <div className="no-scrollbar mb-4 flex gap-1 overflow-x-auto overflow-y-hidden border-b border-line -mx-4 px-4 sm:mx-0 sm:px-0">
      {tabs.map((t) => (
        <button key={t.value} onClick={() => onChange(t.value)}
          className={clsx('relative h-10 px-3 text-sm font-medium whitespace-nowrap transition', value === t.value ? 'text-ink' : 'text-muted hover:text-ink2')}>
          {t.label}
          {!!t.badge && <span className="ml-1.5 rounded-full bg-accent px-1.5 py-0.5 text-[10px] font-semibold text-white">{t.badge}</span>}
          {value === t.value && <span className="absolute inset-x-2 bottom-0 h-0.5 rounded-full bg-accent" />}
        </button>
      ))}
    </div>
  )
}

/** Opens the CFO chat with a question about what is on screen. */
export function AskCFO({ q, label = 'Ask CFO' }: { q: string; label?: string }) {
  return (
    <a href={`#/ai?q=${encodeURIComponent(q)}`} className="btn-ghost h-8 px-2.5 text-xs text-accent">
      <Icon name="spark" size={15} />{label}
    </a>
  )
}

/** A number field that keeps exactly what is typed ("0,", "12.") and reports
 *  a number only once it parses — no NaN, commas accepted. Invalid text gets
 *  a red outline. */
export function NumberInput({ value, onChange, integer = false, className = 'input tnum', placeholder, ...rest }: {
  value: number | null | undefined; onChange: (v: number | undefined) => void; integer?: boolean; className?: string; placeholder?: string
} & Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  const show = (v: number | null | undefined) => (v == null || !Number.isFinite(v) ? '' : String(v))
  const [text, setText] = useState(show(value))
  const last = useRef(value)
  useEffect(() => {
    // Follow outside changes (e.g. AI fill), not our own echo.
    if (value !== last.current) { last.current = value; setText(show(value)) }
  }, [value])
  const n = parseNum(text)
  const invalid = text.trim() !== '' && (n === undefined || (integer && !Number.isInteger(n)))
  return (
    <input {...rest} className={clsx(className, invalid && 'border-bad focus:border-bad')} inputMode={integer ? 'numeric' : 'decimal'} placeholder={placeholder}
      aria-invalid={invalid || undefined} value={text}
      onChange={(e) => {
        setText(e.target.value)
        const v = parseNum(e.target.value)
        const ok = v !== undefined && (!integer || Number.isInteger(v))
        last.current = ok ? v : undefined
        onChange(ok ? v : undefined)
      }} />
  )
}
