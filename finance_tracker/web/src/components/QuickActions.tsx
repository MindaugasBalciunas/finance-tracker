import { useEffect, useRef, useState } from 'react'
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
 *  page — opens the same actions plus Ask CFO as labelled rows over a quarter
 *  circle from the bottom-right corner. */
export function QuickActionsButton({ inboxOpen }: { inboxOpen: number }) {
  const [at, setAt] = useState<DOMRect | null>(null)
  const ref = useRef<HTMLSpanElement>(null)
  const { pathname } = useLocation()
  useEffect(() => setAt(null), [pathname]) // a tab tapped meanwhile
  return (
    <>
      <button onClick={() => setAt(ref.current?.getBoundingClientRect() ?? null)} aria-label="Ask CFO, add, scan or sync" aria-expanded={!!at} className="flex h-14 items-center justify-center">
        <span ref={ref} className="-mt-5 grid h-12 w-12 place-items-center rounded-full bg-accent text-white shadow-lg ring-4 ring-page"><Icon name="spark" size={24} /></span>
      </button>
      {/* Portal: the tab bar's backdrop blur would trap the fixed-position menu. */}
      {at && createPortal(<QuickActionsFan at={at} inboxOpen={inboxOpen} onClose={() => setAt(null)} />, document.body)}
    </>
  )
}

// Rows from the thumb up: Ask CFO nearest, then the daily ledger actions.
const ORDER = ['ask', 'add', 'scan', 'sync', 'transfer', 'balances']

function QuickActionsFan({ at, inboxOpen, onClose }: { at: DOMRect; inboxOpen: number; onClose: () => void }) {
  const { tiles, nav } = useQuickActions(onClose)
  const [out, setOut] = useState(false)
  useEffect(() => {
    const id = requestAnimationFrame(() => setOut(true))
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { cancelAnimationFrame(id); document.removeEventListener('keydown', onKey); document.body.style.overflow = prev }
  }, [onClose])
  const ask: Tile = { key: 'ask', icon: 'spark', label: 'Ask CFO', sub: 'about your money', onClick: () => { onClose(); nav('/ai') }, primary: true }
  const byKey = new Map([...tiles.map((t) => ({ ...t, primary: false })), ask].map((t) => [t.key, t]))
  const rows = ORDER.map((k) => byKey.get(k)).filter((t): t is Tile => !!t)
  if (inboxOpen > 0) rows.push({ key: 'inbox', icon: 'inbox', label: `${inboxOpen} to review`, sub: 'synced from the bank', onClick: () => { onClose(); nav('/ledger/inbox') } })
  const w = window.innerWidth
  const bottom = window.innerHeight - at.top + 14 // just above the star
  return (
    <div className="fixed inset-0 z-40 sm:hidden" role="dialog" aria-modal aria-label="Quick actions">
      <div className={clsx('absolute inset-0 bg-black/45 transition-opacity duration-200', out ? 'opacity-100' : 'opacity-0')} onClick={onClose} />
      {/* A quarter circle from the bottom-right corner, where the thumb rests. */}
      <div className={clsx('pointer-events-none absolute bottom-0 right-0 border-l border-t border-line bg-surface shadow-2xl transition-transform duration-300 ease-out', out ? 'scale-100' : 'scale-0')}
        style={{ width: Math.min(w * 1.2, 500), height: Math.min(w * 1.2, 500), borderTopLeftRadius: '100%', transformOrigin: 'bottom right' }} />
      <div className="absolute right-3 flex w-[15.5rem] flex-col-reverse gap-2" style={{ bottom }}>
        {rows.map((t, i) => (
          <button key={t.key} onClick={t.onClick} disabled={t.busy}
            className={clsx('flex min-h-[3.25rem] w-full items-center justify-end gap-3 rounded-full py-1.5 pl-5 pr-1.5 text-right shadow-sm ring-1 transition-[transform,opacity] duration-200 ease-out active:scale-[0.97]',
              t.primary ? 'bg-accent text-white ring-accent' : 'bg-raised text-ink ring-line')}
            style={{ transform: out ? 'none' : 'translateY(12px) scale(0.92)', opacity: out ? 1 : 0, transitionDelay: `${i * 30}ms` }}>
            <span className="min-w-0 flex-1">
              <span className="block text-[15px] font-semibold leading-tight">{t.label}</span>
              {t.sub && <span className={clsx('block truncate text-xs leading-tight', t.primary ? 'text-white/80' : 'text-muted')}>{t.sub}</span>}
            </span>
            <span className={clsx('grid h-10 w-10 shrink-0 place-items-center rounded-full', t.primary ? 'bg-white/20' : 'bg-sunken')}>
              {t.busy ? <Spinner className="h-4 w-4" /> : <Icon name={t.icon} size={20} />}
            </span>
          </button>
        ))}
      </div>
      {/* The star turns into close, in the same spot. */}
      <button onClick={onClose} aria-label="Close" className="absolute grid h-12 w-12 place-items-center rounded-full bg-accent text-white shadow-lg ring-4 ring-page"
        style={{ left: at.left, top: at.top }}>
        <Icon name="x" size={22} className={clsx('transition-transform duration-200', out ? 'rotate-0' : '-rotate-90')} />
      </button>
    </div>
  )
}
