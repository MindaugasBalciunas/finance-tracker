import { createContext, ReactNode, useContext, useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import { api } from '../lib/api'
import { useQuery } from '@tanstack/react-query'
import { useDeleteTx, useMerchants, useSaveTx, useRefresh } from '../lib/hooks'
import { catColor, useCats } from '../lib/categories'
import { dayLabel, eurc, todayISO } from '../lib/format'
import type { Kind, Tx } from '../lib/types'
import { AccountSelect, CategoryPicker, SPEND_KINDS, TagInput, TRANSFER_FROM_KINDS, TRANSFER_TO_KINDS } from './pickers'
import { ErrorBox, Field, Segmented, Sheet, Spinner, useToast } from './ui'
import { Icon } from './Icon'

type Draft = Partial<Tx> & { kind: Kind }
type Ctx = { open: (t?: Partial<Tx>) => void; scan: () => void }
const EditorCtx = createContext<Ctx>({ open: () => {}, scan: () => {} })
export const useTxEditor = () => useContext(EditorCtx)

const blank = (): Draft => ({ date: todayISO(), kind: 'expense', amount: undefined, category: '', merchant: '', note: '', tags: [], account_id: 'swed', to_account_id: '' })

export function TxEditorProvider({ children }: { children: ReactNode }) {
  const [draft, setDraft] = useState<Draft | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const [scanning, setScanning] = useState(false)
  const toast = useToast()
  const scanFile = async (f: File) => {
    setScanning(true)
    try {
      const fd = new FormData()
      fd.append('image', f)
      const s = await api.post<any>('/ai/scan', fd)
      setDraft({ ...blank(), date: s.date || todayISO(), kind: s.kind, amount: s.amount, category: s.category, merchant: s.merchant, note: s.note,
        account_id: s.account_id || '', tags: s.tags ?? [], _remark: s.remark } as any)
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setScanning(false)
    }
  }
  return (
    <EditorCtx.Provider value={{ open: (t) => setDraft(t ? ({ ...blank(), ...t } as Draft) : blank()), scan: () => fileRef.current?.click() }}>
      {children}
      <input ref={fileRef} type="file" accept="image/*" capture="environment" hidden
        onChange={(e) => { const f = e.target.files?.[0]; if (f) scanFile(f); e.target.value = '' }} />
      {scanning && (
        <div className="fixed inset-0 z-[70] grid place-items-center bg-black/40">
          <div className="card flex items-center gap-3 px-5 py-4 text-sm"><Spinner /> Reading the receipt…</div>
        </div>
      )}
      {draft && <Editor draft={draft} onClose={() => setDraft(null)} />}
    </EditorCtx.Provider>
  )
}

function Editor({ draft, onClose }: { draft: Draft; onClose: () => void }) {
  const [t, setT] = useState<Draft>(draft)
  const [amount, setAmount] = useState(draft.amount != null ? String(draft.amount) : '')
  const [suggested, setSuggested] = useState<string[]>([])
  const [suppressed, setSuppressed] = useState<string[]>([])
  const [touchedCat, setTouchedCat] = useState(!!draft.id || !!draft.category)
  const save = useSaveTx()
  const del = useDeleteTx()
  const cats = useCats()
  const toast = useToast()
  const { data: merchants } = useMerchants()
  const isNew = !t.id
  const set = (patch: Partial<Draft>) => setT((x) => ({ ...x, ...patch }))

  // Fill category/tags from rules + history as the user types a merchant.
  useEffect(() => {
    if (!isNew || touchedCat || !(t.merchant || t.note)) return
    const h = setTimeout(async () => {
      try {
        const r = await api.post<any>('/transactions/suggest', { kind: t.kind, merchant: t.merchant, note: t.note, tags: t.tags })
        const s = r.suggestion as Tx
        if (s.category && cats.byId[s.category]?.kind === t.kind) {
          set({ category: s.category })
          setSuggestedCat(s.category)
        }
        const extra = (s.tags ?? []).filter((x) => !(t.tags ?? []).includes(x) && !suppressed.includes(x))
        if (extra.length) {
          setSuggested(extra)
          set({ tags: [...(t.tags ?? []), ...extra] })
        }
      } catch {}
    }, 350)
    return () => clearTimeout(h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [t.merchant, t.note, t.kind])

  const submit = async () => {
    const value = parseFloat(amount.replace(',', '.'))
    if (!(value > 0)) return toast('Enter an amount', 'bad')
    if (!t.category) return toast('Pick a category', 'bad')
    try {
      await save.mutateAsync({ ...t, amount: value, suppressed_tags: suppressed })
      if (makeRule && t.merchant) {
        await api.post('/rules', { pattern: t.merchant, set_category: t.category, add_tags: (t.tags ?? []).filter((x) => !x.startsWith('trip:') && !x.startsWith('owed:')), enabled: true })
        toast(`From now on ${t.merchant} → ${cats.path(t.category ?? '')}`, 'good')
      }
      toast(isNew ? 'Added' : 'Saved', 'good')
      onClose()
    } catch {}
  }
  const remove = async () => {
    if (!t.id || !confirm('Delete this transaction?')) return
    await del.mutateAsync(t.id)
    toast('Deleted')
    onClose()
  }
  const kinds: { value: Kind; label: string }[] = [
    { value: 'expense', label: 'Expense' }, { value: 'income', label: 'Income' }, { value: 'transfer', label: 'Transfer' },
  ]
  const remark = (draft as any)._remark as string | undefined
  const [splitOpen, setSplitOpen] = useState(false)
  const [describe, setDescribe] = useState('')
  const [assisting, setAssisting] = useState(false)
  const [makeRule, setMakeRule] = useState(false)
  const [suggestedCat, setSuggestedCat] = useState<string>('')
  const { data: owed } = useQuery({ queryKey: ['owed'], queryFn: () => api.get<Record<string, number>>('/owed'), enabled: t.kind === 'income' })
  const assist = async () => {
    if (!describe.trim()) return
    setAssisting(true)
    try {
      const s = await api.post<any>('/ai/assist', { text: describe })
      set({ kind: s.kind, date: s.date || t.date, category: s.category || t.category, merchant: s.merchant || t.merchant, note: s.note || t.note, account_id: s.account_id || t.account_id, tags: s.tags?.length ? s.tags : t.tags })
      if (s.amount) setAmount(String(s.amount))
      setTouchedCat(true)
      if (s.remark) toast(s.remark)
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setAssisting(false)
    }
  }
  const refresh = useRefresh()
  const { data: detail } = useQuery({ queryKey: ['tx', t.id], queryFn: () => api.get<{ transaction: Tx; parts: Tx[] | null }>(`/transactions/${t.id}`), enabled: !!t.id })
  const parts = detail?.parts ?? []
  const unsplit = async () => {
    await api.post(`/transactions/${t.id}/unsplit`)
    toast('Parts merged back', 'good')
    refresh()
    onClose()
  }
  if (splitOpen && t.id) return <SplitSheet tx={{ ...(t as Tx), amount: parseFloat(amount) || (t.amount as number) }} onClose={() => setSplitOpen(false)} onDone={onClose} />
  return (
    <Sheet open onClose={onClose} title={isNew ? 'New transaction' : 'Edit transaction'}
      footer={<>
        {!isNew && <button className="btn-danger mr-auto" onClick={remove}><Icon name="trash" size={16} />Delete</button>}
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={submit} disabled={save.isPending}>{save.isPending ? <Spinner className="h-4 w-4" /> : isNew ? 'Add' : 'Save'}</button>
      </>}>
      <div className="space-y-4">
        {remark && <div className="rounded-xl bg-sunken px-3 py-2 text-xs text-ink2"><b>Receipt:</b> {remark}</div>}
        {isNew && (
          <div className="flex gap-2">
            <input className="input" placeholder="Or describe it: “lunch 12.50 card today”" value={describe} onChange={(e) => setDescribe(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), assist())} />
            <button className="btn-outline shrink-0" onClick={assist} disabled={assisting || !describe.trim()} aria-label="Fill from description">{assisting ? <Spinner className="h-4 w-4" /> : <Icon name="spark" size={16} />}</button>
          </div>
        )}
        <Segmented value={t.kind} onChange={(k) => { set({ kind: k, category: '' }); setTouchedCat(false) }} options={kinds} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="Amount (€)">
            <input autoFocus={isNew} inputMode="decimal" className="input text-lg font-semibold tnum" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="0.00" />
          </Field>
          <Field label="Date"><input type="date" className="input" value={t.date} onChange={(e) => set({ date: e.target.value })} /></Field>
        </div>
        <Field label={t.kind === 'income' ? 'From (payer)' : t.kind === 'transfer' ? 'Counterparty' : 'Merchant'}>
          <input className="input" list="merchants" value={t.merchant ?? ''} onChange={(e) => set({ merchant: e.target.value })} placeholder="e.g. Maxima" />
          <datalist id="merchants">{(merchants ?? []).slice(0, 300).map((m) => <option key={m.merchant} value={m.merchant} />)}</datalist>
        </Field>
        <Field label="Category">
          <CategoryPicker value={t.category ?? ''} kind={t.kind} onChange={(id, kind) => { set({ category: id, kind }); setTouchedCat(true) }} />
        </Field>
        <div className={clsx('grid gap-3', t.kind === 'transfer' ? 'grid-cols-2' : 'grid-cols-1')}>
          <Field label={t.kind === 'income' ? 'Into account' : t.kind === 'transfer' ? 'From account' : 'Paid from'}>
            <AccountSelect value={t.account_id ?? ''} onChange={(v) => set({ account_id: v })} kinds={t.kind === 'transfer' ? TRANSFER_FROM_KINDS : SPEND_KINDS} placeholder={t.kind === 'transfer' ? 'Outside / payroll' : 'Not tracked'} />
          </Field>
          {t.kind === 'transfer' && (
            <Field label="To account"><AccountSelect value={t.to_account_id ?? ''} onChange={(v) => set({ to_account_id: v })} kinds={TRANSFER_TO_KINDS} placeholder="Outside" /></Field>
          )}
        </div>
        <Field label="Note"><input className="input" value={t.note ?? ''} onChange={(e) => set({ note: e.target.value })} placeholder="What was it for?" /></Field>
        {t.kind === 'income' && Object.entries(owed ?? {}).some(([, v]) => v > 0.005) && (
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            <span className="text-ink2">Someone paying you back?</span>
            {Object.entries(owed ?? {}).filter(([, v]) => v > 0.005).map(([who, v]) => (
              <button key={who} className={(t.tags ?? []).includes(`owed:${who}`) ? 'chip-on' : 'chip hover:bg-sunken'}
                onClick={() => { set({ category: 'refunds', tags: [...(t.tags ?? []).filter((x) => !x.startsWith('owed:')), `owed:${who}`], merchant: t.merchant || who }); if (!amount) setAmount(String(v)); setTouchedCat(true) }}>
                <span className="capitalize">{who}</span> owes {eurc(v)}
              </button>
            ))}
          </div>
        )}
        <Field label="Tags" hint={suggested.length ? `Added by your rules: ${suggested.join(', ')}` : 'People, trips (trip:name), properties'}>
          <TagInput value={t.tags ?? []} onChange={(v) => {
            const removed = (t.tags ?? []).filter((x) => !v.includes(x) && suggested.includes(x))
            if (removed.length) setSuppressed([...suppressed, ...removed])
            set({ tags: v })
          }} />
        </Field>
        {t.merchant && t.category && t.category !== suggestedCat && touchedCat && (
          <label className="flex items-center gap-2 text-sm text-ink2">
            <input type="checkbox" checked={makeRule} onChange={(e) => setMakeRule(e.target.checked)} className="h-4 w-4 accent-[rgb(var(--accent))]" />
            Always file <b className="text-ink">{t.merchant}</b> as {cats.path(t.category)}
          </label>
        )}
        {t.external_id && <div className="text-xs text-muted">From the bank · {t.external_id}</div>}
        {!isNew && !t.split_of && (
          <div className="rounded-xl border border-line p-3">
            <div className="flex items-center justify-between text-sm">
              <span className="font-medium">{parts.length ? `Split into ${parts.length + 1} parts` : 'Shared or mixed purchase?'}</span>
              <div className="flex gap-1">
                {parts.length > 0 && <button className="btn-ghost h-8 text-xs" onClick={unsplit}>Unsplit</button>}
                <button className="btn-outline h-8 text-xs" onClick={() => setSplitOpen(true)}>{parts.length ? 'Add part' : 'Split'}</button>
              </div>
            </div>
            {parts.map((p) => <div key={p.id} className="mt-1 flex justify-between text-xs text-ink2"><span>{cats.path(p.category)}{p.tags.find((x) => x.startsWith('owed:')) ? ` · ${p.tags.find((x) => x.startsWith('owed:'))!.slice(5)} owes` : ''}</span><span className="tnum">{eurc(p.amount)}</span></div>)}
          </div>
        )}
        {!!t.split_of && <div className="text-xs text-muted">Part of a split transaction #{t.split_of}</div>}
        <ErrorBox error={save.error || del.error} />
      </div>
    </Sheet>
  )
}

function SplitSheet({ tx, onClose, onDone }: { tx: Tx; onClose: () => void; onDone: () => void }) {
  const [parts, setParts] = useState([{ amount: '', category: tx.category, note: '', owed_by: '' }])
  const refresh = useRefresh()
  const toast = useToast()
  const cats = useCats()
  const used = parts.reduce((a, p) => a + (parseFloat(p.amount) || 0), 0)
  const save = async () => {
    try {
      await api.post(`/transactions/${tx.id}/split`, { parts: parts.filter((p) => parseFloat(p.amount) > 0).map((p) => ({ ...p, amount: parseFloat(p.amount) })) })
      toast('Split saved', 'good')
      refresh()
      onDone()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  return (
    <Sheet open onClose={onClose} title={`Split ${tx.merchant || cats.name(tx.category)} · ${eurc(tx.amount)}`} footer={<>
      <button className="btn-ghost" onClick={onClose}>Back</button>
      <button className="btn-primary" onClick={save} disabled={used <= 0 || used >= tx.amount}>Save split</button>
    </>}>
      <div className="space-y-3">
        <div className="text-sm text-muted">Carve parts off this transaction. The original keeps the remainder (<b className="text-ink tnum">{eurc(tx.amount - used)}</b>), so totals and balances don't change. Mark a part as owed when someone will pay you back.</div>
        {parts.map((p, i) => (
          <div key={i} className="space-y-2 rounded-xl border border-line p-3">
            <div className="grid grid-cols-2 gap-2">
              <input className="input tnum" inputMode="decimal" placeholder="Amount €" value={p.amount} onChange={(e) => setParts(parts.map((x, j) => (j === i ? { ...x, amount: e.target.value } : x)))} />
              <input className="input" placeholder="Owed by (optional)" value={p.owed_by} onChange={(e) => setParts(parts.map((x, j) => (j === i ? { ...x, owed_by: e.target.value } : x)))} />
            </div>
            <CategoryPicker value={p.category} kind={tx.kind} onChange={(id) => setParts(parts.map((x, j) => (j === i ? { ...x, category: id } : x)))} />
            <input className="input" placeholder="Note" value={p.note} onChange={(e) => setParts(parts.map((x, j) => (j === i ? { ...x, note: e.target.value } : x)))} />
          </div>
        ))}
        <button className="btn-outline" onClick={() => setParts([...parts, { amount: '', category: tx.category, note: '', owed_by: '' }])}><Icon name="plus" size={16} />Another part</button>
      </div>
    </Sheet>
  )
}

/** One ledger row. */
export function TxRow({ t, onClick, selected, onSelect, showDate = false }: { t: Tx; onClick?: () => void; selected?: boolean; onSelect?: () => void; showDate?: boolean }) {
  const cats = useCats()
  const sign = t.kind === 'income' ? '+' : t.kind === 'expense' ? '−' : ''
  const title = t.merchant || t.note || cats.name(t.category)
  const sub = [cats.path(t.category), t.merchant && t.note && t.note !== t.merchant ? t.note : '', showDate ? dayLabel(t.date) : ''].filter(Boolean).join(' · ')
  return (
    <div className={clsx('flex items-center gap-3 px-4 py-2.5 transition', onClick && 'cursor-pointer hover:bg-sunken/60', selected && 'bg-accent/5')} onClick={onClick}>
      {onSelect ? (
        <button onClick={(e) => { e.stopPropagation(); onSelect() }} className={clsx('grid h-9 w-9 shrink-0 place-items-center rounded-full border', selected ? 'border-accent bg-accent text-white' : 'border-line text-transparent')} aria-label="Select">
          <Icon name="check" size={16} />
        </button>
      ) : (
        <div className="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[13px] font-semibold text-white" style={{ background: t.kind === 'income' ? 'var(--s6)' : t.kind === 'transfer' ? 'var(--s-other)' : catColor(t.category) }}>
          {(title || '?').trim().charAt(0).toUpperCase()}
        </div>
      )}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-ink">{title}</div>
        <div className="truncate text-xs text-muted">{sub}</div>
      </div>
      <div className="shrink-0 text-right">
        <div className={clsx('tnum text-sm font-semibold', t.kind === 'income' ? 'text-good' : t.kind === 'transfer' ? 'text-ink2' : 'text-ink')}>{sign}{eurc(t.amount).replace('€', '€')}</div>
        {t.tags.length > 0 && <div className="max-w-[9rem] truncate text-[11px] text-muted">{t.tags.join(' · ')}</div>}
      </div>
    </div>
  )
}

export function useAfterChange() {
  return useRefresh()
}
