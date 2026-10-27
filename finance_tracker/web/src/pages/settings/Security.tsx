import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { shortDate } from '../../lib/format'
import { Card, Field, Loading, PinInput, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { deviceName, passkeyRegister } from '../../lib/passkey'
import { useDemo } from '../../lib/hooks'

// ── security ────────────────────────────────────────────────────────

export function Security() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data: st } = useQuery({ queryKey: ['auth-status'], queryFn: () => api.get<any>('/auth/status') })
  const { data: keys } = useQuery({ queryKey: ['passkeys'], queryFn: () => api.get<any[]>('/auth/passkeys'), enabled: !!st })
  const [pin, setPin] = useState({ current_pin: '', pin: '' })
  const [token, setToken] = useState('')
  const demo = useDemo()
  const reload = () => { for (const k of ['auth-status', 'passkeys', 'demo']) qc.invalidateQueries({ queryKey: [k] }) }
  const run = async (fn: () => Promise<any>, ok: string) => {
    try {
      await fn()
      toast(ok, 'good')
      reload()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  if (!st) return <Loading />
  return (
    <div className="max-w-xl space-y-4">
      {demo.on && (
        <div className="flex items-start gap-2 rounded-xl border border-line bg-sunken/40 px-3 py-2 text-sm text-ink2">
          <Icon name="lock" size={16} className="mt-0.5 shrink-0 text-accent" />
          <span>Locked while demo mode is on: the PIN, passkeys and tokens can't be changed until you leave the demo with your PIN.</span>
        </div>
      )}
      <Card title="App lock">
        <div className="mb-3 text-sm">{st.enabled ? <span className="text-good">PIN lock is on.</span> : <span className="text-warn">PIN lock is off — anyone past the web login can see everything.</span>}</div>
        <div className="grid grid-cols-2 gap-3">
          {st.enabled && <Field label="Current PIN"><PinInput value={pin.current_pin} onChange={(v) => setPin({ ...pin, current_pin: v })} aria-label="Current PIN" /></Field>}
          <Field label={st.enabled ? 'New PIN' : 'PIN (4–8 digits)'}><PinInput value={pin.pin} onChange={(v) => setPin({ ...pin, pin: v })} aria-label="New PIN" /></Field>
        </div>
        <div className="mt-3 flex gap-2">
          <button className="btn-primary" onClick={() => run(() => api.post('/auth/pin/setup', pin), st.enabled ? 'PIN changed' : 'Lock enabled')}>{st.enabled ? 'Change PIN' : 'Enable lock'}</button>
          {st.enabled && <button className="btn-danger" onClick={() => run(() => api.post('/auth/pin/disable', { pin: pin.current_pin }), 'Lock disabled')}>Disable</button>}
          {st.enabled && <button className="btn-ghost ml-auto" onClick={() => api.post('/auth/logout').then(() => window.location.reload())}>Lock now</button>}
        </div>
      </Card>
      <Card title="Passkeys (Face ID / fingerprint)">
        <div className="space-y-2">
          {(keys ?? []).map((k) => (
            <div key={k.id} className="flex items-center justify-between text-sm">
              <span>{k.name} <span className="text-xs text-muted">{shortDate(k.created_at.slice(0, 10))}</span></span>
              <button className="text-muted hover:text-bad" onClick={() => run(() => api.del(`/auth/passkeys/${k.id}`), 'Removed')}><Icon name="trash" size={16} /></button>
            </div>
          ))}
          <button className="btn-outline" disabled={!st.enabled} onClick={() => run(() => passkeyRegister(deviceName()), 'Passkey added')}><Icon name="fingerprint" size={16} />Add this device</button>
          {!st.enabled && <div className="text-xs text-muted">Enable the PIN lock first.</div>}
          <div className="text-xs text-muted">Each device unlocks with its own passkey — a phone's passkey isn't on your Mac. Open the app on the MacBook and tap Add this device there to use Touch ID.</div>
        </div>
      </Card>
      <Connectors />
      <Card title="API tokens (MCP server)">
        <div className="mb-2 text-xs text-muted">Read-only tokens see data routes only; read-write tokens may also edit transactions, rules, tags and inbox proposals — never settings, backups or accepting bank rows.</div>
        {token && <div className="mb-2 break-all rounded-xl bg-sunken p-2 font-mono text-xs">{token}<div className="mt-1 font-sans text-warn">Copy it now — it is shown only once.</div></div>}
        <div className="flex flex-wrap gap-2 text-sm">
          <button className="btn-outline" onClick={async () => setToken((await api.post<any>('/auth/token', { rw: false })).token)}>{st.has_token ? 'Replace' : 'Create'} read-only</button>
          {st.has_token && <button className="btn-danger" onClick={() => run(() => api.del('/auth/token'), 'Revoked')}>Revoke</button>}
          <button className="btn-outline" onClick={async () => setToken((await api.post<any>('/auth/token', { rw: true })).token)}>{st.has_token_rw ? 'Replace' : 'Create'} read-write</button>
          {st.has_token_rw && <button className="btn-danger" onClick={() => run(() => api.del('/auth/token', { rw: '1' }), 'Revoked')}>Revoke</button>}
        </div>
      </Card>
    </div>
  )
}

// Stored in UTC; the day shown is the local one.
const localDay = (ts: string) => new Date(ts).toLocaleDateString('sv')

type Grant = { id: number; client_name: string; scope: 'ro' | 'rw'; created_at: string; last_used_at: string }

/** AI apps connected over OAuth: the address to add as a custom connector,
 *  and every connection with what it may do and when it was last used. */
function Connectors() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data } = useQuery({ queryKey: ['oauth-grants'], queryFn: () => api.get<{ grants: Grant[]; mcp_url: string; public_url_set: boolean }>('/oauth/grants') })
  if (!data) return null
  // Without the add-on's public_url the server names itself from this
  // request; the browser's own address is the better guess then.
  const url = data.public_url_set ? data.mcp_url : `${window.location.origin}/mcp`
  const copy = async () => {
    try { await navigator.clipboard.writeText(url); toast('Copied', 'good') } catch { toast('Copy it by hand', 'bad') }
  }
  const revoke = async (g: Grant) => {
    if (!confirm(`Disconnect ${g.client_name}? It stops working at once.`)) return
    try {
      await api.del(`/oauth/grants/${g.id}`)
      qc.invalidateQueries({ queryKey: ['oauth-grants'] })
      toast('Disconnected')
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  return (
    <Card title="AI apps (Claude connector)">
      <div className="space-y-3 text-sm">
        <p className="text-xs text-ink2">Use your finances in Claude chat — including live IBKR data. In Claude: <b>Settings → Connectors → Add custom connector</b>, paste this address, then press Connect and approve it here. You choose read-only or read and write each time.</p>
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-xl bg-sunken px-3 py-2 text-xs">{url}</code>
          <button className="btn-outline shrink-0" onClick={copy} aria-label="Copy the connector address"><Icon name="copy" size={16} />Copy</button>
        </div>
        {!data.public_url_set && <p className="text-[11px] text-muted">This is the address you opened the app at. Claude must reach it over https from the internet — if your public address differs, set it in the add-on's <i>public_url</i> option.</p>}
        {data.grants.length > 0 && (
          <ul className="divide-y divide-line rounded-xl border border-line">
            {data.grants.map((g) => (
              <li key={g.id} className="flex items-center gap-3 px-3 py-2">
                <Icon name="link" size={16} className="text-muted" />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium">{g.client_name} <span className="text-xs font-normal text-muted">· {g.scope === 'rw' ? 'read and write' : 'read only'}</span></div>
                  <div className="text-[11px] text-muted">Connected {shortDate(localDay(g.created_at))}{g.last_used_at ? ` · last used ${shortDate(localDay(g.last_used_at))}` : ''}</div>
                </div>
                <button className="btn-ghost h-8 text-xs text-bad" onClick={() => revoke(g)}>Disconnect</button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  )
}
