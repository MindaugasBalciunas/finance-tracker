import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLabelRules, useDeleteRule, useApplyLabel, useReapplyRules } from '../hooks/useBudgets'
import { budgetsApi } from '../api/budgets'
import { aiApi, type RuleSuggestion } from '../api/insights'
import { useAISettings } from '../hooks/useInsights'
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

      <AIRuleReviewCard onBanner={setBanner} />

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

// AIRuleReviewCard: AI audit of the whole rule set. The backend feeds the
// model every rule with its live footprint plus recurring patterns no rule
// covers; the model proposes adds/updates/deletes, each shown with real match
// counts. Nothing is written until the user applies the checked items.
function AIRuleReviewCard({ onBanner }: { onBanner: (b: Banner) => void }) {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model
  const qc = useQueryClient()
  const [reviewing, setReviewing] = useState(false)
  const [applying, setApplying] = useState(false)
  const [result, setResult] = useState<{ suggestions: RuleSuggestion[]; rules_scanned: number } | null>(null)
  const [checked, setChecked] = useState<Set<number>>(new Set())

  if (!configured) return null

  const review = async () => {
    setReviewing(true)
    try {
      const res = await aiApi.ruleReview()
      setResult(res)
      setChecked(new Set(res.suggestions.map((_, i) => i)))
      if (res.suggestions.length === 0) {
        onBanner({ kind: 'ok', text: `Reviewed ${res.rules_scanned} rules — the AI found nothing worth changing.` })
      }
    } catch (err) {
      onBanner({ kind: 'error', text: `Review failed: ${errText(err)}` })
    } finally {
      setReviewing(false)
    }
  }

  const apply = async () => {
    if (!result) return
    const items = result.suggestions
      .filter((_, i) => checked.has(i))
      .map((s) => ({
        action: s.action,
        ...(s.rule_id ? { rule_id: s.rule_id } : {}),
        label: s.label,
        comment_match: s.comment_match ?? '',
        category: s.category ?? '',
      }))
    if (items.length === 0) return
    setApplying(true)
    try {
      const res = await aiApi.applyRuleSuggestions(items)
      const parts = [
        res.added > 0 ? `${res.added} added` : '',
        res.updated > 0 ? `${res.updated} updated` : '',
        res.deleted > 0 ? `${res.deleted} deleted` : '',
        res.relabeled > 0 ? `${res.relabeled} transactions labeled` : '',
      ].filter(Boolean).join(', ')
      onBanner({ kind: 'ok', text: `Rules applied — ${parts || 'no changes needed'}.` })
      setResult(null)
      for (const key of ['label-rules', 'label-stats', 'label-suggestions', 'labels', 'transactions']) {
        qc.invalidateQueries({ queryKey: [key] })
      }
    } catch (err) {
      onBanner({ kind: 'error', text: `Apply failed: ${errText(err)}` })
    } finally {
      setApplying(false)
    }
  }

  const toggle = (i: number) =>
    setChecked((c) => {
      const next = new Set(c)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })

  const chip = (action: RuleSuggestion['action']) =>
    action === 'add'
      ? <span className="text-[11px] font-medium bg-emerald-50 text-emerald-600 rounded px-1.5 py-0.5">+ new rule</span>
      : action === 'update'
        ? <span className="text-[11px] font-medium bg-amber-50 text-amber-600 rounded px-1.5 py-0.5">✎ update</span>
        : <span className="text-[11px] font-medium bg-red-50 text-red-500 rounded px-1.5 py-0.5">− delete</span>

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-1">
        <h2 className="font-semibold text-gray-900">✦ AI rule review</h2>
        <button
          onClick={review}
          disabled={reviewing}
          className="text-sm px-3 py-1.5 rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
        >
          {reviewing ? 'Reviewing…' : 'Review rules'}
        </button>
      </div>
      <p className="text-xs text-gray-400">
        Audits every rule against your whole history: proposes new rules for recurring merchants you label
        by hand, fixes for too-broad or misfiring patterns, and cleanup of dead rules. Every proposal shows
        its real match count — nothing changes until you apply.
      </p>

      {result && result.suggestions.length > 0 && (
        <>
          <div className="mt-3 max-h-96 overflow-y-auto divide-y divide-gray-50 border border-gray-100 rounded-xl">
            {result.suggestions.map((s, i) => (
              <label key={i} className="flex items-start gap-2.5 px-3 py-2 text-sm hover:bg-gray-50/60 cursor-pointer">
                <input type="checkbox" checked={checked.has(i)} onChange={() => toggle(i)} className="mt-1" />
                <span className="flex-1 min-w-0">
                  <span className="flex flex-wrap items-center gap-1.5">
                    {chip(s.action)}
                    <span className="font-medium text-indigo-700">{s.label}</span>
                    {s.action === 'update' && s.old_comment_match !== s.comment_match && (
                      <span className="text-gray-400 line-through">“{s.old_comment_match}”</span>
                    )}
                    {s.comment_match && <span className="text-gray-700">“{s.comment_match}”</span>}
                    {s.category && <span className="text-xs text-gray-400">· {s.category}</span>}
                    {s.action === 'update' && s.old_category && s.old_category !== s.category && (
                      <span className="text-xs text-gray-400 line-through">· {s.old_category}</span>
                    )}
                  </span>
                  <span className="block text-xs text-gray-400 mt-0.5">
                    {s.action === 'delete'
                      ? `matches ${s.matches} transactions — applied labels stay`
                      : `matches ${s.matches} transactions${s.would_label > 0 ? `, would label ${s.would_label} more` : ''}`}
                  </span>
                  {s.reason && <span className="block text-[11px] text-indigo-400 mt-0.5">✦ {s.reason}</span>}
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
              {applying ? 'Applying…' : `Apply ${checked.size} selected`}
            </button>
            <button
              onClick={() => setChecked(new Set())}
              className="px-2 py-1 text-xs text-gray-400 hover:text-gray-700"
            >
              Uncheck all
            </button>
            <span className="text-xs text-gray-400 ml-auto">
              {result.suggestions.length} suggestion{result.suggestions.length === 1 ? '' : 's'} from {result.rules_scanned} rules
            </span>
          </div>
        </>
      )}
    </div>
  )
}
