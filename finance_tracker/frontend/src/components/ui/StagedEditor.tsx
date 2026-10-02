import { useEffect, useState } from 'react'
import type { StagedEdit, StagedTx } from '../../api/banking'
import { ACCOUNT_LABELS, type AccountKey, type TransactionType } from '../../types'
import { CATEGORIES_BY_TYPE, CATEGORY_HINTS } from '../../constants/categories'
import { useUpdateStaged } from '../../hooks/useBanking'
import { useLabelRules } from '../../hooks/useBudgets'
import { useAIAvailable } from '../../hooks/useInsights'
import { ruleCoversComment } from '../../utils/rulePattern'
import DateInput from './DateInput'
import CategorySuggestion from '../forms/CategorySuggestion'
import CommentInput from '../forms/CommentInput'
import LabelEditor from '../forms/LabelEditor'
import RuleSuggestion from '../forms/RuleSuggestion'
import { AIAssistButton, AIAssistResult, mergeLabels, useAIAssist } from '../forms/AIAssist'

function accountLabel(key: string): string {
  return ACCOUNT_LABELS[key as AccountKey] ?? key
}

/**
 * Correcting one bank row before it becomes a transaction.
 *
 * A bank row is a transaction the user did not type, and for a long time the
 * only thing on offer here was four bare dropdowns and a comma-separated text
 * box — while the create form had history-based category suggestions, label
 * chips, auto-labeling rules, AI assist and rule creation. This is the same
 * set of fields, built from the same components, so a row that arrives badly
 * classified can be fixed as well as one typed by hand.
 *
 * Every change saves immediately: there is no Save button because there is no
 * draft — the staged row IS the draft, and the ledger is still untouched.
 */
export default function StagedEditor({
  row,
  validAccountKeys,
}: {
  row: StagedTx
  validAccountKeys: string[]
}) {
  const update = useUpdateStaged()
  const { data: labelRules = [] } = useLabelRules()
  const aiConfigured = useAIAvailable()
  const assist = useAIAssist()

  const edit = (patch: StagedEdit) => update.mutate({ id: row.id, edit: patch })

  // The comment is the one field typed character by character, so it is held
  // locally and saved on commit. Everything else saves on change.
  const [comment, setComment] = useState(row.comment)
  useEffect(() => setComment(row.comment), [row.comment])

  const saveComment = (v: string) => {
    if (v !== row.comment) edit({ comment: v })
  }

  const currentLabels = row.labels
    .split(',')
    .map((l) => l.trim().toLowerCase())
    .filter(Boolean)

  // The bank narrative behind this row: what the classifier read, and the
  // only place a merchant it failed to recognise is still named. Given to the
  // model as context so it can propose a description the comment lost.
  const bankContext = [row.raw_payee, row.raw_details].filter(Boolean).join(' · ')

  const runAssist = async () => {
    const labels = await assist.run(
      {
        date: row.date.slice(0, 10),
        type: row.type,
        category: row.category,
        amount: row.amount,
        comment,
        labels: row.labels,
        context: bankContext,
      },
      comment,
    )
    if (labels.length) edit({ labels: mergeLabels(row.labels, labels) })
  }

  // A hand-applied label that no saved rule explains is a rule waiting to be
  // born — the same offer the create form makes. Teaching a rule here pays off
  // on every future sync, which is where it matters most.
  const [dismissedSuggestions, setDismissedSuggestions] = useState<string[]>([])
  const [createdSuggestion, setCreatedSuggestion] = useState<string | null>(null)
  const ruleCandidate =
    comment.trim().length >= 3
      ? currentLabels.find(
          (l) => !dismissedSuggestions.includes(l) && !ruleCoversComment(labelRules, l, comment, row.category),
        )
      : undefined
  const suggestionLabel =
    ruleCandidate ?? (createdSuggestion && currentLabels.includes(createdSuggestion) ? createdSuggestion : undefined)

  return (
    <div className="space-y-2.5">
      {row.enrich_note && (
        <p className="text-xs text-gray-500 bg-gray-50 border border-gray-100 rounded-lg px-2.5 py-1.5">
          ✨ {row.enrich_note}
        </p>
      )}

      <div className="grid grid-cols-2 gap-2">
        <label className="block text-xs text-gray-500">
          Date
          <div className="mt-1">
            <DateInput value={row.date.slice(0, 10)} onChange={(v) => v && edit({ date: v })} />
          </div>
        </label>
        <label className="block text-xs text-gray-500">
          Type
          <select
            value={row.type}
            onChange={(e) => edit({ type: e.target.value as TransactionType })}
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
          >
            <option value="expense">Expense</option>
            <option value="income">Income</option>
            <option value="investment">Investment</option>
          </select>
        </label>
      </div>

      <label className="block text-xs text-gray-500">
        Description
        <div className="mt-1">
          <CommentInput
            value={comment}
            onChange={setComment}
            onCommit={saveComment}
            placeholder="What was this?"
            className="w-full text-sm border border-gray-200 rounded-lg px-2 py-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
      </label>

      <label className="block text-xs text-gray-500">
        Category
        <select
          value={row.category}
          onChange={(e) => edit({ category: e.target.value })}
          className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
        >
          <option value="">Uncategorised</option>
          {CATEGORIES_BY_TYPE[row.type].map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </label>
      {CATEGORY_HINTS[row.category] && (
        <p className="text-xs text-gray-400 -mt-1">{CATEGORY_HINTS[row.category]}</p>
      )}

      <CategorySuggestion
        type={row.type}
        comment={comment}
        amount={row.amount}
        current={row.category}
        onPick={(c) => edit({ category: c })}
      />

      <div>
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs text-gray-500">Labels</span>
          {aiConfigured && (
            <AIAssistButton
              onClick={runAssist}
              busy={assist.busy}
              disabled={comment.trim().length < 3 && bankContext.trim().length < 3}
              title="Suggest labels and a cleaner description from your history and the raw bank narrative"
            />
          )}
        </div>
        <LabelEditor
          value={row.labels}
          onChange={(v) => edit({ labels: v })}
          category={row.category || undefined}
        />
        <AIAssistResult
          error={assist.error}
          note={assist.note}
          comment={assist.comment}
          onUseComment={(c) => {
            setComment(c)
            saveComment(c)
            assist.setComment('')
          }}
        />
        {suggestionLabel && (
          <RuleSuggestion
            key={suggestionLabel}
            label={suggestionLabel}
            comment={comment}
            onDismiss={() => setDismissedSuggestions((d) => [...d, suggestionLabel])}
            onCreated={() => setCreatedSuggestion(suggestionLabel)}
          />
        )}
      </div>

      <div className="grid grid-cols-2 gap-2">
        <label className="block text-xs text-gray-500">
          From (debit)
          <select
            value={row.debit_account}
            onChange={(e) => edit({ debit_account: e.target.value })}
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
          >
            <option value="">—</option>
            {validAccountKeys.map((k) => (
              <option key={k} value={k}>
                {accountLabel(k)}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-xs text-gray-500">
          To (credit)
          <select
            value={row.credit_account}
            onChange={(e) => edit({ credit_account: e.target.value })}
            className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5 bg-white"
          >
            <option value="">—</option>
            {validAccountKeys.map((k) => (
              <option key={k} value={k}>
                {accountLabel(k)}
              </option>
            ))}
          </select>
        </label>
      </div>

      {update.isError && (
        <p className="text-xs text-red-600">
          {update.error instanceof Error ? update.error.message : 'Could not save that edit'}
        </p>
      )}
    </div>
  )
}
