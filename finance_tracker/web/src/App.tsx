import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { HashRouter, Navigate, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api, ApiError } from './lib/api'
import { Icon } from './components/Icon'
import { Logo, LogoMark } from './components/Logo'
import { usePrefs } from './lib/hooks'
import { setUsageEnabled, startUsage, trackView } from './lib/usage'
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

/** A sidebar entry: icon + label, or just the icon (with a tooltip) on the rail. */
function SideLink({ to, icon, label, color, collapsed = false, badge = 0 }: { to: string; icon: string; label: string; color: string; collapsed?: boolean; badge?: number }) {
  return (
    <NavLink to={to} end={to === '/'} title={collapsed ? label : undefined} style={({ isActive }) => (isActive ? activeStyle(color) : undefined)}
      className={({ isActive }) => clsx('relative flex h-10 items-center gap-3 rounded-xl text-sm font-medium transition', collapsed ? 'justify-center px-0' : 'px-3', isActive ? '' : 'text-ink2 hover:bg-sunken')}>
      <Icon name={icon} />
      {!collapsed && label}
      {!!badge && (collapsed
        ? <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-accent" />
        : <span className="ml-auto rounded-full bg-accent px-1.5 text-[11px] font-semibold text-white">{badge}</span>)}
    </NavLink>
  )
}

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
  // Desktop sidebar: full or icon rail (a per-device choice).
  const [collapsed, setCollapsedState] = useState(() => { try { return localStorage.getItem('nav-collapsed') === '1' } catch { return false } })
  const setCollapsed = (v: boolean) => { setCollapsedState(v); try { localStorage.setItem('nav-collapsed', v ? '1' : '0') } catch {} }
  const [drawer, setDrawer] = useState(false)
  useEffect(() => setDrawer(false), [loc.pathname])
  // --bleed-w: the full width next to the sidebar (minus the page gutters), so
  // a wide table can break out of the page's max width without moving the header.
  const shellRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = shellRef.current
    if (!el) return
    const update = () => {
      const cs = getComputedStyle(el)
      const main = el.querySelector('main')
      const gutter = main ? parseFloat(getComputedStyle(main).paddingLeft) : 16
      const w = el.clientWidth - parseFloat(cs.paddingLeft) - 2 * gutter
      document.documentElement.style.setProperty('--bleed-w', `${Math.max(0, w)}px`)
    }
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [collapsed])
  const { data: inbox } = useQuery({ queryKey: ['overview'], queryFn: () => api.get<any>('/overview'), select: (o) => o?.inbox_open ?? 0 })
  useEffect(() => {
    window.scrollTo(0, 0)
    trackView(loc.pathname)
  }, [loc.pathname])
  const { prefs } = usePrefs()
  useEffect(() => { setUsageEnabled(!prefs.usage_off); startUsage() }, [prefs.usage_off])
  // Bank consent redirect lands on ?code=&state= before the hash.
  useEffect(() => {
    const p = new URLSearchParams(window.location.search)
    if (p.get('code') || p.get('error')) {
      window.history.replaceState(null, '', window.location.pathname + window.location.hash)
      nav('/settings/banks?' + p.toString())
    }
  }, [nav])
  return (
    <div ref={shellRef} className={clsx('min-h-dvh transition-[padding]', collapsed ? 'sm:pl-16' : 'sm:pl-56')}>
      {/* Desktop sidebar — collapses to an icon rail */}
      <aside className={clsx('fixed inset-y-0 left-0 z-30 hidden flex-col border-r border-line bg-surface transition-[width] sm:flex', collapsed ? 'w-16' : 'w-56')}>
        <div className={clsx('flex h-16 items-center', collapsed ? 'justify-center' : 'justify-between pl-4 pr-2')}>
          {!collapsed && <Logo />}
          <button className="btn-ghost h-9 w-9 px-0" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? 'Expand menu' : 'Collapse menu'} title={collapsed ? 'Expand menu' : 'Collapse menu'}>
            {collapsed ? <LogoMark size={28} /> : <Icon name="menu" />}
          </button>
        </div>
        <nav className={clsx('flex flex-1 flex-col gap-0.5', collapsed ? 'px-2' : 'px-3')}>
          {NAV.map((n) => <SideLink key={n.to} to={n.to} icon={n.icon} label={n.label} color={n.color} collapsed={collapsed} badge={n.to === '/ledger' ? inbox : 0} />)}
          <div className="my-2 border-t border-line" />
          <SideLink to="/ai" icon="spark" label="Ask CFO" color="var(--s4)" collapsed={collapsed} />
        </nav>
        <div className={clsx('pb-4', collapsed ? 'px-2' : 'px-3')}>
          <SideLink to="/settings" icon="settings" label="Settings" color="var(--s-other)" collapsed={collapsed} />
        </div>
      </aside>

      {/* Mobile top bar with a hamburger menu */}
      <header className="sticky top-0 z-30 flex h-12 items-center justify-between border-b border-line bg-page/90 px-2 backdrop-blur sm:hidden">
        <div className="flex items-center gap-1">
          <button className="btn-ghost h-9 w-9 px-0" onClick={() => setDrawer(true)} aria-label="Open menu"><Icon name="menu" /></button>
          <Logo />
        </div>
        <div className="flex items-center gap-1">
          <NavLink to="/ai" className="btn-ghost h-9 w-9 px-0" aria-label="Ask CFO"><Icon name="spark" /></NavLink>
          <NavLink to="/settings" className="btn-ghost h-9 w-9 px-0" aria-label="Settings"><Icon name="settings" /></NavLink>
        </div>
      </header>
      {drawer && (
        <div className="fixed inset-0 z-50 sm:hidden" role="dialog" aria-modal>
          <div className="absolute inset-0 bg-black/40" onClick={() => setDrawer(false)} />
          <div className="absolute inset-y-0 left-0 flex w-72 max-w-[85vw] flex-col bg-surface shadow-xl">
            <div className="flex h-14 items-center justify-between border-b border-line pl-4 pr-2">
              <Logo />
              <button className="btn-ghost h-9 w-9 px-0" onClick={() => setDrawer(false)} aria-label="Close menu"><Icon name="x" /></button>
            </div>
            <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto p-3" onClick={() => setDrawer(false)}>
              {NAV.map((n) => <SideLink key={n.to} to={n.to} icon={n.icon} label={n.label} color={n.color} badge={n.to === '/ledger' ? inbox : 0} />)}
              <div className="my-2 border-t border-line" />
              <SideLink to="/ai" icon="spark" label="Ask CFO" color="var(--s4)" />
              <SideLink to="/insights/review" icon="chart" label="Month review" color="var(--s5)" />
              <SideLink to="/wealth/history" icon="calendar" label="Balance history" color="var(--s7)" />
              <div className="mt-auto" />
              <SideLink to="/settings" icon="settings" label="Settings" color="var(--s-other)" />
            </nav>
          </div>
        </div>
      )}

      {/* One width for every page (no jumps between tabs); wide enough for tables. */}
      <main className="mx-auto max-w-screen-2xl px-4 pb-28 pt-4 sm:px-6 sm:pt-8 sm:pb-12 lg:px-8">
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
