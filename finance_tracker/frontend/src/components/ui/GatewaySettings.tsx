import { useEffect, useState } from 'react'
import { aiApi, type AIProvider } from '../../api/insights'
import { useAISettings, useSaveAISettings } from '../../hooks/useInsights'

const DEFAULT_URL: Record<AIProvider, string> = {
  gateway: 'https://api.nexos.ai/v1',
  anthropic: 'https://api.anthropic.com/v1',
}

const PROVIDER_COPY: Record<AIProvider, { blurb: string; keyHint: string; modelHint: string }> = {
  gateway: {
    blurb: "Calls go to the gateway's Anthropic-native Messages endpoint (prompt-cache-preserving). Get a key and model list from your nexos.ai workspace.",
    keyHint: 'nxs-…',
    modelHint: 'e.g. Claude Opus 5',
  },
  anthropic: {
    blurb: 'Calls go straight to the Claude API with your own Anthropic key — no gateway in between. Billed to your Anthropic console account.',
    keyHint: 'sk-ant-…',
    modelHint: 'e.g. claude-opus-5',
  },
}

// GatewaySettingsModal is the centered-overlay presentation of the panel
// below. It lives here because two places open it: the ⚙️ in the AI sub-nav,
// and the app chrome when AI is switched off (and the AI tab is therefore
// hidden) — which is the only way back to the switch.
export function GatewaySettingsModal({ onClose }: { onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 overflow-y-auto py-8"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div className="max-w-lg w-full px-4" onClick={(e) => e.stopPropagation()}>
        <GatewaySettings onClose={onClose} />
      </div>
    </div>
  )
}

export default function GatewaySettings({ onClose }: { onClose: () => void }) {
  const { data: settings } = useAISettings()
  const save = useSaveAISettings()
  const [form, setForm] = useState<{
    gateway_url: string
    model: string
    api_key: string
    provider: AIProvider
  }>({ gateway_url: '', model: '', api_key: '', provider: 'gateway' })
  const [loaded, setLoaded] = useState(false)
  const [status, setStatus] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)
  const [testing, setTesting] = useState(false)
  // Model catalogue from the provider. `ok:false` means it exposes no models
  // endpoint — the field stays a plain free-text input.
  const [models, setModels] = useState<{ list: string[]; ok: boolean }>({ list: [], ok: false })
  const [modelsLoading, setModelsLoading] = useState(false)

  useEffect(() => {
    if (settings && !loaded) {
      setForm({
        gateway_url: settings.gateway_url,
        model: settings.model,
        api_key: '',
        provider: settings.provider,
      })
      setLoaded(true)
    }
  }, [settings, loaded])

  const loadModels = async () => {
    setModelsLoading(true)
    try {
      const { models: list, ok } = await aiApi.models()
      setModels({ list, ok })
    } finally {
      setModelsLoading(false)
    }
  }

  useEffect(() => {
    loadModels()
  }, [])

  // Switching provider carries the URL along, but only while it is still a
  // default — a hand-entered endpoint is never overwritten.
  const switchProvider = (provider: AIProvider) => {
    setForm((f) => {
      const url = f.gateway_url.trim()
      const isDefault = url === '' || url === DEFAULT_URL.gateway || url === DEFAULT_URL.anthropic
      return { ...f, provider, gateway_url: isDefault ? DEFAULT_URL[provider] : f.gateway_url }
    })
  }

  const doSave = async () => {
    try {
      await save.mutateAsync({
        gateway_url: form.gateway_url.trim(),
        model: form.model.trim(),
        provider: form.provider,
        ...(form.api_key.trim() ? { api_key: form.api_key.trim() } : {}),
      })
      setForm((f) => ({ ...f, api_key: '' }))
      setStatus({ kind: 'ok', text: 'Saved.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Save failed' })
    }
  }

  // The master switch saves on its own — it must work without the rest of the
  // form being valid, and a partial PUT leaves the stored config untouched.
  const toggleEnabled = async (enabled: boolean) => {
    setStatus(null)
    try {
      await save.mutateAsync({ enabled })
      setStatus({ kind: 'ok', text: enabled ? 'AI features on.' : 'AI features off.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Failed' })
    }
  }

  // A native <select> never triggers Firefox's password manager (unlike a
  // text <input> sharing this form with the API-key field). Use it whenever
  // the provider returned a catalogue; otherwise fall back to a free-text input
  // with autofill explicitly suppressed.
  const useModelSelect = models.ok && models.list.length > 0
  // Always keep the current value selectable, even a custom/legacy model the
  // provider no longer lists.
  const modelOptions =
    form.model && !models.list.includes(form.model) ? [form.model, ...models.list] : models.list

  const doTest = async () => {
    setTesting(true)
    setStatus(null)
    try {
      await aiApi.test()
      setStatus({ kind: 'ok', text: 'Provider reachable — model replied.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Test failed' })
    } finally {
      setTesting(false)
    }
  }

  const enabled = settings?.enabled ?? true
  const copy = PROVIDER_COPY[form.provider]

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex items-center justify-between mb-3">
        <h2 className="font-semibold text-gray-900">⚙️ AI provider</h2>
        <button onClick={onClose} className="text-gray-300 hover:text-gray-600">✕</button>
      </div>

      {/* Master switch — first, because everything below only matters when it's on. */}
      <div className="flex items-start justify-between gap-3 p-3 rounded-xl bg-gray-50 border border-gray-100">
        <div>
          <div className="text-sm font-medium text-gray-900">AI features</div>
          <p className="text-xs text-gray-400 mt-0.5">
            {enabled
              ? 'Insights, chat, receipt scanning, auto-labelling and forecasts are available.'
              : 'Every AI surface is hidden and the AI endpoints are switched off. Your settings below are kept.'}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          aria-label="AI features"
          disabled={save.isPending}
          onClick={() => toggleEnabled(!enabled)}
          className={`relative shrink-0 w-11 h-6 rounded-full transition-colors disabled:opacity-50 ${
            enabled ? 'bg-indigo-600' : 'bg-gray-300'
          }`}
        >
          <span
            className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${
              enabled ? 'translate-x-5' : ''
            }`}
          />
        </button>
      </div>

      <p className="text-xs text-gray-400 mt-3 mb-3">{copy.blurb}</p>

      <div className="grid sm:grid-cols-2 gap-3">
        <label className="text-xs text-gray-500">
          Provider
          <select
            value={form.provider}
            onChange={(e) => switchProvider(e.target.value as AIProvider)}
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2 bg-white"
          >
            <option value="gateway">nexos.ai gateway</option>
            <option value="anthropic">Claude API (Anthropic)</option>
          </select>
        </label>
        <label className="text-xs text-gray-500">
          {form.provider === 'anthropic' ? 'API base URL' : 'Gateway URL'}
          <input
            value={form.gateway_url}
            onChange={(e) => setForm({ ...form, gateway_url: e.target.value })}
            placeholder={DEFAULT_URL[form.provider]}
            autoComplete="off"
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-3 py-2"
          />
        </label>
        <label className="text-xs text-gray-500">
          Model
          <div className="flex items-center gap-1.5 mt-1">
            {useModelSelect ? (
              <select
                value={form.model}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                className="block w-full text-sm border border-gray-200 rounded-lg px-3 py-2 bg-white"
              >
                {!form.model && (
                  <option value="" disabled>
                    Select a model…
                  </option>
                )}
                {modelOptions.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            ) : (
              <input
                value={form.model}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                placeholder={copy.modelHint}
                // Suppress Firefox/1Password/LastPass autofill on this free-text
                // model field — it shares the form with the API-key password.
                autoComplete="off"
                name="ai-model"
                data-1p-ignore="true"
                data-lpignore="true"
                className="block w-full text-sm border border-gray-200 rounded-lg px-3 py-2"
              />
            )}
            <button
              type="button"
              onClick={loadModels}
              disabled={modelsLoading}
              aria-label="Refresh model list"
              title="Refresh model list"
              className="shrink-0 px-2.5 py-2 rounded-lg border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-50"
            >
              <span className={modelsLoading ? 'inline-block animate-spin' : ''}>↻</span>
            </button>
          </div>
          <span className="block mt-1 text-xs text-gray-400">
            {modelsLoading
              ? 'Loading models…'
              : models.ok
                ? `${models.list.length} model${models.list.length === 1 ? '' : 's'} from your provider`
                : 'no model list available — type the model name'}
          </span>
        </label>
        <label className="text-xs text-gray-500">
          API key {settings?.has_key && <span className="text-emerald-600">(saved — leave blank to keep)</span>}
          <input
            type="password"
            value={form.api_key}
            onChange={(e) => setForm({ ...form, api_key: e.target.value })}
            placeholder={settings?.has_key ? '••••••••' : copy.keyHint}
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
          disabled={testing || !settings?.has_key || !enabled}
          title={enabled ? undefined : 'Turn AI features on to test the connection'}
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
                await save.mutateAsync({ clear_key: true })
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
