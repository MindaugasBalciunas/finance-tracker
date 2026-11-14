import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastProvider } from '../components/ui'
import { SyncToken } from '../pages/settings/Security'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

const serve = () => {
  const calls: { method: string; url: string }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ method: init?.method ?? 'GET', url })
    const body = init?.method === 'POST' ? { token: 'ftsy_abc123' } : { ok: true }
    return new Response(JSON.stringify(body), { status: 200 })
  }))
  return calls
}
const show = (has: boolean) => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <ToastProvider><SyncToken has={has} /></ToastProvider>
  </QueryClientProvider>)

describe('sync token', () => {
  it('shows a new token once, with the Keychain command to store it', async () => {
    const calls = serve()
    show(false)
    fireEvent.click(screen.getByRole('button', { name: 'Create sync token' }))
    expect(await screen.findByText('ftsy_abc123')).toBeInTheDocument()
    expect(screen.getByText(/security add-generic-password -U -s finance-tracker-prod-sync -a http.* -w/)).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('auth/sync-token'))).toBe(true)
  })

  it('revokes an existing token', async () => {
    const calls = serve()
    show(true)
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.includes('auth/sync-token'))).toBe(true))
    expect(screen.getByRole('button', { name: 'Replace sync token' })).toBeInTheDocument()
  })
})
