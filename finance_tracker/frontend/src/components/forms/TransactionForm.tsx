import { useEffect, useMemo, useState, useRef } from 'react'
import { useForm, useWatch, Controller } from 'react-hook-form'
import type { CreateTransactionInput, TransactionType, AccountKey } from '../../types'
import { ACCOUNT_LABELS } from '../../types'
import { CATEGORIES_BY_TYPE, CATEGORY_HINTS } from '../../constants/categories'
import DateInput from '../ui/DateInput'
import { useQuery } from '@tanstack/react-query'
import { transactionsApi } from '../../api/transactions'
import { useTransactionComments } from '../../hooks/useTransactions'
import { useLabels, useLabelRules } from '../../hooks/useBudgets'
import { ruleCoversComment, commentPatternMatches } from '../../utils/rulePattern'
import { aiApi } from '../../api/insights'
import { useAISettings } from '../../hooks/useInsights'
import RuleSuggestion from './RuleSuggestion'

interface Props {
  onSubmit: (data: CreateTransactionInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateTransactionInput>
}

const ALL_ACCOUNTS = Object.entries(ACCOUNT_LABELS) as [AccountKey, string][]

function AccountSelect({ label, name, register }: { label: string; name: 'debit_account' | 'credit_account'; register: any }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 mb-1">
        {label} <span className="text-gray-400 font-normal">(optional)</span>
      </label>
      <select
        {...register(name)}
        className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        <option value="">None</option>
        {ALL_ACCOUNTS.map(([key, label]) => (
          <option key={key} value={key}>{label}</option>
        ))}
      </select>
    </div>
  )
}

export default function TransactionForm({ onSubmit, onCancel, isSubmitting, defaultValues }: Props) {
  const today = new Date().toISOString().slice(0, 10)
  const { register, handleSubmit, formState: { errors }, control, setValue, watch } = useForm<CreateTransactionInput>({
    defaultValues: { type: 'expense', date: today, ...defaultValues },
  })

  const selectedType = useWatch({ control, name: 'type' })
  const categories = CATEGORIES_BY_TYPE[selectedType] ?? []

  const { data: allComments = [] } = useTransactionComments()
  const commentValue = watch('comment') ?? ''
  const [showSuggestions, setShowSuggestions] = useState(false)
  const commentRef = useRef<HTMLDivElement>(null)

  const suggestions = commentValue.length > 0
    ? allComments.filter(c => c.toLowerCase().includes(commentValue.toLowerCase()) && c !== commentValue).slice(0, 8)
    : []

  // Label suggestions: existing labels as one-tap chips, plus a live preview
  // of labels the saved rules will apply automatically on save.
  const { data: allLabels = [] } = useLabels()
  const { data: labelRules = [] } = useLabelRules()
  const labelsValue = watch('labels') ?? ''
  const selectedCategory = watch('category') ?? ''
  // Labels that historically co-occur with the chosen category float to the
  // front of the chip list (Kids → education/entertainment/food first).
  const { data: categoryLabels = [] } = useLabels(selectedCategory ? String(selectedCategory) : undefined)
  const currentLabels = labelsValue.split(',').map((l) => l.trim().toLowerCase()).filter(Boolean)

  const autoLabels = labelRules
    .filter((r) => {
      const catOk = !r.category || r.category.toLowerCase() === String(selectedCategory).toLowerCase()
      const commentOk = !r.comment_match || commentPatternMatches(r.comment_match, commentValue)
      return (r.category !== '' || r.comment_match !== '') && catOk && commentOk
    })
    .map((r) => r.label)
    .filter((l, i, arr) => arr.indexOf(l) === i)

  const [labelDraft, setLabelDraft] = useState('')
  const [showAllChips, setShowAllChips] = useState(false)
  // Auto labels the user removed from the field — sent as suppressed_labels
  // on save so the matching rules are skipped for this transaction only.
  const [dismissedAuto, setDismissedAuto] = useState<string[]>([])

  // AI assist: labels merged into the field as normal removable chips, the
  // cleaner description offered beside the form (never applied silently).
  const { data: aiSettings } = useAISettings()
  const aiConfigured = !!aiSettings?.has_key && !!aiSettings?.model
  const [assistBusy, setAssistBusy] = useState(false)
  const [assistNote, setAssistNote] = useState('')
  const [assistComment, setAssistComment] = useState('')
  const [assistError, setAssistError] = useState('')

  const runAssist = async () => {
    setAssistBusy(true)
    setAssistError('')
    setAssistNote('')
    setAssistComment('')
    try {
      const res = await aiApi.assistTransaction({
        date: watch('date'), type: watch('type'), category: String(watch('category') ?? ''),
        amount: Number(watch('amount')) || 0, comment: watch('comment') ?? '', labels: watch('labels') ?? '',
      })
      const existing = (watch('labels') ?? '').split(',').map((l) => l.trim()).filter(Boolean)
      const merged = [...existing, ...res.labels.filter((l) => !existing.includes(l))]
      setValue('labels', merged.join(','))
      const current = (watch('comment') ?? '').trim()
      if (res.comment && res.comment !== current) setAssistComment(res.comment)
      if (res.note) setAssistNote(res.note)
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setAssistError(e.response?.data?.error ?? e.message ?? 'Suggestion failed')
    } finally {
      setAssistBusy(false)
    }
  }

  // All known labels: ones used on transactions plus ones defined by rules
  // (a fresh rule's label may not exist on any transaction yet).
  const knownLabels = useMemo(
    () => [...new Set([...allLabels, ...labelRules.map((r) => r.label)])].sort(),
    [allLabels, labelRules]
  )

  // Typing in the labels input filters the chips — that's the autocomplete.
  // Stable sort: category co-occurring labels first, alphabetical within.
  // Dismissed auto labels reappear here so they can be re-added manually.
  const categorySet = useMemo(() => new Set(categoryLabels), [categoryLabels])
  const labelChips = knownLabels
    .filter((l) => !currentLabels.includes(l) && (!autoLabels.includes(l) || dismissedAuto.includes(l)))
    .filter((l) => !labelDraft.trim() || l.includes(labelDraft.trim().toLowerCase()))
    .sort((a, b) => Number(categorySet.has(b)) - Number(categorySet.has(a)))

  function addLabelChip(label: string) {
    const l = label.trim().toLowerCase()
    if (!l || currentLabels.includes(l)) return
    setDismissedAuto((d) => d.filter((x) => x !== l))
    setValue('labels', [...currentLabels, l].join(','))
  }

  function removeLabel(label: string) {
    setValue('labels', currentLabels.filter((x) => x !== label).join(','))
  }

  function commitLabelDraft() {
    if (labelDraft.trim()) addLabelChip(labelDraft)
    setLabelDraft('')
  }

  // A hand-applied label that no saved rule explains is a rule waiting to be
  // born — offer to create one (labels teach the app, nothing is hardcoded).
  const [dismissedSuggestions, setDismissedSuggestions] = useState<string[]>([])
  const [createdSuggestion, setCreatedSuggestion] = useState<string | null>(null)
  const ruleCandidate =
    commentValue.trim().length >= 3
      ? currentLabels.find(
          (l) =>
            !dismissedSuggestions.includes(l) &&
            !ruleCoversComment(labelRules, l, commentValue, String(selectedCategory)),
        )
      : undefined
  // Creating a rule makes the label covered, so keep the suggestion mounted
  // to show its confirmation.
  const suggestionLabel = ruleCandidate ?? (createdSuggestion && currentLabels.includes(createdSuggestion) ? createdSuggestion : undefined)

  // Category suggestion from similar historical transactions (debounced).
  const amountValue = watch('amount')
  const [debounced, setDebounced] = useState({ comment: '', amount: 0 })
  useEffect(() => {
    const t = setTimeout(() => setDebounced({ comment: commentValue, amount: Number(amountValue) || 0 }), 400)
    return () => clearTimeout(t)
  }, [commentValue, amountValue])

  const { data: suggestion } = useQuery({
    queryKey: ['suggest-category', selectedType, debounced.comment, debounced.amount],
    queryFn: () => transactionsApi.suggestCategory({
      type: selectedType,
      comment: debounced.comment,
      amount: debounced.amount,
    }),
    enabled: debounced.comment.trim().length >= 3 || debounced.amount > 0,
    staleTime: 30_000,
  })

  useEffect(() => {
    if (!defaultValues?.category) {
      setValue('category', '' as any)
    }
  }, [selectedType, defaultValues?.category, setValue])

  return (
    <form
      onSubmit={handleSubmit((data) =>
        onSubmit(dismissedAuto.length ? { ...data, suppressed_labels: dismissedAuto.join(',') } : data)
      )}
      className="space-y-4"
    >
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Date</label>
          <Controller
            name="date"
            control={control}
            rules={{ required: 'Date is required' }}
            render={({ field }) => <DateInput value={field.value ?? ''} onChange={field.onChange} />}
          />
          {errors.date && <p className="text-xs text-red-600 mt-1">{errors.date.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
          <select
            {...register('type', { required: 'Type is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {(['expense', 'income', 'investment'] as TransactionType[]).map((t) => (
              <option key={t} value={t}>{t.charAt(0).toUpperCase() + t.slice(1)}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Amount (€)</label>
          <input
            type="number"
            step="0.01"
            min="0.01"
            {...register('amount', { required: 'Amount is required', valueAsNumber: true, min: { value: 0.01, message: 'Must be > 0' } })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.amount && <p className="text-xs text-red-600 mt-1">{errors.amount.message}</p>}
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Category</label>
          <select
            {...register('category', { required: 'Category is required' })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">Select a category...</option>
            {categories.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          {errors.category && <p className="text-xs text-red-600 mt-1">{errors.category.message}</p>}
          {selectedCategory && CATEGORY_HINTS[String(selectedCategory)] && (
            <p className="text-xs text-gray-400 mt-1">{CATEGORY_HINTS[String(selectedCategory)]}</p>
          )}
        </div>
      </div>

      {suggestion && suggestion.category && suggestion.category !== String(selectedCategory) && (
        <p className="text-xs text-gray-600 bg-blue-50 border border-blue-100 rounded-lg px-3 py-2 -mt-1">
          💡 {suggestion.matches} similar transaction{suggestion.matches === 1 ? '' : 's'}{' '}
          ({suggestion.basis === 'amount' ? 'same amount' : 'matching comment'}) are usually{' '}
          <span className="font-semibold text-gray-800">{suggestion.category}</span>
          <button
            type="button"
            onClick={() => setValue('category', suggestion.category as any)}
            className="ml-2 text-blue-600 hover:text-blue-800 font-medium"
          >
            Use it
          </button>
        </p>
      )}

      <div className="relative" ref={commentRef}>
        <label className="block text-sm font-medium text-gray-700 mb-1">Comment</label>
        <input
          type="text"
          {...register('comment')}
          placeholder="Optional description..."
          autoComplete="off"
          onFocus={() => setShowSuggestions(true)}
          onBlur={() => setTimeout(() => setShowSuggestions(false), 150)}
          className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        {showSuggestions && suggestions.length > 0 && (
          <ul className="absolute z-50 left-0 right-0 mt-1 bg-white border border-gray-200 rounded-lg shadow-lg overflow-hidden">
            {suggestions.map((s) => (
              <li
                key={s}
                onMouseDown={() => { setValue('comment', s); setShowSuggestions(false) }}
                className="px-3 py-2 text-sm text-gray-700 hover:bg-blue-50 cursor-pointer truncate"
              >
                {s}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="block text-sm font-medium text-gray-700">Labels</label>
          {aiConfigured && (
            <button
              type="button"
              onClick={runAssist}
              disabled={assistBusy || commentValue.trim().length < 3}
              title="Suggest labels and a cleaner description from your history"
              className="text-xs font-medium text-indigo-500 hover:text-indigo-700 disabled:opacity-40"
            >
              {assistBusy ? '✦ thinking…' : '✦ AI suggest'}
            </button>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-1.5 border border-gray-300 rounded-lg px-2 py-1.5 focus-within:ring-2 focus-within:ring-blue-500">
          {currentLabels.map((l) => (
            <span key={l} className="inline-flex items-center gap-1 text-xs font-medium bg-indigo-50 text-indigo-600 rounded-md px-2 py-1">
              {l}
              <button type="button" onClick={() => removeLabel(l)} className="text-indigo-400 hover:text-indigo-700 text-sm leading-none px-0.5 -mr-0.5" aria-label={`remove ${l}`}>×</button>
            </span>
          ))}
          {/* Labels the saved rules will apply, prefilled in place. Removing
              one suppresses that rule for this transaction only. */}
          {autoLabels.filter((l) => !currentLabels.includes(l) && !dismissedAuto.includes(l)).map((l) => (
            <span
              key={`auto-${l}`}
              title="Applied by your label rules on save — remove to skip it this time"
              className="inline-flex items-center gap-0.5 text-xs font-medium bg-green-50 text-green-700 border border-green-200 rounded-md px-2 py-1"
            >
              ⚡{l}
              <button
                type="button"
                onClick={() => setDismissedAuto((d) => [...d, l])}
                className="text-green-500 hover:text-green-800 text-sm leading-none px-0.5 -mr-0.5"
                aria-label={`skip auto label ${l}`}
              >
                ×
              </button>
            </span>
          ))}
          <input
            type="text"
            value={labelDraft}
            onChange={(e) => {
              const v = e.target.value
              if (v.includes(',')) {
                v.split(',').map((part) => part.trim().toLowerCase()).filter(Boolean).forEach(addLabelChip)
                setLabelDraft('')
              } else {
                setLabelDraft(v)
              }
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                commitLabelDraft()
              } else if (e.key === 'Tab' && labelDraft.trim() && labelChips.length > 0) {
                e.preventDefault()
                addLabelChip(labelChips[0])
                setLabelDraft('')
              } else if (e.key === 'Backspace' && labelDraft === '' && currentLabels.length > 0) {
                removeLabel(currentLabels[currentLabels.length - 1])
              }
            }}
            onBlur={commitLabelDraft}
            placeholder={currentLabels.length === 0 ? 'Add label — tap a chip or type…' : ''}
            autoComplete="off"
            className="flex-1 min-w-28 text-sm focus:outline-none py-0.5"
          />
        </div>
        {labelChips.length > 0 && (
          <p className="mt-1.5">
            {(labelDraft.trim() ? labelChips.slice(0, 20) : labelChips.slice(0, showAllChips ? labelChips.length : 12)).map((l) => (
              <button
                key={l}
                type="button"
                // preventDefault on mousedown keeps focus in the input, so its
                // onBlur can't commit a half-typed draft before this click.
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => { addLabelChip(l); setLabelDraft('') }}
                className="inline-block text-xs font-medium bg-indigo-50 text-indigo-600 border border-indigo-100 rounded-md px-2 py-1 mr-1.5 mb-1.5 hover:bg-indigo-100 active:bg-indigo-200"
              >
                + {l}
              </button>
            ))}
            {!labelDraft.trim() && labelChips.length > 12 && (
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => setShowAllChips((s) => !s)}
                className="inline-block text-xs text-gray-500 underline px-1 py-1 mb-1.5"
              >
                {showAllChips ? 'show less' : `+${labelChips.length - 12} more`}
              </button>
            )}
          </p>
        )}
        {(assistComment || assistNote || assistError) && (
          <div className="text-xs rounded-lg border border-indigo-100 bg-indigo-50/60 px-2.5 py-2 space-y-1 mt-1.5">
            {assistError && <p className="text-red-600">{assistError}</p>}
            {assistComment && (
              <p className="text-gray-700">
                ✦ Clearer description: “{assistComment}”{' '}
                <button
                  type="button"
                  onClick={() => { setValue('comment', assistComment); setAssistComment('') }}
                  className="font-medium text-indigo-600 hover:underline"
                >
                  Use it
                </button>
              </p>
            )}
            {assistNote && <p className="text-gray-400">{assistNote}</p>}
          </div>
        )}
        {suggestionLabel && (
          <RuleSuggestion
            key={suggestionLabel}
            label={suggestionLabel}
            comment={commentValue}
            onDismiss={() => setDismissedSuggestions((d) => [...d, suggestionLabel])}
            onCreated={() => setCreatedSuggestion(suggestionLabel)}
          />
        )}
      </div>

      {/* Account fields — context-aware based on transaction type */}
      {selectedType === 'expense' && (
        <AccountSelect label="From Account" name="debit_account" register={register} />
      )}

      {selectedType === 'income' && (
        <AccountSelect label="To Account" name="credit_account" register={register} />
      )}

      {selectedType === 'investment' && (
        <div className="grid grid-cols-2 gap-4">
          <AccountSelect label="From Account" name="debit_account" register={register} />
          <AccountSelect label="To Account" name="credit_account" register={register} />
        </div>
      )}

      <div className="flex justify-end gap-3 pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={isSubmitting}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {isSubmitting ? 'Saving...' : 'Save'}
        </button>
      </div>
    </form>
  )
}
