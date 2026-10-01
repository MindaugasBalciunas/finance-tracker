import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../api/insights'
import { useAIAvailable } from '../hooks/useInsights'
import LabelsNav, { BannerAlert, type Banner } from '../components/ui/LabelsNav'
import { formatEuro } from '../utils/format'

// AI tagging on its own page: bulk-scan unlabeled transactions for label
// proposals, or audit already-labeled ones for remaps. Vocabulary-only,
// fixed labels never removed, nothing written until the user applies.
export default function LabelAI() {
  const configured = useAIAvailable()
  const qc = useQueryClient()
  const [banner, setBanner] = useState<Banner>(null)
  const [mode, setMode] = useState<'unlabeled' | 'review'>('unlabeled')
  const [offset, setOffset] = useState(0)
  const [scanning, setScanning] = useState(false)
  const [applying, setApplying] = useState(false)
  const [result, setResult] = useState<Awaited<ReturnType<typeof aiApi.labelReindex>> | null>(null)
  const [checked, setChecked] = useState<Set<number>>(new Set())

  const scan = async (m: 'unlabeled' | 'review', nextOffset: number) => {
    setScanning(true)
    setMode(m)
    try {
      const res = await aiApi.labelReindex(m, nextOffset)
      setResult(res)
      setOffset(nextOffset + res.scanned)
      // Additions are pre-checked; anything that removes a label defaults to
      // unchecked so destructive changes need an explicit opt-in.
      setChecked(new Set(res.suggestions.filter((s) => !(s.remove?.length)).map((s) => s.id)))
      if (res.suggestions.length === 0) {
        setBanner({ kind: 'ok', text: res.scanned === 0
          ? (m === 'unlabeled'
              ? 'Nothing to tag — every transaction with a description already has labels.'
              : 'Nothing left to review in this pass.')
          : m === 'unlabeled'
            ? `Scanned ${res.scanned} unlabeled transactions — the AI found no confident matches.`
            : `Reviewed ${res.scanned} labeled transactions — everything looks consistent.` })
      }
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setBanner({ kind: 'error', text: `Scan failed: ${e.response?.data?.error ?? e.message ?? 'unknown error'}` })
    } finally {
      setScanning(false)
    }
  }

  const apply = async () => {
    if (!result) return
    const items = result.suggestions
      .filter((s) => checked.has(s.id))
      .map((s) => ({ id: s.id, add: s.add ?? [], remove: s.remove ?? [] }))
    if (items.length === 0) return
    setApplying(true)
    try {
      const applied = await aiApi.applyLabelSuggestions(items)
      setBanner({ kind: 'ok', text: mode === 'unlabeled'
        ? `Tagged ${applied} transaction${applied === 1 ? '' : 's'} with AI suggestions.`
        : `Remapped labels on ${applied} transaction${applied === 1 ? '' : 's'}.` })
      setResult(null)
      for (const key of ['transactions', 'transactions-summary', 'labels', 'label-stats', 'label-suggestions']) {
        qc.invalidateQueries({ queryKey: [key] })
      }
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setBanner({ kind: 'error', text: `Apply failed: ${e.response?.data?.error ?? e.message ?? 'unknown error'}` })
    } finally {
      setApplying(false)
    }
  }

  const toggle = (id: number) =>
    setChecked((c) => {
      const next = new Set(c)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  // Label operations among the checked suggestions — the apply button calls
  // out removals so they can't ride along unnoticed.
  const selected = (result?.suggestions ?? []).filter((s) => checked.has(s.id))
  const addCount = selected.reduce((n, s) => n + (s.add?.length ?? 0), 0)
  const removeCount = selected.reduce((n, s) => n + (s.remove?.length ?? 0), 0)
  const applyText = removeCount > 0
    ? `Apply (${[
        addCount > 0 ? `${addCount} addition${addCount === 1 ? '' : 's'}` : '',
        `${removeCount} removal${removeCount === 1 ? '' : 's'}`,
      ].filter(Boolean).join(', ')})`
    : `Apply ${checked.size} selected`

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-5xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">✦ AI tagging</h1>
          <p className="text-xs text-gray-400">
            AI-proposed labels and remaps — review everything before it's written
          </p>
        </div>
        <LabelsNav />
      </div>

      <BannerAlert banner={banner} onClose={() => setBanner(null)} />

      {!configured ? (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5 text-sm text-gray-500">
          AI tagging needs the gateway configured first — add your API key and model on the{' '}
          <Link to="/ai" className="text-indigo-600 hover:underline">AI page</Link>.
        </div>
      ) : (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
          <div className="flex flex-wrap items-center justify-between gap-2 mb-1">
            <h2 className="font-semibold text-gray-900">Bulk scan</h2>
            <span className="flex gap-2">
              <button
                onClick={() => scan('unlabeled', 0)}
                disabled={scanning}
                className="text-sm px-3 py-1.5 rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
              >
                {scanning && mode === 'unlabeled' ? 'Scanning…' : 'Scan unlabeled'}
              </button>
              <button
                onClick={() => scan('review', 0)}
                disabled={scanning}
                title="Audit already-labeled transactions and propose remaps where labels don't fit"
                className="text-sm px-3 py-1.5 rounded-lg border border-indigo-200 text-indigo-700 hover:bg-indigo-50 disabled:opacity-50"
              >
                {scanning && mode === 'review' ? 'Reviewing…' : 'Review labeled'}
              </button>
            </span>
          </div>
          <p className="text-xs text-gray-400">
            “Scan unlabeled” proposes labels for untagged rows; “Review labeled” audits existing labels against
            your history and suggests remaps where they don't make sense (with the reason). 75 rows per pass,
            vocabulary-only, fixed labels are never removed — and nothing is written until you apply.
          </p>

          {result && result.suggestions.length > 0 && (
            <>
              <div className="mt-3 max-h-96 overflow-y-auto divide-y divide-gray-50 border border-gray-100 rounded-xl">
                {result.suggestions.map((s) => (
                  <label key={s.id} className="flex items-start gap-2.5 px-3 py-2 text-sm hover:bg-gray-50/60 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={checked.has(s.id)}
                      onChange={() => toggle(s.id)}
                      className="mt-1"
                    />
                    <span className="flex-1 min-w-0">
                      <span className="block text-gray-700 truncate">{s.comment}</span>
                      <span className="block text-xs text-gray-400">
                        {s.date} · {formatEuro(s.amount)} · {s.category}
                      </span>
                      {s.reason && <span className="block text-[11px] text-indigo-400 mt-0.5">✦ {s.reason}</span>}
                    </span>
                    <span className="flex flex-wrap gap-1 justify-end max-w-44">
                      {(s.remove ?? []).map((l) => (
                        <span key={`rm-${l}`} className="text-[11px] font-medium bg-red-50 text-red-500 rounded px-1.5 py-0.5 line-through">−{l}</span>
                      ))}
                      {(s.add ?? []).map((l) => (
                        <span key={l} className="text-[11px] font-medium bg-emerald-50 text-emerald-600 rounded px-1.5 py-0.5">+{l}</span>
                      ))}
                    </span>
                  </label>
                ))}
              </div>
              <div className="flex flex-wrap items-center gap-2 mt-3">
                <button
                  onClick={apply}
                  disabled={applying || checked.size === 0}
                  className="px-3 py-1.5 text-sm rounded-lg bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-50"
                >
                  {applying ? 'Applying…' : applyText}
                </button>
                <button
                  onClick={() => setChecked(new Set())}
                  className="px-2 py-1 text-xs text-gray-400 hover:text-gray-700"
                >
                  Uncheck all
                </button>
                {result.remaining_unlabeled > 0 && (
                  <button
                    onClick={() => scan(mode, offset)}
                    disabled={scanning}
                    className="px-2.5 py-1 text-xs rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
                  >
                    {mode === 'unlabeled' ? 'Scan next batch' : 'Review next batch'}
                  </button>
                )}
                <span className="text-xs text-gray-400 ml-auto">
                  {result.suggestions.length} suggestion{result.suggestions.length === 1 ? '' : 's'} from {result.scanned} scanned
                  {result.remaining_unlabeled > 0 && <> · {result.remaining_unlabeled} left</>}
                </span>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
