import { useState, useEffect, useCallback } from 'react'
import { authApi, type AuthStatus } from '../../api/auth'
import LockScreen from './LockScreen'
import LoadingSpinner from './LoadingSpinner'

// Wraps the app: checks the lock status on load and shows the lock screen
// until a valid session exists. Also re-locks when any API call returns 401
// (session expired), signalled via the 'ft-locked' window event.
export default function AuthGate({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [error, setError] = useState(false)

  const refresh = useCallback(() => {
    authApi
      .status()
      .then(setStatus)
      .catch(() => setError(true))
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    function onLocked() {
      setStatus((s) => (s ? { ...s, unlocked: false } : s))
    }
    window.addEventListener('ft-locked', onLocked)
    return () => window.removeEventListener('ft-locked', onLocked)
  }, [])

  if (error) {
    // Backend unreachable or too old to know about auth — don't brick the app.
    return <>{children}</>
  }
  if (!status) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <LoadingSpinner />
      </div>
    )
  }
  if (status.enabled && !status.unlocked) {
    return <LockScreen status={status} onUnlocked={refresh} />
  }
  return <>{children}</>
}
