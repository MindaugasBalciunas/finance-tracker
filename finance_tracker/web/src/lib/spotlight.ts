// The card's hover light follows the pointer: the edge nearest it glows
// most and the rainbow rim turns around it (index.css reads --mx/--my in
// pixels and --px/--py as 0–1 across and down). One listener for the whole app, at
// most one update a frame; only for a mouse or trackpad, and never with
// reduced motion asked for.
export function startSpotlight() {
  if (typeof window === 'undefined' || !window.matchMedia?.('(hover: hover) and (pointer: fine)').matches) return
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  let last: HTMLElement | null = null
  let frame = 0
  let at = { x: 0, y: 0, target: null as Element | null }
  const clear = () => {
    last?.style.removeProperty('--mx')
    last?.style.removeProperty('--my')
    last?.style.removeProperty('--px')
    last?.style.removeProperty('--py')
    last = null
  }
  const paint = () => {
    frame = 0
    const card = (at.target as Element | null)?.closest?.('.card') as HTMLElement | null
    if (card !== last) clear()
    if (!card) return
    last = card
    const r = card.getBoundingClientRect()
    card.style.setProperty('--mx', `${at.x - r.left}px`)
    card.style.setProperty('--my', `${at.y - r.top}px`)
    // Where in the card, 0–1 across and down: the edge nearest the pointer glows most.
    card.style.setProperty('--px', Math.min(1, Math.max(0, (at.x - r.left) / r.width)).toFixed(3))
    card.style.setProperty('--py', Math.min(1, Math.max(0, (at.y - r.top) / r.height)).toFixed(3))
  }
  window.addEventListener('pointermove', (e) => {
    at = { x: e.clientX, y: e.clientY, target: e.target as Element | null }
    if (!frame) frame = requestAnimationFrame(paint)
  }, { passive: true })
  // Leaving the window (pointerout with nowhere to go) puts the light out.
  window.addEventListener('pointerout', (e) => { if (!e.relatedTarget) clear() }, { passive: true })
}
