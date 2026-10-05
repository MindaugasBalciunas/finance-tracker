import { LogoMark } from '../components/Logo'
import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { Icon } from '../components/Icon'
import { passkeyLogin } from '../lib/passkey'

export default function Lock({ onUnlock }: { onUnlock: () => void }) {
  const [pin, setPin] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [passkeys, setPasskeys] = useState(0)

  useEffect(() => {
    api.get<{ passkeys: number }>('/auth/status').then((s) => setPasskeys(s.passkeys)).catch(() => {})
  }, [])

  const submit = async (p: string) => {
    setBusy(true)
    setErr('')
    try {
      await api.post('/auth/pin/login', { pin: p })
      onUnlock()
    } catch (e) {
      setErr((e as Error).message)
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
  const bio = async () => {
    setErr('')
    try {
      await passkeyLogin()
      onUnlock()
    } catch (e) {
      setErr((e as Error).message || 'Passkey failed')
    }
  }
  return (
    <div data-no-track className="flex min-h-dvh flex-col items-center justify-center gap-8 px-6">
      <div className="flex flex-col items-center gap-2">
        <LogoMark size={52} />
        <div className="text-lg font-semibold">Finance is locked</div>
        <div className="h-5 text-sm text-bad">{err}</div>
      </div>
      <div className="flex gap-3">
        {[0, 1, 2, 3].map((i) => (
          <span key={i} className={`h-3 w-3 rounded-full ${i < pin.length ? 'bg-ink' : 'bg-axis'}`} />
        ))}
      </div>
      <div className="grid grid-cols-3 gap-3">
        {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => (
          <button key={d} onClick={() => press(d)} className="h-16 w-16 rounded-full bg-surface border border-line text-xl font-medium active:bg-sunken">{d}</button>
        ))}
        <button onClick={bio} disabled={!passkeys} className="grid h-16 w-16 place-items-center rounded-full text-accent disabled:opacity-0" aria-label="Unlock with passkey"><Icon name="fingerprint" size={26} /></button>
        <button onClick={() => press('0')} className="h-16 w-16 rounded-full bg-surface border border-line text-xl font-medium active:bg-sunken">0</button>
        <button onClick={() => setPin(pin.slice(0, -1))} className="grid h-16 w-16 place-items-center rounded-full text-ink2" aria-label="Delete"><Icon name="chevronL" /></button>
      </div>
      {pin.length > 4 && <button className="btn-primary" onClick={() => submit(pin)}>Unlock</button>}
    </div>
  )
}
