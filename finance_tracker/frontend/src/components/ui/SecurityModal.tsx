import { useState, useEffect, useCallback } from 'react'
import { authApi, type AuthStatus, type WebauthnCredentialInfo } from '../../api/auth'
import { webauthnAvailable } from '../../utils/webauthnCodec'

interface Props {
  onClose: () => void
}

export default function SecurityModal({ onClose }: Props) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [creds, setCreds] = useState<WebauthnCredentialInfo[]>([])
  const [pin, setPin] = useState('')
  const [pinConfirm, setPinConfirm] = useState('')
  const [currentPin, setCurrentPin] = useState('')
  const [deviceName, setDeviceName] = useState('')
  const [message, setMessage] = useState<{ text: string; ok: boolean } | null>(null)
  const [busy, setBusy] = useState(false)
  const [mode, setMode] = useState<'overview' | 'setup' | 'change' | 'disable'>('overview')

  const refresh = useCallback(() => {
    authApi.status().then((s) => {
      setStatus(s)
      if (s.enabled && s.unlocked) {
        authApi.listCredentials().then(setCreds).catch(() => setCreds([]))
      }
    }).catch(() => {})
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  function resetInputs() {
    setPin(''); setPinConfirm(''); setCurrentPin(''); setMessage(null)
  }

  async function run(action: () => Promise<void>, okMessage: string) {
    setBusy(true)
    setMessage(null)
    try {
      await action()
      setMessage({ text: okMessage, ok: true })
      resetInputs()
      setMode('overview')
      refresh()
    } catch (err) {
      setMessage({ text: (err as Error).message, ok: false })
    } finally {
      setBusy(false)
    }
  }

  const pinValid = /^\d{4,8}$/.test(pin) && pin === pinConfirm

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 overflow-y-auto py-8"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-1">
          <h3 className="text-lg font-semibold text-gray-900">🔒 Security</h3>
          <button onClick={onClose} aria-label="Close" className="text-gray-400 hover:text-gray-600 text-xl leading-none">×</button>
        </div>
        <p className="text-xs text-gray-400 mb-4">
          Lock the app behind a PIN{webauthnAvailable() ? ' and fingerprint' : ''}. Applies to every device.
        </p>

        {message && (
          <p className={`text-sm rounded-lg px-3 py-2 mb-3 border ${message.ok ? 'text-green-700 bg-green-50 border-green-200' : 'text-red-600 bg-red-50 border-red-200'}`}>
            {message.text}
          </p>
        )}

        {!status ? (
          <p className="text-sm text-gray-400 py-4">Loading…</p>
        ) : mode === 'overview' ? (
          <div className="space-y-3">
            <div className="flex items-center justify-between bg-gray-50 rounded-lg px-4 py-3">
              <div>
                <p className="text-sm font-medium text-gray-800">App lock</p>
                <p className="text-xs text-gray-400">{status.enabled ? 'Enabled — PIN required' : 'Disabled'}</p>
              </div>
              <span className={`text-xs font-semibold px-2 py-1 rounded-full ${status.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-200 text-gray-600'}`}>
                {status.enabled ? 'ON' : 'OFF'}
              </span>
            </div>

            {!status.enabled ? (
              <button
                onClick={() => { resetInputs(); setMode('setup') }}
                className="w-full px-4 py-2.5 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
              >
                Set up PIN lock
              </button>
            ) : (
              <>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    onClick={() => { resetInputs(); setMode('change') }}
                    className="px-4 py-2.5 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
                  >
                    Change PIN
                  </button>
                  <button
                    onClick={() => { resetInputs(); setMode('disable') }}
                    className="px-4 py-2.5 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100"
                  >
                    Disable lock
                  </button>
                </div>

                {/* Fingerprint enrollment */}
                <div className="border-t border-gray-100 pt-3">
                  <p className="text-sm font-medium text-gray-800 mb-1">Fingerprint / Face unlock</p>
                  {webauthnAvailable() ? (
                    <>
                      {creds.map((c) => (
                        <div key={c.id} className="flex items-center justify-between text-sm py-1.5">
                          <span className="text-gray-700">👆 {c.name}</span>
                          <button
                            onClick={() => run(async () => { await authApi.deleteCredential(c.id) }, 'Device removed')}
                            className="text-xs text-red-500 hover:text-red-700"
                          >
                            remove
                          </button>
                        </div>
                      ))}
                      <div className="flex gap-2 mt-2">
                        <input
                          type="text"
                          value={deviceName}
                          onChange={(e) => setDeviceName(e.target.value)}
                          placeholder="Device name (e.g. My phone)"
                          className="flex-1 border border-gray-300 rounded-lg px-3 py-2 text-sm"
                        />
                        <button
                          disabled={busy}
                          onClick={() => run(
                            async () => { await authApi.registerFingerprint(deviceName || 'Device'); setDeviceName('') },
                            'Fingerprint enrolled on this device'
                          )}
                          className="px-3 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 whitespace-nowrap"
                        >
                          Enroll
                        </button>
                      </div>
                      <p className="text-xs text-gray-400 mt-1.5">
                        Enroll on each device you use. The fingerprint never leaves the device.
                      </p>
                    </>
                  ) : (
                    <p className="text-xs text-gray-400">
                      Not available here — fingerprint requires HTTPS or localhost. The PIN works everywhere.
                    </p>
                  )}
                </div>

                <button
                  onClick={() => run(async () => { await authApi.logout(); window.location.reload() }, '')}
                  className="w-full px-4 py-2.5 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
                >
                  Lock now
                </button>
              </>
            )}
          </div>
        ) : (
          <div className="space-y-3">
            {(mode === 'change' || mode === 'disable') && (
              <input
                type="password" inputMode="numeric" maxLength={8} autoComplete="off"
                value={currentPin}
                onChange={(e) => setCurrentPin(e.target.value.replace(/\D/g, ''))}
                placeholder="Current PIN"
                className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
              />
            )}
            {mode !== 'disable' && (
              <>
                <input
                  type="password" inputMode="numeric" maxLength={8} autoComplete="off"
                  value={pin}
                  onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))}
                  placeholder="New PIN (4–8 digits)"
                  className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
                />
                <input
                  type="password" inputMode="numeric" maxLength={8} autoComplete="off"
                  value={pinConfirm}
                  onChange={(e) => setPinConfirm(e.target.value.replace(/\D/g, ''))}
                  placeholder="Repeat new PIN"
                  className="w-full border border-gray-300 rounded-lg px-3 py-2.5 text-sm"
                />
                {pin && pinConfirm && pin !== pinConfirm && (
                  <p className="text-xs text-red-500">PINs don't match</p>
                )}
              </>
            )}
            <div className="flex gap-2">
              <button
                disabled={busy || (mode === 'disable' ? currentPin.length < 4 : !pinValid)}
                onClick={() => {
                  if (mode === 'setup') run(async () => { await authApi.pinSetup(pin) }, 'App lock enabled')
                  else if (mode === 'change') run(async () => { await authApi.pinSetup(pin, currentPin) }, 'PIN changed')
                  else run(async () => { await authApi.pinDisable(currentPin) }, 'App lock disabled')
                }}
                className={`flex-1 px-4 py-2.5 text-sm font-medium text-white rounded-lg disabled:opacity-40 ${mode === 'disable' ? 'bg-red-600 hover:bg-red-700' : 'bg-blue-600 hover:bg-blue-700'}`}
              >
                {busy ? 'Working…' : mode === 'setup' ? 'Enable lock' : mode === 'change' ? 'Change PIN' : 'Disable lock'}
              </button>
              <button
                onClick={() => { resetInputs(); setMode('overview') }}
                className="px-4 py-2.5 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
