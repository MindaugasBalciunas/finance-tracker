import { useEffect, useState } from 'react'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useBankConnections, useInbox, useRefresh } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { dayLabel, eurc } from '../../lib/format'
import type { BankSyncResult, CommitResult, InboxRow } from '../../lib/types'
import { Empty, ErrorBox, Field, Loading, NumberInput, Segmented, Sheet, Spinner, useToast } from '../../components/ui'
import { AccountSelect, CategoryPicker, MerchantInput, NoteInput, TagInput, TRANSFER_FROM_KINDS, TRANSFER_TO_KINDS } from '../../components/pickers'
import { Icon } from '../../components/Icon'
import { MerchantMark } from '../../components/MerchantLogo'
import { DateInput } from '../../components/DateInput'

// A transfer between two synced banks arrives twice, once from each; the app keeps one.
const mergedText = (n: number) => `${n} transfer${n === 1 ? '' : 's'} merged (both banks reported ${n === 1 ? 'it' : 'them'})`

// ── bank inbox ──────────────────────────────────────────────────────

export function Inbox() {
  const [view, setView] = useState<'open' | 'dismissed' | 'imported'>('open')
  const { data, isLoading, error } = useInbox(view)
  const refresh = useRefresh()
  const toast = useToast()
  const [syncing, setSyncing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [edit, setEdit] = useState<InboxRow | null>(null)
  const { data: conns } = useBankConnections()

  const rows = data ?? []
  const ready = rows.filter((r) => !r.pending && r.verdict !== 'needs_review')
  const pending = rows.filter((r) => r.pending)
  const review = rows.filter((r) => !r.pending && r.verdict === 'needs_review')
  const [picked, setPicked] = useState<Set<number>>(new Set())
  useEffect(() => {
    setPicked(new Set(ready.filter((r) => r.verdict === 'new' && !r.match && !r.guessed).map((r) => r.id)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const sync = async (days?: number) => {
    setSyncing(true)
    try {
      const r = await api.post<BankSyncResult>('/bank/sync', {}, days ? { days } : undefined)
      const failed = r.accounts.filter((a) => a.error || a.skipped)
      toast(`${r.new} new · ${r.auto_linked} linked${r.merged ? ` · ${mergedText(r.merged)}` : ''} · ${r.fetched} fetched${failed.length ? ` · ${failed.length} account(s) need attention` : ''}`, failed.length ? 'bad' : 'good')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setSyncing(false)
    }
  }
  const commit = async (ids: number[]) => {
    setBusy(true)
    try {
      const r = await api.post<CommitResult>('/bank/inbox/commit', { ids })
      toast(`${r.imported.length} added${r.merged ? ` · ${mergedText(r.merged)}` : ''}${r.skipped ? ` · ${r.skipped} skipped` : ''}`, 'good')
      if (r.notes?.length) setTimeout(() => toast(r.notes!.join(' · ')), 400)
      refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    } finally {
      setBusy(false)
    }
  }
  const state = async (ids: number[], action: 'dismiss' | 'restore') => {
    try {
      await api.post(`/bank/inbox/${action}`, { ids })
      refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const link = async (r: InboxRow) => {
    try {
      await api.post(`/bank/inbox/${r.id}/link`, { tx_id: r.match!.id })
      toast('Linked to your entry', 'good')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const live = (conns ?? []).filter((c) => c.status === 'authorized')

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Segmented value={view} onChange={setView} size="sm" options={[{ value: 'open', label: 'To review' }, { value: 'imported', label: 'Added' }, { value: 'dismissed', label: 'Dismissed' }]} />
        <div className="ml-auto flex gap-2">
          <button className="btn-outline h-9" onClick={() => sync(7)} disabled={syncing}>Last 7 days</button>
          <button className="btn-primary h-9" onClick={() => sync()} disabled={syncing}>{syncing ? <Spinner className="h-4 w-4" /> : <Icon name="refresh" size={16} />}Sync banks</button>
        </div>
      </div>
      {live.length === 0 && (
        <div className="mb-3 rounded-xl border border-line bg-sunken px-3 py-2 text-sm text-ink2">No live bank connection. Connect Swedbank / SEB under <a className="text-accent" href="#/settings/banks">Settings → Banks</a>.</div>
      )}
      <ErrorBox error={error} />
      {isLoading ? <Loading /> : rows.length === 0 ? (
        <Empty title={view === 'open' ? 'Inbox zero' : 'Nothing here'}>{view === 'open' ? 'Every bank transaction is in your ledger.' : ''}</Empty>
      ) : view !== 'open' ? (
        <div className="card divide-y divide-line">
          {rows.map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-4 py-2.5">
              <InboxSummary r={r} />
              {view === 'dismissed' && <button className="btn-outline h-8 text-xs" onClick={() => state([r.id], 'restore')}>Restore</button>}
            </div>
          ))}
        </div>
      ) : (
        <div className="space-y-4">
          {review.length > 0 && <InboxGroup title="Needs your input" rows={review} picked={picked} setPicked={setPicked} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')} />}
          <InboxGroup title="Ready" rows={ready} picked={picked} setPicked={setPicked} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')}
            action={picked.size > 0 && <button className="btn-primary h-8 text-xs" disabled={busy} onClick={() => commit([...picked])}>Add {picked.size} to ledger</button>} />
          {pending.length > 0 && <InboxGroup title="Reserved by the bank (not booked yet)" rows={pending} picked={new Set()} setPicked={() => {}} onEdit={setEdit} onLink={link} onDismiss={(id) => state([id], 'dismiss')} muted
            action={<button className="btn-outline h-8 text-xs" disabled={busy} onClick={() => commit(pending.filter((r) => r.category).map((r) => r.id))}>Add as pending</button>} />}
        </div>
      )}
      {edit && <InboxEditor row={edit} onClose={() => setEdit(null)} onDismiss={(id) => state([id], 'dismiss')} />}
    </div>
  )
}

function InboxSummary({ r }: { r: InboxRow }) {
  const cats = useCats()
  return (
    <div className="flex min-w-0 flex-1 items-center gap-3">
      <MerchantMark merchant={r.merchant || r.raw_payee} category={r.category} transfer={r.kind === 'transfer'} to={r.to_account_id} size={32} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-2">
          <span className="truncate text-sm font-medium">{r.merchant || r.raw_payee || r.note}</span>
          <span className={clsx('tnum text-sm font-semibold', r.kind === 'income' && 'text-good')}>{r.kind === 'income' ? '+' : r.kind === 'expense' ? '−' : ''}{eurc(r.amount)}</span>
        </div>
        <div className="truncate text-xs text-muted">{dayLabel(r.date)} · {r.category ? cats.path(r.category) : 'no category'}{r.tags.length ? ' · ' + r.tags.join(', ') : ''}</div>
      </div>
    </div>
  )
}

function InboxGroup({ title, rows, picked, setPicked, onEdit, onLink, onDismiss, action, muted }: {
  title: string; rows: InboxRow[]; picked: Set<number>; setPicked: (s: Set<number>) => void; onEdit: (r: InboxRow) => void
  onLink: (r: InboxRow) => void; onDismiss: (id: number) => void; action?: React.ReactNode; muted?: boolean
}) {
  if (!rows.length) return null
  return (
    <section className="card overflow-hidden">
      <div className="flex items-center justify-between gap-2 border-b border-line px-4 py-2.5">
        <div className="text-sm font-semibold">{title} <span className="text-muted font-normal">{rows.length}</span></div>
        {action}
      </div>
      <div className="divide-y divide-line">
        {rows.map((r) => {
          const on = picked.has(r.id)
          return (
            <div key={r.id} className={clsx('px-4 py-2.5', muted && 'opacity-70')}>
              <div className="flex items-center gap-3">
                {!muted && (
                  <button onClick={() => { const n = new Set(picked); on ? n.delete(r.id) : n.add(r.id); setPicked(n) }}
                    className={clsx('grid h-6 w-6 shrink-0 place-items-center rounded-md border', on ? 'border-accent bg-accent text-white' : 'border-axis text-transparent')} aria-label="Pick">
                    <Icon name="check" size={14} />
                  </button>
                )}
                <button className="min-w-0 flex-1 text-left" onClick={() => onEdit(r)}><InboxSummary r={r} /></button>
              </div>
              {/* Every row can be dismissed, not only one with a note: a clean
                  row the bank sent (a Revolut pocket move, say) is often the
                  one you don't want in the ledger. */}
              <div className="mt-1.5 flex flex-wrap items-center gap-2 pl-9 text-xs">
                {r.match ? (
                  <>
                    <span className="text-ink2">Looks like your entry “{r.match.merchant || r.match.note}” on {dayLabel(r.match.date)}</span>
                    <button className="chip-on" onClick={() => onLink(r)}><Icon name="link" size={12} />Link to it</button>
                  </>
                ) : r.guessed ? (
                  <span className="text-warn">Category is a guess — check it</span>
                ) : r.verdict_note ? (
                  <span className="text-muted">{r.verdict_note}</span>
                ) : null}
                <button className="ml-auto -my-2 px-2 py-2 text-muted hover:text-bad" onClick={() => onDismiss(r.id)} aria-label={`Dismiss ${r.merchant || r.raw_payee || r.note || 'bank row'}`}>Dismiss</button>
              </div>
            </div>
          )
        })}
      </div>
    </section>
  )
}

function InboxEditor({ row, onClose, onDismiss }: { row: InboxRow; onClose: () => void; onDismiss: (id: number) => void }) {
  const [r, setR] = useState(row)
  const [raw, setRaw] = useState('')
  const refresh = useRefresh()
  const toast = useToast()
  const save = async (andAdd: boolean) => {
    try {
      await api.put(`/bank/inbox/${r.id}`, { date: r.date, amount: r.amount, category: r.category, merchant: r.merchant, note: r.note, tags: r.tags, account_id: r.account_id, to_account_id: r.to_account_id })
      if (andAdd) await api.post('/bank/inbox/commit', { ids: [r.id] })
      toast(andAdd ? 'Added to ledger' : 'Saved', 'good')
      refresh()
      onClose()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  return (
    <Sheet open onClose={onClose} title="Bank transaction" footer={<>
      <button className="btn-ghost mr-auto text-bad" onClick={() => { onDismiss(r.id); onClose() }}>Dismiss</button>
      <button className="btn-ghost" onClick={() => save(false)}>Save</button>
      <button className="btn-primary" onClick={() => save(true)}>{r.pending ? 'Save & add as pending' : 'Save & add'}</button>
    </>}>
      <div className="space-y-4">
        <div className="rounded-xl bg-sunken px-3 py-2 text-xs text-ink2">
          <div className="font-medium text-ink">{r.raw_payee || '—'}</div>
          <div className="break-words">{r.raw_details}</div>
          <div className="mt-1 flex gap-3 text-muted"><span>{r.date}</span><span>{r.raw_currency}</span>{r.pending && <span className="text-warn">reserved</span>}</div>
          {!raw ? <button className="mt-1 text-accent" onClick={async () => setRaw((await api.get<{ raw: string }>(`/bank/inbox/${r.id}/raw`)).raw)}>Show raw bank data</button>
            : <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap text-[10px]">{JSON.stringify(JSON.parse(raw || '{}'), null, 2)}</pre>}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <label><span className="label">Amount €</span><NumberInput value={r.amount} onChange={(v) => setR({ ...r, amount: v as number })} /></label>
          <label><span className="label">Date</span><DateInput clearable value={r.date} onChange={(v) => setR({ ...r, date: v })} /></label>
        </div>
        <Field label="Merchant"><MerchantInput value={r.merchant} onChange={(v) => setR({ ...r, merchant: v })} placeholder={r.raw_payee || 'e.g. Maxima'} /></Field>
        <div><span className="label">Category</span><CategoryPicker value={r.category} onChange={(id, kind) => setR({ ...r, category: id, kind })} /></div>
        {r.kind === 'transfer' && (
          <div className="grid grid-cols-2 gap-3">
            <div><span className="label">From</span><AccountSelect value={r.account_id} onChange={(v) => setR({ ...r, account_id: v })} kinds={TRANSFER_FROM_KINDS} placeholder="Outside" /></div>
            <div><span className="label">To</span><AccountSelect value={r.to_account_id} onChange={(v) => setR({ ...r, to_account_id: v })} kinds={TRANSFER_TO_KINDS} placeholder="Outside" /></div>
          </div>
        )}
        <Field label="Note"><NoteInput value={r.note} onChange={(v) => setR({ ...r, note: v })} merchant={r.merchant || r.raw_payee} /></Field>
        <div><span className="label">Tags</span><TagInput value={r.tags} onChange={(v) => setR({ ...r, tags: v })} /></div>
      </div>
    </Sheet>
  )
}
