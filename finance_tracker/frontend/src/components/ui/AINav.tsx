import { useState } from 'react'
import { NavLink } from 'react-router-dom'
import clsx from 'clsx'
import { GatewaySettingsModal } from './GatewaySettings'
import ModelSelect from './ModelSelect'

// Sub-navigation for the AI pages: the full-screen chat, the financial
// overview (analysis) and the About-me context briefing. Rendered
// identically at the top of every AI page, so the gateway settings (opened
// via the ⚙️ button as a centered modal) are reachable from all of them.
const tabs = [
  { to: '/ai', label: '💬 Chat' },
  { to: '/ai/overview', label: '✦ Overview' },
  { to: '/ai/about', label: '🧠 About me' },
]

export default function AINav() {
  const [settingsOpen, setSettingsOpen] = useState(false)

  return (
    <div className="flex items-center justify-between gap-2">
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
      <div className="flex items-center gap-2 min-w-0">
        <ModelSelect />
        <button
          onClick={() => setSettingsOpen(true)}
          title="AI gateway settings"
          aria-label="AI gateway settings"
          className="shrink-0 bg-gray-100 rounded-xl px-3 py-1.5 text-sm font-medium text-gray-500 hover:text-gray-800"
        >
          ⚙️
        </button>
      </div>

      {settingsOpen && <GatewaySettingsModal onClose={() => setSettingsOpen(false)} />}
    </div>
  )
}
