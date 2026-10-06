import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { useRefresh } from '../lib/hooks'
import { useTxEditor } from './TxEditor'
import { Sheet, Spinner, useToast } from './ui'
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

type Tile = { key: string; icon: string; label: string; sub?: string; onClick: () => void; primary?: boolean; busy?: boolean }

/** The ledger comes first: enter, scan or sync a transaction in one tap, plus
 *  the two other things done by hand — a transfer between your accounts and
 *  updating balances of manual accounts. Shared by the desktop bar on Home
 *  and the phone's "+" in the tab bar. */
function useQuickActions(after?: () => void) {
  const editor = useTxEditor()
  const nav = useNavigate()
  const toast = useToast()
  const refresh = useRefresh()
  const { data: conns, refetch } = useQuery({ queryKey: ['bank-connections'], queryFn: () => api.get<any[]>('/bank/connections'), staleTime: 60_000, retry: false })
  const [syncing, setSyncing] = useState(false)
  const connected = (conns ?? []).filter((c) => c.status === 'authorized')
  const last = connected.flatMap((c) => (c.accounts ?? []).map((a: any) => a.last_synced_at as string)).filter(Boolean).sort().pop()
  const { data: ib } = useQuery({ queryKey: ['ibkr'], queryFn: () => api.get<any>('/ibkr'), staleTime: 60_000, retry: false })
  const ibkr = !!ib?.status?.connected
  const sync = async () => {
    if (!connected.length && !ibkr) return nav('/settings/banks')
    setSyncing(true)
    try {
      // IBKR too, when connected: today's account value and the trade check.
      if (ibkr) {
        try {
          const x = await api.post<any>('/ibkr/sync', {})
          if (x.new || x.mismatches) toast(`IBKR: ${x.new ? `${x.new} trade${x.new === 1 ? '' : 's'} to add` : ''}${x.new && x.mismatches ? ' · ' : ''}${x.mismatches ? `${x.mismatches} position${x.mismatches === 1 ? '' : 's'} differ` : ''}`, 'bad')
        } catch (e) { toast(`IBKR: ${(e as Error).message}`, 'bad') }
      }
      if (!connected.length) { refresh(); after?.(); return }
      const r = await api.post<any>('/bank/sync', {})
      const failed = (r.accounts ?? []).filter((a: any) => a.error || a.skipped)
      toast(`${r.new} new · ${r.auto_linked} linked${failed.length ? ` · ${failed.length} account(s) need attention` : ''}`, failed.length ? 'bad' : 'good')
      refresh(); refetch()
      after?.()
      if (r.new > 0) nav('/ledger/inbox')
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setSyncing(false)
    }
  }
  const then = (f: () => void) => () => { after?.(); f() }
  const tiles: Tile[] = [
    { key: 'add', icon: 'plus', label: 'Add', sub: 'expense or income', onClick: then(() => editor.open()), primary: true },
    { key: 'transfer', icon: 'swap', label: 'Transfer', sub: 'between accounts', onClick: then(() => editor.open({ kind: 'transfer', category: 'transfer.internal' })) },
    { key: 'scan', icon: 'camera', label: 'Scan', sub: 'a receipt', onClick: then(editor.scan) },
    { key: 'sync', icon: 'refresh', label: connected.length || ibkr ? 'Sync' : 'Bank', sub: connected.length ? (last ? `synced ${ago(last)}` : 'from your bank') : ibkr ? 'IBKR' : 'connect for auto sync', onClick: sync, busy: syncing },
    { key: 'balances', icon: 'bank', label: 'Balances', sub: 'update by hand', onClick: then(() => nav('/wealth?update=1')) },
  ]
  return { tiles, nav }
}

/** Desktop: a bar at the top of Home. Phones use the tab bar's "+". */
export function QuickActions({ inboxOpen }: { inboxOpen: number }) {
  const { tiles, nav } = useQuickActions()
  return (
    <section className="card hidden p-2 sm:block">
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

/** Phones: the raised "+" in the middle of the tab bar — within thumb reach on
 *  every page — opens the same actions as a sheet. */
export function QuickActionsButton({ inboxOpen }: { inboxOpen: number }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button onClick={() => setOpen(true)} aria-label="Add, scan or sync" className="flex h-14 items-center justify-center">
        <span className="-mt-5 grid h-12 w-12 place-items-center rounded-full bg-accent text-white shadow-lg ring-4 ring-page"><Icon name="plus" size={24} /></span>
      </button>
      {/* Portal: the tab bar's backdrop blur would trap the fixed-position sheet. */}
      {open && createPortal(<QuickActionsSheet inboxOpen={inboxOpen} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

function QuickActionsSheet({ inboxOpen, onClose }: { inboxOpen: number; onClose: () => void }) {
  const { tiles, nav } = useQuickActions(onClose)
  return (
    <Sheet open onClose={onClose} title="Add or update">
      <div className="grid grid-cols-1 gap-1">
        {tiles.map((t) => (
          <button key={t.key} onClick={t.onClick} disabled={t.busy} className="flex items-center gap-3 rounded-xl px-2 py-2.5 text-left hover:bg-sunken/60">
            <span className={clsx('grid h-11 w-11 shrink-0 place-items-center rounded-full', t.primary ? 'bg-accent text-white' : 'bg-sunken text-ink')}>
              {t.busy ? <Spinner className="h-4 w-4" /> : <Icon name={t.icon} size={20} />}
            </span>
            <span className="min-w-0">
              <span className="block text-sm font-medium">{t.label}</span>
              {t.sub && <span className="block text-xs text-muted">{t.sub}</span>}
            </span>
          </button>
        ))}
      </div>
      {inboxOpen > 0 && (
        <button onClick={() => { onClose(); nav('/ledger/inbox') }} className="mt-2 flex w-full items-center gap-2 rounded-xl bg-sunken/50 px-3 py-2.5 text-left text-sm text-ink2">
          <Icon name="inbox" size={16} className="text-accent" />
          <span className="flex-1"><b className="text-ink">{inboxOpen}</b> synced {inboxOpen === 1 ? 'transaction waits' : 'transactions wait'} for review</span>
          <Icon name="chevronR" size={16} className="text-muted" />
        </button>
      )}
    </Sheet>
  )
}
