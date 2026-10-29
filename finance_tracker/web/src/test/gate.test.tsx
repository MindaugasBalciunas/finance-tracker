import { render, screen, waitFor } from '@testing-library/react'
import { resumedUnlock } from '../App'

/**
 * Refreshing an unlocked page keeps it unlocked; everything else still asks for
 * the fingerprint. The marks are the only thing that can skip it, so every way
 * in is pinned here together with its denied neighbour.
 */
describe('resuming an unlock across a page load', () => {
  const now = Date.now()
  beforeEach(() => sessionStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  const marks = (unlockedAgo: number, leftAgo: number) => {
    sessionStorage.setItem('ft-unlocked-at', String(now - unlockedAgo))
    sessionStorage.setItem('ft-left-at', String(now - leftAgo))
  }
  const navType = (type: string) => vi.spyOn(performance, 'getEntriesByType').mockReturnValue([{ type }] as any)

  it('resumes a reload that follows straight on leaving the page', () => {
    marks(2_000, 500)
    navType('reload')
    expect(resumedUnlock()).toBe(true)
  })

  it('does not resume a fresh open', () => {
    marks(2_000, 500)
    navType('navigate')
    expect(resumedUnlock()).toBe(false)
  })

  it('does not resume a restored tab, whose sessionStorage comes back too', () => {
    marks(2_000, 500)
    navType('back_forward')
    expect(resumedUnlock()).toBe(false)
  })

  it('does not resume a page that comes back long after it was left', () => {
    marks(2_000, 60_000)
    navType('reload')
    expect(resumedUnlock()).toBe(false)
  })

  it('does not resume an unlock older than the idle window', () => {
    marks(16 * 60_000, 500)
    navType('reload')
    expect(resumedUnlock()).toBe(false)
  })

  it('does not resume without the marks', () => {
    navType('reload')
    expect(resumedUnlock()).toBe(false)
  })

  it('does not resume once the mark is cleared, as locking does', () => {
    marks(2_000, 500)
    sessionStorage.removeItem('ft-unlocked-at')
    navType('reload')
    expect(resumedUnlock()).toBe(false)
  })
})

describe('the lock gate', () => {
  const calls: string[] = []
  beforeEach(() => {
    calls.length = 0
    sessionStorage.clear()
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      calls.push(url)
      // The server still holds a live session: only the app decides to end it.
      if (url.includes('auth/status')) return new Response(JSON.stringify({ enabled: true, unlocked: true, passkeys: 0 }))
      return new Response('{"ok":true}')
    }))
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('ends a session left from before this page load and asks again', async () => {
    // Each test gets a fresh module, which is what a page load is.
    vi.resetModules()
    vi.spyOn(performance, 'getEntriesByType').mockReturnValue([{ type: 'navigate' }] as any)
    const { default: App } = await import('../App')
    render(<App />)
    expect(await screen.findByText('Finance is locked')).toBeInTheDocument()
    await waitFor(() => expect(calls.some((u) => u.includes('auth/logout'))).toBe(true))
  })
})
