import { createContext, useContext, useState } from 'react'
import type { DateRange, Preset } from '../utils/dateRange'
import { presetRange, DEFAULT_PRESET } from '../utils/dateRange'

const STORAGE_KEY = 'finance-tracker.date-range'

interface StoredRange {
  preset: Preset
  custom?: DateRange
}

interface DateRangeState {
  preset: Preset
  range: DateRange
  custom: DateRange
}

interface DateRangeContextValue {
  dateRange: DateRange
  preset: Preset
  customRange: DateRange
  selectPreset: (p: Preset) => void
  setCustomRange: (r: DateRange) => void
}

const DateRangeContext = createContext<DateRangeContextValue>({
  dateRange: {},
  preset: DEFAULT_PRESET,
  customRange: {},
  selectPreset: () => {},
  setCustomRange: () => {},
})

// Relative presets ('6m', 'ytd', …) are stored by name and recomputed against
// today at load time, so a saved "6 months" never goes stale.
function loadInitial(): DateRangeState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const stored = JSON.parse(raw) as StoredRange
      if (stored.preset === 'custom') {
        const custom = stored.custom ?? {}
        return { preset: 'custom', range: custom, custom }
      }
      if (stored.preset) {
        return { preset: stored.preset, range: presetRange(stored.preset), custom: {} }
      }
    }
  } catch {
    // Corrupted or unavailable storage — fall through to the default.
  }
  return { preset: DEFAULT_PRESET, range: presetRange(DEFAULT_PRESET), custom: {} }
}

function persist(stored: StoredRange) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(stored))
  } catch {
    // Storage full or blocked — the selection still works for this session.
  }
}

export function DateRangeProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<DateRangeState>(loadInitial)

  function selectPreset(p: Preset) {
    if (p === 'custom') {
      setState((s) => {
        persist({ preset: 'custom', custom: s.custom })
        // Keep the current range until custom dates are actually entered.
        const range = s.custom.date_from || s.custom.date_to ? s.custom : s.range
        return { ...s, preset: 'custom', range }
      })
    } else {
      persist({ preset: p })
      setState((s) => ({ ...s, preset: p, range: presetRange(p) }))
    }
  }

  function setCustomRange(r: DateRange) {
    persist({ preset: 'custom', custom: r })
    setState((s) => ({ ...s, preset: 'custom', custom: r, range: r }))
  }

  return (
    <DateRangeContext.Provider
      value={{
        dateRange: state.range,
        preset: state.preset,
        customRange: state.custom,
        selectPreset,
        setCustomRange,
      }}
    >
      {children}
    </DateRangeContext.Provider>
  )
}

export function useDateRange() {
  return useContext(DateRangeContext)
}
