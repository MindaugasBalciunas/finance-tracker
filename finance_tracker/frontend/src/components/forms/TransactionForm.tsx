import { useEffect, useId, useState, useRef } from 'react'
import { useForm, useWatch, Controller } from 'react-hook-form'
import type { CreateTransactionInput, TransactionType, AccountKey } from '../../types'
import { ACCOUNT_LABELS } from '../../types'
import { useAddedAccounts } from '../../hooks/useAccounts'
import { CATEGORIES_BY_TYPE, CATEGORY_HINTS } from '../../constants/categories'
import DateInput from '../ui/DateInput'
import { useLabelRules } from '../../hooks/useBudgets'
import { ruleCoversComment, commentPatternMatches } from '../../utils/rulePattern'
import { aiApi } from '../../api/insights'
import { useAIAvailable } from '../../hooks/useInsights'
import RuleSuggestion from './RuleSuggestion'
import LabelEditor from './LabelEditor'
import CategorySuggestion from './CategorySuggestion'
import LabelSuggestion from './LabelSuggestion'
import CommentInput from './CommentInput'
import { AIAssistButton, AIAssistResult, mergeLabels, useAIAssist } from './AIAssist'

interface Props {
  onSubmit: (data: CreateTransactionInput) => void
  onCancel: () => void
  isSubmitting?: boolean
  defaultValues?: Partial<CreateTransactionInput>
  /** Primary button text. "Save" unless the caller is doing something else. */
  submitLabel?: string
  /**
   * Source material behind the description that it no longer contains — for a
   * bank row, the raw payee and remittance narrative. Passed to AI assist so
   * it can name a merchant the importer flattened away.
   */
  aiContext?: string
  /**
   * Fold the rule-preview (⚡) labels into the submitted value.
   *
   * On the create path the server applies the rules itself, so the ⚡ chips
   * are a preview and the form sends only what the user typed. The bank
   * review path stores labels verbatim — so there, what the user was shown
   * has to be what is saved, or the preview is a promise nothing keeps.
   */
  includeAutoLabels?: boolean
  /**
   * A photo to read into the draft as soon as the form opens.
   *
   * The picker is opened by the caller, not here: a file dialog needs a real
   * user gesture, and one fired from an effect after the modal mounts is
   * blocked on iOS. So the Add menu collects the file and hands it over.
   */
  scanFile?: File | null
  /**
   * Whether the form carries its own "Scan photo" button.
   *
   * False wherever the caller already made that choice for the user: the Add
   * menu offers scanning as its own entry, so repeating it inside a form the
   * user opened by picking "Enter manually" is offering a decision they have
   * already made. Editing a saved transaction has no such menu, so there it
   * stays — it is the only way to read a receipt into an existing row.
   */
  showScan?: boolean
}

const ALL_ACCOUNTS = Object.entries(ACCOUNT_LABELS) as [AccountKey, string][]

function AccountSelect({ label, name, register }: { label: string; name: 'debit_account' | 'credit_account'; register: any }) {
  const added = useAddedAccounts()
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
        {added.filter((a) => !a.archived).map((a) => (
          <option key={a.key} value={a.key}>{a.label}</option>
        ))}
      </select>
    </div>
  )
}

export default function TransactionForm({
  onSubmit,
  onCancel,
  isSubmitting,
  defaultValues,
  submitLabel = 'Save',
  aiContext,
  includeAutoLabels,
  scanFile,
  showScan = true,
}: Props) {
  const today = new Date().toISOString().slice(0, 10)
  // Labels are tied to their fields by id: the form renders inside modals and
  // more than once per session, so the ids have to be unique per instance.
  const uid = useId()
  const { register, handleSubmit, formState: { errors }, control, setValue, watch } = useForm<CreateTransactionInput>({
    defaultValues: { type: 'expense', date: today, ...defaultValues },
  })

  const selectedType = useWatch({ control, name: 'type' })
  const categories = CATEGORIES_BY_TYPE[selectedType] ?? []

  const commentValue = watch('comment') ?? ''

  // Label suggestions: existing labels as one-tap chips, plus a live preview
  // of labels the saved rules will apply automatically on save.
  const { data: labelRules = [] } = useLabelRules()
  const labelsValue = watch('labels') ?? ''
  const selectedCategory = watch('category') ?? ''
  const currentLabels = labelsValue.split(',').map((l) => l.trim().toLowerCase()).filter(Boolean)

  const autoLabels = labelRules
    .filter((r) => {
      const catOk = !r.category || r.category.toLowerCase() === String(selectedCategory).toLowerCase()
      const commentOk = !r.comment_match || commentPatternMatches(r.comment_match, commentValue)
      return (r.category !== '' || r.comment_match !== '') && catOk && commentOk
    })
    .map((r) => r.label)
    .filter((l, i, arr) => arr.indexOf(l) === i)

  // Auto labels the user removed from the field — sent as suppressed_labels
  // on save so the matching rules are skipped for this transaction only.
  const [dismissedAuto, setDismissedAuto] = useState<string[]>([])

  // AI assist: labels merged into the field as normal removable chips, the
  // cleaner description offered beside the form (never applied silently).
  const aiConfigured = useAIAvailable()
  const assist = useAIAssist()

  // Scan a photo to prefill the whole draft. Reuses the assist note/error
  // display so results surface in the same spot beside the labels.
  const scanInputRef = useRef<HTMLInputElement>(null)
  const [scanBusy, setScanBusy] = useState(false)

  const runScan = async (file: File) => {
    setScanBusy(true)
    assist.reset()
    try {
      const res = await aiApi.scanTransaction(file)
      // Prefill only fields the scan could actually read; a blank / non-positive
      // value keeps the form's own default rather than clobbering it.
      if (res.type) setValue('type', res.type)
      if (res.amount > 0) setValue('amount', res.amount)
      if (res.comment) setValue('comment', res.comment)
      if (res.category) setValue('category', res.category as any)
      if (res.date) setValue('date', res.date)
      if (res.labels?.length) {
        const existing = (watch('labels') ?? '').split(',').map((l) => l.trim()).filter(Boolean)
        const merged = [...existing, ...res.labels.filter((l) => !existing.includes(l))]
        setValue('labels', merged.join(','))
      }
      // Account recognized from the image (e.g. a PzM sąskaita / Swedbank
      // screenshot) — prefill so the balance adjusts on save.
      if (res.debit_account) setValue('debit_account', res.debit_account)
      if (res.credit_account) setValue('credit_account', res.credit_account)
      if (res.note) assist.setNote(res.note)
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      assist.setError(e.response?.data?.error ?? e.message ?? 'Scan failed')
    } finally {
      setScanBusy(false)
    }
  }

  // A photo handed in by the Add menu is read once, on open.
  const scannedRef = useRef<File | null>(null)
  useEffect(() => {
    if (!scanFile || scannedRef.current === scanFile) return
    scannedRef.current = scanFile
    void runScan(scanFile)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scanFile])

  const runAssist = async () => {
    const labels = await assist.run(
      {
        date: watch('date'), type: watch('type'), category: String(watch('category') ?? ''),
        amount: Number(watch('amount')) || 0, comment: watch('comment') ?? '', labels: watch('labels') ?? '',
        context: aiContext,
      },
      watch('comment') ?? '',
    )
    if (labels.length) setValue('labels', mergeLabels(watch('labels') ?? '', labels))
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

  // CategorySuggestion does its own debounced lookup from these.
  const amountValue = watch('amount')

  useEffect(() => {
    if (!defaultValues?.category) {
      setValue('category', '' as any)
    }
  }, [selectedType, defaultValues?.category, setValue])

  return (
    <form
      onSubmit={handleSubmit((data) => {
        const pendingAuto = autoLabels.filter(
          (l) => !currentLabels.includes(l) && !dismissedAuto.includes(l)
        )
        const out: CreateTransactionInput = includeAutoLabels
          ? { ...data, labels: mergeLabels(data.labels ?? '', pendingAuto) }
          : { ...data }
        if (dismissedAuto.length) out.suppressed_labels = dismissedAuto.join(',')
        onSubmit(out)
      })}
      className="space-y-4"
    >
      {aiConfigured && showScan && (
        <div className="flex justify-end -mb-1">
          {/* capture=environment hints the phone camera; accept keeps the
              desktop file picker working. */}
          <input
            ref={scanInputRef}
            type="file"
            accept="image/*"
            capture="environment"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) runScan(file)
              // Clear so picking the same file again re-triggers onChange.
              e.target.value = ''
            }}
          />
          <button
            type="button"
            onClick={() => scanInputRef.current?.click()}
            disabled={scanBusy}
            title="Scan a receipt or invoice photo to prefill this transaction"
            className="inline-flex items-center gap-1.5 text-sm font-medium border border-indigo-200 text-indigo-600 bg-indigo-50 rounded-lg px-3 py-1.5 hover:bg-indigo-100 disabled:opacity-40"
          >
            <span>📷</span>
            {scanBusy ? 'Scanning…' : 'Scan photo'}
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label htmlFor={`${uid}-date`} className="block text-sm font-medium text-gray-700 mb-1">Date</label>
          <Controller
            name="date"
            control={control}
            rules={{ required: 'Date is required' }}
            render={({ field }) => (
              <DateInput id={`${uid}-date`} value={field.value ?? ''} onChange={field.onChange} />
            )}
          />
          {errors.date && <p className="text-xs text-red-600 mt-1">{errors.date.message}</p>}
        </div>

        <div>
          <label htmlFor={`${uid}-type`} className="block text-sm font-medium text-gray-700 mb-1">Type</label>
          <select
            id={`${uid}-type`}
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
          <label htmlFor={`${uid}-amount`} className="block text-sm font-medium text-gray-700 mb-1">Amount (€)</label>
          <input
            id={`${uid}-amount`}
            type="number"
            step="0.01"
            min="0.01"
            {...register('amount', { required: 'Amount is required', valueAsNumber: true, min: { value: 0.01, message: 'Must be > 0' } })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {errors.amount && <p className="text-xs text-red-600 mt-1">{errors.amount.message}</p>}
        </div>

        <div>
          <label htmlFor={`${uid}-category`} className="block text-sm font-medium text-gray-700 mb-1">Category</label>
          <select
            id={`${uid}-category`}
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

      <div className="-mt-1">
        <CategorySuggestion
          type={selectedType}
          comment={commentValue}
          amount={Number(amountValue) || 0}
          current={String(selectedCategory)}
          onPick={(c) => setValue('category', c as any)}
        />
      </div>

      <div>
        <label htmlFor={`${uid}-comment`} className="block text-sm font-medium text-gray-700 mb-1">Comment</label>
        <CommentInput
          id={`${uid}-comment`}
          value={commentValue}
          onChange={(v) => setValue('comment', v)}
          placeholder="Optional description..."
        />
      </div>

      <div>
        <div className="flex items-center justify-between mb-1">
          <label className="block text-sm font-medium text-gray-700">Labels</label>
          {aiConfigured && (
            <AIAssistButton
              onClick={runAssist}
              busy={assist.busy}
              disabled={commentValue.trim().length < 3 && (aiContext ?? '').trim().length < 3}
            />
          )}
        </div>
        <LabelEditor
          value={labelsValue}
          onChange={(v) => setValue('labels', v)}
          category={selectedCategory ? String(selectedCategory) : undefined}
          autoLabels={autoLabels}
          dismissedAuto={dismissedAuto}
          onDismissAuto={(l) => setDismissedAuto((d) => [...d, l])}
          onRestoreAuto={(l) => setDismissedAuto((d) => d.filter((x) => x !== l))}
        />
        <LabelSuggestion
          type={selectedType}
          comment={commentValue}
          current={labelsValue}
          onApply={(labels) => setValue('labels', mergeLabels(labelsValue, labels))}
        />
        <AIAssistResult
          error={assist.error}
          note={assist.note}
          comment={assist.comment}
          onUseComment={(c) => { setValue('comment', c); assist.setComment('') }}
        />
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
          {isSubmitting ? 'Saving...' : submitLabel}
        </button>
      </div>
    </form>
  )
}
