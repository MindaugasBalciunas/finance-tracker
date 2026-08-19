import { useState, useRef, useEffect } from 'react'
import { createPortal } from 'react-dom'
import clsx from 'clsx'
import DateInput from './DateInput'
import { useDateRange } from '../../context/DateRangeContext'
import type { Preset } from '../../utils/dateRange'

// Re-exported so existing imports from this file keep working.
export type { DateRange, Preset } from '../../utils/dateRange'
export { presetRange, DEFAULT_PRESET } from '../../utils/dateRange'

interface Props {
  className?: string
  // compact renders a single dropdown button (for the crowded desktop
  // header) instead of the full inline pill bar; the presets live in the
  // popover. The inline bar is still used inside the mobile menu.
  compact?: boolean
}

const PRESETS: { key: Preset; label: string }[] = [
  { key: 'all',        label: 'All' },
  { key: 'this-month', label: 'This mo' },
  { key: 'last-month', label: 'Last mo' },
  { key: '3m',         label: '3M' },
  { key: '6m',         label: '6M' },
  { key: '1y',         label: '1Y' },
  { key: 'ytd',        label: 'YTD' },
  { key: 'custom',     label: 'Custom' },
]

export default function DateRangeFilter({ className, compact = false }: Props) {
  const { preset, customRange, selectPreset, setCustomRange } = useDateRange()
  const [popoverOpen, setPopoverOpen] = useState(false)
  // The popover is portaled to <body> so it escapes the filter bar's
  // horizontal-scroll clip; position is anchored under the trigger and
  // clamped so it stays fully on-screen on any viewport.
  const PANEL_W = 320
  const [pos, setPos] = useState<{ top: number; left: number }>({ top: 0, left: 0 })
  const panelRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const activeRef = useRef<HTMLButtonElement>(null)

  // Keep the selected preset visible when the bar overflows on narrow headers.
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [preset])

  function anchorPopover() {
    const r = buttonRef.current?.getBoundingClientRect()
    if (!r) return
    const w = Math.min(PANEL_W, window.innerWidth - 16)
    // Right-align to the trigger, then clamp both edges into the viewport.
    const left = Math.min(Math.max(8, r.right - w), window.innerWidth - w - 8)
    setPos({ top: r.bottom + 8, left })
  }

  // Close popover on outside click (trigger + portaled panel both count as
  // "inside"), and reposition/close on viewport changes.
  useEffect(() => {
    if (!popoverOpen) return
    function handle(e: MouseEvent) {
      const t = e.target as Node
      if (!panelRef.current?.contains(t) && !buttonRef.current?.contains(t)) {
        setPopoverOpen(false)
      }
    }
    function reflow() { anchorPopover() }
    document.addEventListener('mousedown', handle)
    window.addEventListener('resize', reflow)
    window.addEventListener('scroll', reflow, true)
    return () => {
      document.removeEventListener('mousedown', handle)
      window.removeEventListener('resize', reflow)
      window.removeEventListener('scroll', reflow, true)
    }
  }, [popoverOpen])

  function updateCustom(field: 'date_from' | 'date_to', val: string) {
    setCustomRange({ ...customRange, [field]: val || undefined })
  }

  const customLabel = customRange.date_from
    ? `${customRange.date_from}${customRange.date_to ? ` → ${customRange.date_to}` : ''}`
    : 'Custom'

  // The friendly label for the current selection, shown on the compact trigger.
  const currentLabel = preset === 'custom'
    ? customLabel
    : PRESETS.find((p) => p.key === preset)?.label ?? 'All'

  const customInputs = (
    <div className="flex flex-wrap items-center gap-2">
      <DateInput
        value={customRange.date_from ?? ''}
        onChange={(val) => updateCustom('date_from', val)}
        className="border border-gray-300 rounded-lg px-2 py-1 text-sm w-32"
      />
      <span className="text-gray-400 text-sm">to</span>
      <DateInput
        value={customRange.date_to ?? ''}
        onChange={(val) => updateCustom('date_to', val)}
        className="border border-gray-300 rounded-lg px-2 py-1 text-sm w-32"
      />
      <button
        onClick={() => setPopoverOpen(false)}
        className="ml-auto text-xs text-blue-600 hover:text-blue-800 font-medium px-2 py-1"
      >
        Apply
      </button>
    </div>
  )

  // ── Compact: one dropdown trigger, presets + custom inputs in the popover ──
  if (compact) {
    function pick(p: Preset) {
      selectPreset(p)
      if (p !== 'custom') setPopoverOpen(false) // custom keeps the panel open to edit dates
    }
    return (
      <>
        <button
          ref={buttonRef}
          onClick={() => { anchorPopover(); setPopoverOpen((o) => !o) }}
          className={clsx(
            'flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-lg transition-colors whitespace-nowrap max-w-[200px]',
            popoverOpen ? 'bg-gray-200 text-gray-900' : 'bg-gray-100 text-gray-700 hover:bg-gray-200',
            className,
          )}
          aria-haspopup="true"
          aria-expanded={popoverOpen}
        >
          <span aria-hidden>📅</span>
          <span className="truncate">{currentLabel}</span>
          <svg className="w-3 h-3 text-gray-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {popoverOpen && createPortal(
          <div
            ref={panelRef}
            style={{ position: 'fixed', top: pos.top, left: pos.left, width: `min(${PANEL_W}px, calc(100vw - 16px))`, zIndex: 60 }}
            className="bg-white border border-gray-200 rounded-xl shadow-lg p-2"
          >
            <div className="flex flex-wrap gap-1">
              {PRESETS.map((p) => (
                <button
                  key={p.key}
                  onClick={() => pick(p.key)}
                  className={clsx(
                    'px-2.5 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap',
                    preset === p.key ? 'bg-blue-50 text-blue-700' : 'text-gray-600 hover:bg-gray-100',
                  )}
                >
                  {p.key === 'custom' && preset === 'custom' ? customLabel : p.label}
                </button>
              ))}
            </div>
            {preset === 'custom' && (
              <div className="mt-2 pt-2 border-t border-gray-100">{customInputs}</div>
            )}
          </div>,
          document.body,
        )}
      </>
    )
  }

  // ── Inline pill bar (mobile menu): scrolls horizontally if it overflows ──
  function handleSelect(p: Preset) {
    selectPreset(p)
    if (p === 'custom') {
      anchorPopover()
      setPopoverOpen((open) => (preset === 'custom' ? !open : true))
    } else {
      setPopoverOpen(false)
    }
  }

  return (
    <div className={clsx('flex items-center gap-1 bg-gray-100 rounded-lg p-1 max-w-full overflow-x-auto', className)}>
      {PRESETS.map((p) => (
        <button
          key={p.key}
          ref={p.key === 'custom' ? buttonRef : preset === p.key ? activeRef : undefined}
          onClick={() => handleSelect(p.key)}
          className={clsx(
            'px-2 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap',
            preset === p.key ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-700',
          )}
        >
          {p.key === 'custom' ? (preset === 'custom' ? customLabel : 'Custom') : p.label}
        </button>
      ))}

      {popoverOpen && createPortal(
        <div
          ref={panelRef}
          style={{ position: 'fixed', top: pos.top, left: pos.left, width: `min(${PANEL_W}px, calc(100vw - 16px))`, zIndex: 60 }}
          className="bg-white border border-gray-200 rounded-xl shadow-lg p-3"
        >
          {customInputs}
        </div>,
        document.body,
      )}
    </div>
  )
}
