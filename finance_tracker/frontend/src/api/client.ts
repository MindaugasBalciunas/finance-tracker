import axios from 'axios'

const client = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
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
    return Promise.reject(new Error(message))
  }
)

export default client
