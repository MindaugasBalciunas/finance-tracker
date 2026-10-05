import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { useAccounts, useNetWorthHistory, useRefresh } from '../../lib/hooks'
import { eur, eurc, eurk, parseNum, pct, shortDate, todayISO } from '../../lib/format'
import { rangeFrom } from '../../lib/periods'
import { Card, Empty, ErrorBox, Field, Loading, Sheet, useToast } from '../../components/ui'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { Icon } from '../../components/Icon'
import { AccountSelect } from '../../components/pickers'

// ── loans ───────────────────────────────────────────────────────────

/** Structured loan terms (replaces hand-edited JSON). */
type Num = number | string // raw text while typing ("3." must survive), a number once saved
type LoanTerms = {
  lender?: string; asset_id?: string; base_rate_name?: string; base_rate?: Num; margin?: Num; rate_reset_date?: string
  monthly_payment?: Num; payment_day?: Num; start_date?: string; start_principal?: Num; end_date?: string
}
const LOAN_NUMS = ['base_rate', 'margin', 'monthly_payment', 'payment_day', 'start_principal'] as const
const toNum = (v?: Num) => (v == null || v === '' ? undefined : parseNum(String(v)) ?? NaN)

/** Numbers as numbers; rejects anything that isn't one. */
export function cleanTerms(d: LoanTerms): LoanTerms {
  const out: LoanTerms = { ...d }
  for (const k of LOAN_NUMS) {
    const n = toNum(d[k])
    if (n !== undefined && !Number.isFinite(n)) throw new Error(`${k.replace('_', ' ')} must be a number`)
    out[k] = n
  }
  const p = out.payment_day as number | undefined
  if (p != null && (p < 1 || p > 31 || !Number.isInteger(p))) throw new Error('Payment day must be 1–31')
  return out
}

export function LoanFields({ d, onChange }: { d: LoanTerms; onChange: (d: LoanTerms) => void }) {
  const num = (k: keyof LoanTerms) => ({
    className: 'input tnum', inputMode: 'decimal' as const, value: d[k] ?? '',
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...d, [k]: e.target.value === '' ? undefined : e.target.value }),
  })
  const txt = (k: keyof LoanTerms, type = 'text') => ({
    className: 'input', type, value: (d[k] as string) ?? '',
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...d, [k]: e.target.value || undefined }),
  })
  const rate = (toNum(d.base_rate) ?? 0) + (toNum(d.margin) ?? 0)
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Lender"><input {...txt('lender')} placeholder="SEB" /></Field>
        <Field label="Secured on"><AccountSelect value={d.asset_id ?? ''} onChange={(v) => onChange({ ...d, asset_id: v || undefined })} placeholder="Nothing" kinds={['property', 'vehicle']} /></Field>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <Field label="Base rate"><input {...txt('base_rate_name')} placeholder="6M EURIBOR" /></Field>
        <Field label="Base %"><input {...num('base_rate')} placeholder="2.10" /></Field>
        <Field label="Margin %"><input {...num('margin')} placeholder="1.85" /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Rate resets on" hint={rate > 0 ? `Rate now ${rate.toFixed(2)}%` : undefined}><input {...txt('rate_reset_date', 'date')} /></Field>
        <Field label="Monthly payment €" hint="Empty = computed from the end date"><input {...num('monthly_payment')} /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Started"><input {...txt('start_date', 'date')} /></Field>
        <Field label="Ends"><input {...txt('end_date', 'date')} /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Payment day"><input {...num('payment_day')} placeholder="17" /></Field>
        <Field label="Original principal €"><input {...num('start_principal')} /></Field>
      </div>
    </div>
  )
}

function LoanEditor({ id, onClose }: { id: string; onClose: () => void }) {
  const { data: accounts } = useAccounts()
  const a = accounts?.find((x) => x.id === id)
  const [d, setD] = useState<LoanTerms | null>(null)
  const [owed, setOwed] = useState('')
  const [owedDate, setOwedDate] = useState(todayISO())
  const refresh = useRefresh()
  const toast = useToast()
  useEffect(() => { if (a && !d) setD({ ...(a.details ?? {}) }) }, [a, d])
  const save = useMutation({
    mutationFn: async () => {
      if (!a || !d) return
      await api.put(`/accounts/${a.id}`, { ...a, details: { ...(a.details ?? {}), ...cleanTerms(d) } })
      if (owed.trim()) await api.post('/balances', { date: owedDate, values: [{ account_id: a.id, value: -Math.abs(parseNum(owed) ?? NaN) }] })
    },
    onSuccess: () => { refresh(); toast('Loan updated', 'good'); onClose() },
  })
  return (
    <Sheet open onClose={onClose} title={a ? `Edit ${a.name}` : 'Edit loan'} footer={<>
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={!d || save.isPending}>Save</button>
    </>}>
      {!d ? <Loading /> : (
        <div className="space-y-4">
          <LoanFields d={d} onChange={setD} />
          <div className="rounded-xl bg-sunken p-3">
            <div className="section-title mb-2">Balance owed</div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Owed €" hint={a?.balance != null ? `now ${eurc(Math.abs(a.balance))}` : undefined}><input className="input tnum" inputMode="decimal" value={owed} onChange={(e) => setOwed(e.target.value)} placeholder="leave empty to keep" /></Field>
              <Field label="As of"><input type="date" className="input" value={owedDate} onChange={(e) => setOwedDate(e.target.value)} /></Field>
            </div>
          </div>
          <ErrorBox error={save.error} />
        </div>
      )}
    </Sheet>
  )
}

export function Loans() {
  const [editing, setEditing] = useState<string | null>(null)
  const { data, isLoading } = useQuery({ queryKey: ['loans'], queryFn: () => api.get<any[]>('/loans') })
  const { data: hist } = useNetWorthHistory(rangeFrom('5y'), 'month', true)
  if (isLoading) return <Loading />
  if (!data?.length) return <Empty title="No loans">Add a loan account to track its balance, interest and payoff.</Empty>
  return (
    <div className="space-y-4">
      {data.map((l) => {
        const series = (hist ?? []).map((h) => ({ date: h.date, balance: -(h.by_account?.[l.account_id] ?? 0), equity: (h.by_account?.[l.details.asset_id] ?? 0) + (h.by_account?.[l.account_id] ?? 0) })).filter((x) => x.balance > 0)
        return (
          <Card key={l.account_id} title={`${l.name}${l.details.lender ? ` · ${l.details.lender}` : ''}`} action={<button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => setEditing(l.account_id)}><Icon name="edit" size={15} />Edit</button>}>
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
              <div><div className="text-xs text-muted">Owed</div><div className="text-xl font-semibold">{eur(l.balance)}</div><div className="text-xs text-muted">as of {shortDate(l.balance_date)}</div></div>
              <div><div className="text-xs text-muted">Rate</div><div className="text-xl font-semibold">{l.rate.toFixed(2)}%</div><div className="text-xs text-muted">{l.details.base_rate_name} {l.details.base_rate}% + {l.details.margin}%</div></div>
              <div><div className="text-xs text-muted">Home equity</div><div className="text-xl font-semibold">{eur(l.equity)}</div><div className="text-xs text-muted">LTV {pct(l.ltv)}</div></div>
              <div><div className="text-xs text-muted">Paid off</div><div className="text-xl font-semibold">{l.payoff_date}</div><div className="text-xs text-muted">{l.months_left} payments left</div></div>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-3 rounded-xl bg-sunken p-3 text-sm sm:grid-cols-4">
              <div><div className="text-xs text-muted">Next payment</div><div className="tnum">{eur(l.next_interest + l.next_principal)}</div></div>
              <div><div className="text-xs text-muted">…interest / principal</div><div className="tnum">{eur(l.next_interest)} / {eur(l.next_principal)}</div></div>
              <div><div className="text-xs text-muted">Last 12 months</div><div className="tnum">{eur(l.paid_interest_12m)} interest · {eur(l.paid_principal_12m)} principal</div></div>
              <div><div className="text-xs text-muted">Interest still to pay</div><div className="tnum">{eur(l.total_interest_left)}</div></div>
            </div>
            {l.start_principal > 0 && (
              <div className="mt-4">
                <div className="flex items-baseline justify-between text-sm">
                  <span className="text-ink2">Repaid <b className="tnum text-ink">{eurc(l.repaid)}</b> of {eurc(l.start_principal)}</span>
                  <span className="tnum text-xs text-muted">{pct(l.repaid_pct, 1)} · since {shortDate(l.details.start_date)}</span>
                </div>
                <div className="mt-1.5 h-2 rounded-full bg-sunken"><div className="h-full rounded-full bg-good" style={{ width: `${Math.min(100, Math.max(1, l.repaid_pct * 100))}%` }} /></div>
              </div>
            )}
            {l.purchase_price > 0 && l.equity > 0 && (() => {
              // Where the equity came from (the three parts sum to value − owed).
              const parts = [
                { label: 'Down payment', v: l.down_payment ?? 0, color: 'var(--s1)', hint: `${eurc(l.purchase_price)} purchase − ${eurc(l.start_principal)} borrowed` },
                { label: 'Principal repaid', v: l.repaid ?? 0, color: 'var(--s6)', hint: 'paid off the loan so far' },
                { label: l.appreciation >= 0 ? 'Rise in value' : 'Fall in value', v: l.appreciation ?? 0, color: 'var(--s5)', hint: `${eurc(l.purchase_price)} → ${eurc(l.asset_value)}` },
              ].filter((x) => x.v !== 0)
              const pos = parts.filter((x) => x.v > 0).reduce((a, x) => a + x.v, 0) || 1
              return (
                <div className="mt-4">
                  <div className="flex items-baseline justify-between text-sm"><span className="text-ink2">Where your equity comes from</span><b className="tnum">{eurc(l.equity)}</b></div>
                  <div className="mt-1.5 flex h-2.5 gap-0.5 overflow-hidden rounded-full">
                    {parts.filter((x) => x.v > 0).map((x) => <div key={x.label} style={{ width: `${(x.v / pos) * 100}%`, background: x.color }} title={`${x.label}: ${eurc(x.v)}`} />)}
                  </div>
                  <div className="mt-2 grid grid-cols-1 gap-1 text-xs sm:grid-cols-3">
                    {parts.map((x) => (
                      <div key={x.label} className="flex items-center gap-1.5" title={x.hint}>
                        <span className="h-2 w-2 shrink-0 rounded-[3px]" style={{ background: x.color }} />
                        <span className="text-ink2">{x.label}</span>
                        <b className={clsx('ml-auto tnum sm:ml-1', x.v < 0 && 'text-bad')}>{x.v < 0 ? '−' : ''}{eurc(Math.abs(x.v))}</b>
                      </div>
                    ))}
                  </div>
                </div>
              )
            })()}
            {l.days_to_reset > 0 && <div className="mt-3 flex items-center gap-1.5 text-sm text-warn"><Icon name="alert" size={16} />Rate resets in {l.days_to_reset} days ({l.details.rate_reset_date})</div>}
            {series.length > 1 && (
              <div className="mt-4">
                <div className="h-52">
                  <ResponsiveContainer>
                    <LineChart data={series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
                      <CartesianGrid {...gridProps} />
                      <XAxis dataKey="date" {...axisProps} minTickGap={40} tickFormatter={(d) => d.slice(0, 4)} />
                      <YAxis {...axisProps} tickFormatter={eurk} width={48} />
                      <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={shortDate(label)} rows={payload.map((p: any) => ({ color: p.stroke, label: p.name, value: eur(p.value) }))} /> : null} />
                      <Line dataKey="balance" name="Owed" stroke="var(--s7)" strokeWidth={2} dot={false} isAnimationActive={false} />
                      <Line dataKey="equity" name="Equity" stroke="var(--s3)" strokeWidth={2} dot={false} isAnimationActive={false} />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
                <div className="mt-2"><Legend items={[{ color: 'var(--s7)', label: 'Owed' }, { color: 'var(--s3)', label: 'Equity' }]} /></div>
                <div className="mt-1 text-xs text-muted">Balances before the bank split principal and interest are reconstructed from payments.</div>
              </div>
            )}
          </Card>
        )
      })}
      {editing && <LoanEditor id={editing} onClose={() => setEditing(null)} />}
    </div>
  )
}
