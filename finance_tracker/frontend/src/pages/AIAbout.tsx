import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../api/insights'
import AINav from '../components/ui/AINav'
import Markdown from '../components/ui/Markdown'

// About me: the user's CFO-context briefing on its own tab. It rides along
// on EVERY AI call (chat, overview, view reviews) as a cached system block,
// MCP clients read it via get_user_context, and it travels with backups and
// the AI dataset ZIP. The chat can append dated entries to its Decisions log
// via record_user_decision.
export default function AIAbout() {
  const qc = useQueryClient()
  const { data: ctx } = useQuery({ queryKey: ['ai-context'], queryFn: aiApi.getContext })
  const [editing, setEditing] = useState(false)
  const [preview, setPreview] = useState(false)
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
    <div className="p-4 sm:p-6 space-y-4 max-w-4xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">🧠 About me</h1>
          <p className="text-xs text-gray-400">
            Your standing brief for the AI — included in every chat, overview and view review
          </p>
        </div>
        <span className="flex flex-wrap items-center justify-end gap-2">
          {!editing && hasContext && (
            <button
              onClick={() => setPreview((p) => !p)}
              className="text-sm px-3 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50"
            >
              {preview ? 'Raw' : 'Preview'}
            </button>
          )}
          {!editing && (
            <button
              onClick={startEdit}
              className="text-sm px-3 py-1.5 rounded-lg bg-indigo-600 text-white hover:bg-indigo-700"
            >
              {hasContext ? '✎ Edit' : '+ Write it'}
            </button>
          )}
          <AINav />
        </span>
      </div>

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
        <p className="text-xs text-gray-400 mb-3">
          Who you are, income structure, investment framework, rules the AI must follow, open decisions and
          how you want to be spoken to (markdown, up to 32KB). MCP clients (Claude, Gemini) read it via the
          get_user_context tool; it travels with your backups and the AI dataset ZIP. Decisions you state in
          chat get appended to its Decisions log. Keep it current — stale facts here mislead every answer.
        </p>

        {editing ? (
          <>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              rows={26}
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
          preview ? (
            <div className="rounded-xl border border-gray-100 bg-gray-50/60 p-4 text-gray-700">
              <Markdown>{ctx!.content}</Markdown>
            </div>
          ) : (
            <div className="rounded-xl border border-gray-100 bg-gray-50/60 p-3 text-xs text-gray-600 whitespace-pre-wrap font-mono">
              {ctx!.content}
            </div>
          )
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
    </div>
  )
}
