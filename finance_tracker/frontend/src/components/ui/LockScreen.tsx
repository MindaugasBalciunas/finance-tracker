import { useState, useEffect, useRef } from 'react'
import { authApi, type AuthStatus } from '../../api/auth'
import { webauthnAvailable } from '../../utils/webauthnCodec'

interface Props {
  status: AuthStatus
  onUnlocked: () => void
}

export default function LockScreen({ status, onUnlocked }: Props) {
  const [pin, setPin] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const canFingerprint = status.webauthn_registered && webauthnAvailable()

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  async function submitPin(e?: React.FormEvent) {
    e?.preventDefault()
    if (pin.length < 4 || busy) return
    setBusy(true)
    setError(null)
    try {
      await authApi.pinLogin(pin)
      onUnlocked()
    } catch (err) {
      setError((err as Error).message === 'wrong PIN' ? 'Wrong PIN — try again' : (err as Error).message)
      setPin('')
      inputRef.current?.focus()
    } finally {
      setBusy(false)
    }
  }

  async function useFingerprint() {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      await authApi.fingerprintLogin()
      onUnlocked()
    } catch (err) {
      const msg = (err as Error).message
      setError(msg.includes('cancel') || msg.includes('NotAllowed') ? 'Fingerprint not recognised' : msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center p-4">
      <div className="w-full max-w-xs text-center">
        <div className="text-4xl mb-3">🔒</div>
        <h1 className="text-xl font-bold text-gray-900">Finance Tracker</h1>
        <p className="text-sm text-gray-500 mt-1 mb-6">Enter your PIN to unlock</p>

        <form onSubmit={submitPin}>
          <input
            ref={inputRef}
            type="password"
            inputMode="numeric"
            pattern="[0-9]*"
            autoComplete="off"
            maxLength={8}
            value={pin}
            onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))}
            className="w-full text-center text-2xl tracking-[0.5em] border border-gray-300 rounded-xl px-4 py-3 focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="••••"
          />
          {error && <p className="text-sm text-red-600 mt-3">{error}</p>}
          <button
            type="submit"
            disabled={pin.length < 4 || busy}
            className="w-full mt-4 px-4 py-3 text-sm font-semibold text-white bg-blue-600 rounded-xl hover:bg-blue-700 disabled:opacity-40 transition-colors"
          >
            {busy ? 'Checking…' : 'Unlock'}
          </button>
        </form>

        {canFingerprint && (
          <button
            onClick={useFingerprint}
            disabled={busy}
            className="w-full mt-3 px-4 py-3 text-sm font-semibold text-blue-700 bg-blue-50 border border-blue-200 rounded-xl hover:bg-blue-100 disabled:opacity-40 transition-colors"
          >
            👆 Unlock with fingerprint
          </button>
        )}
        {status.webauthn_registered && !webauthnAvailable() && (
          <p className="text-xs text-gray-400 mt-4">
            Fingerprint unlock needs HTTPS or localhost — use your PIN here.
          </p>
        )}
      </div>
    </div>
  )
}
