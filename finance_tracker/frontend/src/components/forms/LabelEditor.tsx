import { useMemo, useState } from 'react'
import { useLabels, useLabelRules } from '../../hooks/useBudgets'

interface Props {
  /** Comma-separated labels, the shape both the form and the API use. */
  value: string
  onChange: (labels: string) => void
  /** Floats labels that historically co-occur with this category to the front. */
  category?: string
  /**
   * Labels the saved rules WILL apply on save, shown as ⚡ chips in place.
   * Only the create form passes these — a bank row has already had the rules
   * applied server-side, so its labels are literal and removing one is just a
   * removal.
   */
  autoLabels?: string[]
  dismissedAuto?: string[]
  onDismissAuto?: (label: string) => void
  /** Re-adding a dismissed auto label by hand un-dismisses it. */
  onRestoreAuto?: (label: string) => void
  placeholder?: string
}

/**
 * The label field: current labels as removable chips, every known label as a
 * one-tap chip below, and free typing with comma/Enter/Tab/Backspace.
 *
 * Shared by the transaction form and the bank review queue so the two cannot
 * drift — a bank row is a transaction the user did not type, and there is no
 * reason for it to get a worse labeling surface than one they did.
 */
export default function LabelEditor({
  value,
  onChange,
  category,
  autoLabels = [],
  dismissedAuto = [],
  onDismissAuto,
  onRestoreAuto,
  placeholder,
}: Props) {
  const { data: allLabels = [] } = useLabels()
  const { data: labelRules = [] } = useLabelRules()
  // Labels that historically co-occur with the chosen category float to the
  // front of the chip list (Kids → education/entertainment/food first).
  const { data: categoryLabels = [] } = useLabels(category || undefined)

  const [draft, setDraft] = useState('')
  const [showAll, setShowAll] = useState(false)

  const current = value
    .split(',')
    .map((l) => l.trim().toLowerCase())
    .filter(Boolean)

  // All known labels: ones used on transactions plus ones defined by rules
  // (a fresh rule's label may not exist on any transaction yet).
  const known = useMemo(
    () => [...new Set([...allLabels, ...labelRules.map((r) => r.label)])].sort(),
    [allLabels, labelRules]
  )

  // Typing in the input filters the chips — that's the autocomplete.
  // Stable sort: category co-occurring labels first, alphabetical within.
  // Dismissed auto labels reappear here so they can be re-added manually.
  const categorySet = useMemo(() => new Set(categoryLabels), [categoryLabels])
  const chips = known
    .filter((l) => !current.includes(l) && (!autoLabels.includes(l) || dismissedAuto.includes(l)))
    .filter((l) => !draft.trim() || l.includes(draft.trim().toLowerCase()))
    .sort((a, b) => Number(categorySet.has(b)) - Number(categorySet.has(a)))

  function add(label: string) {
    const l = label.trim().toLowerCase()
    if (!l || current.includes(l)) return
    onRestoreAuto?.(l)
    onChange([...current, l].join(','))
  }

  function remove(label: string) {
    onChange(current.filter((x) => x !== label).join(','))
  }

  function commitDraft() {
    if (draft.trim()) add(draft)
    setDraft('')
  }

  const pendingAuto = autoLabels.filter((l) => !current.includes(l) && !dismissedAuto.includes(l))

  return (
    <>
      <div className="flex flex-wrap items-center gap-1.5 border border-gray-300 rounded-lg px-2 py-1.5 focus-within:ring-2 focus-within:ring-blue-500">
        {current.map((l) => (
          <span
            key={l}
            className="inline-flex items-center gap-1 text-xs font-medium bg-indigo-50 text-indigo-600 rounded-md px-2 py-1"
          >
            {l}
            <button
              type="button"
              onClick={() => remove(l)}
              className="text-indigo-400 hover:text-indigo-700 text-sm leading-none px-0.5 -mr-0.5"
              aria-label={`remove ${l}`}
            >
              ×
            </button>
          </span>
        ))}
        {/* Labels the saved rules will apply, prefilled in place. Removing
            one suppresses that rule for this transaction only. */}
        {pendingAuto.map((l) => (
          <span
            key={`auto-${l}`}
            title="Applied by your label rules on save — remove to skip it this time"
            className="inline-flex items-center gap-0.5 text-xs font-medium bg-green-50 text-green-700 border border-green-200 rounded-md px-2 py-1"
          >
            ⚡{l}
            <button
              type="button"
              onClick={() => onDismissAuto?.(l)}
              className="text-green-500 hover:text-green-800 text-sm leading-none px-0.5 -mr-0.5"
              aria-label={`skip auto label ${l}`}
            >
              ×
            </button>
          </span>
        ))}
        <input
          type="text"
          value={draft}
          onChange={(e) => {
            const v = e.target.value
            if (v.includes(',')) {
              v.split(',')
                .map((part) => part.trim().toLowerCase())
                .filter(Boolean)
                .forEach(add)
              setDraft('')
            } else {
              setDraft(v)
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commitDraft()
            } else if (e.key === 'Tab' && draft.trim() && chips.length > 0) {
              e.preventDefault()
              add(chips[0])
              setDraft('')
            } else if (e.key === 'Backspace' && draft === '' && current.length > 0) {
              remove(current[current.length - 1])
            }
          }}
          onBlur={commitDraft}
          placeholder={current.length === 0 ? (placeholder ?? 'Add label — tap a chip or type…') : ''}
          autoComplete="off"
          className="flex-1 min-w-28 text-sm focus:outline-none py-0.5"
        />
      </div>
      {chips.length > 0 && (
        <p className="mt-1.5">
          {(draft.trim() ? chips.slice(0, 20) : chips.slice(0, showAll ? chips.length : 12)).map((l) => (
            <button
              key={l}
              type="button"
              // preventDefault on mousedown keeps focus in the input, so its
              // onBlur can't commit a half-typed draft before this click.
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                add(l)
                setDraft('')
              }}
              className="inline-block text-xs font-medium bg-indigo-50 text-indigo-600 border border-indigo-100 rounded-md px-2 py-1 mr-1.5 mb-1.5 hover:bg-indigo-100 active:bg-indigo-200"
            >
              + {l}
            </button>
          ))}
          {!draft.trim() && chips.length > 12 && (
            <button
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => setShowAll((s) => !s)}
              className="inline-block text-xs text-gray-500 underline px-1 py-1 mb-1.5"
            >
              {showAll ? 'show less' : `+${chips.length - 12} more`}
            </button>
          )}
        </p>
      )}
    </>
  )
}
