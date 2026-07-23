import { useState, useRef, useEffect } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import DateRangeFilter from '../ui/DateRangeFilter'

const navItems = [
  { to: '/', label: 'Dashboard', icon: '📊' },
  { to: '/transactions', label: 'Transactions', icon: '💸' },
  { to: '/balances', label: 'Balances', icon: '🏦' },
  { to: '/stocks', label: 'Stocks', icon: '📉' },
  { to: '/assets', label: 'Assets', icon: '🏠' },
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
  imported: { transactions: number; balances: number; stock_trades: number; assets: number }
  skipped:  { transactions: number; balances: number; stock_trades: number; assets: number }
  imported_tx_ids?: number[]
}

function ImportButton({ onDone }: { onDone?: () => void }) {
  const qc = useQueryClient()
  const [importing, setImporting] = useState(false)
  const [undoing, setUndoing] = useState(false)
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
      qc.invalidateQueries()
      e.target.value = ''
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  const handleUndo = async () => {
    if (!result?.imported_tx_ids?.length) return
    if (!confirm(`Delete the ${result.imported_tx_ids.length} transactions that were just imported?`)) return
    setUndoing(true)
    try {
      const res = await fetch('/api/v1/transactions/batch', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: result.imported_tx_ids }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      setResult(null)
      qc.invalidateQueries()
      onDone?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Undo failed')
    } finally {
      setUndoing(false)
    }
  }

  return (
    <div>
      <input ref={inputRef} type="file" accept=".json" className="hidden" onChange={handleFile} />
      <button
        onClick={() => { setResult(null); setError(null); inputRef.current?.click() }}
        disabled={importing}
        className="w-full text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg disabled:opacity-50 transition-colors"
      >
        {importing ? 'Importing…' : '⬆ Import JSON'}
      </button>
      {error && <p className="px-3 py-1 text-xs text-red-600">{error}</p>}
      {result && (
        <div className="px-3 py-2 text-xs text-green-700 bg-green-50 rounded-lg mx-3 mb-1">
          <p className="font-semibold">Import complete</p>
          <p>+{result.imported.transactions} tx, +{result.imported.balances} balances, +{result.imported.stock_trades} trades, +{result.imported.assets} assets</p>
          <p className="text-gray-400">skipped {result.skipped.transactions + result.skipped.balances + result.skipped.stock_trades + result.skipped.assets} duplicates</p>
          <div className="flex items-center gap-3 mt-2">
            <button onClick={() => { setResult(null); onDone?.() }} className="text-gray-500 underline">Close</button>
            {(result.imported_tx_ids?.length ?? 0) > 0 && (
              <button
                onClick={handleUndo}
                disabled={undoing}
                className="text-red-600 underline disabled:opacity-50"
              >
                {undoing ? 'Undoing…' : `Undo (delete ${result.imported_tx_ids!.length} imported)`}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function ExportSection() {
  const [status, setStatus] = useState<ExportStatusData | null>(null)

  useEffect(() => {
    fetch('/api/v1/export/status').then(r => r.json()).then(setStatus).catch(() => {})
  }, [])

  return (
    <div>
      <p className="px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wide">Export</p>
      {CSV_EXPORTS.map((item) => (
        <a key={item.href} href={item.href} download
          className="block px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg transition-colors">
          ⬇ {item.label}
        </a>
      ))}
      <a href="/api/v1/export/finances.json" download
        className="block px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg transition-colors">
        ⬇ All Data (Claude.ai)
        {status?.last_full_export && <span className="block text-xs text-gray-400 pl-4">last: {status.last_full_export}</span>}
      </a>
      <a href="/api/v1/export/finances-partial.json" download
        className="block px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 rounded-lg transition-colors">
        ⬇ New Data Since Last Export
      </a>
    </div>
  )
}

// Desktop-only export dropdown (unchanged from before)
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
      fetch('/api/v1/export/status').then(r => r.json()).then(setStatus).catch(() => {})
    }
  }, [open, status])

  return (
    <div ref={ref} className="relative">
      <button onClick={() => setOpen(v => !v)}
        className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors">
        Export
        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {open && (
        <div className="absolute right-0 mt-1 w-56 bg-white border border-gray-200 rounded-lg shadow-lg z-50 py-1">
          {CSV_EXPORTS.map((item) => (
            <a key={item.href} href={item.href} download onClick={() => setOpen(false)}
              className="flex items-center gap-2 px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors">
              <svg className="w-3.5 h-3.5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              {item.label}
            </a>
          ))}
          <div className="border-t border-gray-100 my-1" />
          <a href="/api/v1/export/finances.json" download onClick={() => { setStatus(null); setOpen(false) }}
            className="flex flex-col px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors">
            <span className="flex items-center gap-2">
              <svg className="w-3.5 h-3.5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              All Data for Claude.ai
            </span>
            {status?.last_full_export && <span className="text-gray-400 mt-0.5 pl-5">last: {status.last_full_export}</span>}
          </a>
          <a href="/api/v1/export/finances-partial.json" download onClick={() => { setStatus(null); setOpen(false) }}
            className="flex flex-col px-3 py-2 text-xs text-gray-700 hover:bg-gray-50 transition-colors">
            <span className="flex items-center gap-2">
              <svg className="w-3.5 h-3.5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              New Data Since Last Export
            </span>
            {status?.last_full_export
              ? <span className="text-gray-400 mt-0.5 pl-5">since: {status.last_full_export}</span>
              : <span className="text-gray-400 mt-0.5 pl-5">export full first</span>}
          </a>
        </div>
      )}
    </div>
  )
}

// Desktop import button (inline, small)
function DesktopImportButton() {
  const qc = useQueryClient()
  const [importing, setImporting] = useState(false)
  const [undoing, setUndoing] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) { setResult(null); setError(null) }
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setImporting(true); setResult(null); setError(null)
    try {
      const form = new FormData()
      form.append('file', file)
      const res = await fetch('/api/v1/import/json', { method: 'POST', body: form })
      if (!res.ok) { const b = await res.json().catch(() => ({})); throw new Error(b.error ?? `HTTP ${res.status}`) }
      setResult(await res.json())
      qc.invalidateQueries()
      e.target.value = ''
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally { setImporting(false) }
  }

  const handleUndo = async () => {
    if (!result?.imported_tx_ids?.length) return
    if (!confirm(`Delete the ${result.imported_tx_ids.length} transactions that were just imported?`)) return
    setUndoing(true)
    try {
      const res = await fetch('/api/v1/transactions/batch', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: result.imported_tx_ids }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      setResult(null); setError(null)
      qc.invalidateQueries()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Undo failed')
    } finally { setUndoing(false) }
  }

  return (
    <div ref={ref} className="relative">
      <input ref={inputRef} type="file" accept=".json" className="hidden" onChange={handleFile} />
      <button onClick={() => { setResult(null); setError(null); inputRef.current?.click() }} disabled={importing}
        className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 font-medium px-2 py-1 rounded-md hover:bg-gray-100 transition-colors disabled:opacity-50">
        {importing ? 'Importing…' : 'Import'}
      </button>
      {(result || error) && (
        <div className={`absolute right-0 mt-1 w-72 border rounded-lg shadow-lg z-50 p-3 text-xs ${error ? 'bg-red-50 border-red-200 text-red-700' : 'bg-green-50 border-green-200 text-green-800'}`}>
          <button onClick={() => { setResult(null); setError(null) }} className="absolute top-2 right-2 text-gray-400 hover:text-gray-600">✕</button>
          {error && <p>{error}</p>}
          {result && (<>
            <p className="font-semibold mb-1">Import complete</p>
            <p>Transactions: +{result.imported.transactions} ({result.skipped.transactions} skipped)</p>
            <p>Balances: +{result.imported.balances} ({result.skipped.balances} skipped)</p>
            <p>Stock trades: +{result.imported.stock_trades} ({result.skipped.stock_trades} skipped)</p>
            <p>Assets: +{result.imported.assets} ({result.skipped.assets} skipped)</p>
            {(result.imported_tx_ids?.length ?? 0) > 0 && (
              <button onClick={handleUndo} disabled={undoing}
                className="mt-2 text-red-600 underline disabled:opacity-50">
                {undoing ? 'Undoing…' : `Undo — delete ${result.imported_tx_ids!.length} imported transactions`}
              </button>
            )}
          </>)}
        </div>
      )}
    </div>
  )
}

function DesktopDeleteAllButton() {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handle(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  const handle = async () => {
    if (!confirm('Delete ALL transactions, balances, stock trades and assets? This cannot be undone.')) return
    setDeleting(true)
    try {
      await Promise.all([
        fetch('/api/v1/transactions', { method: 'DELETE' }),
        fetch('/api/v1/balances', { method: 'DELETE' }),
        fetch('/api/v1/stocks', { method: 'DELETE' }),
        fetch('/api/v1/assets', { method: 'DELETE' }),
      ])
      qc.invalidateQueries()
      setOpen(false)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div ref={ref} className="relative">
      <button onClick={() => setOpen(v => !v)}
        className="text-xs text-red-500 hover:text-red-700 font-medium px-2 py-1 rounded-md hover:bg-red-50 transition-colors">
        Delete all
      </button>
      {open && (
        <div className="absolute right-0 mt-1 w-56 bg-white border border-red-200 rounded-lg shadow-lg z-50 p-3">
          <p className="text-xs text-gray-700 font-semibold mb-1">Delete all data?</p>
          <p className="text-xs text-gray-400 mb-3">Permanently removes all transactions, balances, stock trades and assets.</p>
          <div className="flex gap-2">
            <button onClick={handle} disabled={deleting}
              className="flex-1 px-3 py-1.5 text-xs font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50">
              {deleting ? 'Deleting…' : 'Yes, delete all'}
            </button>
            <button onClick={() => setOpen(false)}
              className="flex-1 px-3 py-1.5 text-xs font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200">
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function DeleteAllTransactionsButton({ onDone }: { onDone?: () => void }) {
  const qc = useQueryClient()
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handle = async () => {
    if (!confirm('Delete ALL transactions, balances, stock trades and assets? This cannot be undone.')) return
    if (!confirm('Are you sure? Every transaction, balance, stock trade and asset will be permanently deleted.')) return
    setDeleting(true)
    setError(null)
    try {
      const [r1, r2, r3, r4] = await Promise.all([
        fetch('/api/v1/transactions', { method: 'DELETE' }),
        fetch('/api/v1/balances', { method: 'DELETE' }),
        fetch('/api/v1/stocks', { method: 'DELETE' }),
        fetch('/api/v1/assets', { method: 'DELETE' }),
      ])
      if (!r1.ok || !r2.ok || !r3.ok || !r4.ok) throw new Error(`HTTP ${r1.status}/${r2.status}/${r3.status}/${r4.status}`)
      qc.invalidateQueries()
      onDone?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div>
      <button
        onClick={handle}
        disabled={deleting}
        className="w-full text-left px-3 py-2 text-sm text-red-600 hover:bg-red-50 rounded-lg disabled:opacity-50 transition-colors"
      >
        {deleting ? 'Deleting…' : '🗑 Delete all transactions'}
      </button>
      {error && <p className="px-3 py-1 text-xs text-red-600">{error}</p>}
    </div>
  )
}

export default function Layout() {
  const [drawerOpen, setDrawerOpen] = useState(false)

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
            <DesktopImportButton />
            <ExportDropdown />
            <DesktopDeleteAllButton />
            <a href="http://localhost:8080/swagger/index.html" target="_blank" rel="noopener noreferrer"
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
                <DateRangeFilter />
              </div>
              <div className="border-t border-gray-100" />
              {/* Import */}
              <div>
                <p className="px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wide">Import</p>
                <ImportButton onDone={() => setDrawerOpen(false)} />
              </div>
              <div className="border-t border-gray-100" />
              {/* Export */}
              <ExportSection />
              <div className="border-t border-gray-100" />
              {/* Danger zone */}
              <div>
                <p className="px-3 py-1 text-xs font-semibold text-gray-400 uppercase tracking-wide">Danger zone</p>
                <DeleteAllTransactionsButton onDone={() => setDrawerOpen(false)} />
              </div>
              <div className="border-t border-gray-100" />
              <div className="px-3">
                <a href="/api/v1/swagger/index.html" target="_blank" rel="noopener noreferrer"
                  className="text-sm text-blue-600">API Docs</a>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Main content ── */}
      <main className="flex-1 w-full px-3 py-4 pb-24 md:px-6 md:py-6 md:pb-6">
        <Outlet />
      </main>

      {/* ── Mobile bottom tab bar ── */}
      <nav className="fixed bottom-0 left-0 right-0 bg-white border-t border-gray-200 z-10 md:hidden">
        <div className="grid grid-cols-6 h-16">
          {navItems.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.to === '/'}
              className={({ isActive }) => clsx(
                'flex flex-col items-center justify-center gap-0.5 text-xs font-medium transition-colors',
                isActive ? 'text-blue-600' : 'text-gray-500'
              )}>
              <span className="text-lg leading-none">{item.icon}</span>
              <span className="leading-none">{item.label.split(' ')[0]}</span>
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}
