import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { useRefresh } from '../lib/hooks'
import { useTxEditor } from './TxEditor'
import { Spinner, useToast } from './ui'
import { Icon } from './Icon'

function ago(iso: string) {
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60000)
  if (!isFinite(mins) || mins < 0) return ''
  if (mins < 2) return 'just now'
  if (mins < 60) return `${mins} min ago`
  const h = Math.round(mins / 60)
  if (h < 24) return `${h} h ago`
  return `${Math.round(h / 24)} d ago`
}

/** The ledger comes first: enter, scan or sync a transaction in one tap from
 *  the landing page, plus the two other things you do by hand — a transfer
 *  between your accounts and updating balances of manual accounts. */
export function QuickActions({ inboxOpen }: { inboxOpen: number }) {
  const editor = useTxEditor()
  const nav = useNavigate()
  const toast = useToast()
  const refresh = useRefresh()
  const { data: conns, refetch } = useQuery({ queryKey: ['bank-connections'], queryFn: () => api.get<any[]>('/bank/connections'), staleTime: 60_000, retry: false })
  const [syncing, setSyncing] = useState(false)
  const connected = (conns ?? []).filter((c) => c.status === 'authorized')
  const last = connected.flatMap((c) => (c.accounts ?? []).map((a: any) => a.last_synced_at as string)).filter(Boolean).sort().pop()
  const sync = async () => {
    if (!connected.length) return nav('/settings/banks')
    setSyncing(true)
    try {
      const r = await api.post<any>('/bank/sync', {})
      const failed = (r.accounts ?? []).filter((a: any) => a.error || a.skipped)
      toast(`${r.new} new · ${r.auto_linked} linked${failed.length ? ` · ${failed.length} account(s) need attention` : ''}`, failed.length ? 'bad' : 'good')
      refresh(); refetch()
      if (r.new > 0) nav('/ledger/inbox')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setSyncing(false)
    }
  }
  const tiles: { key: string; icon: string; label: string; sub?: string; onClick: () => void; primary?: boolean; busy?: boolean }[] = [
    { key: 'add', icon: 'plus', label: 'Add', sub: 'expense or income', onClick: () => editor.open(), primary: true },
    { key: 'transfer', icon: 'swap', label: 'Transfer', sub: 'between accounts', onClick: () => editor.open({ kind: 'transfer', category: 'transfer.internal' }) },
    { key: 'scan', icon: 'camera', label: 'Scan', sub: 'a receipt', onClick: editor.scan },
    { key: 'sync', icon: 'refresh', label: connected.length ? 'Sync' : 'Bank', sub: connected.length ? (last ? `synced ${ago(last)}` : 'from your bank') : 'connect for auto sync', onClick: sync, busy: syncing },
    { key: 'balances', icon: 'bank', label: 'Balances', sub: 'update by hand', onClick: () => nav('/wealth?update=1') },
  ]
  return (
    <section className="card p-2">
      <div className="grid grid-cols-5 gap-1">
        {tiles.map((t) => (
          <button key={t.key} onClick={t.onClick} disabled={t.busy}
            className={clsx('flex min-w-0 flex-col items-center gap-1 rounded-xl px-1 py-2 text-center transition hover:bg-sunken/60 sm:flex-row sm:gap-2.5 sm:px-3 sm:text-left', t.primary && 'sm:bg-accent/10')}>
            <span className={clsx('grid h-10 w-10 shrink-0 place-items-center rounded-full', t.primary ? 'bg-accent text-white' : 'bg-sunken text-ink')}>
              {t.busy ? <Spinner className="h-4 w-4" /> : <Icon name={t.icon} size={18} />}
            </span>
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium sm:text-sm">{t.label}</span>
              {t.sub && <span className="hidden truncate text-[11px] text-muted sm:block">{t.sub}</span>}
            </span>
          </button>
        ))}
      </div>
      {inboxOpen > 0 && (
        <button onClick={() => nav('/ledger/inbox')} className="mt-1 flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-xs text-ink2 hover:bg-sunken/60">
          <Icon name="inbox" size={14} className="text-accent" />
          <span className="flex-1"><b className="text-ink">{inboxOpen}</b> synced {inboxOpen === 1 ? 'transaction waits' : 'transactions wait'} for review</span>
          <Icon name="chevronR" size={14} className="text-muted" />
        </button>
      )}
    </section>
  )
}
