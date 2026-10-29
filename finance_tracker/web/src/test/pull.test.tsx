import { render, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PullToRefresh } from '../components/PullToRefresh'

/** The handler reads exactly these fields off the touch stream. */
function touch(type: 'touchstart' | 'touchmove' | 'touchend', x: number, y: number) {
  const ev = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperty(ev, 'touches', { value: type === 'touchend' ? [] : [{ clientX: x, clientY: y }] })
  Object.defineProperty(ev, 'target', { value: document.body })
  return ev
}
const drag = async (ys: number[], x = 100) => {
  await act(async () => {
    window.dispatchEvent(touch('touchstart', x, ys[0]))
    for (const y of ys.slice(1)) window.dispatchEvent(touch('touchmove', x, y))
  })
  return ys
}
const lift = async (y: number) => {
  await act(async () => { window.dispatchEvent(touch('touchend', 100, y)) })
}

describe('pull to refresh', () => {
  let qc: QueryClient
  let refetch: ReturnType<typeof vi.fn>

  beforeEach(() => {
    qc = new QueryClient()
    refetch = vi.fn(async () => {})
    qc.invalidateQueries = refetch as any
    window.scrollY = 0
    document.body.style.overflow = ''
    render(<QueryClientProvider client={qc}><PullToRefresh /></QueryClientProvider>)
  })

  it('refreshes after a long enough pull from the top', async () => {
    await drag([100, 120, 160, 200, 230])
    await lift(230)
    await waitFor(() => expect(refetch).toHaveBeenCalled())
  })

  it('does nothing for a short pull', async () => {
    await drag([100, 120, 150])
    await lift(150)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('leaves a sideways swipe alone', async () => {
    await act(async () => {
      window.dispatchEvent(touch('touchstart', 100, 100))
      window.dispatchEvent(touch('touchmove', 180, 110))
      window.dispatchEvent(touch('touchmove', 260, 130))
    })
    await lift(130)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('leaves the page alone once it is scrolled down', async () => {
    window.scrollY = 300
    await drag([100, 140, 200, 240])
    await lift(240)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('stays out of the way while a sheet is open', async () => {
    document.body.style.overflow = 'hidden'
    await drag([100, 140, 200, 240])
    await lift(240)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('stays out of the way under a dialog', async () => {
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    document.body.appendChild(dialog)
    await drag([100, 140, 200, 240])
    await lift(240)
    expect(refetch).not.toHaveBeenCalled()
    dialog.remove()
  })

  it('hands the page back when the pull is reversed, instead of holding the scroll', async () => {
    await drag([100, 130, 160])
    // Back up past the start: the page must scroll normally again.
    const back = touch('touchmove', 100, 90)
    await act(async () => { window.dispatchEvent(back) })
    const after = touch('touchmove', 100, 60)
    await act(async () => { window.dispatchEvent(after) })
    expect(after.defaultPrevented).toBe(false)
    await lift(60)
    expect(refetch).not.toHaveBeenCalled()
  })
})
