import { useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import type { Need, Tx } from '../../lib/types'
import { useSetNeed } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { TxRow } from '../../components/TxEditor'
import { useToast } from '../../components/ui'

// One action's width; the row slides left by as many as there are.
const ACTION = 72
// Finger travel before a drag counts as a swipe rather than a tap or scroll.
const SLOP = 10

const LABEL: Record<Exclude<Need, ''>, string> = { must: 'Must', optional: 'Optional' }
const HUE: Record<Exclude<Need, ''>, string> = { must: 'var(--s1)', optional: 'var(--s7)' } // the Free money card's colours

type RowProps = { t: Tx; onClick?: () => void; selected?: boolean; onSelect?: () => void; showDate?: boolean }

/**
 * A Ledger row whose expense can be marked a must or optional. Swipe it left
 * (touch) or press the small Must/Optional pill (mouse, keyboard) to reveal
 * the actions. The pill shows where the payment counts now: solid when you
 * marked it, dashed when it follows its category. Transfers, income, and
 * expenses a fixed line of the plan claims are in neither half, so they stay
 * plain rows.
 */
export function NeedRow(p: RowProps) {
  const { t } = p
  if (t.kind !== 'expense' || t.fixed || p.onSelect) return <TxRow {...p} />
  return <Markable {...p} />
}

function Markable(p: RowProps) {
  const { t } = p
  const cats = useCats()
  const toast = useToast()
  const save = useSetNeed()
  // What was just chosen, shown until the list comes back with it.
  const [chosen, setChosen] = useState<Need | undefined>()
  useEffect(() => setChosen(undefined), [t.need])
  const need: Need = chosen ?? t.need ?? ''
  const shown: Exclude<Need, ''> = need || (cats.byId[t.category]?.essential ? 'must' : 'optional')

  const actions: Need[] = need ? ['must', 'optional', ''] : ['must', 'optional']
  const width = ACTION * actions.length
  const [x, setX] = useState(0) // how far the row has slid left
  const [dragging, setDragging] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const tray = useRef<HTMLDivElement>(null)
  const pillRef = useRef<HTMLButtonElement>(null)
  const drag = useRef<{ x: number; y: number; base: number; mode: '' | 'swipe' | 'no' } | null>(null)
  const swiped = useRef(false) // swallow the click a finished swipe may still send
  const open = x > 0

  // Closing from the keyboard hands focus back to the pill: the tray it was
  // in is hidden, and focus would otherwise fall to the top of the page.
  const shut = () => {
    if (tray.current?.contains(document.activeElement)) pillRef.current?.focus()
    setX(0)
  }
  // Open rows close when anything else is touched, or on Escape.
  useEffect(() => {
    if (!open) return
    const away = (e: Event) => { if (!root.current?.contains(e.target as Node)) setX(0) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') shut() }
    document.addEventListener('pointerdown', away, true)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('pointerdown', away, true); document.removeEventListener('keydown', esc) }
  }, [open])

  const mark = (v: Need) => {
    shut()
    if (v === need) return
    setChosen(v)
    save.mutate({ id: t.id, need: v }, {
      onSuccess: () => toast(v ? `Marked ${v === 'must' ? 'a must' : 'optional'}` : 'Back to its category', 'good'),
      onError: (e) => { setChosen(undefined); toast((e as Error).message, 'bad') },
    })
  }

  const start = (e: React.TouchEvent) => {
    drag.current = e.touches.length === 1 ? { x: e.touches[0].clientX, y: e.touches[0].clientY, base: x, mode: '' } : null
  }
  const move = (e: React.TouchEvent) => {
    const d = drag.current
    if (!d || d.mode === 'no') return
    const dx = d.x - e.touches[0].clientX
    const dy = Math.abs(d.y - e.touches[0].clientY)
    if (d.mode === '') {
      // Up or down is the page scrolling (and pull-to-refresh); a swipe to
      // the right on a closed row is nobody's. Only a clearly sideways drag
      // is taken.
      if (dy > SLOP && dy >= Math.abs(dx)) { d.mode = 'no'; return }
      if (Math.abs(dx) < SLOP || Math.abs(dx) < dy * 1.5) return
      if (d.base === 0 && dx < 0) { d.mode = 'no'; return }
      d.mode = 'swipe'
      swiped.current = true
      setDragging(true)
    }
    setX(Math.max(0, Math.min(width + 24, d.base + dx)))
  }
  const end = (cancelled: boolean) => {
    const d = drag.current
    drag.current = null
    if (!d || d.mode !== 'swipe') return
    setDragging(false)
    setX((now) => (cancelled ? d.base : (d.base === 0 ? now > 40 : now > width - 40) ? width : 0))
    setTimeout(() => { swiped.current = false }, 350)
  }

  const pill = (
    <button ref={pillRef} type="button" aria-expanded={open} title={need ? 'You marked this payment' : 'From its category — press to change'}
      aria-label={`${LABEL[shown]}${need ? '' : ', from its category'}. Change`}
      onClick={(e) => {
        e.stopPropagation()
        if (open) return setX(0)
        setX(width)
        requestAnimationFrame(() => tray.current?.querySelector('button')?.focus())
      }}
      className={clsx('relative shrink-0 rounded-full border px-1.5 text-[10px] font-medium leading-4 before:absolute before:-inset-x-2 before:top-0 before:-bottom-3 before:content-[""]',
        !need && 'border-dashed border-axis text-muted')}
      style={need ? { color: HUE[shown], borderColor: HUE[shown], background: `color-mix(in srgb, ${HUE[shown]} 12%, transparent)` } : undefined}>
      {LABEL[shown]}
    </button>
  )

  return (
    <div ref={root} className="relative overflow-hidden" style={{ touchAction: 'pan-y' }}
      onTouchStart={start} onTouchMove={move} onTouchEnd={() => end(false)} onTouchCancel={() => end(true)}
      onClickCapture={(e) => {
        const inTray = tray.current?.contains(e.target as Node)
        if (swiped.current && !inTray) { e.stopPropagation(); e.preventDefault(); return }
        // A tap on an open row closes it instead of opening the editor.
        if (open && !inTray && !(e.target as Element).closest('[aria-expanded]')) { e.stopPropagation(); setX(0) }
      }}>
      <div ref={tray} className="absolute inset-y-0 right-0 flex" style={{ visibility: open ? 'visible' : 'hidden' }}>
        {actions.map((a) => (
          <button key={a || 'reset'} type="button" onClick={() => mark(a)} aria-pressed={a ? need === a : undefined}
            aria-label={a ? `Mark ${a === 'must' ? 'a must' : 'optional'}` : 'Clear the mark, back to its category'}
            className={clsx('grid place-items-center text-xs font-semibold', a ? 'text-white' : 'bg-sunken text-ink2')}
            style={{ width: ACTION, background: a ? `color-mix(in srgb, ${HUE[a]} 72%, black)` : undefined }}>
            {a ? LABEL[a] : 'Clear'}
          </button>
        ))}
      </div>
      {/* Opaque only while it slides, so a resting row keeps the card's own wash. */}
      <div className={clsx('relative', (open || dragging) && 'bg-surface')} style={{ transform: `translateX(${-x}px)`, transition: dragging ? 'none' : 'transform 180ms ease' }}>
        <TxRow {...p} mark={pill} />
      </div>
    </div>
  )
}
