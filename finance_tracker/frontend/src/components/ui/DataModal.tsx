import { useState, useRef, useEffect, useMemo } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'

interface Props {
  onClose: () => void
  // 'backup' = backup/restore/imports + danger zone; 'export' = AI exports.
  mode: 'backup' | 'export'
}

type ExportStatusData = {
  last_full_export: string | null
  last_partial_export: string | null
}

type ImportResult = {
  imported: { transactions: number; balances: number; stock_trades: number; assets: number; budgets: number; label_rules: number }
  skipped:  { transactions: number; balances: number; stock_trades: number; assets: number; budgets: number; label_rules: number }
  imported_tx_ids?: number[]
  relabeled: number
}

type PeriodPreset = 'all' | 'ytd' | '12m' | 'month' | 'custom'

const PRESETS: { key: PeriodPreset; label: string }[] = [
  { key: 'all', label: 'All time' },
  { key: 'ytd', label: 'This year' },
  { key: '12m', label: 'Last 12 mo' },
  { key: 'month', label: 'This month' },
  { key: 'custom', label: 'Custom' },
]

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10)
}

export default function DataModal({ onClose, mode }: Props) {
  const qc = useQueryClient()
  const [status, setStatus] = useState<ExportStatusData | null>(null)
  const [preset, setPreset] = useState<PeriodPreset>('all')
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')

  const [importing, setImporting] = useState(false)
  const [undoing, setUndoing] = useState(false)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const swedRef = useRef<HTMLInputElement>(null)
  const swedModeRef = useRef<'import' | 'enrich'>('import')
  const [swedImporting, setSwedImporting] = useState(false)
  const [swedResult, setSwedResult] = useState<{
    imported: number; duplicate: number; internal: number; relabeled: number
    balances?: number; enriched?: number; unmatched?: number
    date_from?: string; date_to?: string
  } | null>(null)
  const invlRef = useRef<HTMLInputElement>(null)
  const [invlImporting, setInvlImporting] = useState(false)
  const [invlResult, setInvlResult] = useState<{
    points: number; enriched: number; created: number; skipped: number
    tx_created: number; tx_skipped: number
    date_from: string; date_to: string
  } | null>(null)

  useEffect(() => {
    fetch('/api/v1/export/status').then(r => r.json()).then(setStatus).catch(() => {})
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  // Query string for the selected export period; '' means everything.
  const rangeQuery = useMemo(() => {
    const now = new Date()
    let from = ''
    let to = ''
    if (preset === 'ytd') from = `${now.getFullYear()}-01-01`
    if (preset === '12m') {
      const start = new Date(now)
      start.setFullYear(start.getFullYear() - 1)
      from = isoDay(start)
    }
    if (preset === 'month') from = `${isoDay(now).slice(0, 8)}01`
    if (preset === 'custom') { from = customFrom; to = customTo }
    const params = new URLSearchParams()
    if (from) params.set('from', from)
    if (to) params.set('to', to)
    const qs = params.toString()
    return qs ? `?${qs}` : ''
  }, [preset, customFrom, customTo])

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
      setError(err instanceof Error ? err.message : 'Restore failed')
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
      setResult(null)
      qc.invalidateQueries()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Undo failed')
    } finally { setUndoing(false) }
  }

  const handleSwedbankFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setSwedImporting(true); setSwedResult(null); setError(null)
    try {
      const form = new FormData()
      form.append('file', file)
      const url = swedModeRef.current === 'enrich' ? '/api/v1/import/swedbank?mode=enrich' : '/api/v1/import/swedbank'
      const res = await fetch(url, { method: 'POST', body: form })
      if (!res.ok) { const b = await res.json().catch(() => ({})); throw new Error(b.error ?? `HTTP ${res.status}`) }
      setSwedResult(await res.json())
      qc.invalidateQueries()
      e.target.value = ''
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Statement import failed')
    } finally { setSwedImporting(false) }
  }

  const handleInvlFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setInvlImporting(true); setInvlResult(null); setError(null)
    try {
      const form = new FormData()
      form.append('file', file)
      const res = await fetch('/api/v1/import/invl', { method: 'POST', body: form })
      if (!res.ok) { const b = await res.json().catch(() => ({})); throw new Error(b.error ?? `HTTP ${res.status}`) }
      setInvlResult(await res.json())
      qc.invalidateQueries()
      e.target.value = ''
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Pension statement import failed')
    } finally { setInvlImporting(false) }
  }

  const handleDeleteAll = async () => {
    if (!confirm('Delete ALL transactions, balances, stock trades and assets? This cannot be undone.')) return
    if (!confirm('Are you sure? Download a full backup first if you have not.')) return
    setDeleting(true)
    try {
      const responses = await Promise.all([
        fetch('/api/v1/transactions', { method: 'DELETE' }),
        fetch('/api/v1/balances', { method: 'DELETE' }),
        fetch('/api/v1/stocks', { method: 'DELETE' }),
        fetch('/api/v1/assets', { method: 'DELETE' }),
      ])
      if (responses.some(r => !r.ok)) throw new Error('Some data could not be deleted')
      qc.invalidateQueries()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    } finally { setDeleting(false) }
  }

  const exportLinkCls = 'flex items-center gap-2 px-3 py-2.5 sm:py-2 text-sm text-gray-700 bg-gray-50 hover:bg-gray-100 rounded-lg transition-colors'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-3 sm:p-6 overflow-y-auto"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-lg max-h-[92vh] overflow-y-auto">
        <div className="flex items-center justify-between px-4 sm:px-5 py-3 border-b border-gray-100 sticky top-0 bg-white rounded-t-2xl">
          <h2 className="font-semibold text-gray-900">{mode === 'backup' ? <>💾 Backup &amp; restore</> : <>🤖 Export to AI</>}</h2>
          <button onClick={onClose} className="p-1 text-gray-400 hover:text-gray-700">✕</button>
        </div>

        <div className="p-4 sm:p-5 space-y-6">
          {/* ── Backup & restore ── */}
          {mode === 'backup' && (
          <section>
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Backup &amp; restore</h3>
            <div className="space-y-2">
              <a href="/api/v1/export/finances.json" download onClick={() => setStatus(null)}
                className="flex items-start gap-3 p-3 rounded-xl border border-blue-100 bg-blue-50/60 hover:bg-blue-50 transition-colors">
                <span className="text-lg leading-none mt-0.5">⬇</span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-blue-900">Download full backup (JSON)</span>
                  <span className="block text-xs text-blue-700/70 mt-0.5">
                    Everything: transactions, balance snapshots, stock trades, assets, budgets, label rules &amp; settings.
                    Saved as backup_finances_&lt;date&gt;.json.
                    {status?.last_full_export && <> Last backup: {status.last_full_export}.</>}
                  </span>
                </span>
              </a>
              <button onClick={() => { setResult(null); setError(null); fileRef.current?.click() }} disabled={importing}
                className="w-full flex items-start gap-3 p-3 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors text-left disabled:opacity-50">
                <span className="text-lg leading-none mt-0.5">⬆</span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-gray-900">{importing ? 'Restoring…' : 'Restore from backup'}</span>
                  <span className="block text-xs text-gray-500 mt-0.5">
                    Pick a backup_finances_*.json file (partial_finances_*.json works too). Safe to re-run: existing records are skipped, labels are re-applied.
                  </span>
                </span>
              </button>
              <input ref={fileRef} type="file" accept=".json,application/json" className="hidden" onChange={handleFile} />
              <button onClick={() => { setSwedResult(null); setError(null); swedModeRef.current = 'import'; swedRef.current?.click() }} disabled={swedImporting}
                className="w-full flex items-start gap-3 p-3 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors text-left disabled:opacity-50">
                <span className="text-lg leading-none mt-0.5">🏦</span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-gray-900">{swedImporting ? 'Importing statement…' : 'Import Swedbank statement (CSV)'}</span>
                  <span className="block text-xs text-gray-500 mt-0.5">
                    Auto-categorizes, labels, and skips rows you already have — safe to re-run.
                  </span>
                </span>
              </button>
              <button onClick={() => { setSwedResult(null); setError(null); swedModeRef.current = 'enrich'; swedRef.current?.click() }} disabled={swedImporting}
                className="w-full flex items-start gap-3 p-3 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors text-left disabled:opacity-50">
                <span className="text-lg leading-none mt-0.5">🪄</span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-gray-900">{swedImporting ? 'Working…' : 'Enrich comments from statement (CSV)'}</span>
                  <span className="block text-xs text-gray-500 mt-0.5">
                    Adds statement detail ("Būsto paskola", "Vaikams"…) to matching rows. Never creates or overwrites hand-written comments.
                  </span>
                </span>
              </button>
              <input ref={swedRef} type="file" accept=".csv,text/csv" className="hidden" onChange={handleSwedbankFile} />
              <button onClick={() => { setInvlResult(null); setError(null); invlRef.current?.click() }} disabled={invlImporting}
                className="w-full flex items-start gap-3 p-3 rounded-xl border border-gray-200 hover:bg-gray-50 transition-colors text-left disabled:opacity-50">
                <span className="text-lg leading-none mt-0.5">🏛️</span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-gray-900">{invlImporting ? 'Importing statement…' : 'Import INVL pension statement (CSV)'}</span>
                  <span className="block text-xs text-gray-500 mt-0.5">
                    Restores the Artea balance history from the fund's unit ledger. Months already tracking Artea are left untouched — safe to re-run.
                  </span>
                </span>
              </button>
              <input ref={invlRef} type="file" accept=".csv,text/csv" className="hidden" onChange={handleInvlFile} />
              <p className="text-[11px] text-gray-400 px-1">
                App lock (PIN/fingerprint) is device-specific and intentionally not part of backups.
              </p>
            </div>

            {swedResult && (
              <div className="mt-2 p-3 rounded-lg bg-green-50 border border-green-200 text-xs text-green-800 space-y-1">
                <p className="font-semibold">{swedResult.enriched !== undefined && swedModeRef.current === 'enrich' ? 'Comments enriched' : 'Statement imported'}</p>
                {swedModeRef.current === 'enrich' ? (
                  <p>{swedResult.enriched} comments enriched · {swedResult.duplicate} already descriptive · {swedResult.unmatched} left untouched (nothing added)</p>
                ) : (
                  <p>
                    +{swedResult.imported} transactions
                    {swedResult.date_from && <> ({swedResult.date_from} → {swedResult.date_to})</>}
                    {' '}· {swedResult.duplicate} already existed · {swedResult.internal} own-account transfers skipped
                    {(swedResult.balances ?? 0) > 0 && <> · {swedResult.balances} monthly balance snapshots</>}
                  </p>
                )}
                {swedResult.relabeled > 0 && <p className="text-green-700/80">{swedResult.relabeled} transactions labeled by rules.</p>}
                <button onClick={() => setSwedResult(null)} className="text-gray-500 underline">Close</button>
              </div>
            )}

            {invlResult && (
              <div className="mt-2 p-3 rounded-lg bg-green-50 border border-green-200 text-xs text-green-800 space-y-1">
                <p className="font-semibold">Artea history restored</p>
                <p>
                  {invlResult.points} valuation points ({invlResult.date_from} → {invlResult.date_to})
                  {' '}· {invlResult.enriched} snapshots gained an Artea value · {invlResult.created} new snapshots
                  {invlResult.skipped > 0 && <> · {invlResult.skipped} already tracked</>}
                </p>
                <p>
                  +{invlResult.tx_created} transactions (payroll contributions & payouts)
                  {invlResult.tx_skipped > 0 && <> · {invlResult.tx_skipped} already existed</>}
                </p>
                <button onClick={() => setInvlResult(null)} className="text-gray-500 underline">Close</button>
              </div>
            )}

            {error && (
              <div className="mt-2 p-3 rounded-lg bg-red-50 border border-red-200 text-xs text-red-700 flex items-start justify-between gap-2">
                <span>{error}</span>
                <button onClick={() => setError(null)} className="text-red-400 hover:text-red-600">✕</button>
              </div>
            )}
            {result && (
              <div className="mt-2 p-3 rounded-lg bg-green-50 border border-green-200 text-xs text-green-800 space-y-1">
                <p className="font-semibold">Restore complete</p>
                <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
                  <span>Transactions: +{result.imported.transactions} ({result.skipped.transactions} existed)</span>
                  <span>Snapshots: +{result.imported.balances} ({result.skipped.balances} existed)</span>
                  <span>Stock trades: +{result.imported.stock_trades} ({result.skipped.stock_trades} existed)</span>
                  <span>Assets: +{result.imported.assets} ({result.skipped.assets} existed)</span>
                  <span>Budgets: +{result.imported.budgets} ({result.skipped.budgets} existed)</span>
                  <span>Label rules: +{result.imported.label_rules} ({result.skipped.label_rules} existed)</span>
                </div>
                {result.relabeled > 0 && <p className="text-green-700/80">{result.relabeled} transactions re-labeled by rules.</p>}
                <div className="flex items-center gap-3 pt-1">
                  <button onClick={() => setResult(null)} className="text-gray-500 underline">Close</button>
                  {(result.imported_tx_ids?.length ?? 0) > 0 && (
                    <button onClick={handleUndo} disabled={undoing} className="text-red-600 underline disabled:opacity-50">
                      {undoing ? 'Undoing…' : `Undo (delete ${result.imported_tx_ids!.length} imported)`}
                    </button>
                  )}
                </div>
              </div>
            )}
          </section>
          )}

          {/* ── Export for AI — everything meant to leave the app for
                 analysis: the AI dataset, incremental JSON and period CSVs. ── */}
          {mode === 'export' && (
          <section>
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-1">Export for AI</h3>
            <p className="text-[11px] text-gray-400 mb-2">
              Files sized for AI chat uploads and analysis. Pick a period for the CSVs; the dataset ZIP always
              covers full history.
            </p>
            <div className="grid sm:grid-cols-2 gap-2 mb-3">
              <a href="/api/v1/export/ai.zip" download className={exportLinkCls}>
                <span className="min-w-0">
                  🤖 AI dataset (ZIP)
                  <span className="block text-[11px] text-gray-400">
                    Per-year CSVs + README — sized for AI chat uploads, full history
                  </span>
                </span>
              </a>
              <a href="/api/v1/export/finances-partial.json" download onClick={() => setStatus(null)} className={exportLinkCls}>
                <span className="min-w-0">
                  ✨ New since last backup
                  <span className="block text-[11px] text-gray-400">
                    {status?.last_full_export ? `since ${status.last_full_export}` : 'make a full backup first'}
                  </span>
                </span>
              </a>
            </div>
            <div className="flex flex-wrap gap-1.5 mb-2">
              {PRESETS.map(p => (
                <button key={p.key} onClick={() => setPreset(p.key)}
                  className={clsx('px-2.5 py-1 rounded-full text-xs font-medium transition-colors',
                    preset === p.key ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200')}>
                  {p.label}
                </button>
              ))}
            </div>
            {preset === 'custom' && (
              <div className="flex items-center gap-2 mb-2">
                <input type="date" value={customFrom} onChange={e => setCustomFrom(e.target.value)}
                  className="flex-1 min-w-0 px-2 py-1.5 text-sm border border-gray-200 rounded-lg" aria-label="From date" />
                <span className="text-gray-400 text-xs shrink-0">to</span>
                <input type="date" value={customTo} onChange={e => setCustomTo(e.target.value)}
                  className="flex-1 min-w-0 px-2 py-1.5 text-sm border border-gray-200 rounded-lg" aria-label="To date" />
              </div>
            )}
            <div className="grid sm:grid-cols-2 gap-2">
              <a href={`/api/v1/export/transactions.csv${rangeQuery}`} download className={exportLinkCls}>
                📄 Transactions CSV
              </a>
              <a href={`/api/v1/export/balances.csv${rangeQuery}`} download className={exportLinkCls}>
                📄 Balances CSV
              </a>
            </div>
            {error && (
              <div className="mt-3 p-3 rounded-lg bg-red-50 border border-red-200 text-xs text-red-700 flex items-start justify-between gap-2">
                <span>{error}</span>
                <button onClick={() => setError(null)} className="text-red-400 hover:text-red-600">✕</button>
              </div>
            )}
          </section>
          )}

          {/* ── Danger zone ── */}
          {mode === 'backup' && (
          <section>
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Danger zone</h3>
            <button onClick={handleDeleteAll} disabled={deleting}
              className="w-full text-left px-3 py-2 text-sm text-red-600 border border-red-200 hover:bg-red-50 rounded-lg disabled:opacity-50 transition-colors">
              {deleting ? 'Deleting…' : '🗑 Delete all data'}
            </button>
            <p className="text-[11px] text-gray-400 mt-1 px-1">
              Removes every transaction, snapshot, trade and asset. To move to a fresh device: full backup → delete → restore.
            </p>
          </section>
          )}
        </div>
      </div>
    </div>
  )
}
