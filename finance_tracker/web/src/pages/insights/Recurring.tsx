import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { catIcon, useCats } from '../../lib/categories'
import { eur, eurc, parseNum, shortDate } from '../../lib/format'
import type { Recurring } from '../../lib/types'
import { Card, Empty, ErrorBox, Field, Loading, Segmented, Sheet, Stat, useToast } from '../../components/ui'
import { AccountSelect, CategoryPicker, SPEND_KINDS, TRANSFER_FROM_KINDS, TRANSFER_TO_KINDS } from '../../components/pickers'
import { useAccounts } from '../../lib/hooks'
import { Icon, IconTile } from '../../components/Icon'

// ── recurring ───────────────────────────────────────────────────────

const CADENCE_LABEL: Record<string, string> = { monthly: 'monthly', quarterly: 'quarterly', yearly: 'yearly' }
const KIND_TITLE: Record<string, string> = { bill: 'Bills', transfer: 'Transfers & standing orders', income: 'Income' }
const KIND_HINT: Record<string, string> = {
  bill: 'Money leaving on a schedule',
  transfer: 'Between your accounts — standing orders, or ones you plan to make. Not spending.',
  income: 'Money arriving on a schedule',
}

export function RecurringView() {
  const { data: accounts } = useAccounts()
  const acctName = (id?: string) => (id ? accounts?.find((a) => a.id === id)?.name ?? id : '')
  const { data, isLoading } = useQuery({ queryKey: ['recurring'], queryFn: () => api.get<{ items: Recurring[]; hidden: Recurring[]; monthly_total: number; suggestions?: any[] }>('/insights/recurring') })
  const cats = useCats()
  const [edit, setEdit] = useState<{ r: Recurring | null; prefill?: Partial<EditorValues> } | null>(null)
  const [showHidden, setShowHidden] = useState(false)
  const refresh = useRefresh()
  if (isLoading || !data) return <Loading />
  const restore = async (r: Recurring) => { await api.put(`/recurring/${r.id}`, { ...toItem(r), hidden: false }); refresh() }
  const dismiss = async (s: any) => { await api.post('/recurring', { merchant: s.name, category: s.category, cadence: 'monthly', amount: s.amount, hidden: true }); refresh() }
  const suggestions = data.suggestions ?? []
  // Telia phone and Telia internet: the note goes in the title so the two rows differ.
  const sharesMerchant = (r: Recurring) => data.items.filter((x) => x.merchant.toLowerCase() === r.merchant.toLowerCase()).length > 1
  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between gap-3">
        <Stat icon="repeat" color="var(--s7)" label="Recurring costs" value={`${eur(data.monthly_total)}/mo`} sub={`${eur(data.monthly_total * 12)} a year across ${data.items.filter((r) => r.kind === 'bill').length} bills`} />
        <button className="btn-primary shrink-0" onClick={() => setEdit({ r: null })}><Icon name="plus" size={16} />Add</button>
      </div>
      {suggestions.length > 0 && (
        <Card pad={false} icon="spark" color="var(--s4)" title="Looks recurring" action={<span className="text-xs text-muted">from your last 12 months</span>}>
          <div className="divide-y divide-line border-t border-line">
            {suggestions.map((s) => (
              <div key={s.name} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-4 py-2.5">
                <IconTile name={catIcon(s.category)[0]} color={catIcon(s.category)[1]} size={34} round />
                <div className="min-w-0 flex-1 basis-[calc(100%-3rem)] sm:basis-0">
                  <div className="truncate text-sm font-medium">{s.name}</div>
                  <div className="truncate text-xs text-muted">{eur(s.amount)} · {s.flexible ? `about every ${s.every_days} days (±${s.spread_days})` : 'monthly'} · {s.count}× · next around {shortDate(s.next)}</div>
                  {s.notes?.length > 0 && <div className="truncate text-[11px] text-muted">“{s.notes.slice(0, 3).join('”, “')}”</div>}
                </div>
                <div className="flex w-full justify-end gap-1.5 sm:w-auto">
                  <button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => dismiss(s)}>Not recurring</button>
                  <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => setEdit({ r: null, prefill: { merchant: s.name, category: s.category, amount: String(s.amount), next_date: s.next,
                    cadence: s.flexible ? 'flexible' : 'monthly', every_days: s.flexible ? String(s.every_days) : '' } })}>Add</button>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
      {!data.items.length ? <Empty title="Nothing recurring yet">Add rent, insurance, standing orders or anything on a schedule.</Empty> : (['bill', 'transfer', 'income'] as const).map((kind) => {
        const rows = data.items.filter((r) => (r.kind ?? 'bill') === kind)
        if (!rows.length) return null
        return (
          <Card key={kind} pad={false} title={<span>{KIND_TITLE[kind]} <span className="font-normal text-muted">{rows.length}</span></span>}>
            <div className="-mt-1 px-4 pb-2 text-xs text-muted">{KIND_HINT[kind]}</div>
            <div className="divide-y divide-line border-t border-line">
              {rows.map((r) => (
                <button key={`${r.id ?? ""}|${r.merchant}|${r.note ?? ""}`} onClick={() => setEdit({ r })} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40">
                  {kind === 'bill' ? <IconTile name={catIcon(r.category)[0]} color={catIcon(r.category)[1]} size={34} round />
                    : <IconTile name={kind === 'income' ? 'briefcase' : 'swap'} color={kind === 'income' ? 'var(--s6)' : 'var(--s7)'} size={34} round />}
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 truncate text-sm font-medium">
                      <span className="truncate">{r.merchant}{sharesMerchant(r) && r.note ? <span className="font-normal text-ink2"> · {r.note}</span> : null}</span>
                      {r.every_days ? <span className="shrink-0 rounded-full bg-sunken px-1.5 text-[10px] text-ink2" title={`about every ${r.every_days} days`}>~{r.every_days}d</span>
                        : r.cadence !== 'monthly' && <span className="shrink-0 rounded-full bg-sunken px-1.5 text-[10px] text-ink2">{CADENCE_LABEL[r.cadence]}</span>}
                      {r.source === 'manual' && <span className="shrink-0 rounded-full bg-accent/10 px-1.5 text-[10px] text-accent">{kind === 'transfer' && !r.last ? 'planned' : 'added'}</span>}
                      {r.source === 'edited' && <span className="shrink-0 rounded-full bg-accent/10 px-1.5 text-[10px] text-accent">edited</span>}
                      {r.done && <span className="shrink-0 rounded-full bg-good/15 px-1.5 text-[10px] text-good">done this month</span>}
                      {r.overdue && <span className="shrink-0 rounded-full bg-warn/15 px-1.5 text-[10px] text-warn">not yet</span>}
                    </div>
                    <div className="truncate text-xs text-muted">
                      {kind === 'transfer' ? `${acctName(r.from_account) || '?'} → ${acctName(r.to_account) || '?'}`
                        : kind === 'income' ? `into ${acctName(r.to_account) || '—'}`
                        : [r.category ? cats.path(r.category) : 'No category', r.from_account && `from ${acctName(r.from_account)}`].filter(Boolean).join(' · ')}
                      {r.next && <> · {r.done ? 'next' : r.overdue ? 'was due' : 'next'} {r.source === 'detected' && !r.day ? 'around ' : ''}{shortDate(r.next)}</>}{r.note && !sharesMerchant(r) ? ` · ${r.note}` : ''}
                    </div>
                  </div>
                  <div className="text-right">
                    <div className={clsx('tnum text-sm font-semibold', kind === 'income' && 'text-good')}>{kind === 'income' ? '+' : ''}{eur(r.amount)}</div>
                    {kind === 'bill' && (r.cadence !== 'monthly' || r.every_days ? <div className="text-xs text-muted tnum">{eur(r.monthly)}/mo</div> : r.changed && <div className="text-xs text-warn tnum">last {eur(r.last_amount)}</div>)}
                  </div>
                  <Icon name="chevronR" size={14} className="text-muted" />
                </button>
              ))}
            </div>
          </Card>
        )
      })}
      {data.hidden.length > 0 && (
        <div>
          <button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowHidden(!showHidden)} aria-expanded={showHidden}>
            <Icon name="chevronD" size={14} className={clsx('transition', showHidden && 'rotate-180')} />Not recurring ({data.hidden.length})
          </button>
          {showHidden && (
            <Card pad={false} className="mt-2">
              <div className="divide-y divide-line">
                {data.hidden.map((r) => (
                  <div key={`${r.id ?? ""}|${r.merchant}|${r.note ?? ""}`} className="flex items-center gap-3 px-4 py-2 text-sm">
                    <span className="min-w-0 flex-1 truncate text-ink2">{r.merchant}</span>
                    <span className="tnum text-muted">{eur(r.amount)}</span>
                    <button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => restore(r)}>Restore</button>
                  </div>
                ))}
              </div>
            </Card>
          )}
        </div>
      )}
      {edit && <RecurringEditor r={edit.r} prefill={edit.prefill} onClose={() => setEdit(null)} />}
    </div>
  )
}

const toItem = (r: Recurring) => ({ merchant: r.merchant, category: r.category, cadence: r.cadence, amount: r.amount, next_date: r.next, note: r.note ?? '', every_days: r.every_days ?? 0,
  kind: r.kind ?? 'bill', from_account: r.from_account ?? '', to_account: r.to_account ?? '', day: r.day ?? 0 })

type EditorValues = { merchant: string; category: string; cadence: string; amount: string; next_date: string; note: string; every_days: string
  kind: string; from_account: string; to_account: string; day: string }

/** Add a recurring cost, correct a detected one, or mark it not recurring. */
function RecurringEditor({ r, prefill, onClose }: { r: Recurring | null; prefill?: Partial<EditorValues>; onClose: () => void }) {
  const [v, setV] = useState<EditorValues>({ merchant: r?.merchant ?? '', category: r?.category ?? '', cadence: r?.every_days ? 'flexible' : r?.cadence ?? 'monthly',
    amount: r ? String(r.amount) : '', next_date: r?.next ?? '', note: r?.note ?? '', every_days: r?.every_days ? String(r.every_days) : '',
    kind: r?.kind ?? 'bill', from_account: r?.from_account ?? '', to_account: r?.to_account ?? '', day: r?.day ? String(r.day) : '', ...prefill })
  const isBill = v.kind === 'bill'
  const kindLocked = !!r && r.source !== 'manual' // a detected bill stays a bill
  const refresh = useRefresh()
  const toast = useToast()
  const done = (msg: string) => { refresh(); toast(msg, 'good'); onClose() }
  const body = (extra: object = {}) => {
    const amount = parseNum(v.amount)
    if (!v.merchant.trim()) throw new Error('Name it — the merchant or payee')
    if (amount === undefined || amount < 0) throw new Error('Amount must be a number')
    // Flexible = "about every N days" (stored as a monthly item with a rhythm).
    const every = v.cadence === 'flexible' ? parseNum(v.every_days) : 0
    if (v.cadence === 'flexible' && (!every || every < 7 || every > 400)) throw new Error('About every how many days? (7–400)')
    const day = v.day ? parseNum(v.day) : 0
    if (day === undefined || day < 0 || day > 31) throw new Error('Day of month is 1–31')
    if (v.kind === 'transfer' && (!v.from_account || !v.to_account)) throw new Error('Pick both accounts for the transfer')
    if (v.kind === 'transfer' && v.from_account === v.to_account) throw new Error('A transfer needs two different accounts')
    return { ...v, merchant: v.merchant.trim(), amount, cadence: v.cadence === 'flexible' ? 'monthly' : v.cadence, every_days: Math.round(every ?? 0), day: Math.round(day), ...extra }
  }
  const save = useMutation({
    mutationFn: () => (r?.id ? api.put(`/recurring/${r.id}`, body()) : api.post('/recurring', body())),
    onSuccess: () => done('Saved'),
  })
  const hide = useMutation({
    mutationFn: () => (r?.id ? api.put(`/recurring/${r.id}`, { ...toItem(r), hidden: true }) : api.post('/recurring', { ...toItem(r!), hidden: true })),
    onSuccess: () => done('Marked not recurring'),
  })
  const remove = useMutation({ mutationFn: () => api.del(`/recurring/${r!.id}`), onSuccess: () => done(r?.source === 'manual' ? 'Removed' : 'Back to the detected values') })
  const err = save.error || hide.error || remove.error
  return (
    <Sheet open onClose={onClose} title={r ? r.merchant : 'Add recurring'} footer={<>
      {r && r.source !== 'manual' && <button className="btn-danger mr-auto" onClick={() => hide.mutate()}>Not recurring</button>}
      {r?.source === 'manual' && <button className="btn-danger mr-auto" onClick={() => confirm(`Remove ${r.merchant}?`) && remove.mutate()}>Remove</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={save.isPending}>Save</button>
    </>}>
      <div className="space-y-4">
        {r?.source === 'detected' && <div className="rounded-xl bg-sunken p-3 text-xs text-ink2">Found in your transactions: {r.count} charges, last {shortDate(r.last)} ({eurc(r.last_amount)}). Saving keeps your values from now on.</div>}
        {!kindLocked && <Segmented value={v.kind} onChange={(k) => setV({ ...v, kind: k, category: '' })} options={[{ value: 'bill', label: 'Bill' }, { value: 'transfer', label: 'Transfer' }, { value: 'income', label: 'Income' }]} />}
        {v.kind === 'transfer' && <div className="rounded-xl bg-sunken p-3 text-xs text-ink2">A standing order, or a transfer you plan to make each month. It counts in <b>Cash until payday</b> and is ticked off when the real transfer appears — the app never moves money or creates transactions.</div>}
        <Field label={isBill ? 'Merchant or payee' : 'Name'}><input className="input" value={v.merchant} disabled={!!r && r.source !== 'manual'} onChange={(e) => setV({ ...v, merchant: e.target.value })} placeholder={v.kind === 'transfer' ? 'e.g. IBKR monthly' : v.kind === 'income' ? 'e.g. Child benefit' : 'e.g. Landlord'} /></Field>
        {v.kind === 'transfer' ? (
          <div className="grid grid-cols-2 gap-3">
            <Field label="From"><AccountSelect value={v.from_account} onChange={(id) => setV({ ...v, from_account: id })} kinds={TRANSFER_FROM_KINDS} placeholder="Choose" /></Field>
            <Field label="To"><AccountSelect value={v.to_account} onChange={(id) => setV({ ...v, to_account: id })} kinds={TRANSFER_TO_KINDS} placeholder="Choose" /></Field>
          </div>
        ) : v.kind === 'income' ? (
          <Field label="Paid into"><AccountSelect value={v.to_account} onChange={(id) => setV({ ...v, to_account: id })} kinds={SPEND_KINDS} placeholder="Choose" /></Field>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Category"><CategoryPicker value={v.category} kind="expense" onChange={(id) => setV({ ...v, category: id })} /></Field>
            <Field label="Paid from" hint="For Cash until payday"><AccountSelect value={v.from_account} onChange={(id) => setV({ ...v, from_account: id })} kinds={SPEND_KINDS} placeholder="Usual account" /></Field>
          </div>
        )}
        {v.kind === 'income' && <Field label="Category"><CategoryPicker value={v.category} kind="income" onChange={(id) => setV({ ...v, category: id })} /></Field>}
        <Field label="How often"><Segmented value={v.cadence} onChange={(c) => setV({ ...v, cadence: c, every_days: c === 'flexible' && !v.every_days ? '35' : v.every_days })}
          options={[{ value: 'monthly', label: 'Monthly' }, { value: 'quarterly', label: 'Quarterly' }, { value: 'yearly', label: 'Yearly' }, { value: 'flexible', label: 'Flexible' }]} /></Field>
        {v.cadence === 'flexible' && (
          <Field label="About every … days" hint="No fixed date — the next one is expected this long after the last payment (haircut, dentist, car wash).">
            <input className="input w-28 tnum" inputMode="numeric" value={v.every_days} onChange={(e) => setV({ ...v, every_days: e.target.value })} />
          </Field>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Amount €"><input className="input tnum" inputMode="decimal" value={v.amount} onChange={(e) => setV({ ...v, amount: e.target.value })} /></Field>
          {isBill || v.cadence === 'flexible'
            ? <Field label="Next charge"><input type="date" className="input" value={v.next_date} onChange={(e) => setV({ ...v, next_date: e.target.value })} /></Field>
            : <Field label="Day of month"><input className="input w-24 tnum" inputMode="numeric" value={v.day} placeholder="e.g. 11" onChange={(e) => setV({ ...v, day: e.target.value })} /></Field>}
        </div>
        <Field label="Note" hint="Two bills from one merchant? Give each a note (e.g. phone, internet) — payments are matched by the words in their note."><input className="input" value={v.note} onChange={(e) => setV({ ...v, note: e.target.value })} placeholder="optional, e.g. phone" /></Field>
        {r?.source === 'edited' && <button className="btn-ghost h-8 px-2 text-xs" onClick={() => remove.mutate()}><Icon name="refresh" size={14} />Reset to detected values</button>}
        {r && r.source !== 'manual' && <a className="block text-xs text-accent" href={`#/ledger?period=365&merchant=${encodeURIComponent(r.merchant)}`}>See its transactions →</a>}
        <ErrorBox error={err} />
      </div>
    </Sheet>
  )
}
