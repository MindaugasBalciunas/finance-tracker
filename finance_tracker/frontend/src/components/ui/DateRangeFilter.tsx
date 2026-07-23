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

export default function DateRangeFilter({ className }: Props) {
  const { preset, customRange, selectPreset, setCustomRange } = useDateRange()
  const [popoverOpen, setPopoverOpen] = useState(false)
  // The popover is portaled to <body> so it escapes the filter bar's
  // horizontal-scroll clip; position is anchored under the Custom button and
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
    // Right-align to the button, then clamp both edges into the viewport.
    const left = Math.min(Math.max(8, r.right - w), window.innerWidth - w - 8)
    setPos({ top: r.bottom + 8, left })
  }

  // Close popover on outside click (button + portaled panel both count as
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

  function handleSelect(p: Preset) {
    selectPreset(p)
    if (p === 'custom') {
      anchorPopover()
      setPopoverOpen((open) => (preset === 'custom' ? !open : true))
    } else {
      setPopoverOpen(false)
    }
  }

  function updateCustom(field: 'date_from' | 'date_to', val: string) {
    setCustomRange({ ...customRange, [field]: val || undefined })
  }

  const customLabel = customRange.date_from
    ? `${customRange.date_from}${customRange.date_to ? ` → ${customRange.date_to}` : ''}`
    : 'Custom'

  return (
    <div className={clsx('flex items-center gap-1 bg-gray-100 rounded-lg p-1 max-w-full overflow-x-auto', className)}>
      {PRESETS.map((p) => {
        if (p.key === 'custom') {
          return (
            <button
              key="custom"
              ref={buttonRef}
              onClick={() => handleSelect('custom')}
              className={clsx(
                'px-2 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap',
                preset === 'custom'
                  ? 'bg-white text-gray-900 shadow-sm'
                  : 'text-gray-500 hover:text-gray-700'
              )}
            >
              {preset === 'custom' ? customLabel : 'Custom'}
            </button>
          )
        }
        return (
          <button
            key={p.key}
            ref={preset === p.key ? activeRef : undefined}
            onClick={() => handleSelect(p.key)}
            className={clsx(
              'px-2 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap',
              preset === p.key
                ? 'bg-white text-gray-900 shadow-sm'
                : 'text-gray-500 hover:text-gray-700'
            )}
          >
            {p.label}
          </button>
        )
      })}

      {popoverOpen && createPortal(
        <div
          ref={panelRef}
          style={{ position: 'fixed', top: pos.top, left: pos.left, width: `min(${PANEL_W}px, calc(100vw - 16px))`, zIndex: 60 }}
          className="bg-white border border-gray-200 rounded-xl shadow-lg p-3 flex flex-wrap items-center gap-2"
        >
          <DateInput
            value={customRange.date_from ?? ''}
            onChange={(val) => updateCustom('date_from', val)}
            className="border border-gray-300 rounded-lg px-2 py-1 text-sm w-36"
          />
          <span className="text-gray-400 text-sm">to</span>
          <DateInput
            value={customRange.date_to ?? ''}
            onChange={(val) => updateCustom('date_to', val)}
            className="border border-gray-300 rounded-lg px-2 py-1 text-sm w-36"
          />
          <button
            onClick={() => setPopoverOpen(false)}
            className="ml-1 text-xs text-blue-600 hover:text-blue-800 font-medium px-2 py-1"
          >
            Apply
          </button>
        </div>,
        document.body
      )}
    </div>
  )
}
