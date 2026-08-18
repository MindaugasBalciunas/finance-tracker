export interface DateRange {
  date_from?: string
  date_to?: string
}

export type Preset = 'all' | 'this-month' | 'last-month' | '3m' | '6m' | '1y' | 'ytd' | 'custom'

// Format from local date parts — toISOString() converts to UTC first, which
// shifts the date by a day around midnight in non-UTC zones (e.g. UTC+2/+3).
function toLocalIso(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function startOfMonth(monthsAgo: number): string {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - monthsAgo)
  return toLocalIso(d)
}

function endOfPreviousMonth(): string {
  const d = new Date()
  d.setDate(0)
  return toLocalIso(d)
}

export function presetRange(preset: Preset): DateRange {
  switch (preset) {
    case 'this-month': return { date_from: startOfMonth(0) }
    case 'last-month': return { date_from: startOfMonth(1), date_to: endOfPreviousMonth() }
    case '3m':         return { date_from: startOfMonth(3) }
    case '6m':         return { date_from: startOfMonth(6) }
    case '1y':         return { date_from: startOfMonth(12) }
    case 'ytd':        return { date_from: `${new Date().getFullYear()}-01-01` }
    default:           return {}
  }
}

// Preset every page starts on when nothing is persisted yet.
export const DEFAULT_PRESET: Preset = '6m'
