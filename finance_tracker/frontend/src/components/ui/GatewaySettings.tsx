import { useEffect, useState } from 'react'
import { aiApi } from '../../api/insights'
import { useAISettings, useSaveAISettings } from '../../hooks/useInsights'

export default function GatewaySettings({ onClose }: { onClose: () => void }) {
  const { data: settings } = useAISettings()
  const save = useSaveAISettings()
  const [form, setForm] = useState({ gateway_url: '', model: '', api_key: '' })
  const [loaded, setLoaded] = useState(false)
  const [status, setStatus] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)
  const [testing, setTesting] = useState(false)

  useEffect(() => {
    if (settings && !loaded) {
      setForm({ gateway_url: settings.gateway_url, model: settings.model, api_key: '' })
      setLoaded(true)
    }
  }, [settings, loaded])

  const doSave = async () => {
    try {
      await save.mutateAsync({
        gateway_url: form.gateway_url.trim(),
        model: form.model.trim(),
        ...(form.api_key.trim() ? { api_key: form.api_key.trim() } : {}),
      })
      setForm((f) => ({ ...f, api_key: '' }))
      setStatus({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Save failed' })
    }
  }

  const doTest = async () => {
    setTesting(true)
    setStatus(null)
    try {
      await aiApi.test()
      setStatus({ kind: 'ok', text: 'Gateway reachable — model replied.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Test failed' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex items-center justify-between mb-1">
        <h2 className="font-semibold text-gray-900">⚙️ nexos.ai gateway</h2>
        <button onClick={onClose} className="text-gray-300 hover:text-gray-600">✕</button>
      </div>
      <p className="text-xs text-gray-400 mb-3">
        Calls go to the gateway's Anthropic-native Messages endpoint (prompt-cache-preserving).
        Get a key and model list from your nexos.ai workspace.
      </p>
      <div className="grid sm:grid-cols-2 gap-3">
        <label className="text-xs text-gray-500 sm:col-span-2">
          Gateway URL
          <input
            value={form.gateway_url}
            onChange={(e) => setForm({ ...form, gateway_url: e.target.value })}
            placeholder="https://api.nexos.ai/v1"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
        <label className="text-xs text-gray-500">
          Model
          <input
            value={form.model}
            onChange={(e) => setForm({ ...form, model: e.target.value })}
            placeholder="e.g. Claude Opus 5"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
        <label className="text-xs text-gray-500">
          API key {settings?.has_key && <span className="text-emerald-600">(saved — leave blank to keep)</span>}
          <input
            type="password"
            value={form.api_key}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
            placeholder={settings?.has_key ? '••••••••' : 'nxs-…'}
            autoComplete="new-password"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-2 mt-3">
        <button
          onClick={doSave}
          disabled={save.isPending || !form.model.trim()}
          className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          {save.isPending ? 'Saving…' : 'Save'}
        </button>
        <button
          onClick={doTest}
          disabled={testing || !settings?.has_key}
          className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
        >
          {testing ? 'Testing…' : 'Test connection'}
        </button>
        {settings?.has_key && (
          <button
            onClick={async () => {
              // Only clear the key — keep the saved URL/model, not whatever
              // half-typed values happen to be in the form right now.
              try {
                await save.mutateAsync({
                  gateway_url: settings.gateway_url, model: settings.model, clear_key: true,
                })
                setForm((f) => ({ ...f, api_key: '' }))
                setStatus({ kind: 'ok', text: 'Key removed.' })
              } catch (err) {
                const e = err as { response?: { data?: { error?: string } }; message?: string }
                setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Failed' })
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
  )
}
