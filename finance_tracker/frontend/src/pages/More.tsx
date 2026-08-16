import { useState } from 'react'
import { Link } from 'react-router-dom'
import DataModal from '../components/ui/DataModal'
import SecurityModal from '../components/ui/SecurityModal'

// "More" collects the pages and tools that don't earn a bottom-bar tab on
// mobile: secondary pages, backup/import, security and the API docs.
export default function More() {
  const [dataOpen, setDataOpen] = useState(false)
  const [securityOpen, setSecurityOpen] = useState(false)

  // Labels is intentionally omitted on mobile to keep the surface lean — it
  // stays available on desktop. Stocks moved to the bottom tab bar.
  const pageLinks = [
    { to: '/assets', icon: '🏠', title: 'Assets', hint: 'Property, vehicles and loans' },
  ]

  return (
    <div className="p-4 sm:p-6 space-y-4 max-w-2xl mx-auto">
      <div>
        <h1 className="text-xl font-bold text-gray-900">☰ More</h1>
        <p className="text-xs text-gray-400">Everything that doesn't need a tab of its own</p>
      </div>

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm divide-y divide-gray-50">
        {pageLinks.map((l) => (
          <Link key={l.to} to={l.to} className="flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50">
            <span className="text-xl">{l.icon}</span>
            <span className="flex-1 min-w-0">
              <span className="block text-sm font-medium text-gray-800">{l.title}</span>
              <span className="block text-xs text-gray-400">{l.hint}</span>
            </span>
            <span className="text-gray-300">›</span>
          </Link>
        ))}
      </div>

      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm divide-y divide-gray-50">
        <button
          onClick={() => setDataOpen(true)}
          className="w-full flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50 text-left"
        >
          <span className="text-xl">💾</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">Data</span>
            <span className="block text-xs text-gray-400">Backup, export, statement imports & restore</span>
          </span>
          <span className="text-gray-300">›</span>
        </button>
        <button
          onClick={() => setSecurityOpen(true)}
          className="w-full flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50 text-left"
        >
          <span className="text-xl">🔒</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">Security</span>
            <span className="block text-xs text-gray-400">App lock, PIN and fingerprint</span>
          </span>
          <span className="text-gray-300">›</span>
        </button>
        <Link to="/ai" className="flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50">
          <span className="text-xl">✦</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">AI &amp; gateway settings</span>
            <span className="block text-xs text-gray-400">Chat, analysis and the nexos.ai API key</span>
          </span>
          <span className="text-gray-300">›</span>
        </Link>
        <a
          href="/swagger/index.html"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50"
        >
          <span className="text-xl">📚</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">API docs</span>
            <span className="block text-xs text-gray-400">Swagger UI for the REST API</span>
          </span>
          <span className="text-gray-300">↗</span>
        </a>
      </div>

      {dataOpen && <DataModal onClose={() => setDataOpen(false)} />}
      {securityOpen && <SecurityModal onClose={() => setSecurityOpen(false)} />}
    </div>
  )
}