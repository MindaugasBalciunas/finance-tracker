import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Icon } from './Icon'

// Finger travel that commits to a refresh, and how far the badge follows.
const TRIGGER = 100
const DAMP = 0.5
const MAX = 80

/** Is the finger over something that scrolls itself (a long list in a panel)? */
function scrollsItself(target: EventTarget | null) {
  let el = target instanceof Element ? target : null
  while (el && el !== document.body) {
    const style = getComputedStyle(el)
    // Only when it has somewhere to go: a list already at its top can't scroll
    // further up, so the pull belongs to the page.
    if (/auto|scroll/.test(style.overflowY) && el.scrollHeight > el.clientHeight && el.scrollTop > 0) return true
    el = el.parentElement
  }
  return false
}

/** A sheet, the menu drawer or the quick actions are over the page. */
function covered() {
  return document.body.style.overflow === 'hidden' || !!document.querySelector('[role=dialog]')
}

/**
 * Pull down at the top of a page to refetch what's on screen. Reloading the
 * page would ask for the fingerprint again; this refreshes the numbers without
 * leaving the app. The browser's own pull gesture stays off (`overscroll-behavior`).
 */
export function PullToRefresh() {
  const qc = useQueryClient()
  const [dist, setDist] = useState(0)
  const [busy, setBusy] = useState(false)
  // While the finger is down the badge follows it exactly; it only animates
  // when it springs back or settles into the spinner.
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    let from: { x: number; y: number } | null = null
    let pulling = false
    let armed = false
    let working = false

    // Watching every touchmove in the app costs the fast scroll path, so the
    // non-passive listener only exists for the length of a candidate gesture.
    const release = () => {
      from = null
      pulling = false
      armed = false
      window.removeEventListener('touchmove', move)
      setDragging(false)
    }

    function move(e: TouchEvent) {
      if (!from || e.touches.length !== 1) return release()
      const dy = e.touches[0].clientY - from.y
      const dx = e.touches[0].clientX - from.x
      if (!pulling) {
        // Wait until the drag is clearly downwards: an upward scroll or a
        // sideways swipe (tab strips, charts) must pass straight through.
        if (dy < 10 || Math.abs(dx) > Math.abs(dy)) {
          if (dy < -4 || Math.abs(dx) > 10) release()
          return
        }
        if (scrollsItself(e.target)) return release()
        pulling = true
        setDragging(true)
      }
      // Changed their mind and pulled back up: hand the page straight back
      // rather than swallowing the rest of the gesture.
      if (dy <= 0) {
        release()
        return setDist(0)
      }
      if (e.cancelable) e.preventDefault()
      setDist(Math.min(MAX, dy * DAMP))
      if (dy >= TRIGGER && !armed) {
        armed = true
        navigator.vibrate?.(8)
      }
    }

    const begin = (e: TouchEvent) => {
      release()
      if (working || e.touches.length !== 1) return
      if (window.scrollY > 0 || covered()) return
      from = { x: e.touches[0].clientX, y: e.touches[0].clientY }
      window.addEventListener('touchmove', move, { passive: false })
    }

    const finish = async () => {
      const go = pulling && armed
      release()
      if (!go) return setDist(0)
      working = true
      setBusy(true)
      setDist(56)
      // Mark everything stale and refetch what's mounted; the request also
      // keeps the session alive, so the lock screen stays away.
      const done = qc.invalidateQueries().catch(() => {})
      await Promise.all([done, new Promise((r) => setTimeout(r, 450))])
      working = false
      setBusy(false)
      setDist(0)
    }

    window.addEventListener('touchstart', begin, { passive: true })
    window.addEventListener('touchend', finish, { passive: true })
    window.addEventListener('touchcancel', finish, { passive: true })
    return () => {
      window.removeEventListener('touchstart', begin)
      window.removeEventListener('touchend', finish)
      window.removeEventListener('touchcancel', finish)
      window.removeEventListener('touchmove', move)
    }
  }, [qc])

  if (dist <= 0) return null
  const ready = busy || dist >= TRIGGER * DAMP
  return (
    <div aria-hidden className="pointer-events-none fixed inset-x-0 top-0 z-40 flex justify-center">
      <div className={`grid h-10 w-10 place-items-center rounded-full border border-line bg-surface shadow-lg ${dragging ? '' : 'transition-transform duration-200'}`}
        style={{ transform: `translateY(${dist - 48}px)`, opacity: Math.min(1, dist / 24) }}>
        <Icon name="refresh" size={18}
          className={busy ? 'animate-spin text-accent' : ready ? 'text-accent' : 'text-muted'}
          style={busy ? undefined : { transform: `rotate(${dist * 4}deg)` }} />
      </div>
    </div>
  )
}
