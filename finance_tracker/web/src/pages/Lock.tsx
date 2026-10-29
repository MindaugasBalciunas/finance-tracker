import { LogoMark } from '../components/Logo'
import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { Icon } from '../components/Icon'
import { cancelPasskeyLogin, passkeyCancelled, passkeyLogin, passkeySupported } from '../lib/passkey'

export default function Lock({ onUnlock }: { onUnlock: () => void }) {
  const [pin, setPin] = useState('')
  const [err, setErr] = useState('')
  // A dismissed or timed-out fingerprint isn't an error: it's a nudge to tap again.
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  // The phone's own fingerprint sheet is open — the button says so, because on
  // some phones the sheet takes a moment to appear.
  const [reading, setReading] = useState(false)
  const [passkeys, setPasskeys] = useState(0)
  // With a passkey the fingerprint comes first (no web password needed);
  // the PIN pad is one tap away.
  const [showPin, setShowPin] = useState(false)
  const inFlight = useRef(false)
  const offered = useRef(false)
  const canBio = useRef(false)
  const cancels = useRef(0)

  const bio = async () => {
    // One reading at a time: a tap during the sheet used to cancel both.
    if (inFlight.current) return
    inFlight.current = true
    offered.current = true
    setErr('')
    setNote('')
    setReading(true)
    try {
      await passkeyLogin()
      onUnlock()
    } catch (e) {
      // WebAuthn reports a refusal the same way whether the sheet was
      // dismissed or the passkey can't be used here, so after a couple of
      // goes point at the PIN rather than let them tap forever.
      if (passkeyCancelled(e)) setNote(++cancels.current > 1 ? 'Still no fingerprint — try again, or use your PIN' : 'Fingerprint cancelled — tap to try again')
      else setErr((e as Error).message || 'Passkey failed')
    } finally {
      inFlight.current = false
      setReading(false)
    }
  }

  useEffect(() => {
    let alive = true
    let timer = 0
    // Offer the fingerprint as the app opens — once, and only with the app on
    // screen: a request from a hidden page is refused, and a sheet over a page
    // that hasn't painted yet gets swiped away before the finger lands.
    const offer = () => {
      if (!alive || !canBio.current || offered.current || inFlight.current) return
      if (document.visibilityState !== 'visible') return
      offered.current = true
      timer = window.setTimeout(() => { if (alive) bio() }, 150)
    }
    document.addEventListener('visibilitychange', offer)
    api.get<{ passkeys: number }>('/auth/status').then((s) => {
      if (!alive) return
      setPasskeys(s.passkeys)
      canBio.current = s.passkeys > 0 && passkeySupported()
      offer()
    }).catch(() => {})
    return () => {
      alive = false
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', offer)
      cancelPasskeyLogin()
    }
  }, [])

  const submit = async (p: string) => {
    setBusy(true)
    setErr('')
    setNote('')
    try {
      await api.post('/auth/pin/login', { pin: p })
      onUnlock()
    } catch (e) {
      // A 401 that isn't the app's own answer came from the web password
      // prompt, which PIN unlock still needs.
      setErr(e instanceof ApiError && e.status === 401 && !e.data ? 'PIN unlock needs your web password' + (passkeys ? ' — or use your fingerprint' : '') : (e as Error).message)
      setPin('')
    } finally {
      setBusy(false)
    }
  }
  const press = (d: string) => {
    if (busy) return
    const next = (pin + d).slice(0, 8)
    setPin(next)
    if (next.length >= 4 && next.length === 4) submit(next)
  }
  // Typing a PIN with the fingerprint sheet still open fights for the screen.
  const toPin = () => {
    offered.current = true
    cancelPasskeyLogin()
    setReading(false)
    setShowPin(true)
  }
  // A browser that can't read passkeys never offers one — it would only throw.
  const bioReady = passkeys > 0 && passkeySupported()
  return (
    <div data-no-track className="flex min-h-dvh flex-col items-center justify-center gap-8 px-6">
      <div className="flex flex-col items-center gap-2">
        <LogoMark size={52} />
        <div className="text-lg font-semibold">Finance is locked</div>
        <div className={`h-5 text-sm ${err ? 'text-bad' : 'text-muted'}`}>{err || note}</div>
      </div>
      {bioReady && !showPin ? (
        <div className="flex w-full max-w-xs flex-col items-center gap-4">
          <button onClick={bio} className={`grid h-24 w-24 place-items-center rounded-full bg-accent/10 text-accent ring-1 ring-accent/30 active:bg-accent/20 ${reading ? 'animate-pulse ring-2' : ''}`} aria-label="Unlock with fingerprint"><Icon name="fingerprint" size={44} /></button>
          <button className="btn-primary w-full" onClick={bio}>{reading ? 'Waiting for your fingerprint…' : 'Unlock with fingerprint'}</button>
          <button className="text-sm text-muted hover:text-ink" onClick={toPin}>Use PIN instead</button>
        </div>
      ) : <>
      <div className="flex gap-3">
        {[0, 1, 2, 3].map((i) => (
          <span key={i} className={`h-3 w-3 rounded-full ${i < pin.length ? 'bg-ink' : 'bg-axis'}`} />
        ))}
      </div>
      <div className="grid grid-cols-3 gap-3">
        {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => (
          <button key={d} onClick={() => press(d)} className="h-16 w-16 rounded-full bg-surface border border-line text-xl font-medium active:bg-sunken">{d}</button>
        ))}
        <button onClick={bio} disabled={!bioReady} className={`grid h-16 w-16 place-items-center rounded-full text-accent disabled:opacity-0 ${reading ? 'animate-pulse' : ''}`} aria-label="Unlock with passkey"><Icon name="fingerprint" size={26} /></button>
        <button onClick={() => press('0')} className="h-16 w-16 rounded-full bg-surface border border-line text-xl font-medium active:bg-sunken">0</button>
        <button onClick={() => setPin(pin.slice(0, -1))} className="grid h-16 w-16 place-items-center rounded-full text-ink2" aria-label="Delete"><Icon name="chevronL" /></button>
      </div>
      {pin.length > 4 && <button className="btn-primary" onClick={() => submit(pin)}>Unlock</button>}
      {bioReady && <button className="text-sm text-muted hover:text-ink" onClick={() => { setShowPin(false); bio() }}>Use fingerprint instead</button>}
      </>}
    </div>
  )
}
