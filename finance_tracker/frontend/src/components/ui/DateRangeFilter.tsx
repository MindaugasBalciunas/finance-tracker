import { useState, useRef, useEffect } from 'react'
import clsx from 'clsx'
import DateInput from './DateInput'

export interface DateRange {
  date_from?: string
  date_to?: string
}

type Preset = 'all' | 'this-month' | '3m' | '6m' | '1y' | 'custom'

interface Props {
  value: DateRange
  onChange: (range: DateRange) => void
  className?: string
}

const PRESETS: { key: Preset; label: string }[] = [
  { key: 'all',        label: 'All time' },
  { key: 'this-month', label: 'This month' },
  { key: '3m',         label: '3 months' },
  { key: '6m',         label: '6 months' },
  { key: '1y',         label: '1 year' },
  { key: 'custom',     label: 'Custom' },
]

function startOfMonth(monthsAgo: number): string {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - monthsAgo)
  return d.toISOString().slice(0, 10)
}

export function presetRange(preset: Preset): DateRange {
  switch (preset) {
    case 'this-month': return { date_from: startOfMonth(0) }
    case '3m':         return { date_from: startOfMonth(3) }
    case '6m':         return { date_from: startOfMonth(6) }
    case '1y':         return { date_from: startOfMonth(12) }
    default:           return {}
  }
}

// Preset every page starts on; DateRangeContext must seed its range from this.
export const DEFAULT_PRESET: Preset = '6m'

export default function DateRangeFilter({ onChange, className }: Props) {
  const [preset, setPreset] = useState<Preset>(DEFAULT_PRESET)
  const [custom, setCustom] = useState<DateRange>({})
  const [popoverOpen, setPopoverOpen] = useState(false)
  const popoverRef = useRef<HTMLDivElement>(null)

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

  function selectPreset(p: Preset) {
    setPreset(p)
    if (p === 'custom') {
      setPopoverOpen(true)
    } else {
      setPopoverOpen(false)
      onChange(presetRange(p))
    }
  }

  function updateCustom(field: 'date_from' | 'date_to', val: string) {
    const next = { ...custom, [field]: val || undefined }
    setCustom(next)
    onChange(next)
  }

  const customLabel = custom.date_from
    ? `${custom.date_from}${custom.date_to ? ` → ${custom.date_to}` : ''}`
    : 'Custom'

  return (
    <div className={clsx('flex items-center gap-1 bg-gray-100 rounded-lg p-1', className)}>
      {PRESETS.map((p) => {
        if (p.key === 'custom') {
          return (
            <div key="custom" className="relative" ref={popoverRef}>
              <button
                onClick={() => selectPreset('custom')}
                className={clsx(
                  'px-3 py-1.5 text-sm font-medium rounded-md transition-colors whitespace-nowrap',
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
                    value={custom.date_from ?? ''}
                    onChange={(val) => updateCustom('date_from', val)}
                    className="border border-gray-300 rounded-lg px-2 py-1 text-sm"
                  />
                  <span className="text-gray-400 text-sm">to</span>
                  <DateInput
                    value={custom.date_to ?? ''}
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
            onClick={() => selectPreset(p.key)}
            className={clsx(
              'px-3 py-1.5 text-sm font-medium rounded-md transition-colors whitespace-nowrap',
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
