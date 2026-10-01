import { useEffect, useState } from 'react'
import { useBankSettings, useSaveBankSettings } from '../../hooks/useBanking'

// BankSettingsModal holds the Enable Banking application credentials: the
// application id (an identifier) and the RSA private key it signs requests
// with (a long-lived bank credential — write-only over the API, never echoed
// back, and deliberately excluded from the JSON backup).
export default function BankSettingsModal({ onClose }: { onClose: () => void }) {
  const { data: settings } = useBankSettings()
  const save = useSaveBankSettings()
  const [form, setForm] = useState({
    application_id: '',
    private_key_pem: '',
    environment: 'production' as 'sandbox' | 'production',
    redirect_url: '',
  })
  const [loaded, setLoaded] = useState(false)
  const [status, setStatus] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  useEffect(() => {
    if (settings && !loaded) {
      setForm({
        application_id: settings.application_id,
        private_key_pem: '',
        environment: settings.environment,
        redirect_url: settings.redirect_url || `${window.location.origin}/`,
      })
      setLoaded(true)
    }
  }, [settings, loaded])

  // The key arrives as a .pem download from Enable Banking. Reading it in the
  // browser beats asking anyone to paste a multi-line PEM into a textarea.
  const readKeyFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const text = await file.text()
    if (!text.includes('BEGIN')) {
      setStatus({ kind: 'error', text: 'That file does not look like a PEM private key.' })
      return
    }
    setForm((f) => ({ ...f, private_key_pem: text }))
    setStatus({ kind: 'ok', text: `Key read from ${file.name} — press Save.` })
  }

  const doSave = async () => {
    setStatus(null)
    try {
      await save.mutateAsync({
        application_id: form.application_id.trim(),
        environment: form.environment,
        redirect_url: form.redirect_url.trim(),
        ...(form.private_key_pem.trim() ? { private_key_pem: form.private_key_pem } : {}),
      })
      setForm((f) => ({ ...f, private_key_pem: '' }))
      setStatus({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      setStatus({ kind: 'error', text: err instanceof Error ? err.message : 'Save failed' })
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start sm:items-center justify-center bg-black/40 overflow-y-auto py-8"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div className="max-w-lg w-full px-4" onClick={(e) => e.stopPropagation()}>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
          <div className="flex items-center justify-between mb-3">
            <h2 className="font-semibold text-gray-900">⚙️ Enable Banking application</h2>
            <button onClick={onClose} className="text-gray-300 hover:text-gray-600" aria-label="Close">✕</button>
          </div>

          <p className="text-xs text-gray-400 mb-3">
            Banks don't let an app talk to them directly — that needs a licence. Enable Banking is a
            licensed aggregator that does. Register an application in their control panel, download
            its private key, and paste the application id here.
          </p>

          <div className="space-y-3">
            <label className="block text-xs text-gray-500">
              Application ID
              <input
                value={form.application_id}
                onChange={(e) => setForm({ ...form, application_id: e.target.value })}
                placeholder="00000000-0000-0000-0000-000000000000"
                autoComplete="off"
                className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
              />
            </label>

            <label className="block text-xs text-gray-500">
              Private key{' '}
              {settings?.has_key && (
                <span className="text-emerald-600">(saved — leave blank to keep)</span>
              )}
              <input
                type="file"
                accept=".pem,.key,text/plain"
                onChange={readKeyFile}
                className="block w-full mt-1 text-sm text-gray-600 file:mr-3 file:py-1.5 file:px-3 file:rounded-lg file:border file:border-gray-200 file:text-xs file:bg-gray-50"
              />
              <span className="block mt-1 text-xs text-gray-400">
                The .pem file from Enable Banking. It never leaves this instance and is kept out of
                the JSON backup.
              </span>
            </label>

            <div className="grid sm:grid-cols-2 gap-3">
              <label className="block text-xs text-gray-500">
                Environment
                <select
                  value={form.environment}
                  onChange={(e) =>
                    setForm({ ...form, environment: e.target.value as 'sandbox' | 'production' })
                  }
                  className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2 bg-white"
                >
                  <option value="production">Production</option>
                  <option value="sandbox">Sandbox</option>
                </select>
              </label>
              <label className="block text-xs text-gray-500">
                Redirect URL
                <input
                  value={form.redirect_url}
                  onChange={(e) => setForm({ ...form, redirect_url: e.target.value })}
                  placeholder="https://host:8443/"
                  autoComplete="off"
                  className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
                />
              </label>
            </div>
            <p className="text-xs text-gray-400">
              The redirect URL has to match the one registered with Enable Banking exactly — scheme,
              port and trailing slash included. If the bank can't reach this app afterwards, the
              paste-the-address fallback on the Banking page still works.
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2 mt-4">
            <button
              onClick={doSave}
              disabled={save.isPending || !form.application_id.trim()}
              className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {save.isPending ? 'Saving…' : 'Save'}
            </button>
            {settings?.has_key && (
              <button
                onClick={async () => {
                  if (!confirm('Remove the stored private key? Existing bank connections stop working until a new key is saved.')) return
                  try {
                    await save.mutateAsync({ clear_key: true })
                    setForm((f) => ({ ...f, private_key_pem: '' }))
                    setStatus({ kind: 'ok', text: 'Key removed.' })
                  } catch (err) {
                    setStatus({ kind: 'error', text: err instanceof Error ? err.message : 'Failed' })
                  }
                }}
                className="px-3 py-1.5 text-sm text-red-500 hover:text-red-700"
              >
                Remove key
              </button>
            )}
            {status && (
              <span className={`text-xs ${status.kind === 'ok' ? 'text-emerald-600' : 'text-red-600'}`}>
                {status.text}
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
