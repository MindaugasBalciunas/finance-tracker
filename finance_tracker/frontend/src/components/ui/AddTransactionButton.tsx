import { useEffect, useRef, useState } from 'react'

interface Props {
  onManual: () => void
  /** The picked photo. Opening the picker here keeps it inside a user gesture. */
  onScan: (file: File) => void
  /** Jump to the bank queue. Omitted when no bank is connected. */
  onBank?: () => void
  bankPending?: number
  /** Hides the photo entry when AI is off — there is nothing to read it with. */
  scanAvailable?: boolean
}

/**
 * The three ways a transaction gets into the ledger, behind one button.
 *
 * "+ Add" used to mean "type it in", while scanning a photo was a button
 * hidden inside that form and a bank import lived on another page entirely.
 * They are the same job — the only difference is who does the typing — so
 * they belong in the same menu.
 */
export default function AddTransactionButton({
  onManual,
  onScan,
  onBank,
  bankPending = 0,
  scanAvailable,
}: Props) {
  const [open, setOpen] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  // Click-away and Escape, so the menu never strands the page under itself.
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  // Only one way in? Then there is no menu to open — the button just does it.
  //
  // Decided inside one stable tree rather than by returning a bare <button>
  // early: whether a bank is connected and whether AI is on both arrive from
  // queries, so this flips from 1 to 3 a moment after the page loads. Swapping
  // the returned element type there remounted the component and silently shut
  // a menu the user had just opened.
  const soleOption = !scanAvailable && !onBank

  const item = 'w-full flex items-center gap-3 px-4 py-3.5 text-left hover:bg-gray-50 sm:px-3 sm:py-2.5'

  return (
    <div className="relative shrink-0" ref={boxRef}>
      {/* capture=environment hints the phone camera; accept keeps the desktop
          file picker working. */}
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        capture="environment"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0]
          // Clear first so picking the same file again re-triggers onChange.
          e.target.value = ''
          if (file) {
            setOpen(false)
            onScan(file)
          }
        }}
      />

      <button
        onClick={() => (soleOption ? onManual() : setOpen((o) => !o))}
        aria-haspopup={soleOption ? undefined : 'menu'}
        aria-expanded={soleOption ? undefined : open}
        className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
      >
        + Add
      </button>

      {/* A bottom sheet on a phone, a dropdown on a desktop. Anchored under
          the button at 390px it covered the card below it and read as a
          misplaced overlay rather than a menu. */}
      {open && !soleOption && (
        <div className="fixed inset-0 bg-black/30 z-40 sm:hidden" onClick={() => setOpen(false)} />
      )}
      {open && !soleOption && (
        <div
          role="menu"
          className="fixed inset-x-0 bottom-0 z-50 bg-white rounded-t-2xl border-t border-gray-200 shadow-2xl overflow-hidden pb-[env(safe-area-inset-bottom)]
                     sm:absolute sm:inset-x-auto sm:right-0 sm:bottom-auto sm:top-full sm:mt-1 sm:w-60 sm:rounded-xl sm:border sm:shadow-lg sm:pb-0"
        >
          <p className="px-4 pt-3 pb-1 text-xs font-semibold uppercase tracking-wide text-gray-400 sm:hidden">
            Add a transaction
          </p>
          <button
            role="menuitem"
            onClick={() => {
              setOpen(false)
              onManual()
            }}
            className={item}
          >
            <span className="text-lg">✏️</span>
            <span className="flex-1 min-w-0">
              <span className="block text-sm font-medium text-gray-800">Enter manually</span>
              <span className="block text-xs text-gray-400">Fill in the form yourself</span>
            </span>
          </button>

          {scanAvailable && (
            <button role="menuitem" onClick={() => fileRef.current?.click()} className={item}>
              <span className="text-lg">📷</span>
              <span className="flex-1 min-w-0">
                <span className="block text-sm font-medium text-gray-800">Scan a photo</span>
                <span className="block text-xs text-gray-400">Receipt, invoice or a screenshot</span>
              </span>
            </button>
          )}

          {onBank && (
            <button
              role="menuitem"
              onClick={() => {
                setOpen(false)
                onBank()
              }}
              className={item}
            >
              <span className="text-lg">🔗</span>
              <span className="flex-1 min-w-0">
                <span className="block text-sm font-medium text-gray-800">From your bank</span>
                <span className="block text-xs text-gray-400">
                  {bankPending > 0 ? `${bankPending} waiting to review` : 'Sync and review'}
                </span>
              </span>
              {bankPending > 0 && (
                <span className="px-2 py-0.5 text-xs font-semibold rounded-full bg-indigo-100 text-indigo-700">
                  {bankPending}
                </span>
              )}
            </button>
          )}
          <button
            onClick={() => setOpen(false)}
            className="w-full px-4 py-3 text-sm text-gray-500 border-t border-gray-100 hover:bg-gray-50 sm:hidden"
          >
            Cancel
          </button>
        </div>
      )}
    </div>
  )
}
