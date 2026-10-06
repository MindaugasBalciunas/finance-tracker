import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { eur, shortDate } from '../lib/format'
import { Card, Spinner, useToast } from './ui'
import { Icon } from './Icon'

export type IBKRStatus = { connected: boolean; account: string; scope?: string; connected_at?: string; last_sync?: string; last_error?: string }
export type IBKRReport = {
  at: string; account: string; currency: string; net_liquidation: number; cash: number; app_balance?: number; balance_written: boolean
  positions: { ticker: string; description: string; currency: string; shares: number; app_shares: number; price: number; value: number; avg_price: number; unrealized: number }[]
  trades: { external_id: string; date: string; action: string; ticker: string; name: string; currency: string; shares: number; price: number; commission: number; recorded: boolean }[]
  new: number; mismatches: number
  orders?: IBKROrder[] | null; instructions?: IBKROrder[] | null; orders_error?: string
}
export type IBKROrder = { id: string; symbol: string; description?: string; side: string; type: string; status?: string; quantity: number; price?: number; filled?: number; tif?: string; created?: string; expires?: string }

function OrderRow({ o }: { o: IBKROrder }) {
  return (
    <div className="flex items-center gap-3 px-3 py-2 text-sm">
      <span className={clsx('w-9 shrink-0 text-xs font-medium uppercase', o.side === 'sell' ? 'text-bad' : 'text-good')}>{o.side || '—'}</span>
      <span className="min-w-0 flex-1">
        <b>{o.symbol || o.description}</b> <span className="text-xs text-muted">{o.quantity}{o.filled ? ` (${o.filled} filled)` : ''} · {o.type}{o.price ? ` ${o.price}` : ''}{o.tif ? ` · ${o.tif}` : ''}</span>
        {o.symbol && o.description && <span className="block truncate text-xs text-muted">{o.description}</span>}
      </span>
      {o.status && <span className="shrink-0 rounded-full bg-sunken px-2 py-0.5 text-[11px] text-ink2">{o.status}</span>}
    </div>
  )
}

export const useIBKR = () => useQuery({ queryKey: ['ibkr'], queryFn: () => api.get<{ status: IBKRStatus; report: IBKRReport | null }>('/ibkr'), staleTime: 60_000, retry: false })

function ago(iso?: string) {
  if (!iso) return 'never'
  const m = Math.round((Date.now() - new Date(iso).getTime()) / 60000)
  return m < 2 ? 'just now' : m < 60 ? `${m} min ago` : m < 1440 ? `${Math.round(m / 60)} h ago` : `${Math.round(m / 1440)} d ago`
}

/** Interactive Brokers, read-only: the account value, positions checked
 *  against the app's trades, and IBKR trades the app doesn't have — added
 *  only when you pick them. */
export function IBKRPanel() {
  const { data } = useIBKR()
  const qc = useQueryClient()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  // null = every trade not yet in the app (the usual choice); a set once you untick one.
  const [chosen, setChosen] = useState<Set<string> | null>(null)
  const [showRecorded, setShowRecorded] = useState(false)
  if (!data?.status.connected) return null
  const r = data.report
  const reload = () => { for (const k of ['ibkr', 'portfolio', 'trades', 'accounts', 'overview', 'nw-history', 'balances']) qc.invalidateQueries({ queryKey: [k] }) }
  const sync = async () => {
    setBusy(true)
    try {
      const x = await api.post<IBKRReport>('/ibkr/sync', {})
      toast(`IBKR ${eur(x.net_liquidation)}${x.new ? ` · ${x.new} trade${x.new === 1 ? '' : 's'} to add` : ''}${x.mismatches ? ` · ${x.mismatches} position${x.mismatches === 1 ? '' : 's'} differ` : ''}`, x.mismatches ? 'bad' : 'good')
      setChosen(null)
      reload()
    } catch (e) { toast((e as Error).message, 'bad') } finally { setBusy(false) }
  }
  const add = async () => {
    setBusy(true)
    try {
      const x = await api.post<{ imported: number }>('/ibkr/import', { ids: [...picked] })
      toast(`${x.imported} trade${x.imported === 1 ? '' : 's'} added from IBKR`, 'good')
      setChosen(null); reload()
    } catch (e) { toast((e as Error).message, 'bad') } finally { setBusy(false) }
  }
  const fresh = (r?.trades ?? []).filter((t) => !t.recorded)
  const picked = chosen ?? new Set(fresh.map((t) => t.external_id))
  const setPicked = (f: (s: Set<string>) => Set<string>) => setChosen(f(picked))
  const recorded = (r?.trades ?? []).filter((t) => t.recorded)
  const diff = r?.app_balance != null ? r.net_liquidation - r.app_balance : null
  return (
    <Card title={<span className="flex items-center gap-2">Interactive Brokers <span className="rounded-full bg-good/15 px-1.5 text-[10px] font-medium text-good">read-only</span></span>}
      action={<button className="btn-outline h-8 px-2.5 text-xs" onClick={sync} disabled={busy}>{busy ? <Spinner className="h-4 w-4" /> : <Icon name="refresh" size={14} />}Sync</button>}>
      {data.status.last_error && <div className="mb-2 rounded-lg bg-bad/10 px-3 py-2 text-xs text-bad">{data.status.last_error}</div>}
      {!r ? <div className="text-sm text-muted">Not synced yet — press Sync to read the account.</div> : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1">
            <div><div className="text-xs text-muted">Account value at IBKR</div><div className="text-2xl font-semibold tnum">{eur(r.net_liquidation)}</div></div>
            <div className="text-xs text-muted">
              cash {eur(r.cash)} · synced {ago(r.at)}
              {diff != null && Math.abs(diff) >= 1 && <> · the app had {eur(r.app_balance!)} ({diff > 0 ? '+' : '−'}{eur(Math.abs(diff))})</>}
              {r.balance_written && <> · today's value recorded</>}
            </div>
          </div>

          <div>
            <div className="section-title mb-1.5">Positions — IBKR vs the app's trades</div>
            <div className="divide-y divide-line rounded-xl border border-line text-sm">
              {r.positions.map((p) => {
                const ok = Math.abs(p.shares - p.app_shares) < 1e-6
                return (
                  <div key={p.ticker} className="flex items-center gap-3 px-3 py-2">
                    <Icon name={ok ? 'check' : 'alert'} size={15} className={ok ? 'text-good' : 'text-warn'} />
                    <span className="min-w-0 flex-1"><b>{p.ticker}</b> <span className="text-xs text-muted">{p.shares} at IBKR{!ok && <> · {p.app_shares} in the app</>}</span></span>
                    <span className="tnum">{p.value ? eur(p.value) : '—'}</span>
                  </div>
                )
              })}
            </div>
            {r.mismatches > 0 && <div className="mt-1.5 text-xs text-warn">Different share counts usually mean a trade is missing below, or one was entered on another account.</div>}
          </div>

          {((r.orders ?? []).length > 0 || (r.instructions ?? []).length > 0 || r.orders_error) && (
            <div>
              <div className="section-title mb-1.5">Open orders</div>
              {r.orders_error && <div className="mb-1.5 text-xs text-warn">{r.orders_error}</div>}
              {(r.orders ?? []).length > 0 && <div className="divide-y divide-line rounded-xl border border-line">{r.orders!.map((o) => <OrderRow key={o.id || o.symbol} o={o} />)}</div>}
              {(r.instructions ?? []).length > 0 && (
                <>
                  <div className="mb-1 mt-2 text-xs text-muted">Saved instructions — not orders until you submit them in IBKR</div>
                  <div className="divide-y divide-line rounded-xl border border-line">{r.instructions!.map((o) => <OrderRow key={o.id || o.description} o={o} />)}</div>
                </>
              )}
              <div className="mt-1 text-[11px] text-muted">Shown for information — the app can't place, change or cancel orders.</div>
            </div>
          )}
          <div>
            <div className="mb-1.5 flex items-center justify-between">
              <div className="section-title">Trades this year</div>
              {fresh.length > 0 && <button className="btn-primary h-8 px-2.5 text-xs" onClick={add} disabled={busy || picked.size === 0}>Add {picked.size} to the app</button>}
            </div>
            {fresh.length === 0 ? <div className="text-sm text-muted">Every IBKR trade this year is in the app ✓</div> : (
              <div className="divide-y divide-line rounded-xl border border-line text-sm">
                {fresh.map((t) => (
                  <label key={t.external_id} className="flex cursor-pointer items-center gap-3 px-3 py-2">
                    <input type="checkbox" checked={picked.has(t.external_id)} onChange={() => setPicked((s) => { const n = new Set(s); n.has(t.external_id) ? n.delete(t.external_id) : n.add(t.external_id); return n })} />
                    <span className="min-w-0 flex-1">
                      <span className={clsx('font-medium', t.action === 'sell' ? 'text-bad' : 'text-good')}>{t.action}</span> <b>{t.ticker}</b> <span className="text-xs text-muted">{t.name}</span>
                      <span className="block text-xs text-muted">{shortDate(t.date)} · {t.shares} × {t.price} {t.currency}{t.commission ? ` · fee ${t.commission.toFixed(2)}` : ''}</span>
                    </span>
                  </label>
                ))}
              </div>
            )}
            {recorded.length > 0 && (
              <button className="mt-1.5 text-xs text-muted hover:text-ink" onClick={() => setShowRecorded(!showRecorded)}>{showRecorded ? 'Hide' : 'Show'} {recorded.length} already in the app</button>
            )}
            {showRecorded && <div className="mt-1 space-y-0.5 text-xs text-muted">{recorded.map((t) => <div key={t.external_id}>✓ {shortDate(t.date)} {t.action} {t.shares} {t.ticker}</div>)}</div>}
            <div className="mt-2 text-xs text-muted">Added trades include IBKR's fee in the price (your cost basis) and keep IBKR's trade id, so a trade is never added twice. Nothing is added unless you pick it.</div>
          </div>
        </div>
      )}
    </Card>
  )
}

/** Settings → Banks: connect or disconnect IBKR and pick the app account. */
export function IBKRConnection() {
  const { data } = useIBKR()
  const qc = useQueryClient()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const { data: accounts } = useQuery({ queryKey: ['accounts'], queryFn: () => api.get<any[]>('/accounts') })
  const st = data?.status
  const connect = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ url: string }>('/ibkr/connect', { redirect_uri: window.location.origin + '/' })
      window.location.href = r.url
    } catch (e) { toast((e as Error).message, 'bad'); setBusy(false) }
  }
  const disconnect = async () => {
    if (!confirm('Disconnect Interactive Brokers? The app forgets the sign-in and asks IBKR to revoke it. Trades and balances already in the app stay.')) return
    await api.del('/ibkr'); qc.invalidateQueries({ queryKey: ['ibkr'] }); toast('IBKR disconnected', 'good')
  }
  const setAccount = async (id: string) => { await api.put('/ibkr/account', { account: id }); qc.invalidateQueries({ queryKey: ['ibkr'] }) }
  return (
    <Card title="Interactive Brokers">
      {st?.connected ? (
        <div className="space-y-3 text-sm">
          <div className="flex items-start gap-2"><Icon name="check" size={16} className="mt-0.5 shrink-0 text-good" /><span>Connected, read-only{st.connected_at && <span className="text-muted"> · since {shortDate(st.connected_at.slice(0, 10))}</span>}<span className="text-muted"> · last sync {ago(st.last_sync)}</span></span></div>
          {st.last_error && <div className="rounded-lg bg-bad/10 px-3 py-2 text-xs text-bad">{st.last_error}</div>}
          <label className="flex items-center gap-2 text-xs text-ink2">Value goes to
            <select className="input select-pad h-8 w-auto text-xs" value={st.account} onChange={(e) => setAccount(e.target.value)}>
              {(accounts ?? []).filter((a) => a.kind === 'brokerage' && !a.archived).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
          </label>
          <div className="flex gap-2"><a className="btn-outline h-8 px-2.5 text-xs" href="#/wealth/investments">Positions & trades</a><button className="btn-ghost h-8 px-2.5 text-xs text-bad" onClick={disconnect}>Disconnect</button></div>
        </div>
      ) : (
        <div className="space-y-3 text-sm">
          <div className="text-ink2">Keep the IBKR account value up to date, check positions against your trades, and add IBKR trades in one tap. Ask CFO can also read the account live.</div>
          <ul className="list-disc space-y-0.5 pl-5 text-xs text-muted">
            <li>Read-only: the app asks IBKR only for <code>mcp.read</code> and calls read tools only — it cannot trade or change anything.</li>
            <li>You sign in on IBKR's own page; your IBKR password never reaches this app.</li>
            <li>Revoke any time here, or in IBKR Client Portal → Settings → Manage Third-Party Consents.</li>
            <li>Open the app at its own web address (not inside Home Assistant) to connect — IBKR returns you there.</li>
          </ul>
          <button className="btn-primary" onClick={connect} disabled={busy}>{busy ? <Spinner className="h-4 w-4" /> : null}Connect Interactive Brokers (read-only)</button>
        </div>
      )}
    </Card>
  )
}
