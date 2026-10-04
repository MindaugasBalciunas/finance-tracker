import { lazy, Suspense, useEffect, useState } from 'react'
import { HashRouter, Navigate, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api, ApiError } from './lib/api'
import { Icon } from './components/Icon'
import { Loading, ToastProvider } from './components/ui'
import Lock from './pages/Lock'
import { TxEditorProvider } from './components/TxEditor'

const Home = lazy(() => import('./pages/Home'))
const Ledger = lazy(() => import('./pages/Ledger'))
const Plan = lazy(() => import('./pages/Plan'))
const Wealth = lazy(() => import('./pages/Wealth'))
const Insights = lazy(() => import('./pages/Insights'))
const Assistant = lazy(() => import('./pages/Assistant'))
const Settings = lazy(() => import('./pages/Settings'))

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: (n, e) => !(e instanceof ApiError && e.status < 500) && n < 2,
    },
  },
})

const NAV = [
  { to: '/', label: 'Home', icon: 'home', color: 'var(--s1)' },
  { to: '/ledger', label: 'Ledger', icon: 'list', color: 'var(--s2)' },
  { to: '/plan', label: 'Plan', icon: 'target', color: 'var(--s3)' },
  { to: '/wealth', label: 'Wealth', icon: 'bank', color: 'var(--s7)' },
  { to: '/insights', label: 'Insights', icon: 'chart', color: 'var(--s5)' },
]
// Each section has its own hue when active (tinted pill on desktop, coloured icon on mobile).
const activeStyle = (color: string) => ({ color: `color-mix(in oklab, ${color} 85%, rgb(var(--ink)))`, background: `color-mix(in oklab, ${color} var(--tint), transparent)` })

export function applyTheme() {
  let t: string | null = null
  try {
    t = localStorage.getItem('theme')
  } catch {}
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t
  else delete document.documentElement.dataset.theme
}

function Shell() {
  const loc = useLocation()
  const nav = useNavigate()
  const { data: inbox } = useQuery({ queryKey: ['overview'], queryFn: () => api.get<any>('/overview'), select: (o) => o?.inbox_open ?? 0 })
  useEffect(() => {
    window.scrollTo(0, 0)
  }, [loc.pathname])
  // Bank consent redirect lands on ?code=&state= before the hash.
  useEffect(() => {
    const p = new URLSearchParams(window.location.search)
    if (p.get('code') || p.get('error')) {
      window.history.replaceState(null, '', window.location.pathname + window.location.hash)
      nav('/settings/banks?' + p.toString())
    }
  }, [nav])
  return (
    <div className="min-h-dvh sm:pl-56">
      {/* Desktop sidebar */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-56 flex-col border-r border-line bg-surface sm:flex">
        <div className="flex h-16 items-center gap-2 px-5 text-base font-semibold"><span className="grid h-7 w-7 place-items-center rounded-lg bg-accent text-white text-sm">€</span> Finance</div>
        <nav className="flex flex-1 flex-col gap-0.5 px-3">
          {NAV.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.to === '/'} style={({ isActive }) => (isActive ? activeStyle(n.color) : undefined)}
              className={({ isActive }) => clsx('flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium transition', isActive ? '' : 'text-ink2 hover:bg-sunken')}>
              <Icon name={n.icon} />
              {n.label}
              {n.to === '/ledger' && !!inbox && <span className="ml-auto rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white">{inbox}</span>}
            </NavLink>
          ))}
          <div className="my-2 border-t border-line" />
          <NavLink to="/ai" style={({ isActive }) => (isActive ? activeStyle('var(--s4)') : undefined)} className={({ isActive }) => clsx('flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium transition', isActive ? '' : 'text-ink2 hover:bg-sunken')}>
            <Icon name="spark" /> Ask CFO
          </NavLink>
        </nav>
        <div className="px-3 pb-4">
          <NavLink to="/settings" className={({ isActive }) => clsx('flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium transition', isActive ? 'bg-accent/10 text-accent' : 'text-ink2 hover:bg-sunken')}>
            <Icon name="settings" /> Settings
          </NavLink>
        </div>
      </aside>

      {/* Mobile top bar */}
      <header className="sticky top-0 z-30 flex h-12 items-center justify-between border-b border-line bg-page/90 px-4 backdrop-blur sm:hidden">
        <div className="flex items-center gap-2 text-sm font-semibold"><span className="grid h-6 w-6 place-items-center rounded-md bg-accent text-white text-xs">€</span> Finance</div>
        <div className="flex items-center gap-1">
          <NavLink to="/ai" className="btn-ghost h-9 w-9 px-0" aria-label="Ask CFO"><Icon name="spark" /></NavLink>
          <NavLink to="/settings" className="btn-ghost h-9 w-9 px-0" aria-label="Settings"><Icon name="settings" /></NavLink>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 pb-28 pt-4 sm:px-8 sm:pt-8 sm:pb-12">
        <Suspense fallback={<Loading />}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/ledger/*" element={<Ledger />} />
            <Route path="/plan/*" element={<Plan />} />
            <Route path="/wealth/*" element={<Wealth />} />
            <Route path="/insights/*" element={<Insights />} />
            <Route path="/ai" element={<Assistant />} />
            <Route path="/settings/*" element={<Settings />} />
            {/* v1 bookmarks */}
            <Route path="/banking" element={<Navigate to="/settings/banks" replace />} />
            <Route path="/transactions" element={<Navigate to="/ledger" replace />} />
            <Route path="/budget" element={<Navigate to="/plan" replace />} />
            <Route path="/balances" element={<Navigate to="/wealth" replace />} />
            <Route path="/stocks" element={<Navigate to="/wealth/investments" replace />} />
            <Route path="/reports" element={<Navigate to="/insights" replace />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </main>

      {/* Mobile tab bar */}
      <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-surface/95 backdrop-blur pb-safe sm:hidden">
        <div className="grid grid-cols-5">
          {NAV.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.to === '/'}
              className={({ isActive }) => clsx('relative flex h-14 flex-col items-center justify-center gap-0.5 text-[11px] font-medium', isActive ? 'text-ink' : 'text-muted')}>
              {({ isActive }) => (<>
                <span className="grid h-7 w-12 place-items-center rounded-full transition" style={isActive ? activeStyle(n.color) : undefined}><Icon name={n.icon} size={20} /></span>
                {n.label}
                {n.to === '/ledger' && !!inbox && <span className="absolute right-[22%] top-1.5 rounded-full bg-accent px-1 text-[10px] font-semibold leading-4 text-white">{inbox}</span>}
              </>)}
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}

function Gate() {
  const [locked, setLocked] = useState<boolean | null>(null)
  const check = () =>
    api.get<{ enabled: boolean; unlocked: boolean }>('/auth/status').then((s) => setLocked(s.enabled && !s.unlocked)).catch(() => setLocked(false))
  useEffect(() => {
    check()
    const on = () => setLocked(true)
    window.addEventListener('ft:locked', on)
    return () => window.removeEventListener('ft:locked', on)
  }, [])
  if (locked === null) return <Loading />
  if (locked) return <Lock onUnlock={() => { setLocked(false); qc.invalidateQueries() }} />
  return (
    <TxEditorProvider>
      <Shell />
    </TxEditorProvider>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <HashRouter>
          <Gate />
        </HashRouter>
      </ToastProvider>
    </QueryClientProvider>
  )
}
