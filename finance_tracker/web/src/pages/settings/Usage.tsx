import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useExitDemo } from '../../components/ExitDemo'
import { useDemo, usePrefs } from '../../lib/hooks'
import { Card, Loading, Segmented, Spinner, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

// ── usage analytics ─────────────────────────────────────────────────

/** What you open, how long you stay, and which pages you have to hunt for. */
export function UsageView() {
  const [days, setDays] = useState('30')
  const { prefs, set } = usePrefs()
  const { data: u, isLoading, refetch } = useQuery({ queryKey: ['usage', days], queryFn: () => api.get<any>('/usage/summary', { days }) })
  const toast = useToast()
  if (isLoading || !u) return <Loading />
  for (const k of ['days', 'pages', 'actions', 'hunts', 'unused', 'suggestions']) u[k] ??= []
  // Page names come from the server (one source of truth).
  const pageName = (p: string) => u.names?.[p] ?? p
  const maxDay = Math.max(1, ...u.days.map((d: any) => d.views))
  const maxViews = Math.max(1, ...u.pages.map((p: any) => p.views))
  return (
    <div className="space-y-4">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Toggle checked={!prefs.usage_off} onChange={(v) => set({ usage_off: !v })} label="Record how I use the app" />
          <Segmented size="sm" value={days} onChange={setDays} options={[{ value: '7', label: '7 days' }, { value: '30', label: '30 days' }, { value: '90', label: '90 days' }]} />
        </div>
        <div className="mt-2 text-xs text-muted">Pages, time on each, clicks and where they land — kept only in this app's own database (180 days), never sent anywhere. Search terms and amounts are not recorded. Raw events for analysis: <code className="rounded bg-sunken px-1">GET /api/usage/events?days=90</code></div>
        <div className="mt-3 grid grid-cols-3 gap-3 text-sm">
          <div><div className="text-xs text-muted">Visits</div><div className="text-lg font-semibold tnum">{u.sessions}</div></div>
          <div><div className="text-xs text-muted">Page views</div><div className="text-lg font-semibold tnum">{u.views}</div></div>
          <div><div className="text-xs text-muted">Hard-to-find pages</div><div className={clsx('text-lg font-semibold tnum', u.hunts.length ? 'text-warn' : '')}>{u.hunts.length}</div></div>
        </div>
        <div className="mt-3 flex h-12 items-end gap-0.5" aria-label="Views per day">
          {u.days.map((d: any) => <div key={d.day} title={`${d.day}: ${d.views} views`} className="flex-1 rounded-t bg-accent/70" style={{ height: `${Math.max(d.views ? 6 : 2, (d.views / maxDay) * 100)}%`, opacity: d.views ? 1 : 0.25 }} />)}
        </div>
      </Card>

      {u.suggestions.length > 0 && (
        <Card icon="spark" color="var(--s4)" title="Suggestions">
          <ul className="space-y-1.5 text-sm">{u.suggestions.map((s: string) => <li key={s} className="flex gap-2"><Icon name="chevronR" size={14} className="mt-1 shrink-0 text-muted" />{s}</li>)}</ul>
        </Card>
      )}

      {u.hunts.length > 0 && (
        <Card pad={false} icon="search" color="var(--s2)" title="Pages you had to look for">
          <div className="divide-y divide-line border-t border-line">
            {u.hunts.map((h: any) => (
              <div key={h.target} className="px-4 py-2.5 text-sm">
                <div className="flex items-baseline justify-between gap-2"><b>{pageName(h.target)}</b><span className="text-xs text-muted tnum">{h.count}× · ≈{h.avg_hops} hops · {h.avg_sec}s</span></div>
                <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted">
                  {[...h.typical_path, h.target].map((p: string, i: number) => <span key={i} className="inline-flex items-center gap-1">{i > 0 && <Icon name="chevronR" size={11} />}<span className={clsx('chip h-6', p === h.target && 'chip-on')}>{pageName(p)}</span></span>)}
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card pad={false} icon="chart" color="var(--s1)" title="Most used pages">
          <div className="divide-y divide-line border-t border-line">
            {u.pages.slice(0, 15).map((p: any) => (
              <div key={p.path} className="px-4 py-2 text-sm">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="truncate">{pageName(p.path)}</span>
                  <span className="shrink-0 text-xs text-muted tnum">{p.views} views · {p.avg_sec}s avg{p.bounces ? ` · ${p.bounces} bounced` : ''}</span>
                </div>
                <div className="mt-1 h-1 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent" style={{ width: `${(p.views / maxViews) * 100}%` }} /></div>
              </div>
            ))}
            {!u.pages.length && <div className="px-4 py-6 text-center text-sm text-muted">Nothing recorded yet — use the app for a while.</div>}
          </div>
        </Card>
        <div className="space-y-4">
          <Card pad={false} icon="target" color="var(--s3)" title="Most clicked">
            <div className="divide-y divide-line border-t border-line">
              {u.actions.slice(0, 12).map((a: any) => (
                <div key={a.path + a.label} className="flex items-center justify-between gap-2 px-4 py-2 text-sm">
                  <span className="min-w-0 truncate">{a.label || '(icon)'} <span className="text-xs text-muted">on {pageName(a.path)}</span></span>
                  <span className="tnum text-xs text-muted">{a.count}×</span>
                </div>
              ))}
              {!u.actions.length && <div className="px-4 py-6 text-center text-sm text-muted">No clicks yet.</div>}
            </div>
          </Card>
          {u.unused.length > 0 && (
            <Card title="Not opened in this period">
              <div className="flex flex-wrap gap-1.5">{u.unused.map((p: string) => <a key={p} href={`#${p}`} className="chip">{pageName(p)}</a>)}</div>
            </Card>
          )}
        </div>
      </div>
      <button className="btn-danger" onClick={async () => { if (!confirm('Delete all recorded usage?')) return; await api.del('/usage'); toast('Usage data deleted', 'good'); refetch() }}>
        <Icon name="trash" size={16} />Delete recorded usage
      </button>
    </div>
  )
}

/** Show the app with fictional data — for demos and screenshots. */
export function DemoCard() {
  const { on, protected: locked, switchTo, reset } = useDemo()
  const exitDemo = useExitDemo()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  const run = async (f: () => Promise<void>, msg: string) => {
    setBusy(true)
    try { await f(); toast(msg, 'good') } catch (e: any) { toast(e?.message ?? 'Failed', 'bad') } finally { setBusy(false) }
  }
  return (
    <Card icon="spark" color="var(--s5)" title="Demo mode">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Toggle checked={on} onChange={(v) => !busy && (v ? run(() => switchTo(true), 'Demo data on — your real data is untouched') : exitDemo.exit())} label="Show demo data" />
        {on && <button className="btn-outline h-8 text-xs" disabled={busy} onClick={() => confirm('Throw away changes made in the demo and generate fresh demo data?') && run(reset, 'Fresh demo data')}>{busy ? <Spinner /> : <Icon name="refresh" size={14} />}Reset demo data</button>}
      </div>
      {exitDemo.sheet}
      <div className={clsx('mt-3 flex items-start gap-2 rounded-xl border px-3 py-2 text-xs', locked ? 'border-line bg-sunken/40 text-ink2' : 'border-warn/40 bg-warn/10 text-ink')}>
        <Icon name={locked ? 'lock' : 'alert'} size={14} className={clsx('mt-0.5 shrink-0', locked ? 'text-good' : 'text-warn')} />
        {locked
          ? <span>Protected by your PIN: leaving the demo asks for it, and passkeys, tokens and the PIN can't be changed while it's on — safe to hand the device to someone.</span>
          : <span>No PIN is set, so anyone holding the device can switch back to your data. <a className="text-accent" href="#/settings/security">Set a PIN</a> before showing the app to someone.</span>}
      </div>
      <div className="mt-2 text-xs text-muted">Swaps every page to a separate database of a fictional household — two years of transactions, accounts, budgets, a mortgage and an ETF portfolio — to show the app without showing your money. Your login stays the same; backups, imports, bank sync and AI settings are switched off while it is on, and nothing you do in the demo touches your real data.</div>
    </Card>
  )
}
