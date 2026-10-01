import { useEffect, useRef, useState } from 'react'
import { useBankCallback } from '../../hooks/useBanking'

// Grab the query once, at module scope, before React has mounted anything.
//
// The app is a HashRouter, so the bank's `?code=…&state=…` sits *before* the
// `#` and window.location.search can read it at boot. Both halves of this
// happen here and happen immediately: capture, then strip. The code is a
// bearer credential that is already in browser history and in nginx's access
// log — leaving it in the address bar for the duration of a round-trip adds a
// third place for no benefit. Reading at module scope also means a StrictMode
// double-mount finds nothing the second time.
function takeCode(): { code: string; state: string } | null {
  if (typeof window === 'undefined') return null
  const q = new URLSearchParams(window.location.search)
  const code = q.get('code')
  const state = q.get('state')
  if (!code) return null
  q.delete('code')
  q.delete('state')
  const rest = q.toString()
  window.history.replaceState(
    null,
    '',
    window.location.pathname + (rest ? `?${rest}` : '') + window.location.hash
  )
  return { code, state: state ?? '' }
}

const pending = takeCode()

// BankRedirectCatcher finishes a bank authorisation that landed back on the
// app. It renders nothing unless there is something to report.
export default function BankRedirectCatcher() {
  const callback = useBankCallback()
  const fired = useRef(false)
  const [done, setDone] = useState<'ok' | 'error' | null>(null)
  const [hidden, setHidden] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!pending || fired.current) return
    fired.current = true
    callback
      .mutateAsync(pending)
      .then(() => {
        setDone('ok')
        setMessage('Bank connected. Pick which account this is, then press Sync now.')
        window.location.hash = '#/banking'
      })
      .catch((err) => {
        setDone('error')
        setMessage(err instanceof Error ? err.message : 'Could not finish connecting')
      })
  }, [callback])

  if (!pending || hidden) return null
  if (!done) {
    return (
      <div className="fixed top-2 left-0 right-0 z-50 px-4">
        <div className="max-w-2xl mx-auto bg-gray-900 text-white text-sm rounded-xl px-4 py-2.5 shadow-lg">
          Finishing the bank connection…
        </div>
      </div>
    )
  }
  return (
    <div className="fixed top-2 left-0 right-0 z-50 px-4">
      <div
        className={`max-w-2xl mx-auto text-sm rounded-xl px-4 py-2.5 shadow-lg flex items-start justify-between gap-3 ${
          done === 'ok' ? 'bg-emerald-600 text-white' : 'bg-red-600 text-white'
        }`}
      >
        <span className="min-w-0">{message}</span>
        <button onClick={() => setHidden(true)} className="shrink-0 opacity-80 hover:opacity-100" aria-label="Dismiss">
          ✕
        </button>
      </div>
    </div>
  )
}
