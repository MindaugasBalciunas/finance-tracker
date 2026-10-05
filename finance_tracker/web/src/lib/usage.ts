// Private usage analytics: which pages you open, how long you stay, what you
// click and where — stored only in this app's own database, for improving the
// layout later (analysed with the Claude CLI). Off switch: Settings → Usage.

type Ev = { at: string; kind: 'view' | 'action'; path: string; from?: string; label?: string; dwell_ms?: number; x?: number; y?: number; vw?: number }

let queue: Ev[] = []
let enabled = true
let current: { path: string; from: string; since: number; at: string } | null = null

export function setUsageEnabled(on: boolean) {
  enabled = on
  if (!on) queue = []
}

function flush(useBeacon = false) {
  if (!enabled || !queue.length) return
  const body = JSON.stringify({ events: queue.splice(0, 200) })
  try {
    if (useBeacon && navigator.sendBeacon) navigator.sendBeacon('api/usage', new Blob([body], { type: 'application/json' }))
    else fetch('api/usage', { method: 'POST', body, headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin', keepalive: true }).catch(() => {})
  } catch { /* analytics never break the app */ }
}

/** Close the current page view (with its dwell time) and open the next. */
export function trackView(path: string) {
  const now = Date.now()
  if (current && current.path !== path) {
    if (enabled) queue.push({ at: current.at, kind: 'view', path: current.path, from: current.from, dwell_ms: now - current.since })
  }
  if (!current || current.path !== path) current = { path, from: current?.path ?? '', since: now, at: new Date(now).toISOString() }
  if (queue.length >= 20) flush()
}

export function clickLabel(el: Element): string {
  // Drill-down links carry merchant, tag or category names in their text
  // (and query): record where they lead, never what they say.
  const href = el.getAttribute('href')
  if (href && href.includes('?')) return `link → ${href.replace(/^#/, '').split('?')[0]}`
  const a = el.getAttribute('aria-label') || el.getAttribute('title') || (el as HTMLElement).innerText || ''
  return a.replace(/\s+/g, ' ').trim().slice(0, 48)
}

let started = false
export function startUsage() {
  if (started) return
  started = true
  document.addEventListener('click', (e) => {
    if (!enabled || !current) return
    const target = e.target as Element
    if (target?.closest('[data-no-track]')) return // e.g. the PIN pad
    const el = target?.closest('button, a, [role=tab], [role=switch], select, summary, input[type=checkbox]')
    if (!el) return
    const doc = document.documentElement
    queue.push({
      at: new Date().toISOString(), kind: 'action', path: current.path, label: clickLabel(el),
      x: Math.min(1, Math.max(0, e.pageX / Math.max(1, doc.scrollWidth))),
      y: Math.min(1, Math.max(0, e.pageY / Math.max(1, doc.scrollHeight))),
      vw: window.innerWidth,
    })
  }, { capture: true, passive: true })
  // Count time on the page up to leaving or hiding the app, then flush.
  const close = () => {
    if (current && enabled) {
      const now = Date.now()
      queue.push({ at: current.at, kind: 'view', path: current.path, from: current.from, dwell_ms: now - current.since })
      current = { path: current.path, from: current.path, since: now, at: new Date(now).toISOString() }
    }
    flush(true)
  }
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'hidden') close() })
  window.addEventListener('pagehide', close)
  setInterval(() => flush(), 15_000)
}
