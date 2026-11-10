import { passkeyLogin, passkeyRegister } from '../lib/passkey'

// The fingerprint unlock is how the owner opens the app: the options handed
// to the browser must keep every field the server sent, with the base64url
// ones turned into bytes — and nothing else changed.
const b64u = (bytes: number[]) => btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
const bytes = (b: BufferSource | undefined) => [...new Uint8Array(b as ArrayBuffer)]
const fakeCred = (response: object) => ({ id: 'cred-1', rawId: new Uint8Array([9]).buffer, type: 'public-key', authenticatorAttachment: 'platform', response })

function serve(publicKey: object) {
  const posted: { url: string; body: unknown }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    posted.push({ url, body: init?.body ? JSON.parse(init.body as string) : null })
    return new Response(JSON.stringify(url.includes('/begin') ? { publicKey } : { ok: true }), { status: 200 })
  }))
  return posted
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('passkeys', () => {
  it('sign-in decodes the challenge and credential ids and keeps the rest', async () => {
    serve({ challenge: b64u([1, 2, 3]), rpId: 'example.test', timeout: 60000, userVerification: 'required', allowCredentials: [{ type: 'public-key', id: b64u([7, 8]), transports: ['internal'] }] })
    const get = vi.fn(async () => fakeCred({ authenticatorData: new Uint8Array([1]).buffer, clientDataJSON: new Uint8Array([2]).buffer, signature: new Uint8Array([3]).buffer, userHandle: null }))
    vi.stubGlobal('navigator', { credentials: { get } })
    await passkeyLogin()
    const pk = (get.mock.calls[0] as unknown as [{ publicKey: PublicKeyCredentialRequestOptions }])[0].publicKey
    expect(bytes(pk.challenge)).toEqual([1, 2, 3])
    expect(pk.rpId).toBe('example.test')
    expect(pk.timeout).toBe(60000)
    expect(pk.userVerification).toBe('required')
    expect(pk.allowCredentials).toHaveLength(1)
    expect(bytes(pk.allowCredentials![0].id)).toEqual([7, 8])
    expect(pk.allowCredentials![0].transports).toEqual(['internal'])
  })

  it('registration decodes the challenge and user id and reports the attachment', async () => {
    const posted = serve({ challenge: b64u([4, 5]), rp: { id: 'example.test', name: 'Finance' }, user: { id: b64u([6]), name: 'owner', displayName: 'Owner' }, pubKeyCredParams: [{ type: 'public-key', alg: -7 }], excludeCredentials: [] })
    const create = vi.fn(async () => fakeCred({ attestationObject: new Uint8Array([1]).buffer, clientDataJSON: new Uint8Array([2]).buffer, getTransports: () => ['internal'] }))
    vi.stubGlobal('navigator', { credentials: { create } })
    await passkeyRegister('Mac · Firefox')
    const pk = (create.mock.calls[0] as unknown as [{ publicKey: PublicKeyCredentialCreationOptions }])[0].publicKey
    expect(bytes(pk.challenge)).toEqual([4, 5])
    expect(bytes(pk.user.id)).toEqual([6])
    expect(pk.user.name).toBe('owner')
    expect(pk.rp).toEqual({ id: 'example.test', name: 'Finance' })
    expect(pk.pubKeyCredParams).toEqual([{ type: 'public-key', alg: -7 }])
    const finish = posted.find((p) => p.url.includes('/register/finish'))!.body as { authenticatorAttachment?: string }
    expect(finish.authenticatorAttachment).toBe('platform')
  })
})
