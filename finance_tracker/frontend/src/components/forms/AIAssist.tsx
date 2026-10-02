import { useState } from 'react'
import { aiApi } from '../../api/insights'

/**
 * The ✦ AI suggest surface: labels merged straight into the field, a cleaner
 * description offered beside it (never applied silently).
 *
 * Split into a hook plus two presentational pieces because the button and its
 * result live in different places in the two layouts — the form puts the
 * button in the Labels header, the bank review card puts it under the
 * description. Sharing the logic is the point; sharing the layout is not.
 */

export interface AssistInput {
  date?: string
  type?: string
  category?: string
  amount?: number
  comment?: string
  labels?: string
  /**
   * Source material the description came from but no longer contains — for a
   * bank row, the raw payee and remittance narrative. It is what lets the
   * model name a merchant the classifier flattened away.
   */
  context?: string
}

export function useAIAssist() {
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const [comment, setComment] = useState('')
  const [error, setError] = useState('')

  const reset = () => {
    setError('')
    setNote('')
    setComment('')
  }

  /**
   * Returns the labels to merge, so the caller decides how to combine them
   * with what is already there.
   */
  const run = async (input: AssistInput, currentComment = ''): Promise<string[]> => {
    setBusy(true)
    reset()
    try {
      const res = await aiApi.assistTransaction(input)
      if (res.comment && res.comment !== currentComment.trim()) setComment(res.comment)
      if (res.note) setNote(res.note)
      return res.labels ?? []
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setError(e.response?.data?.error ?? e.message ?? 'Suggestion failed')
      return []
    } finally {
      setBusy(false)
    }
  }

  return { run, busy, note, comment, error, setNote, setComment, setError, reset }
}

/** Merges suggested labels into a comma string without reordering or duplicating. */
export function mergeLabels(existing: string, suggested: string[]): string {
  const current = existing
    .split(',')
    .map((l) => l.trim())
    .filter(Boolean)
  return [...current, ...suggested.filter((l) => !current.includes(l))].join(',')
}

export function AIAssistButton({
  onClick,
  busy,
  disabled,
  title = 'Suggest labels and a cleaner description from your history',
}: {
  onClick: () => void
  busy: boolean
  disabled?: boolean
  title?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy || disabled}
      title={title}
      className="inline-flex items-center gap-1.5 text-sm font-medium border border-indigo-200 text-indigo-600 bg-indigo-50 rounded-lg px-3 py-1.5 hover:bg-indigo-100 disabled:opacity-40"
    >
      <span>✦</span>
      {busy ? 'Thinking…' : 'AI suggest'}
    </button>
  )
}

export function AIAssistResult({
  error,
  note,
  comment,
  onUseComment,
}: {
  error: string
  note: string
  comment: string
  onUseComment: (comment: string) => void
}) {
  if (!error && !note && !comment) return null
  return (
    <div className="text-xs rounded-lg border border-indigo-100 bg-indigo-50/60 px-2.5 py-2 space-y-1 mt-1.5">
      {error && <p className="text-red-600">{error}</p>}
      {comment && (
        <p className="text-gray-700">
          ✦ Clearer description: “{comment}”{' '}
          <button
            type="button"
            onClick={() => onUseComment(comment)}
            className="font-medium text-indigo-600 hover:underline"
          >
            Use it
          </button>
        </p>
      )}
      {note && <p className="text-gray-400">{note}</p>}
    </div>
  )
}
