import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { budgetsApi } from '../../api/budgets'
import { useApplyLabel } from '../../hooks/useBudgets'
import { deriveRulePattern } from '../../utils/rulePattern'

interface Props {
  label: string
  comment: string
  onDismiss: () => void
  // Lets the parent keep this component mounted for the confirmation —
  // rule creation makes the label "covered", which would otherwise unmount
  // the suggestion before the user sees it succeeded.
  onCreated: () => void
}

// Offered when the user hand-types a label no saved rule explains: one tap
// turns it into a rule (with an editable comment pattern) and backfills
// matching history. This is how labels teach the app instead of rules being
// hardcoded.
export default function RuleSuggestion({ label, comment, onDismiss, onCreated }: Props) {
  const [pattern, setPattern] = useState(() => deriveRulePattern(comment))
  const [created, setCreated] = useState<number | null>(null)
  const applyLabel = useApplyLabel()

  // Re-derive when the suggestion switches to a different label/comment.
  useEffect(() => {
    setPattern(deriveRulePattern(comment))
    setCreated(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [label])

  const [debounced, setDebounced] = useState(pattern)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(pattern), 350)
    return () => clearTimeout(t)
  }, [pattern])

  const { data: preview } = useQuery({
    queryKey: ['label-preview', label, debounced],
    queryFn: () => budgetsApi.previewLabel({ label, comment_match: debounced }),
    enabled: debounced.trim().length >= 2 && created === null,
    staleTime: 30_000,
  })

  if (created !== null) {
    return (
      <div className="mt-1.5 text-xs bg-green-50 border border-green-200 rounded-lg px-2.5 py-1.5 text-green-800">
        ✓ Rule saved — “{pattern}” → <span className="font-semibold">{label}</span>
        {created > 0 && <> · {created} past transaction{created === 1 ? '' : 's'} labeled</>}
      </div>
    )
  }

  return (
    <div className="mt-1.5 text-xs bg-amber-50 border border-amber-200 rounded-lg px-2.5 py-2">
      <div className="flex items-center justify-between gap-2 mb-1.5">
        <span className="text-amber-800">
          ⚡ Make <span className="font-semibold">{label}</span> automatic for comments containing:
        </span>
        <button type="button" onClick={onDismiss} className="text-amber-400 hover:text-amber-700 leading-none shrink-0">✕</button>
      </div>
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={pattern}
          onChange={(e) => setPattern(e.target.value.toLowerCase())}
          className="flex-1 min-w-0 border border-amber-300 bg-white rounded px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-amber-500"
          placeholder="text to match…"
          aria-label="Rule pattern"
        />
        <button
          type="button"
          disabled={pattern.trim().length < 2 || applyLabel.isPending}
          onClick={() =>
            applyLabel.mutate(
              { label, comment_match: pattern.trim(), create_rule: true },
              { onSuccess: (r) => { setCreated(r.labeled); onCreated() } },
            )
          }
          className="shrink-0 px-2.5 py-1 rounded bg-amber-600 text-white font-medium hover:bg-amber-700 disabled:opacity-50"
        >
          {applyLabel.isPending ? 'Saving…' : 'Create rule'}
        </button>
      </div>
      {preview && (
        <p className="text-amber-700/80 mt-1">
          matches {preview.matches} past transaction{preview.matches === 1 ? '' : 's'}
          {preview.unlabeled > 0 && <> · {preview.unlabeled} would get the label now</>}
        </p>
      )}
    </div>
  )
}
