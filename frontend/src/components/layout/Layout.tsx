import { useState, useRef, useEffect } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import clsx from 'clsx'
import DateRangeFilter from '../ui/DateRangeFilter'
import { useDateRange } from '../../context/DateRangeContext'

const navItems = [
  { to: '/', label: 'Dashboard', icon: '📊' },
  { to: '/transactions', label: 'Transactions', icon: '💸' },
  { to: '/balances', label: 'Balances', icon: '🏦' },
  { to: '/stocks', label: 'Stocks', icon: '📉' },
  { to: '/reports', label: 'Reports', icon: '📈' },
]

const CSV_EXPORTS = [
  { label: 'Transactions CSV', href: '/api/v1/export/transactions.csv' },
  { label: 'Balances CSV',     href: '/api/v1/export/balances.csv' },
]

type ExportStatusData = {
  last_full_export: string | null
  last_partial_export: string | null
}

type ImportResult = {
  imported: { transactions: number; balances: number; stock_trades: number }
  skipped:  { transactions: number; balances: number; stock_trades: number }
}

function ImportButton() {
  const [importing, setImporting] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setImporting(true)
    setResult(null)
    setError(null)
    try {
      const form = new FormData()
      form.append('file', file)
      const res = await fetch('/api/v1/import/json', { method: 'POST', body: form })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error ?? `HTTP ${res.status}`)
      }
      const data: ImportResult = await res.json()
      setResult(data)
      // Reset so same file can be re-selected
      e.target.value = ''
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="relative">
      <input
        ref={inputRef}
        type="file"
        accept=".json"
        className="hidden"
        onChange={handleFile}
      />
      <button
        onClick={() => { setResult(null); setError(null); inputRef.current?.click() }}
        disabled={importing}
        className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors disabled:opacity-50"
      >
        {importing ? 'Importing…' : 'Import'}
      </button>
      {(result || error) && (
        <div className={`absolute right-0 mt-1 w-64 border rounded-lg shadow-lg z-50 p-3 text-xs ${error ? 'bg-red-50 border-red-200 text-red-700' : 'bg-green-50 border-green-200 text-green-800'}`}>
          <button onClick={() => { setResult(null); setError(null) }} className="absolute top-2 right-2 text-gray-400 hover:text-gray-600">✕</button>
          {error && <p>{error}</p>}
          {result && (
            <>
              <p className="font-semibold mb-1">Import complete</p>
              <p>Transactions: +{result.imported.transactions} ({result.skipped.transactions} skipped)</p>
              <p>Balances: +{result.imported.balances} ({result.skipped.balances} skipped)</p>
              <p>Stock trades: +{result.imported.stock_trades} ({result.skipped.stock_trades} skipped)</p>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function ExportDropdown() {
  const [open, setOpen] = useState(false)
  const [status, setStatus] = useState<ExportStatusData | null>(null)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  useEffect(() => {
    if (open && !status) {
      fetch('/api/v1/export/status')
        .then((r) => r.json())
        .then(setStatus)
        .catch(() => {})
    }
  }, [open, status])

  const handleDownload = () => {
    setStatus(null) // refresh status after next open
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors"
      >
        Export
        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {open && (
        <div className="absolute right-0 mt-1 w-56 bg-white border border-gray-200 rounded-lg shadow-lg z-50 py-1">
          {CSV_EXPORTS.map((item) => (
            <a
              key={item.href}
              href={item.href}
              download
              onClick={() => setOpen(false)}
              className="flex items-center gap-2 px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <svg className="w-3.5 h-3.5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              {item.label}
            </a>
          ))}
          <div className="border-t border-gray-100 my-1" />
          <a
            href="/api/v1/export/finances.json"
            download
            onClick={handleDownload}
            className="flex flex-col px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors"
          >
            <span className="flex items-center gap-2">
              <svg className="w-3.5 h-3.5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              All Data for Claude.ai
            </span>
            {status?.last_full_export && (
              <span className="text-gray-400 mt-0.5 pl-5">last: {status.last_full_export}</span>
            )}
          </a>
          <a
            href="/api/v1/export/finances-partial.json"
            download
            onClick={handleDownload}
            className="flex flex-col px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors"
          >
            <span className="flex items-center gap-2">
              <svg className="w-3.5 h-3.5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              New Data Since Last Export
            </span>
            {status?.last_full_export
              ? <span className="text-gray-400 mt-0.5 pl-5">since: {status.last_full_export}</span>
              : <span className="text-gray-400 mt-0.5 pl-5">export full first</span>
            }
          </a>
        </div>
      )}
    </div>
  )
}

export default function Layout() {
  const { dateRange, setDateRange } = useDateRange()

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      {/* Top nav */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-10">
        <div className="w-full px-6 flex items-center gap-4 h-14 overflow-visible">
          <div className="flex items-center gap-2 shrink-0">
            <h1 className="text-base font-bold text-gray-900">Finance Tracker</h1>
          </div>
          <nav className="flex items-center gap-1 shrink-0">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  clsx(
                    'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors',
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
          <div className="flex-1 flex justify-center">
            <DateRangeFilter value={dateRange} onChange={setDateRange} />
          </div>
          <div className="flex items-center gap-3 shrink-0">
            <ImportButton />
            <ExportDropdown />
            <a
              href="http://localhost:8080/swagger/index.html"
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-blue-600 hover:underline"
            >
              API Docs
            </a>
          </div>
        </div>
      </header>

      {/* Main content — full width */}
      <main className="flex-1 w-full px-6 py-6">
        <Outlet />
      </main>
    </div>
  )
}
