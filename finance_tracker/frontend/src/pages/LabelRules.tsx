import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useLabelRules, useDeleteRule, useApplyLabel, useReapplyRules } from '../hooks/useBudgets'
import { budgetsApi } from '../api/budgets'
import LabelsNav, { BannerAlert, errText, type Banner } from '../components/ui/LabelsNav'
import { CATEGORIES } from '../constants/categories'

// Label rules on their own page: all saved rules grouped by label, a form to
// add one (with live match preview), and the re-apply-everything action.
export default function LabelRules() {
  const { data: rules = [], isLoading } = useLabelRules()
  const deleteRule = useDeleteRule()
  const applyLabel = useApplyLabel()
  const reapply = useReapplyRules()
  const [banner, setBanner] = useState<Banner>(null)
  const [confirmId, setConfirmId] = useState<number | null>(null)
  const [form, setForm] = useState({ label: '', category: '', comment_match: '' })

  const grouped = useMemo(() => {
    const g = new Map<string, typeof rules>()
    for (const r of rules) {
      const list = g.get(r.label) ?? []
      list.push(r)
      g.set(r.label, list)
    }
    return [...g.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [rules])

  const formReady = form.label.trim() !== '' && (form.category !== '' || form.comment_match.trim() !== '')
  const { data: preview } = useQuery({
    queryKey: ['label-preview', form.label, form.category, form.comment_match],
    queryFn: () => budgetsApi.previewLabel({
      label: form.label.trim().toLowerCase(),
      ...(form.category ? { category: form.category } : {}),
      ...(form.comment_match.trim() ? { comment_match: form.comment_match.trim() } : {}),
    }),
    enabled: formReady,
  })

  const submit = async () => {
    try {
      const res = await applyLabel.mutateAsync({
        label: form.label.trim().toLowerCase(),
        ...(form.category ? { category: form.category } : {}),
        ...(form.comment_match.trim() ? { comment_match: form.comment_match.trim() } : {}),
        create_rule: true,
      })
      setBanner({ kind: 'ok', text: `Rule saved — ${res.labeled} existing transactions labeled “${form.label.trim().toLowerCase()}”.` })
      setForm({ label: '', category: '', comment_match: '' })
    } catch (err) {
      setBanner({ kind: 'error', text: `Rule failed: ${errText(err)}` })
    }
  }

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-5xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-bold text-gray-900">⚡ Label rules</h1>
          <p className="text-xs text-gray-400">
            {rules.length} rules — new transactions matching a pattern get the label automatically
          </p>
        </div>
        <LabelsNav />
      </div>

      <BannerAlert banner={banner} onClose={() => setBanner(null)} />

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
          <p className="text-xs text-gray-400">
            Add a rule below — it labels matching history immediately and every future import automatically.
          </p>
          <button
            onClick={async () => {
              try {
                const res = await reapply.mutateAsync()
                setBanner({ kind: 'ok', text: `Re-applied ${res.rules} rules across the whole database — ${res.relabeled} transactions labeled.` })
              } catch (err) {
                setBanner({ kind: 'error', text: `Re-apply failed: ${errText(err)}` })
              }
            }}
            disabled={reapply.isPending}
            className="text-sm px-3 py-1.5 rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {reapply.isPending ? 'Re-applying…' : '↻ Re-apply all rules'}
          </button>
        </div>

        <div className="flex flex-wrap items-end gap-2 pb-4 mb-4 border-b border-gray-100">
          <label className="text-xs text-gray-500">
            Label
            <input
              value={form.label}
              onChange={(e) => setForm({ ...form, label: e.target.value.toLowerCase() })}
              placeholder="groceries"
              className="block mt-1 w-32 text-sm border border-gray-200 rounded-lg px-2.5 py-1.5"
            />
          </label>
          <label className="text-xs text-gray-500">
            Comment contains
            <input
              value={form.comment_match}
              onChange={(e) => setForm({ ...form, comment_match: e.target.value })}
              placeholder="lidl (use ^ to anchor)"
              className="block mt-1 w-44 text-sm border border-gray-200 rounded-lg px-2.5 py-1.5"
            />
          </label>
          <label className="text-xs text-gray-500">
            Category
            <select
              value={form.category}
              onChange={(e) => setForm({ ...form, category: e.target.value })}
              className="block mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
            >
              <option value="">Any</option>
              {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
          <button
            onClick={submit}
            disabled={!formReady || applyLabel.isPending}
            className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {applyLabel.isPending ? 'Saving…' : 'Add rule + label history'}
          </button>
          {formReady && preview && (
            <span className="text-xs text-gray-400 pb-1.5">
              matches {preview.matches} transactions, {preview.unlabeled} still unlabeled
            </span>
          )}
        </div>

        {isLoading && <p className="text-sm text-gray-400">Loading…</p>}
        <div className="space-y-3">
          {grouped.map(([label, list]) => (
            <div key={label}>
              <p className="text-xs font-semibold text-indigo-600 mb-1">{label}</p>
              <div className="flex flex-wrap gap-1.5">
                {list.map((r) => (
                  <span
                    key={r.id}
                    className="inline-flex items-center gap-1.5 text-xs bg-gray-50 border border-gray-200 rounded-full px-2.5 py-1"
                  >
                    <span className="text-gray-700">
                      {r.comment_match ? <>“{r.comment_match}”</> : null}
                      {r.comment_match && r.category ? ' · ' : null}
                      {r.category ? <span className="text-gray-400">{r.category}</span> : null}
                    </span>
                    {confirmId === r.id ? (
                      <button
                        onClick={() => { deleteRule.mutate(r.id); setConfirmId(null) }}
                        className="text-red-600 font-semibold hover:text-red-800"
                      >
                        delete?
                      </button>
                    ) : (
                      <button
                        onClick={() => setConfirmId(r.id)}
                        className="text-gray-300 hover:text-red-500 leading-none"
                        aria-label={`Delete rule ${r.comment_match || r.category} for ${label}`}
                      >
                        ×
                      </button>
                    )}
                  </span>
                ))}
              </div>
            </div>
          ))}
          {!isLoading && rules.length === 0 && (
            <p className="text-sm text-gray-400">No rules yet — add one above, or from any transaction form.</p>
          )}
        </div>
        <p className="text-[11px] text-gray-400 pt-3">
          Deleting a rule stops future auto-labeling; already-applied labels stay on transactions.
        </p>
      </div>
    </div>
  )
}
