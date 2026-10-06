import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { shortDate } from '../../lib/format'
import { Card, Field, Loading, Toggle, useToast } from '../../components/ui'
import { AccountSelect, SPEND_KINDS } from '../../components/pickers'

// ── banks ───────────────────────────────────────────────────────────

export function Banks() {
  const [sp, setSp] = useSearchParams()
  const qc = useQueryClient()
  const toast = useToast()
  const { data: st } = useQuery({ queryKey: ['bank-settings'], queryFn: () => api.get<any>('/bank/settings') })
  const { data: conns, isLoading } = useQuery({ queryKey: ['bank-connections'], queryFn: () => api.get<any[]>('/bank/connections') })
  const [form, setForm] = useState<any>(null)
  const [bank, setBank] = useState('Swedbank')
  const [pasted, setPasted] = useState('')
  const [busy, setBusy] = useState(false)
  const reload = () => { qc.invalidateQueries({ queryKey: ['bank-connections'] }); qc.invalidateQueries({ queryKey: ['bank-settings'] }) }

  // The bank redirects back with ?code&state; finish the consent here.
  useEffect(() => {
    const code = sp.get('code'), state = sp.get('state'), err = sp.get('error_description') || sp.get('error')
    if (err) { toast(`The bank refused: ${err}`, 'bad'); setSp({}, { replace: true }) }
    if (code && state) {
      setSp({}, { replace: true })
      api.post('/bank/callback', { code, state }).then(() => { toast('Bank connected — map its accounts below', 'good'); reload() }).catch((e) => toast(e.message, 'bad'))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const connect = async () => {
    setBusy(true)
    try {
      const r = await api.post<any>('/bank/connect', { bank, country: 'LT' })
      window.location.href = r.url
    } catch (e) {
      toast((e as Error).message, 'bad')
      setBusy(false)
    }
  }
  const finishPasted = async () => {
    try {
      await api.post('/bank/callback', { url: pasted })
      toast('Bank connected', 'good')
      setPasted('')
      reload()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const mapAccount = async (id: number, account_id: string) => {
    await api.put(`/bank/accounts/${id}`, { account_id })
    reload()
  }
  const disconnect = async (id: number) => {
    if (!confirm('Disconnect this bank? The consent is revoked at the bank too.')) return
    await api.del(`/bank/connections/${id}`)
    reload()
  }
  const saveSettings = async () => {
    await api.put('/bank/settings', { ...form, owner_names: typeof form.owner_names === 'string' ? form.owner_names.split('\n').map((s: string) => s.trim()).filter(Boolean) : form.owner_names })
    toast('Saved', 'good')
    setForm(null)
    reload()
  }
  return (
    <div className="space-y-4">
      <Card title="Connections" action={st?.configured && (
        <div className="flex items-center gap-2">
          <select className="input select-pad h-8 w-auto text-xs" value={bank} onChange={(e) => setBank(e.target.value)}>{['Swedbank', 'SEB', 'Luminor', 'Revolut', 'Šiaulių bankas'].map((b) => <option key={b}>{b}</option>)}</select>
          <button className="btn-primary h-8 text-xs" onClick={connect} disabled={busy}>Connect</button>
        </div>)}>
        {!st?.configured ? <BankGuide /> : isLoading ? <Loading /> : (
          <div className="space-y-3">
            {(conns ?? []).filter((c) => c.accounts.length || c.status === 'authorized').map((c) => (
              <div key={c.id} className="rounded-xl border border-line p-3">
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <div className="text-sm font-semibold">{c.aspsp_name}</div>
                    <div className={clsx('text-xs', c.status === 'authorized' ? (c.days_left < 14 ? 'text-warn' : 'text-good') : 'text-bad')}>
                      {c.status === 'authorized' ? `connected · consent ends in ${c.days_left} days` : c.status}{c.last_error ? ` · ${c.last_error}` : ''}
                    </div>
                  </div>
                  <button className="btn-ghost h-8 text-xs text-bad" onClick={() => disconnect(c.id)}>Disconnect</button>
                </div>
                {c.accounts.map((a: any) => (
                  <div key={a.id} className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2 text-sm">
                    <div className="min-w-0 flex-1">
                      <div className="truncate">{a.display_name} <span className="text-xs text-muted">{a.iban}</span></div>
                      <div className="text-xs text-muted">{a.last_synced_at ? `synced ${shortDate(a.last_synced_at.slice(0, 10))}` : 'never synced'}{a.bank_balance != null ? ` · bank says €${a.bank_balance}` : ''}{a.open ? ` · ${a.open} in inbox` : ''}</div>
                    </div>
                    <div className="w-48"><AccountSelect value={a.account_id} onChange={(v) => mapAccount(a.id, v)} placeholder="Don't sync" kinds={SPEND_KINDS} /></div>
                  </div>
                ))}
              </div>
            ))}
            {(() => {
              const dead = (conns ?? []).filter((c) => !c.accounts.length && c.status !== 'authorized')
              if (!dead.length) return null
              return (
                <details className="rounded-xl border border-line px-3 py-2 text-sm">
                  <summary className="cursor-pointer text-muted">{dead.length} unfinished or revoked connection attempts</summary>
                  <div className="mt-2 space-y-1">
                    {dead.map((c) => <div key={c.id} className="flex justify-between text-xs"><span>{c.aspsp_name} · {c.status}{c.last_error ? ` · ${c.last_error}` : ''}</span><button className="text-bad" onClick={() => disconnect(c.id)}>Remove</button></div>)}
                  </div>
                </details>
              )
            })()}
            {!(conns ?? []).some((c) => c.status === 'authorized') && <div className="text-sm text-muted">No live connection — press Connect and approve access in your bank.</div>}
            <div className="text-xs text-muted">If the bank's redirect cannot reach this app (LAN or Tailscale), paste the address you landed on:</div>
            <div className="flex gap-2"><input className="input" placeholder="https://…?code=…&state=…" value={pasted} onChange={(e) => setPasted(e.target.value)} /><button className="btn-outline" onClick={finishPasted} disabled={!pasted}>Finish</button></div>
          </div>
        )}
      </Card>
      <Card title="Card reservations">
        <Toggle checked={!st?.keep_reserved_in_inbox} onChange={async (v) => { await api.put('/bank/settings', { keep_reserved_in_inbox: !v }); qc.invalidateQueries({ queryKey: ['bank-settings'] }) }}
          label="Add to the ledger right away" />
        <div className="mt-2 text-xs text-muted">Card payments the bank has only reserved go straight into the ledger, marked <span className="text-warn">pending</span>, so today's spending counts today. When the bank books one, the same transaction gets the final amount and date — your category and tags stay. A reservation the bank releases is removed. Off: they wait in the inbox until they book.</div>
      </Card>
      <Card title="Enable Banking application" action={!form && <button className="btn-ghost h-8 text-xs" onClick={() => setForm({ ...st, owner_names: (st?.owner_names ?? []).join('\n'), private_key_pem: '' })}>Edit</button>}>
        {!form ? (
          <div className="space-y-1 text-sm">
            <div><span className="text-muted">Application:</span> {st?.application_id || '—'}</div>
            <div><span className="text-muted">Private key:</span> {st?.has_key ? 'stored (never shown)' : 'missing'}</div>
            <div><span className="text-muted">Redirect URL:</span> {st?.redirect_url || '—'}</div>
          </div>
        ) : (
          <div className="space-y-3">
            <Field label="Application ID"><input className="input font-mono text-xs" value={form.application_id} onChange={(e) => setForm({ ...form, application_id: e.target.value })} /></Field>
            <Field label="Private key (PEM)" hint="Leave empty to keep the stored key."><textarea className="input h-24 py-2 font-mono text-xs" value={form.private_key_pem} onChange={(e) => setForm({ ...form, private_key_pem: e.target.value })} /></Field>
            <Field label="Redirect URL" hint="Must match the one registered with Enable Banking exactly."><input className="input" value={form.redirect_url} onChange={(e) => setForm({ ...form, redirect_url: e.target.value })} /></Field>
            <Field label="Your names as banks write them" hint="One per line — transfers to these are your own accounts."><textarea className="input h-20 py-2 text-xs" value={form.owner_names} onChange={(e) => setForm({ ...form, owner_names: e.target.value })} /></Field>
            <div className="flex gap-2"><button className="btn-primary" onClick={saveSettings}>Save</button><button className="btn-ghost" onClick={() => setForm(null)}>Cancel</button></div>
          </div>
        )}
      </Card>
    </div>
  )
}

function BankGuide() {
  return (
    <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink2">
      <li>Create a free account at <b>enablebanking.com</b> and register an application (environment: production, personal use).</li>
      <li>Set its redirect URL to this app's address, e.g. <code className="rounded bg-sunken px-1">{window.location.origin}/</code> — it must match exactly.</li>
      <li>Download the application's private key (PEM) and copy the application ID.</li>
      <li>Paste both below with the same redirect URL, save, then press <b>Connect</b> and approve access in your bank.</li>
      <li>Map each bank account to a ledger account. Sync from <b>Ledger → Bank inbox</b>; consent lasts up to 180 days.</li>
    </ol>
  )
}
