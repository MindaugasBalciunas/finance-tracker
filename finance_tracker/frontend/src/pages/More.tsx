import { useState } from 'react'
import { Link } from 'react-router-dom'
import DataModal from '../components/ui/DataModal'
import SecurityModal from '../components/ui/SecurityModal'
import { useAIAvailable } from '../hooks/useInsights'
import { useBankSettings, useStagedTransactions } from '../hooks/useBanking'

// "More" collects the pages and tools that don't earn a bottom-bar tab on
// mobile: secondary pages, backup/import, security and the API docs.
export default function More() {
  const [dataMode, setDataMode] = useState<'backup' | 'export' | null>(null)
  const [securityOpen, setSecurityOpen] = useState(false)
  const aiOn = useAIAvailable()
  // The badge is the only nudge that bank rows are waiting — there is no
  // background sync, so nothing else would mention them.
  const { data: bankSettings } = useBankSettings()
  const { data: staged } = useStagedTransactions({ state: 'staged', page_size: 1 }, !!bankSettings?.configured)
  const pendingBank = staged?.total ?? 0

  const pageLinks = [
    { to: '/assets', icon: '🏠', title: 'Assets', hint: 'Property, vehicles and loans' },
    {
      to: '/labels', icon: '🏷️', title: 'Labels',
      hint: aiOn ? 'Label management, rules and AI tagging' : 'Label management and rules',
    },
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
          onClick={() => setDataMode('backup')}
          className="w-full flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50 text-left"
        >
          <span className="text-xl">💾</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">Backup &amp; restore</span>
            <span className="block text-xs text-gray-400">Full backup, restore and statement imports</span>
          </span>
          <span className="text-gray-300">›</span>
        </button>
        <button
          onClick={() => setDataMode('export')}
          className="w-full flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50 text-left"
        >
          <span className="text-xl">🤖</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">Export to AI</span>
            <span className="block text-xs text-gray-400">AI dataset ZIP, incremental JSON and period CSVs</span>
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
        <Link to="/banking" className="flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50">
          {/* 🔗 rather than 🏦 — the bottom bar already uses 🏦 for Balances. */}
          <span className="text-xl">🔗</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">Bank connections</span>
            <span className="block text-xs text-gray-400">
              {bankSettings?.configured
                ? 'Pull recent transactions straight from Swedbank and SEB'
                : 'Not set up — connect a bank to import without CSV files'}
            </span>
          </span>
          {pendingBank > 0 && (
            <span className="px-2 py-0.5 text-xs font-semibold rounded-full bg-indigo-100 text-indigo-700">
              {pendingBank}
            </span>
          )}
          <span className="text-gray-300">›</span>
        </Link>
        {/* Always listed, even with AI off — this is a way back to the
            switch. The AI page itself renders the settings panel. */}
        <Link to="/ai" className="flex items-center gap-3 px-4 py-3.5 hover:bg-gray-50">
          <span className="text-xl">✦</span>
          <span className="flex-1 min-w-0">
            <span className="block text-sm font-medium text-gray-800">AI settings</span>
            <span className="block text-xs text-gray-400">
              {aiOn ? 'Chat, analysis, provider and API key' : 'Switched off — tap to turn AI back on'}
            </span>
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

      {dataMode && <DataModal mode={dataMode} onClose={() => setDataMode(null)} />}
      {securityOpen && <SecurityModal onClose={() => setSecurityOpen(false)} />}
    </div>
  )
}