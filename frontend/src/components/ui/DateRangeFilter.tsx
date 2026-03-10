import { useState } from 'react'
import clsx from 'clsx'

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

function presetRange(preset: Preset): DateRange {
  switch (preset) {
    case 'this-month': return { date_from: startOfMonth(0) }
    case '3m':         return { date_from: startOfMonth(3) }
    case '6m':         return { date_from: startOfMonth(6) }
    case '1y':         return { date_from: startOfMonth(12) }
    default:           return {}
  }
}

export default function DateRangeFilter({ value, onChange, className }: Props) {
  const [preset, setPreset] = useState<Preset>('all')
  const [custom, setCustom] = useState<DateRange>({})

  function selectPreset(p: Preset) {
    setPreset(p)
    if (p !== 'custom') {
      onChange(presetRange(p))
    } else {
      onChange(custom)
    }
  }

  function updateCustom(field: 'date_from' | 'date_to', val: string) {
    const next = { ...custom, [field]: val || undefined }
    setCustom(next)
    onChange(next)
  }

  return (
    <div className={clsx('flex flex-wrap items-center gap-2', className)}>
      <div className="flex items-center gap-1 bg-gray-100 rounded-lg p-1">
        {PRESETS.map((p) => (
          <button
            key={p.key}
            onClick={() => selectPreset(p.key)}
            className={clsx(
              'px-3 py-1.5 text-sm font-medium rounded-md transition-colors',
              preset === p.key
                ? 'bg-white text-gray-900 shadow-sm'
                : 'text-gray-500 hover:text-gray-700'
            )}
          >
            {p.label}
          </button>
        ))}
      </div>

      {preset === 'custom' && (
        <div className="flex items-center gap-2">
          <input
            type="date"
            value={custom.date_from ?? ''}
            onChange={(e) => updateCustom('date_from', e.target.value)}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
          />
          <span className="text-gray-400 text-sm">to</span>
          <input
            type="date"
            value={custom.date_to ?? ''}
            onChange={(e) => updateCustom('date_to', e.target.value)}
            className="border border-gray-300 rounded-lg px-3 py-1.5 text-sm"
          />
        </div>
      )}

      {value.date_from && preset !== 'all' && (
        <span className="text-xs text-gray-400">
          From {value.date_from}{value.date_to ? ` to ${value.date_to}` : ''}
        </span>
      )}
    </div>
  )
}
