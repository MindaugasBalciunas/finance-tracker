import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { useDemo, useRefresh } from '../lib/hooks'
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

/** Opening the app (or coming back to it) syncs banks, IBKR and crypto by
 *  itself; the server skips it when a sync ran in the last 30 minutes. Quiet
 *  unless something moved or a source needs you. Read-only throughout. */
export function useAutoSync() {
  const demo = useDemo()
  const qc = useQueryClient()
  const refresh = useRefresh()
  const toast = useToast()
  useEffect(() => {
    if (!demo.ready || demo.on) return
    let alive = true
    const run = () => {
      if (document.visibilityState !== 'visible') return
      api.post<any>('/sync', { auto: true }).then((r) => {
        if (!alive || r.skipped) return
        refresh(); qc.invalidateQueries({ queryKey: ['bank-connections'] })
        const moved: any[] = r.balances ?? []
        const problems: string[] = r.problems ?? []
        const parts = [
          r.bank?.new ? `${r.bank.new} new from the bank` : '',
          moved.length ? `${moved.length} balance${moved.length === 1 ? '' : 's'} updated` : '',
          r.ibkr?.new ? `IBKR: ${r.ibkr.new} trade${r.ibkr.new === 1 ? '' : 's'} to add` : '',
        ].filter(Boolean)
        if (problems.length) toast(`Auto sync: ${problems.join('; ')}`, 'bad')
        else if (parts.length) toast(`Synced · ${parts.join(' · ')}`, 'good')
      }).catch(() => { /* the Sync button shows errors */ })
    }
    run()
    document.addEventListener('visibilitychange', run)
    return () => { alive = false; document.removeEventListener('visibilitychange', run) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [demo.ready, demo.on])
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
  // One call: banks, Interactive Brokers and crypto prices, each writing
  // today's value only; the answer says which balances moved.
  const sync = async () => {
    if (!connected.length && !ibkr) {
      // No bank or broker yet: still revalue crypto, then offer to connect.
      try { await api.post<any>('/sync', {}) } catch { /* nothing to sync */ }
      refresh()
      return nav('/settings/banks')
    }
    setSyncing(true)
    try {
      const r = await api.post<any>('/sync', {})
      const moved: any[] = r.balances ?? []
      const parts = [
        r.bank ? `${r.bank.new} new from the bank` : '',
        moved.length ? `${moved.length} balance${moved.length === 1 ? '' : 's'} updated (${moved.map((b) => b.name).join(', ')})` : 'balances unchanged',
        r.ibkr?.new ? `IBKR: ${r.ibkr.new} trade${r.ibkr.new === 1 ? '' : 's'} to add` : '',
      ].filter(Boolean)
      toast(parts.join(' · ') + ((r.problems ?? []).length ? ` · ${r.problems.join('; ')}` : ''), (r.problems ?? []).length ? 'bad' : 'good')
      refresh(); refetch()
      after?.()
      if (r.bank?.new > 0) nav('/ledger/inbox')
      else if (r.ibkr?.new > 0 || r.ibkr?.mismatches > 0) nav('/wealth/investments')
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

/** Phones: the raised AI star in the tab bar — within thumb reach on every
 *  page — pops up the same actions plus Ask CFO, rising from the button. */
export function QuickActionsButton({ inboxOpen }: { inboxOpen: number }) {
  const [open, setOpen] = useState(false)
  const { pathname } = useLocation()
  useEffect(() => setOpen(false), [pathname]) // a tab tapped meanwhile
  return (
    <>
      <button onClick={() => setOpen(!open)} aria-label="Ask CFO, add, scan or sync" aria-expanded={open} className="flex h-14 items-center justify-center">
        <span className={clsx('-mt-5 grid h-12 w-12 place-items-center rounded-full bg-accent text-white shadow-lg ring-4 ring-page transition', open && 'rotate-90')}><Icon name={open ? 'x' : 'spark'} size={24} /></span>
      </button>
      {/* Portal: the tab bar's backdrop blur would trap the fixed-position menu. */}
      {open && createPortal(<QuickActionsMenu inboxOpen={inboxOpen} onClose={() => setOpen(false)} />, document.body)}
    </>
  )
}

/** A balloon above the tab bar: Ask CFO is the bottom row, nearest the
 *  thumb; the tab bar stays visible so the star closes it again. */
function QuickActionsMenu({ inboxOpen, onClose }: { inboxOpen: number; onClose: () => void }) {
  const { tiles, nav } = useQuickActions(onClose)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])
  const tile = (t: Tile) => (
    <button key={t.key} onClick={t.onClick} disabled={t.busy} className="flex flex-col items-center gap-1.5 rounded-2xl px-1 py-2.5 text-center active:bg-sunken/70">
      <span className="grid h-11 w-11 place-items-center rounded-full bg-sunken text-ink">{t.busy ? <Spinner className="h-4 w-4" /> : <Icon name={t.icon} size={20} />}</span>
      <span className="text-xs font-medium leading-tight">{t.label}</span>
    </button>
  )
  return (
    <div className="fixed inset-0 z-[25] sm:hidden" role="dialog" aria-modal aria-label="Quick actions">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-[1px]" onClick={onClose} />
      <div className="absolute left-3 w-[min(22rem,calc(100vw-1.5rem))] origin-bottom-left animate-[pop_160ms_ease-out] rounded-[1.75rem] border border-line bg-surface p-2 shadow-2xl"
        style={{ bottom: 'calc(3.5rem + env(safe-area-inset-bottom) + 0.75rem)' }}>
        {inboxOpen > 0 && (
          <button onClick={() => { onClose(); nav('/ledger/inbox') }} className="mb-1 flex w-full items-center gap-2 rounded-2xl bg-sunken/50 px-3 py-2.5 text-left text-sm text-ink2">
            <Icon name="inbox" size={16} className="text-accent" />
            <span className="flex-1"><b className="text-ink">{inboxOpen}</b> synced {inboxOpen === 1 ? 'transaction waits' : 'transactions wait'} for review</span>
            <Icon name="chevronR" size={16} className="text-muted" />
          </button>
        )}
        <div className="grid grid-cols-5 gap-0.5">{tiles.map(tile)}</div>
        <button onClick={() => { onClose(); nav('/ai') }} className="mt-1 flex w-full items-center gap-3 rounded-[1.25rem] bg-accent px-3 py-3 text-left text-white active:opacity-90">
          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-white/20"><Icon name="spark" size={20} /></span>
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-semibold">Ask CFO</span>
            <span className="block text-xs text-white/80">a question about your money</span>
          </span>
          <Icon name="chevronR" size={18} className="text-white/80" />
        </button>
      </div>
    </div>
  )
}
