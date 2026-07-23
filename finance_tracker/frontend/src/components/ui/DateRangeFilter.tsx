import { useState, useRef, useEffect } from 'react'
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
  const popoverRef = useRef<HTMLDivElement>(null)
  const activeRef = useRef<HTMLButtonElement>(null)

  // Keep the selected preset visible when the bar overflows on narrow headers.
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [preset])

  // Close popover on outside click
  useEffect(() => {
    function handle(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setPopoverOpen(false)
      }
    }
    if (popoverOpen) document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [popoverOpen])

  function handleSelect(p: Preset) {
    selectPreset(p)
    setPopoverOpen(p === 'custom')
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
            <div key="custom" className="relative" ref={popoverRef}>
              <button
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

              {popoverOpen && (
                <div className="absolute top-full left-1/2 -translate-x-1/2 mt-2 bg-white border border-gray-200 rounded-xl shadow-lg p-3 z-50 flex items-center gap-2 whitespace-nowrap">
                  <DateInput
                    value={customRange.date_from ?? ''}
                    onChange={(val) => updateCustom('date_from', val)}
                    className="border border-gray-300 rounded-lg px-2 py-1 text-sm"
                  />
                  <span className="text-gray-400 text-sm">to</span>
                  <DateInput
                    value={customRange.date_to ?? ''}
                    onChange={(val) => updateCustom('date_to', val)}
                    className="border border-gray-300 rounded-lg px-2 py-1 text-sm"
                  />
                  <button
                    onClick={() => setPopoverOpen(false)}
                    className="ml-1 text-xs text-blue-600 hover:text-blue-800 font-medium"
                  >
                    Apply
                  </button>
                </div>
              )}
            </div>
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
    </div>
  )
}
