import { useState } from 'react'
import { useTransactionComments } from '../../hooks/useTransactions'

interface Props {
  value: string
  onChange: (value: string) => void
  /** Called when editing finishes — the review queue saves per field. */
  onCommit?: (value: string) => void
  placeholder?: string
  className?: string
  id?: string
}

/**
 * A description field that autocompletes from descriptions already in the
 * ledger, so the same merchant keeps the same name. Shared by the transaction
 * form and the bank review queue — consistent naming is what makes the
 * comment-based category suggestion and the label rules work at all.
 */
export default function CommentInput({ value, onChange, onCommit, placeholder, className, id }: Props) {
  const { data: allComments = [] } = useTransactionComments()
  const [open, setOpen] = useState(false)

  const suggestions =
    value.length > 0
      ? allComments
          .filter((c) => c.toLowerCase().includes(value.toLowerCase()) && c !== value)
          .slice(0, 8)
      : []

  return (
    <div className="relative">
      <input
        id={id}
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        onFocus={() => setOpen(true)}
        onBlur={() => {
          // Delayed so a click on a suggestion lands before the list unmounts.
          setTimeout(() => setOpen(false), 150)
          onCommit?.(value)
        }}
        className={
          className ??
          'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'
        }
      />
      {open && suggestions.length > 0 && (
        <ul className="absolute z-50 left-0 right-0 mt-1 bg-white border border-gray-200 rounded-lg shadow-lg overflow-hidden">
          {suggestions.map((s) => (
            <li
              key={s}
              onMouseDown={() => {
                onChange(s)
                onCommit?.(s)
                setOpen(false)
              }}
              className="px-3 py-2 text-sm text-gray-700 hover:bg-blue-50 cursor-pointer truncate"
            >
              {s}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
