import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import clsx from 'clsx'
import DateRangeFilter from '../ui/DateRangeFilter'
import SecurityModal from '../ui/SecurityModal'
import DataModal from '../ui/DataModal'
// Note: the mobile menu lives solely in the bottom "More" tab (/more) — the
// old top-bar hamburger drawer was removed so there is one menu, not two.

// Desktop shows every page; the mobile bottom bar keeps only the daily
// drivers plus "More" (which holds Stocks, Assets, Data, Security — Labels
// is desktop-only to keep the mobile surface lean).
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

      {/* ── Mobile header — title + the date range (the only global control
             that needs to stay one tap away); everything else lives in the
             "More" tab so there is a single menu. ── */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-20 md:hidden">
        <div className="flex items-center gap-3 px-3 h-12">
          <h1 className="text-sm font-bold text-gray-900 shrink-0">Finance</h1>
          <div className="min-w-0 flex-1 overflow-x-auto no-scrollbar">
            <DateRangeFilter />
          </div>
        </div>
      </header>

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
