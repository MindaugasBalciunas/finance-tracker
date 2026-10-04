import { useCallback, useState } from 'react'

// Which accounts the history table shows. Remembered per device: explicit
// choices win; with no choice, an account that is empty in the latest
// snapshot (a closed one, like Luminor) starts hidden.
const KEY = 'history-columns'

type Choice = { hide: string[]; show: string[] }

function load(): Choice {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) {
      const c = JSON.parse(raw)
      return { hide: Array.isArray(c.hide) ? c.hide : [], show: Array.isArray(c.show) ? c.show : [] }
    }
  } catch {
    // unavailable or corrupt — fall back to defaults
  }
  return { hide: [], show: [] }
}

function save(c: Choice) {
  try {
    localStorage.setItem(KEY, JSON.stringify(c))
  } catch {
    // not persisted; the choice lasts for this visit
  }
}

export function useHistoryColumns(closedByDefault: (key: string) => boolean) {
  const [choice, setChoice] = useState<Choice>(load)
  const isHidden = useCallback(
    (key: string) => choice.hide.includes(key) || (!choice.show.includes(key) && closedByDefault(key)),
    [choice, closedByDefault],
  )
  const setVisible = useCallback((keys: string[], visible: boolean) => {
    setChoice((c) => {
      const next: Choice = {
        hide: visible ? c.hide.filter((k) => !keys.includes(k)) : [...new Set([...c.hide, ...keys])],
        show: visible ? [...new Set([...c.show, ...keys])] : c.show.filter((k) => !keys.includes(k)),
      }
      save(next)
      return next
    })
  }, [])
  const reset = useCallback(() => {
    const next = { hide: [], show: [] }
    save(next)
    setChoice(next)
  }, [])
  return { isHidden, setVisible, reset }
}
