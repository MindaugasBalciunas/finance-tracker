import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { shortDate } from '../../lib/format'
import { applyTheme } from '../../App'
import { Card, Segmented, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { DemoCard } from './Usage'

// ── data ────────────────────────────────────────────────────────────

export function Data() {
  const toast = useToast()
  const qc = useQueryClient()
  const { data: snaps } = useQuery({ queryKey: ['backups'], queryFn: () => api.get<any>('/backups') })
  const [busy, setBusy] = useState(false)
  const upload = async (path: string, file: File) => {
    if (!confirm('This REPLACES all data in the app with the file’s contents. A snapshot is taken first. Continue?')) return
    setBusy(true)
    try {
      const r = await fetch(`api${path}?confirm=replace`, { method: 'POST', body: await file.text(), headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin' })
      const d = await r.json()
      if (!r.ok) throw new Error(d.error)
      toast('Restored', 'good')
      qc.invalidateQueries()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }
  const pick = (path: string) => {
    const i = document.createElement('input')
    i.type = 'file'
    i.accept = 'application/json,.json'
    i.onchange = () => i.files?.[0] && upload(path, i.files[0])
    i.click()
  }
  return (
    <div className="max-w-2xl space-y-4">
      <DemoCard />
      <Card title="Backup">
        <div className="mb-3 text-sm text-muted">A full copy of every table. Without secrets it is safe to keep anywhere; a restore then keeps this instance's keys and PIN.{snaps?.last_download ? ` Last downloaded ${shortDate(snaps.last_download.slice(0, 10))}.` : ''}</div>
        <div className="flex flex-wrap gap-2">
          <a className="btn-primary" href="api/export/backup.json"><Icon name="download" size={16} />Download backup</a>
          <a className="btn-outline" href="api/export/backup.json?secrets=1" onClick={(e) => !confirm('This file will contain your AI key, bank private key and PIN hash. Store it safely.') && e.preventDefault()}>With secrets</a>
          <button className="btn-outline" onClick={() => pick('/import/backup')} disabled={busy}><Icon name="upload" size={16} />Restore…</button>
        </div>
      </Card>
      <Card title="Exports">
        <div className="flex flex-wrap gap-2">
          <a className="btn-outline" href="api/export/transactions.csv">Transactions CSV</a>
          <a className="btn-outline" href="api/export/ai.zip">Export for AI (zip)</a>
        </div>
        <div className="mt-2 text-xs text-muted">The AI export bundles every transaction, monthly cash flow and net worth, balances, loans and your brief with a README and a start-here PROMPT.md — for any AI assistant.</div>
      </Card>
      <Card title="Snapshots on the server" action={<button className="btn-ghost h-8 text-xs" onClick={async () => { await api.post('/backups'); qc.invalidateQueries({ queryKey: ['backups'] }); toast('Snapshot taken', 'good') }}>Take one now</button>}>
        <div className="max-h-64 divide-y divide-line overflow-y-auto text-sm">
          <div className="mb-1.5 text-xs text-warn">Snapshots are full copies of the database — including the bank connection, AI key and PIN hash. Keep downloads private. (The IBKR sign-in is left out.)</div>
          {(snaps?.snapshots ?? []).map((b: any) => (
            <a key={b.name} href={`api/backups/${b.name}`} className="flex justify-between py-1.5 hover:text-accent"><span className="font-mono text-xs">{b.name}</span><span className="text-xs text-muted">{(b.size / 1e6).toFixed(1)} MB</span></a>
          ))}
        </div>
      </Card>
      <Card title="Import from the old app (v1)">
        <div className="mb-2 text-sm text-muted">Converts a v1 finances.json backup — replaces everything. On first start the app already converted your v1 database automatically.</div>
        <button className="btn-outline" onClick={() => pick('/import/v1')} disabled={busy}>Import v1 backup…</button>
      </Card>
    </div>
  )
}

export function Appearance() {
  const [t, setT] = useState(() => { try { return localStorage.getItem('theme') || 'system' } catch { return 'system' } })
  const set = (v: string) => {
    setT(v)
    try { v === 'system' ? localStorage.removeItem('theme') : localStorage.setItem('theme', v) } catch {}
    applyTheme()
  }
  return (
    <Card title="Theme">
      <Segmented value={t} onChange={set} options={[{ value: 'system', label: 'System' }, { value: 'light', label: 'Light' }, { value: 'dark', label: 'Dark' }]} />
    </Card>
  )
}
