import client from './client'
import {
  toCreationOptions,
  toRequestOptions,
  serializeRegistration,
  serializeAssertion,
} from '../utils/webauthnCodec'

export interface AuthStatus {
  enabled: boolean
  unlocked: boolean
  webauthn_registered: boolean
  webauthn_credentials: number
}

export interface WebauthnCredentialInfo {
  id: number
  name: string
  created_at: string
}

export const authApi = {
  status: async (): Promise<AuthStatus> => {
    const { data } = await client.get<AuthStatus>('/auth/status')
    return data
  },

  pinLogin: async (pin: string): Promise<void> => {
    await client.post('/auth/pin/login', { pin })
  },

  pinSetup: async (pin: string, currentPin?: string): Promise<void> => {
    await client.post('/auth/pin/setup', { pin, current_pin: currentPin ?? '' })
  },

  pinDisable: async (pin: string): Promise<void> => {
    await client.post('/auth/pin/disable', { pin })
  },

  logout: async (): Promise<void> => {
    await client.post('/auth/logout')
  },

  listCredentials: async (): Promise<WebauthnCredentialInfo[]> => {
    const { data } = await client.get<WebauthnCredentialInfo[]>('/auth/webauthn/credentials')
    return data
  },

  deleteCredential: async (id: number): Promise<void> => {
    await client.delete(`/auth/webauthn/credentials/${id}`)
  },

  // Full registration ceremony: server challenge → browser fingerprint prompt → server verify.
  registerFingerprint: async (name: string): Promise<void> => {
    const { data: options } = await client.post('/auth/webauthn/register/begin')
    const cred = (await navigator.credentials.create(toCreationOptions(options))) as PublicKeyCredential
    if (!cred) throw new Error('Fingerprint enrollment was cancelled')
    await client.post(
      `/auth/webauthn/register/finish?name=${encodeURIComponent(name)}`,
      serializeRegistration(cred)
    )
  },

  // Full login ceremony.
  fingerprintLogin: async (): Promise<void> => {
    const { data: options } = await client.post('/auth/webauthn/login/begin')
    const cred = (await navigator.credentials.get(toRequestOptions(options))) as PublicKeyCredential
    if (!cred) throw new Error('Fingerprint prompt was cancelled')
    await client.post('/auth/webauthn/login/finish', serializeAssertion(cred))
  },
}
