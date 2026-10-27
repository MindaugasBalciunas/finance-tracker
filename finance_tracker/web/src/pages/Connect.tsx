import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { Card, Spinner } from '../components/ui'
import { Icon, IconTile } from '../components/Icon'

const sentence = (m: string) => m.charAt(0).toUpperCase() + m.slice(1).replace(/\.?$/, '.')

const SCOPES = [
  { id: 'ro', title: 'Read only', text: 'See your accounts, transactions, plan, goals, investments and IBKR — change nothing.' },
  { id: 'rw', title: 'Read and write', text: 'Also add and edit transactions, rules, tags, the plan and goals when you ask it to. Never accepts bank rows, changes settings or touches backups.' },
]

/** The owner's yes or no to an AI app (a custom connector in Claude chat,
 *  Claude Code…) asking to connect. Reached from /oauth/authorize, behind
 *  the app's password and lock; the answer goes back to the app. */
export default function Connect() {
  const params = new URLSearchParams(useLocation().search)
  const req = Object.fromEntries(['client_id', 'redirect_uri', 'state', 'code_challenge', 'code_challenge_method', 'response_type'].map((k) => [k, params.get(k) ?? '']))
  const { data, error, isLoading } = useQuery({ queryKey: ['oauth-request', req], queryFn: () => api.get<{ client_name: string; redirect_host: string }>('/oauth/request', req), retry: false })
  const [scope, setScope] = useState('ro')
  const [busy, setBusy] = useState(false)
  const [fail, setFail] = useState('')
  const answer = async (allow: boolean) => {
    setBusy(true)
    setFail('')
    try {
      const r = await api.post<{ redirect: string }>('/oauth/approve', { ...req, allow, scope })
      window.location.assign(r.redirect)
    } catch (e) {
      setFail((e as Error).message)
      setBusy(false)
    }
  }
  return (
    <div className="mx-auto max-w-md space-y-4 pt-2">
      <Card icon="link">
        {isLoading ? <div className="grid h-40 place-items-center"><Spinner /></div> : error || !data ? (
          <div className="space-y-2">
            <div className="flex items-center gap-2 font-semibold"><Icon name="alert" size={18} className="text-bad" />This connection request can't be used</div>
            <p className="text-sm text-ink2">{sentence((error as Error)?.message ?? 'something is missing from it')} Start again from the AI app.</p>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center gap-3">
              <IconTile name="spark" color="var(--s7)" size={44} />
              <div className="min-w-0">
                <div className="text-lg font-semibold leading-tight">Connect {data.client_name}?</div>
                <div className="text-xs text-muted">It will answer back to {data.redirect_host}</div>
              </div>
            </div>
            <p className="text-sm text-ink2"><b className="text-ink">{data.client_name}</b> wants to use your Finance Tracker through its tools. Only connect an app you just added yourself.</p>
            <div className="space-y-2" role="radiogroup" aria-label="What it may do">
              {SCOPES.map((s) => (
                <button key={s.id} type="button" role="radio" aria-checked={scope === s.id} onClick={() => setScope(s.id)}
                  className={clsx('flex w-full items-start gap-3 rounded-xl border p-3 text-left transition', scope === s.id ? 'border-accent bg-accent/5' : 'border-line hover:bg-sunken')}>
                  <span className={clsx('mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full border', scope === s.id ? 'border-accent bg-accent text-white' : 'border-line')}>
                    {scope === s.id && <Icon name="check" size={12} />}
                  </span>
                  <span>
                    <span className="block text-sm font-semibold">{s.title}</span>
                    <span className="block text-xs text-ink2">{s.text}</span>
                  </span>
                </button>
              ))}
            </div>
            {fail && <div className="rounded-xl bg-bad/10 px-3 py-2 text-sm text-bad">{fail}</div>}
            <div className="flex gap-2">
              <button className="btn-ghost flex-1" disabled={busy} onClick={() => answer(false)}>Don't allow</button>
              <button className="btn-primary flex-1" disabled={busy} onClick={() => answer(true)}>{busy ? <Spinner className="h-4 w-4" /> : 'Allow'}</button>
            </div>
            <p className="text-[11px] text-muted">You can end the connection any time in Settings → Security.</p>
          </div>
        )}
      </Card>
    </div>
  )
}
