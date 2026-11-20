import { useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import type { Need, Tx } from '../../lib/types'
import { useSetNeed } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { TxRow } from '../../components/TxEditor'
import { useToast } from '../../components/ui'

// One action's width in the tray the pill opens.
const ACTION = 72
// Finger travel before a drag counts as a swipe rather than a tap or scroll.
const SLOP = 8
// How far a swipe must travel to mark the payment when you let go — or a
// quick flick of at least FLICK_MIN at FLICK_SPEED px/ms does it too.
const COMMIT = 80
const FLICK_MIN = 32
const FLICK_SPEED = 0.45

const LABEL: Record<Exclude<Need, ''>, string> = { must: 'Must', optional: 'Optional' }
const HUE: Record<Exclude<Need, ''>, string> = { must: 'var(--s1)', optional: 'var(--s7)' } // the Free money card's colours

type RowProps = { t: Tx; onClick?: () => void; selected?: boolean; onSelect?: () => void; showDate?: boolean }

/**
 * A Ledger row whose expense can be marked a must or optional. On a phone one
 * swipe does it: right marks it a must, left optional, and swiping toward the
 * mark it already has clears it. Let go past the line (you feel a tick), or
 * flick. With a mouse or keyboard, press the small Must/Optional pill for
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
  const [x, setX] = useState(0) // how far the pill's tray has slid the row left
  const root = useRef<HTMLDivElement>(null)
  const slide = useRef<HTMLDivElement>(null)
  const under = useRef<HTMLDivElement>(null)
  const underText = useRef<HTMLSpanElement>(null)
  const tray = useRef<HTMLDivElement>(null)
  const pillRef = useRef<HTMLButtonElement>(null)
  const drag = useRef<{ x: number; y: number; t: number; dx: number; v: number; mode: '' | 'swipe' | 'no'; armed: boolean } | null>(null)
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

  // What a swipe sets: right is Must, left is Optional; swiping toward the
  // mark the payment already has takes it off again.
  const target = (dx: number): Need => {
    const v: Need = dx > 0 ? 'must' : 'optional'
    return v === need ? '' : v
  }
  // The swipe moves the row straight in the DOM: a React render on every
  // touchmove made the row stutter behind the finger on a phone.
  const paint = (dx: number, armed: boolean, animate: boolean) => {
    const row = slide.current
    const bg = under.current
    const text = underText.current
    if (!row || !bg || !text) return
    row.style.transition = animate ? 'transform 180ms ease' : 'none'
    row.style.transform = dx ? `translateX(${dx}px)` : ''
    if (!dx) {
      // Keep what's underneath until the row has slid back over it.
      setTimeout(() => {
        if (drag.current) return
        bg.style.visibility = 'hidden'
        row.classList.remove('bg-surface')
      }, animate ? 180 : 0)
      return
    }
    row.classList.add('bg-surface')
    const v = target(dx)
    bg.style.visibility = 'visible'
    bg.style.justifyContent = dx > 0 ? 'flex-start' : 'flex-end'
    bg.style.background = v ? `color-mix(in srgb, ${HUE[v]} ${armed ? 72 : 40}%, black)` : 'var(--color-sunken)'
    bg.style.color = v ? 'white' : 'var(--color-ink2)'
    text.textContent = v ? LABEL[v] : 'Clear'
    // The word fades in as room opens for it, rather than showing cut off.
    text.style.opacity = String(Math.min(1, Math.max(0, (Math.abs(dx) - 52) / (COMMIT - 52))))
  }

  const start = (e: React.TouchEvent) => {
    if (open || e.touches.length !== 1) { drag.current = null; return }
    drag.current = { x: e.touches[0].clientX, y: e.touches[0].clientY, t: e.timeStamp, dx: 0, v: 0, mode: '', armed: false }
  }
  const move = (e: React.TouchEvent) => {
    const d = drag.current
    if (!d || d.mode === 'no') return
    const dx = e.touches[0].clientX - d.x
    const dy = Math.abs(e.touches[0].clientY - d.y)
    if (d.mode === '') {
      // Up or down is the page scrolling (and pull-to-refresh); only a drag
      // more sideways than up or down is taken.
      // A swipe saves at once, so a slanted scroll must never pass for one.
      if (dy > SLOP && dy >= Math.abs(dx)) { d.mode = 'no'; return }
      if (Math.abs(dx) < SLOP || Math.abs(dx) < dy * 1.5) return
      d.mode = 'swipe'
      swiped.current = true
    }
    const dt = e.timeStamp - d.t
    if (dt > 0) d.v = (dx - d.dx) / dt
    d.t = e.timeStamp
    // Past the point of no return the row follows at half speed.
    const far = Math.abs(dx) > COMMIT ? Math.sign(dx) * (COMMIT + (Math.abs(dx) - COMMIT) / 2) : dx
    d.dx = dx
    const armed = Math.abs(dx) >= COMMIT
    if (armed !== d.armed) {
      d.armed = armed
      if (armed) navigator.vibrate?.(10) // feel it: letting go now marks it
    }
    paint(far, armed, false)
  }
  const end = (cancelled: boolean, at: number) => {
    const d = drag.current
    drag.current = null
    if (!d || d.mode !== 'swipe') return
    // A finger that stopped before lifting is a held drag, not a flick.
    const flick = at - d.t < 80 && Math.abs(d.dx) >= FLICK_MIN && Math.abs(d.v) >= FLICK_SPEED && Math.sign(d.v) === Math.sign(d.dx)
    paint(0, false, true)
    if (!cancelled && (d.armed || flick)) mark(target(d.dx))
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
      onTouchStart={start} onTouchMove={move} onTouchEnd={(e) => end(false, e.timeStamp)} onTouchCancel={(e) => end(true, e.timeStamp)}
      onClickCapture={(e) => {
        const inTray = tray.current?.contains(e.target as Node)
        if (swiped.current && !inTray) { e.stopPropagation(); e.preventDefault(); return }
        // A tap on an open row closes it instead of opening the editor.
        if (open && !inTray && !(e.target as Element).closest('[aria-expanded]')) { e.stopPropagation(); setX(0) }
      }}>
      {/* What the swipe will do, revealed under the row as it slides. */}
      <div ref={under} aria-hidden className="absolute inset-0 flex items-center px-5 text-sm font-semibold" style={{ visibility: 'hidden' }}><span ref={underText} /></div>
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
      <div ref={slide} className={clsx('relative', open && 'bg-surface')} style={{ transform: open ? `translateX(${-x}px)` : undefined, transition: 'transform 180ms ease' }}>
        <TxRow {...p} mark={pill} />
      </div>
    </div>
  )
}
