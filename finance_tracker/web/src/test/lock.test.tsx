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

  // On the public address the web password gate answers the PIN call itself:
  // an HTML page, and over HTTP/2 no status text. That used to leave the lock
  // screen blank after a correct PIN.
  it('says the web password is missing when the gate refuses the PIN', async () => {
    const gate = '<html><head><title>401 Authorization Required</title></head><body>nginx</body></html>'
    vi.stubGlobal('fetch', vi.fn(async (url: string) =>
      url.includes('status') ? new Response('{"passkeys":0}') : new Response(gate, { status: 401, statusText: '' })))
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    for (const d of ['1', '2', '2', '4']) fireEvent.click(screen.getByText(d))
    expect(await screen.findByText('PIN unlock needs your web password')).toBeInTheDocument()
    expect(onUnlock).not.toHaveBeenCalled()
  })

  it('says the same when the gate refuses with no body at all', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) =>
      url.includes('status') ? new Response('{"passkeys":0}') : new Response(null, { status: 401, statusText: '' })))
    render(<Lock onUnlock={vi.fn()} />)
    for (const d of ['1', '2', '2', '4']) fireEvent.click(screen.getByText(d))
    expect(await screen.findByText('PIN unlock needs your web password')).toBeInTheDocument()
  })
})

describe('fingerprint unlock', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  /** A browser with one passkey; `get` resolves when the "fingerprint" is read. */
  const withPasskey = (get: (opts: any) => Promise<any>) => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) =>
      new Response(JSON.stringify(url.includes('status') ? { passkeys: 1 } : { publicKey: { challenge: 'AA', allowCredentials: [] } }), { status: 200 })))
    vi.stubGlobal('PublicKeyCredential', class {})
    vi.stubGlobal('navigator', Object.create(navigator, { credentials: { value: { get: vi.fn(get) } } }))
    return (navigator.credentials.get as any)
  }
  const assertion = {
    id: 'a', rawId: new ArrayBuffer(1), type: 'public-key',
    response: { authenticatorData: new ArrayBuffer(1), clientDataJSON: new ArrayBuffer(1), signature: new ArrayBuffer(1), userHandle: null },
  }

  it('asks for the fingerprint as the screen opens', async () => {
    const get = withPasskey(async () => assertion)
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    await waitFor(() => expect(onUnlock).toHaveBeenCalled())
    expect(get).toHaveBeenCalledTimes(1)
  })

  it('offers another try after a cancelled prompt, without an error', async () => {
    const get = withPasskey(async () => { throw new DOMException('cancelled', 'NotAllowedError') })
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    expect(await screen.findByText(/tap to try again/i)).toBeInTheDocument()
    expect(onUnlock).not.toHaveBeenCalled()
    // The button still works, and each tap is one reading.
    fireEvent.click(screen.getByText('Unlock with fingerprint'))
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  })

  it('ignores taps while the phone is already reading a fingerprint', async () => {
    let release: (v: any) => void = () => {}
    const get = withPasskey(() => new Promise((r) => { release = r }))
    const onUnlock = vi.fn()
    render(<Lock onUnlock={onUnlock} />)
    await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
    const button = await screen.findByText(/waiting for your fingerprint/i)
    fireEvent.click(button)
    fireEvent.click(button)
    expect(get).toHaveBeenCalledTimes(1)
    release(assertion)
    await waitFor(() => expect(onUnlock).toHaveBeenCalled())
  })
})
