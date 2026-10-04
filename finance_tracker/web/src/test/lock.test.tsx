import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Lock from '../pages/Lock'

describe('lock screen', () => {
  afterEach(() => vi.restoreAllMocks())

  it('submits after four digits and unlocks on success', async () => {
    const calls: { url: string; body: any }[] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, body: init?.body ? JSON.parse(init.body as string) : null })
      const body = url.includes('status') ? { passkeys: 0 } : { ok: true }
      return new Response(JSON.stringify(body), { status: 200 })
    }))
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    for (const d of ['1', '2', '2', '4']) fireEvent.click(screen.getByText(d))
    await waitFor(() => expect(onUnlock).toHaveBeenCalled())
    expect(calls.find((c) => c.url.includes('pin/login'))?.body).toEqual({ pin: '1224' })
  })

  it('shows the error and clears the PIN on a wrong guess', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) =>
      url.includes('status') ? new Response('{"passkeys":0}') : new Response('{"error":"wrong PIN"}', { status: 401 })))
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    for (const d of ['0', '0', '0', '0']) fireEvent.click(screen.getByText(d))
    expect(await screen.findByText('wrong PIN')).toBeInTheDocument()
    expect(onUnlock).not.toHaveBeenCalled()
  })
})
