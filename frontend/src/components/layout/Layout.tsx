import { NavLink, Outlet } from 'react-router-dom'
import clsx from 'clsx'

const navItems = [
  { to: '/', label: 'Dashboard', icon: '📊' },
  { to: '/transactions', label: 'Transactions', icon: '💸' },
  { to: '/balances', label: 'Balances', icon: '🏦' },
  { to: '/reports', label: 'Reports', icon: '📈' },
]

export default function Layout() {
  return (
    <div className="min-h-screen bg-gray-50 flex">
      {/* Sidebar */}
      <aside className="w-56 bg-white border-r border-gray-200 flex flex-col">
        <div className="px-6 py-5 border-b border-gray-200">
          <h1 className="text-xl font-bold text-gray-900">Finance Tracker</h1>
          <p className="text-xs text-gray-500 mt-0.5">Personal budget manager</p>
        </div>
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                clsx(
                  'flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-blue-50 text-blue-700'
                    : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                )
              }
            >
              <span>{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="px-6 py-4 border-t border-gray-200">
          <a
            href="http://localhost:8080/swagger/index.html"
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-blue-600 hover:underline"
          >
            API Docs (Swagger)
          </a>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto">
        <div className="max-w-7xl mx-auto px-6 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
