import { createContext, ReactNode, useContext, useEffect, useRef, useState } from 'react'
import clsx from 'clsx'
import { api } from '../lib/api'
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
        if (s.category && cats.byId[s.category]?.kind === t.kind) set({ category: s.category })
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
  return (
    <Sheet open onClose={onClose} title={isNew ? 'New transaction' : 'Edit transaction'}
      footer={<>
        {!isNew && <button className="btn-danger mr-auto" onClick={remove}><Icon name="trash" size={16} />Delete</button>}
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={submit} disabled={save.isPending}>{save.isPending ? <Spinner className="h-4 w-4" /> : isNew ? 'Add' : 'Save'}</button>
      </>}>
      <div className="space-y-4">
        {remark && <div className="rounded-xl bg-sunken px-3 py-2 text-xs text-ink2"><b>Receipt:</b> {remark}</div>}
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
        <Field label="Tags" hint={suggested.length ? `Added by your rules: ${suggested.join(', ')}` : 'People, trips (trip:name), properties'}>
          <TagInput value={t.tags ?? []} onChange={(v) => {
            const removed = (t.tags ?? []).filter((x) => !v.includes(x) && suggested.includes(x))
            if (removed.length) setSuppressed([...suppressed, ...removed])
            set({ tags: v })
          }} />
        </Field>
        {t.external_id && <div className="text-xs text-muted">From the bank · {t.external_id}</div>}
        <ErrorBox error={save.error || del.error} />
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
