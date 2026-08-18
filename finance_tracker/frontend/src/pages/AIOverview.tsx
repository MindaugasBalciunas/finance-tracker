import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
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

      <AIContextCard />

      <p className="text-[11px] text-gray-400">
        Each analysis sends your aggregated financial summary (scoped to the app's date range) to the
        configured gateway. Conversations and analyses are stored in your own database.
      </p>
    </div>
  )
}

// AIContextCard edits the user's CFO-context briefing: who they are, their
// investment framework, standing rules and communication style. It rides
// along on EVERY AI call (chat, overview, view reviews) as a cached system
// block, and MCP clients read it via get_user_context.
function AIContextCard() {
  const qc = useQueryClient()
  const { data: ctx } = useQuery({ queryKey: ['ai-context'], queryFn: aiApi.getContext })
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [status, setStatus] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  const hasContext = !!ctx?.content?.trim()
  const startEdit = () => {
    setDraft(ctx?.content ?? '')
    setStatus(null)
    setEditing(true)
  }
  const save = async () => {
    setSaving(true)
    setStatus(null)
    try {
      const res = await aiApi.saveContext(draft)
      qc.setQueryData(['ai-context'], res)
      setEditing(false)
      setStatus({ kind: 'ok', text: 'Saved — every AI feature now advises with this context.' })
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setStatus({ kind: 'error', text: e.response?.data?.error ?? e.message ?? 'Save failed' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-1">
        <h2 className="font-semibold text-gray-900">🧠 AI context — about me</h2>
        {!editing && (
          <button
            onClick={startEdit}
            className="text-sm px-3 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50"
          >
            {hasContext ? '✎ Edit' : '+ Write it'}
          </button>
        )}
      </div>
      <p className="text-xs text-gray-400 mb-3">
        Your standing brief for the AI: who you are, income structure, investment framework, rules it must
        follow, open decisions and how you want to be spoken to (markdown, up to 32KB). It is included in every
        chat, overview and view review — and MCP clients (Claude, Gemini) read it via the get_user_context tool.
        Keep it current: stale facts here mislead every future answer.
      </p>

      {editing ? (
        <>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={16}
            placeholder={'# Who I am\n…\n\n## Income\n…\n\n## Investment framework\n…\n\n## Rules for the AI\n- Direct, no sugar-coating\n- Make a discipline point once, then drop it'}
            className="w-full text-sm font-mono border border-gray-200 rounded-xl px-3 py-2.5 focus:outline-none focus:ring-2 focus:ring-indigo-200"
          />
          <div className="flex flex-wrap items-center gap-2 mt-2">
            <button
              onClick={save}
              disabled={saving}
              className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Save context'}
            </button>
            <button
              onClick={() => setEditing(false)}
              className="px-3 py-1.5 text-sm rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50"
            >
              Cancel
            </button>
            <span className="text-xs text-gray-400 ml-auto">{new Blob([draft]).size.toLocaleString()} / 32,000 bytes</span>
          </div>
        </>
      ) : hasContext ? (
        <div className="max-h-56 overflow-y-auto rounded-xl border border-gray-100 bg-gray-50/60 p-3 text-xs text-gray-600 whitespace-pre-wrap font-mono">
          {ctx!.content}
        </div>
      ) : (
        <p className="text-sm text-gray-400 italic">
          Nothing written yet — paste your CFO briefing here and every AI answer starts advising like it knows you.
        </p>
      )}
      {status && (
        <p className={`mt-2 text-xs ${status.kind === 'ok' ? 'text-emerald-600' : 'text-red-600'}`}>{status.text}</p>
      )}
      {hasContext && !editing && ctx?.updated_at && (
        <p className="mt-2 text-[11px] text-gray-400">Last updated {new Date(ctx.updated_at).toLocaleDateString()}</p>
      )}
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
