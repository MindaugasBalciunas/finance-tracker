import { api } from './api'

// WebAuthn helpers: the server speaks go-webauthn's JSON (base64url fields).
const b64uToBuf = (s: string) => {
  const pad = '='.repeat((4 - (s.length % 4)) % 4)
  const b = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(b, (c) => c.charCodeAt(0)).buffer
}
const bufToB64u = (b: ArrayBuffer) => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

export async function passkeyLogin() {
  const opts = await api.post<any>('/auth/passkey/login/begin')
  const pk = opts.publicKey
  pk.challenge = b64uToBuf(pk.challenge)
  pk.allowCredentials = (pk.allowCredentials ?? []).map((c: any) => ({ ...c, id: b64uToBuf(c.id) }))
  const cred = (await navigator.credentials.get({ publicKey: pk })) as PublicKeyCredential
  const r = cred.response as AuthenticatorAssertionResponse
  await api.post('/auth/passkey/login/finish', {
    id: cred.id, rawId: bufToB64u(cred.rawId), type: cred.type,
    response: { authenticatorData: bufToB64u(r.authenticatorData), clientDataJSON: bufToB64u(r.clientDataJSON), signature: bufToB64u(r.signature), userHandle: r.userHandle ? bufToB64u(r.userHandle) : null },
  })
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
    response: { attestationObject: bufToB64u(r.attestationObject), clientDataJSON: bufToB64u(r.clientDataJSON) },
  }, { name })
}
