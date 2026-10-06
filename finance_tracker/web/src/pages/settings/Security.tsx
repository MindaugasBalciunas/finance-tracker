import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { shortDate } from '../../lib/format'
import { Card, Field, Loading, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { passkeyRegister } from '../../lib/passkey'
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
          {st.enabled && <Field label="Current PIN"><input type="password" inputMode="numeric" className="input" value={pin.current_pin} onChange={(e) => setPin({ ...pin, current_pin: e.target.value })} /></Field>}
          <Field label={st.enabled ? 'New PIN' : 'PIN (4–8 digits)'}><input type="password" inputMode="numeric" className="input" value={pin.pin} onChange={(e) => setPin({ ...pin, pin: e.target.value })} /></Field>
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
          <button className="btn-outline" disabled={!st.enabled} onClick={() => run(() => passkeyRegister(navigator.platform || 'Device'), 'Passkey added')}><Icon name="fingerprint" size={16} />Add this device</button>
          {!st.enabled && <div className="text-xs text-muted">Enable the PIN lock first.</div>}
        </div>
      </Card>
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
