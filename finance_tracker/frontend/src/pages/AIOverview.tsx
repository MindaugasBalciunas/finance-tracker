import { useEffect, useState } from 'react'
import { aiApi } from '../api/insights'
import { useAISettings, useSaveAISettings } from '../hooks/useInsights'
import AINav from '../components/ui/AINav'
import AIInsightCard from '../components/ui/AIInsightCard'

// AI Financial Overview: the per-section analysis (now with a detailed
// category review) plus the nexos.ai gateway configuration. The chat lives
// on its own full-screen tab.
export default function AIOverview() {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [autoOpened, setAutoOpened] = useState(false)

  // Open the settings panel once on first load when nothing is configured,
  // then leave it under the user's control (so the ✕ actually closes it).
  useEffect(() => {
    if (settings && !configured && !autoOpened) {
      setSettingsOpen(true)
      setAutoOpened(true)
    }
  }, [settings, configured, autoOpened])

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">✦ AI Financial Overview</h1>
          <p className="text-xs text-gray-400">
            Per-section analysis with detailed category reviews
            {configured && settings?.model && <> · <span className="text-indigo-500">{settings.model}</span></>}
          </p>
        </div>
        <span className="flex items-center gap-2">
          <button
            onClick={() => setSettingsOpen((o) => !o)}
            className={`text-sm px-3 py-1.5 rounded-lg border ${settingsOpen ? 'bg-gray-100 border-gray-300 text-gray-800' : 'border-gray-200 text-gray-600 hover:bg-gray-50'}`}
          >
            ⚙️
          </button>
          <AINav />
        </span>
      </div>

      {settingsOpen && <GatewaySettings onClose={() => setSettingsOpen(false)} />}

      <AIInsightCard />

      <p className="text-[11px] text-gray-400">
        Each analysis sends your aggregated financial summary (scoped to the app's date range) to the
        configured gateway. Conversations and analyses are stored in your own database.
      </p>
    </div>
  )
}

function GatewaySettings({ onClose }: { onClose: () => void }) {
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
