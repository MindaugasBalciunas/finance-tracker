import axios from 'axios'

const client = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  // Above nginx's 300s AI window (backend bounds a chat to 240s), so a slow
  // AI answer surfaces the server's message instead of a client-side abort.
  timeout: 310_000,
})

client.interceptors.response.use(
  (res) => res,
  (err) => {
    // Session expired or app locked — tell the AuthGate to show the lock screen.
    // Auth endpoints handle their own 401s (e.g. wrong PIN).
    if (err.response?.status === 401 && !err.config?.url?.startsWith('/auth/')) {
      window.dispatchEvent(new Event('ft-locked'))
    }
    const message = err.response?.data?.error ?? err.message
    // Keep the HTTP status on the error so callers can tell a 429 from a 500.
    return Promise.reject(Object.assign(new Error(message), { status: err.response?.status }))
  }
)

export default client
