import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { bankingApi } from '../../api/banking'

vi.mock('../../api/banking', () => ({
  bankingApi: { callback: vi.fn() },
}))

const api = vi.mocked(bankingApi)

// The catcher reads the query string at module scope, so each case has to set
// the URL first and then import a fresh copy of the module.
async function mount(search: string) {
  window.history.replaceState(null, '', `/${search}#/dashboard`)
  vi.resetModules()
  const { default: BankRedirectCatcher } = await import('./BankRedirectCatcher')
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <BankRedirectCatcher />
    </QueryClientProvider>
  )
}

describe('BankRedirectCatcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.callback.mockResolvedValue({ connection_id: 7 })
  })
  afterEach(() => window.history.replaceState(null, '', '/'))

  it('posts the code once and strips it from the address bar', async () => {
    await mount('?code=abc123&state=nonce-9')

    // Stripped synchronously, before the round-trip: the code is already in
    // browser history and the access log without leaving it on screen too.
    expect(window.location.search).toBe('')
    expect(window.location.href).not.toContain('abc123')

    await waitFor(() => expect(api.callback).toHaveBeenCalledTimes(1))
    expect(api.callback).toHaveBeenCalledWith({ code: 'abc123', state: 'nonce-9' })
    expect(await screen.findByText(/Bank connected/)).toBeInTheDocument()
  })

  it('keeps unrelated query params', async () => {
    await mount('?code=abc123&state=n1&debug=1')
    expect(window.location.search).toBe('?debug=1')
    await waitFor(() => expect(api.callback).toHaveBeenCalledTimes(1))
  })

  // The component mounts on every page load, so the no-code path has to be
  // completely inert — no request, no banner.
  it('does nothing and renders nothing without a code', async () => {
    await mount('')
    expect(api.callback).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toContain('Finishing')
  })

  it('surfaces a failed exchange instead of failing silently', async () => {
    api.callback.mockRejectedValue(new Error('state no longer valid'))
    await mount('?code=abc123&state=stale')
    expect(await screen.findByText(/state no longer valid/)).toBeInTheDocument()
  })
})
