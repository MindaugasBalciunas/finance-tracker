import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { catIcon, useCats } from '../../lib/categories'
import { eur, eurc, parseNum, shortDate } from '../../lib/format'
import type { Recurring } from '../../lib/types'
import { Card, Empty, ErrorBox, Field, Loading, Segmented, Sheet, Stat, useToast } from '../../components/ui'
import { CategoryPicker } from '../../components/pickers'
import { Icon, IconTile } from '../../components/Icon'

// ── recurring ───────────────────────────────────────────────────────

const CADENCE_LABEL: Record<string, string> = { monthly: 'monthly', quarterly: 'quarterly', yearly: 'yearly' }

export function RecurringView() {
  const { data, isLoading } = useQuery({ queryKey: ['recurring'], queryFn: () => api.get<{ items: Recurring[]; hidden: Recurring[]; monthly_total: number; suggestions?: any[] }>('/insights/recurring') })
  const cats = useCats()
  const [edit, setEdit] = useState<{ r: Recurring | null; prefill?: Partial<EditorValues> } | null>(null)
  const [showHidden, setShowHidden] = useState(false)
  const refresh = useRefresh()
  if (isLoading || !data) return <Loading />
  const restore = async (r: Recurring) => { await api.put(`/recurring/${r.id}`, { ...toItem(r), hidden: false }); refresh() }
  const dismiss = async (s: any) => { await api.post('/recurring', { merchant: s.name, category: s.category, cadence: 'monthly', amount: s.amount, hidden: true }); refresh() }
  const suggestions = data.suggestions ?? []
  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between gap-3">
        <Stat icon="repeat" color="var(--s7)" label="Recurring costs" value={`${eur(data.monthly_total)}/mo`} sub={`${eur(data.monthly_total * 12)} a year across ${data.items.length} merchants`} />
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
      {!data.items.length ? <Empty title="Nothing recurring yet">Add rent, insurance or anything billed on a schedule.</Empty> : (
        <Card pad={false}>
          <div className="divide-y divide-line">
            {data.items.map((r) => (
              <button key={r.merchant} onClick={() => setEdit({ r })} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40">
                <IconTile name={catIcon(r.category)[0]} color={catIcon(r.category)[1]} size={34} round />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 truncate text-sm font-medium">
                    <span className="truncate">{r.merchant}</span>
                    {r.every_days ? <span className="shrink-0 rounded-full bg-sunken px-1.5 text-[10px] text-ink2" title={`about every ${r.every_days} days`}>~{r.every_days}d</span>
                      : r.cadence !== 'monthly' && <span className="shrink-0 rounded-full bg-sunken px-1.5 text-[10px] text-ink2">{CADENCE_LABEL[r.cadence]}</span>}
                    {r.source === 'manual' && <span className="shrink-0 rounded-full bg-accent/10 px-1.5 text-[10px] text-accent">added</span>}
                    {r.source === 'edited' && <span className="shrink-0 rounded-full bg-accent/10 px-1.5 text-[10px] text-accent">edited</span>}
                  </div>
                  <div className="truncate text-xs text-muted">{r.category ? cats.path(r.category) : 'No category'} · next {r.source === 'detected' || r.every_days ? 'around ' : ''}{shortDate(r.next)}{r.note ? ` · ${r.note}` : ''}</div>
                </div>
                <div className="text-right">
                  <div className="tnum text-sm font-semibold">{eur(r.amount)}</div>
                  {r.cadence !== 'monthly' || r.every_days ? <div className="text-xs text-muted tnum">{eur(r.monthly)}/mo</div> : r.changed && <div className="text-xs text-warn tnum">last {eur(r.last_amount)}</div>}
                </div>
                <Icon name="chevronR" size={14} className="text-muted" />
              </button>
            ))}
          </div>
        </Card>
      )}
      {data.hidden.length > 0 && (
        <div>
          <button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowHidden(!showHidden)} aria-expanded={showHidden}>
            <Icon name="chevronD" size={14} className={clsx('transition', showHidden && 'rotate-180')} />Not recurring ({data.hidden.length})
          </button>
          {showHidden && (
            <Card pad={false} className="mt-2">
              <div className="divide-y divide-line">
                {data.hidden.map((r) => (
                  <div key={r.merchant} className="flex items-center gap-3 px-4 py-2 text-sm">
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

const toItem = (r: Recurring) => ({ merchant: r.merchant, category: r.category, cadence: r.cadence, amount: r.amount, next_date: r.next, note: r.note ?? '', every_days: r.every_days ?? 0 })

type EditorValues = { merchant: string; category: string; cadence: string; amount: string; next_date: string; note: string; every_days: string }

/** Add a recurring cost, correct a detected one, or mark it not recurring. */
function RecurringEditor({ r, prefill, onClose }: { r: Recurring | null; prefill?: Partial<EditorValues>; onClose: () => void }) {
  const [v, setV] = useState<EditorValues>({ merchant: r?.merchant ?? '', category: r?.category ?? '', cadence: r?.every_days ? 'flexible' : r?.cadence ?? 'monthly',
    amount: r ? String(r.amount) : '', next_date: r?.next ?? '', note: r?.note ?? '', every_days: r?.every_days ? String(r.every_days) : '', ...prefill })
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
    return { ...v, merchant: v.merchant.trim(), amount, cadence: v.cadence === 'flexible' ? 'monthly' : v.cadence, every_days: Math.round(every ?? 0), ...extra }
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
    <Sheet open onClose={onClose} title={r ? r.merchant : 'Add recurring cost'} footer={<>
      {r && r.source !== 'manual' && <button className="btn-danger mr-auto" onClick={() => hide.mutate()}>Not recurring</button>}
      {r?.source === 'manual' && <button className="btn-danger mr-auto" onClick={() => confirm(`Remove ${r.merchant}?`) && remove.mutate()}>Remove</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={save.isPending}>Save</button>
    </>}>
      <div className="space-y-4">
        {r?.source === 'detected' && <div className="rounded-xl bg-sunken p-3 text-xs text-ink2">Found in your transactions: {r.count} charges, last {shortDate(r.last)} ({eurc(r.last_amount)}). Saving keeps your values from now on.</div>}
        <Field label="Merchant or payee"><input className="input" value={v.merchant} disabled={!!r && r.source !== 'manual'} onChange={(e) => setV({ ...v, merchant: e.target.value })} placeholder="e.g. Landlord" /></Field>
        <Field label="Category"><CategoryPicker value={v.category} kind="expense" onChange={(id) => setV({ ...v, category: id })} /></Field>
        <Field label="How often"><Segmented value={v.cadence} onChange={(c) => setV({ ...v, cadence: c, every_days: c === 'flexible' && !v.every_days ? '35' : v.every_days })}
          options={[{ value: 'monthly', label: 'Monthly' }, { value: 'quarterly', label: 'Quarterly' }, { value: 'yearly', label: 'Yearly' }, { value: 'flexible', label: 'Flexible' }]} /></Field>
        {v.cadence === 'flexible' && (
          <Field label="About every … days" hint="No fixed date — the next one is expected this long after the last payment (haircut, dentist, car wash).">
            <input className="input w-28 tnum" inputMode="numeric" value={v.every_days} onChange={(e) => setV({ ...v, every_days: e.target.value })} />
          </Field>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Amount €"><input className="input tnum" inputMode="decimal" value={v.amount} onChange={(e) => setV({ ...v, amount: e.target.value })} /></Field>
          <Field label="Next charge"><input type="date" className="input" value={v.next_date} onChange={(e) => setV({ ...v, next_date: e.target.value })} /></Field>
        </div>
        <Field label="Note"><input className="input" value={v.note} onChange={(e) => setV({ ...v, note: e.target.value })} placeholder="optional" /></Field>
        {r?.source === 'edited' && <button className="btn-ghost h-8 px-2 text-xs" onClick={() => remove.mutate()}><Icon name="refresh" size={14} />Reset to detected values</button>}
        {r && r.source !== 'manual' && <a className="block text-xs text-accent" href={`#/ledger?period=365&merchant=${encodeURIComponent(r.merchant)}`}>See its transactions →</a>}
        <ErrorBox error={err} />
      </div>
    </Sheet>
  )
}
