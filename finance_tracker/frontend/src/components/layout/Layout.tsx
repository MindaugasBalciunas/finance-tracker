import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import clsx from 'clsx'
import DateRangeFilter from '../ui/DateRangeFilter'
import SecurityModal from '../ui/SecurityModal'
import DataModal from '../ui/DataModal'
import ViewInsightBar from '../ui/ViewInsightBar'
// The single mobile menu is the top hamburger: date range + the tools that
// don't earn a bottom tab (Assets, Data, Security, AI settings, API docs).

// Desktop shows every page; the mobile bottom bar keeps the daily drivers
// as icon-only tabs (Labels is desktop-only to keep mobile lean).
const navItems = [
  { to: '/', label: 'Dashboard', short: 'Home', icon: '📊' },
  { to: '/transactions', label: 'Transactions', short: 'Txns', icon: '💸' },
  { to: '/balances', label: 'Balances', short: 'Bal', icon: '🏦' },
  { to: '/stocks', label: 'Stocks', short: 'Stocks', icon: '📉' },
  { to: '/assets', label: 'Assets', short: 'Assets', icon: '🏠' },
  { to: '/budget', label: 'Budget', short: 'Budget', icon: '🎯' },
  { to: '/reports', label: 'Reports', short: 'Reports', icon: '📈' },
  { to: '/labels', label: 'Labels', short: 'Labels', icon: '🏷️' },
  { to: '/ai', label: 'AI', short: 'AI', icon: '✦' },
]

// Routes that get the auto AI review bar (the AI page reviews itself, and
// More is chrome, not data).
const VIEW_BY_PATH: Record<string, string> = {
  '/': 'dashboard',
  '/transactions': 'transactions',
  '/balances': 'balances',
  '/stocks': 'stocks',
  '/assets': 'assets',
  '/budget': 'budget',
  '/reports': 'reports',
  '/labels': 'labels',
}

// Icon-only tabs (labels hidden to save space) — aria-labels carry the names.
const bottomBarItems = [
  { to: '/', name: 'Dashboard', icon: '📊' },
  { to: '/transactions', name: 'Transactions', icon: '💸' },
  { to: '/balances', name: 'Balances', icon: '🏦' },
  { to: '/stocks', name: 'Stocks', icon: '📉' },
  { to: '/budget', name: 'Budget', icon: '🎯' },
  { to: '/reports', name: 'Reports', icon: '📈' },
  { to: '/ai', name: 'AI', icon: '✦' },
]

export default function Layout() {
  const [securityOpen, setSecurityOpen] = useState(false)
  const [dataMode, setDataMode] = useState<'backup' | 'export' | null>(null)
  const [dateMenuOpen, setDateMenuOpen] = useState(false)
  // Navigating away closes the date menu so it never lingers over content.
  const location = useLocation()
  useEffect(() => { setDateMenuOpen(false) }, [location.pathname])

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">

      {/* ── Desktop header ── */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-10 hidden md:block">
        <div className="w-full px-6 flex items-center gap-4 h-14 overflow-visible">
          <div className="flex items-center gap-2 shrink-0">
            <h1 className="text-base font-bold text-gray-900">Finance Tracker</h1>
          </div>
          <nav className="flex items-center gap-1 shrink-0">
            {navItems.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.to === '/'}
                className={({ isActive }) => clsx(
                  'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors',
                  isActive ? 'bg-blue-50 text-blue-700' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                )}>
                <span>{item.icon}</span>{item.label}
              </NavLink>
            ))}
          </nav>
          <div className="flex-1 flex justify-center min-w-0">
            <DateRangeFilter />
          </div>
          <div className="flex items-center gap-3 shrink-0">
            <button onClick={() => setDataMode('backup')}
              className="text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors">
              💾 Backup
            </button>
            <button onClick={() => setDataMode('export')}
              className="text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors">
              🤖 Export to AI
            </button>
            <button onClick={() => setSecurityOpen(true)}
              className="text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors">
              Security
            </button>
            <a href="/swagger/index.html" target="_blank" rel="noopener noreferrer"
              className="text-xs text-blue-600 hover:underline">API Docs</a>
          </div>
        </div>
      </header>

      {/* ── Mobile header — title + one hamburger that opens the date picker.
             Page navigation lives in the bottom bar; tools live in "More". ── */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-20 md:hidden">
        <div className="flex items-center justify-between px-4 h-12">
          <h1 className="text-base font-bold text-gray-900">Finance Tracker</h1>
          <button
            onClick={() => setDateMenuOpen((o) => !o)}
            aria-label="Menu"
            className={clsx('p-2 rounded-lg transition-colors', dateMenuOpen ? 'bg-gray-100 text-gray-900' : 'text-gray-600 hover:bg-gray-100')}
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
        </div>
        {dateMenuOpen && (
          <div className="px-3 pb-3 border-t border-gray-100 pt-2 space-y-3">
            <div>
              <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Date range</p>
              <DateRangeFilter className="flex-wrap" />
            </div>
            <div className="border-t border-gray-100 pt-2 grid grid-cols-2 gap-1.5">
              <NavLink to="/assets" onClick={() => setDateMenuOpen(false)}
                className="px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">🏠 Assets</NavLink>
              <NavLink to="/labels" onClick={() => setDateMenuOpen(false)}
                className="px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">🏷️ Labels</NavLink>
              <button onClick={() => { setDataMode('backup'); setDateMenuOpen(false) }}
                className="text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">💾 Backup &amp; restore</button>
              <button onClick={() => { setDataMode('export'); setDateMenuOpen(false) }}
                className="text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">🤖 Export to AI</button>
              <button onClick={() => { setSecurityOpen(true); setDateMenuOpen(false) }}
                className="text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg">🔒 Security</button>
              <a href="/swagger/index.html" target="_blank" rel="noopener noreferrer"
                className="px-3 py-2 text-sm text-blue-600 hover:bg-gray-50 rounded-lg">📚 API docs</a>
            </div>
          </div>
        )}
      </header>

      {/* ── Backup & restore / Export to AI ── */}
      {dataMode && <DataModal mode={dataMode} onClose={() => setDataMode(null)} />}

      {/* ── Security settings ── */}
      {securityOpen && <SecurityModal onClose={() => setSecurityOpen(false)} />}

      {/* ── Main content ── */}
      <main className="flex-1 w-full max-w-screen-2xl mx-auto px-3 py-4 pb-24 md:px-6 md:py-6 md:pb-6">
        {VIEW_BY_PATH[location.pathname] && <ViewInsightBar view={VIEW_BY_PATH[location.pathname]} />}
        <Outlet />
      </main>

      {/* ── Mobile bottom tab bar ── */}
      <nav className="fixed bottom-0 left-0 right-0 bg-white border-t border-gray-200 z-10 md:hidden pb-[env(safe-area-inset-bottom)]">
        <div className="grid grid-cols-7 h-14">
          {bottomBarItems.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.to === '/'}
              aria-label={item.name} title={item.name}
              className={({ isActive }) => clsx(
                'flex items-center justify-center transition-colors min-w-0 rounded-lg m-1',
                isActive ? 'text-blue-600 bg-blue-50' : 'text-gray-500'
              )}>
              <span className="text-xl leading-none">{item.icon}</span>
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}
