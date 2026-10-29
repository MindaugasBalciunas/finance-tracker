import { api } from './api'

// WebAuthn helpers: the server speaks go-webauthn's JSON (base64url fields).
const b64uToBuf = (s: string) => {
  const pad = '='.repeat((4 - (s.length % 4)) % 4)
  const b = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(b, (c) => c.charCodeAt(0)).buffer
}
const bufToB64u = (b: ArrayBuffer) => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

/** Can this browser read a passkey at all? */
export function passkeySupported() {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials?.get
}

/**
 * The prompt closed without a fingerprint: dismissed, timed out, or replaced
 * by a newer one. Not a failure to report — the screen just offers it again.
 */
export function passkeyCancelled(e: unknown) {
  const name = (e as { name?: string } | null)?.name
  return name === 'AbortError' || name === 'NotAllowedError'
}

// Android's credential manager (Samsung Pass) allows one open request at a
// time: a second one while its sheet is up answers neither, which left the
// lock screen doing nothing. Keep one ceremony and cancel it before the next.
let live: AbortController | null = null

export function cancelPasskeyLogin() {
  live?.abort()
  live = null
}

export async function passkeyLogin() {
  cancelPasskeyLogin()
  const ctrl = new AbortController()
  live = ctrl
  try {
    const opts = await api.post<any>('/auth/passkey/login/begin')
    if (ctrl.signal.aborted) throw new DOMException('replaced', 'AbortError')
    const pk = opts.publicKey
    pk.challenge = b64uToBuf(pk.challenge)
    pk.allowCredentials = (pk.allowCredentials ?? []).map((c: any) => ({ ...c, id: b64uToBuf(c.id) }))
    const cred = (await navigator.credentials.get({ publicKey: pk, signal: ctrl.signal })) as PublicKeyCredential
    const r = cred.response as AuthenticatorAssertionResponse
    await api.post('/auth/passkey/login/finish', {
      id: cred.id, rawId: bufToB64u(cred.rawId), type: cred.type,
      response: { authenticatorData: bufToB64u(r.authenticatorData), clientDataJSON: bufToB64u(r.clientDataJSON), signature: bufToB64u(r.signature), userHandle: r.userHandle ? bufToB64u(r.userHandle) : null },
    })
  } finally {
    if (live === ctrl) live = null
  }
}

export async function passkeyRegister(name: string) {
  const opts = await api.post<any>('/auth/passkey/register/begin')
  const pk = opts.publicKey
  pk.challenge = b64uToBuf(pk.challenge)
  pk.user.id = b64uToBuf(pk.user.id)
  pk.excludeCredentials = (pk.excludeCredentials ?? []).map((c: any) => ({ ...c, id: b64uToBuf(c.id) }))
  const cred = (await navigator.credentials.create({ publicKey: pk })) as PublicKeyCredential
  const r = cred.response as AuthenticatorAttestationResponse
  await api.post('/auth/passkey/register/finish', {
    id: cred.id, rawId: bufToB64u(cred.rawId), type: cred.type,
    // Transports tell a later sign-in whether this passkey lives on this
    // device (Touch ID) or a phone, so the browser offers the right one.
    response: { attestationObject: bufToB64u(r.attestationObject), clientDataJSON: bufToB64u(r.clientDataJSON), transports: r.getTransports?.() ?? [] },
    authenticatorAttachment: (cred as any).authenticatorAttachment ?? undefined,
  }, { name })
}

/** A readable name for this device's passkey, e.g. "Mac · Firefox". */
export function deviceName() {
  const ua = navigator.userAgent
  const os = /Android/.test(ua) ? (ua.match(/;\s*([^;)]+?)\s+Build\//)?.[1] ?? 'Android') : /iPhone|iPad/.test(ua) ? (/iPad/.test(ua) ? 'iPad' : 'iPhone')
    : /Macintosh|Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows PC' : /Linux/.test(ua) ? 'Linux' : 'Device'
  const browser = /Firefox\//.test(ua) ? 'Firefox' : /Edg\//.test(ua) ? 'Edge' : /SamsungBrowser\//.test(ua) ? 'Samsung Internet' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : ''
  return browser ? `${os} · ${browser}` : os
}
