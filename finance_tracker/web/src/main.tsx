import React from 'react'
import ReactDOM from 'react-dom/client'
import App, { applyTheme } from './App'
import './index.css'

applyTheme()

// Phones: when the keyboard opens, keep the focused text field in view and
// let the tab bar step aside (it would sit on the keyboard). "Open" is read
// from the visible height, not from focus: Android's back key closes the
// keyboard but leaves the field focused, and Firefox doesn't report focus
// leaving a sheet that closes.
if (typeof window !== 'undefined' && window.visualViewport && window.matchMedia?.('(pointer: coarse)').matches) {
  const vv = window.visualViewport
  const text = (el: Element | null): el is HTMLElement =>
    !!el && el.matches('input:not([type=checkbox]):not([type=radio]):not([type=file]):not([type=date]):not([type=range]), textarea')
  let tallest = vv.height
  let open = false
  const update = () => {
    tallest = Math.max(tallest, vv.height)
    const now = text(document.activeElement) && vv.height < tallest - 120
    if (now === open) return
    open = now
    if (open) {
      document.documentElement.dataset.typing = '1'
      const el = document.activeElement as HTMLElement
      const r = el.getBoundingClientRect()
      if (r.bottom > vv.height + vv.offsetTop - 16 || r.top < vv.offsetTop + 56) el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    } else delete document.documentElement.dataset.typing
  }
  vv.addEventListener('resize', update)
  document.addEventListener('focusin', () => setTimeout(update, 50))
  document.addEventListener('focusout', () => setTimeout(update, 50))
  window.addEventListener('orientationchange', () => { tallest = 0; setTimeout(update, 300) })
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
