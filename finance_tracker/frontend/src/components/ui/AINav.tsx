import { NavLink } from 'react-router-dom'
import clsx from 'clsx'

// Sub-navigation for the two AI pages: the full-screen chat and the
// financial overview (analysis + gateway settings).
const tabs = [
  { to: '/ai', label: '💬 Chat' },
  { to: '/ai/overview', label: '✦ Overview' },
]

export default function AINav() {
  return (
    <div className="flex gap-1 bg-gray-100 rounded-xl p-1 w-fit shrink-0">
      {tabs.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end
          className={({ isActive }) => clsx(
            'px-3 py-1.5 rounded-lg text-sm font-medium transition-colors whitespace-nowrap',
            isActive ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-800'
          )}
        >
          {t.label}
        </NavLink>
      ))}
    </div>
  )
}
