// Conversions between the JSON the Go webauthn library speaks (base64url
// strings) and the ArrayBuffers the browser WebAuthn API requires.

export function base64urlToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

export function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function webauthnAvailable(): boolean {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential === 'function'
}

// Server CredentialCreation JSON → navigator.credentials.create options
export function toCreationOptions(server: any): CredentialCreationOptions {
  const pk = server.publicKey
  return {
    publicKey: {
      ...pk,
      challenge: base64urlToBuffer(pk.challenge),
      user: { ...pk.user, id: base64urlToBuffer(pk.user.id) },
      excludeCredentials: (pk.excludeCredentials ?? []).map((c: any) => ({
        ...c,
        id: base64urlToBuffer(c.id),
      })),
    },
  }
}

// Server CredentialAssertion JSON → navigator.credentials.get options
export function toRequestOptions(server: any): CredentialRequestOptions {
  const pk = server.publicKey
  return {
    publicKey: {
      ...pk,
      challenge: base64urlToBuffer(pk.challenge),
      allowCredentials: (pk.allowCredentials ?? []).map((c: any) => ({
        ...c,
        id: base64urlToBuffer(c.id),
      })),
    },
  }
}

export function serializeRegistration(cred: PublicKeyCredential) {
  const resp = cred.response as AuthenticatorAttestationResponse
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufferToBase64url(resp.attestationObject),
      clientDataJSON: bufferToBase64url(resp.clientDataJSON),
    },
  }
}

export function serializeAssertion(cred: PublicKeyCredential) {
  const resp = cred.response as AuthenticatorAssertionResponse
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufferToBase64url(resp.authenticatorData),
      clientDataJSON: bufferToBase64url(resp.clientDataJSON),
      signature: bufferToBase64url(resp.signature),
      userHandle: resp.userHandle ? bufferToBase64url(resp.userHandle) : null,
    },
  }
}
