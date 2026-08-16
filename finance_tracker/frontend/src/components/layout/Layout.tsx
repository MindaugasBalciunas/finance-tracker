import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import clsx from 'clsx'
import DateRangeFilter from '../ui/DateRangeFilter'
import SecurityModal from '../ui/SecurityModal'
import DataModal from '../ui/DataModal'

// Desktop shows every page; the mobile bottom bar keeps only the daily
// drivers plus "More" (which holds Stocks, Assets, Labels, Data, Security).
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

const bottomBarItems = [
  { to: '/', short: 'Home', icon: '📊' },
  { to: '/transactions', short: 'Txns', icon: '💸' },
  { to: '/balances', short: 'Bal', icon: '🏦' },
  { to: '/budget', short: 'Budget', icon: '🎯' },
  { to: '/reports', short: 'Reports', icon: '📈' },
  { to: '/ai', short: 'AI', icon: '✦' },
  { to: '/more', short: 'More', icon: '☰' },
]

export default function Layout() {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [securityOpen, setSecurityOpen] = useState(false)
  const [dataOpen, setDataOpen] = useState(false)

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
            <button onClick={() => setDataOpen(true)}
              className="text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors">
              💾 Data
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

      {/* ── Mobile header ── */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-20 md:hidden">
        <div className="flex items-center justify-between px-4 h-12">
          <h1 className="text-base font-bold text-gray-900">Finance Tracker</h1>
          <button onClick={() => setDrawerOpen(true)}
            className="p-2 rounded-lg text-gray-600 hover:bg-gray-100 transition-colors">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
        </div>
      </header>

      {/* ── Mobile drawer ── */}
      {drawerOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          {/* Backdrop */}
          <div className="absolute inset-0 bg-black/40" onClick={() => setDrawerOpen(false)} />
          {/* Panel */}
          <div className="absolute right-0 top-0 bottom-0 w-72 bg-white shadow-xl flex flex-col">
            <div className="flex items-center justify-between px-4 h-12 border-b border-gray-100">
              <span className="font-semibold text-gray-900 text-sm">Menu</span>
              <button onClick={() => setDrawerOpen(false)} className="p-1 text-gray-500 hover:text-gray-800">✕</button>
            </div>
            <div className="flex-1 overflow-y-auto py-3 space-y-4">
              {/* Date filter */}
              <div className="px-3">
                <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Date Range</p>
                <DateRangeFilter className="flex-wrap" />
              </div>
              <div className="border-t border-gray-100" />
              {/* Data */}
              <div>
                <p className="px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wide">Data</p>
                <button
                  onClick={() => { setDataOpen(true); setDrawerOpen(false) }}
                  className="w-full text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg transition-colors"
                >
                  💾 Backup, export &amp; restore
                </button>
              </div>
              <div className="border-t border-gray-100" />
              {/* Security */}
              <div>
                <p className="px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wide">Security</p>
                <button
                  onClick={() => { setSecurityOpen(true); setDrawerOpen(false) }}
                  className="w-full text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg transition-colors"
                >
                  🔒 App lock & fingerprint
                </button>
              </div>
              <div className="border-t border-gray-100" />
              <div className="px-3">
                <a href="/swagger/index.html" target="_blank" rel="noopener noreferrer"
                  className="text-sm text-blue-600">API Docs</a>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Data (backup / export / restore) ── */}
      {dataOpen && <DataModal onClose={() => setDataOpen(false)} />}

      {/* ── Security settings ── */}
      {securityOpen && <SecurityModal onClose={() => setSecurityOpen(false)} />}

      {/* ── Main content ── */}
      <main className="flex-1 w-full max-w-screen-2xl mx-auto px-3 py-4 pb-24 md:px-6 md:py-6 md:pb-6">
        <Outlet />
      </main>

      {/* ── Mobile bottom tab bar ── */}
      <nav className="fixed bottom-0 left-0 right-0 bg-white border-t border-gray-200 z-10 md:hidden pb-[env(safe-area-inset-bottom)]">
        <div className="grid grid-cols-7 h-16">
          {bottomBarItems.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.to === '/'}
              className={({ isActive }) => clsx(
                'flex flex-col items-center justify-center gap-1 text-[11px] font-medium transition-colors px-0.5 min-w-0',
                isActive ? 'text-blue-600' : 'text-gray-500'
              )}>
              <span className="text-lg leading-none">{item.icon}</span>
              <span className="leading-none truncate max-w-full">{item.short}</span>
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}
