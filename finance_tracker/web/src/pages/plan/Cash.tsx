import { useState } from 'react'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { CashAccount, CashItem, CashPlan } from '../../lib/types'
import { useCashCalendar, useOverview } from '../../lib/hooks'
import { dayLabel, eur, parseNum, shortDate, todayISO } from '../../lib/format'
import { ErrorBox, Loading, useToast } from '../../components/ui'
import { useTxEditor } from '../../components/TxEditor'
import { Icon, IconTile } from '../../components/Icon'
import { CashCalendarCard } from './CashCalendar'

/** Plan → Cash: the money between now and the next salary — what leaves
 *  each account, the top-ups that keep it above its buffer — then the next
 *  60 days day by day. Home shows the next step of this as one strip. */
export function CashView() {
  const { data: o, isLoading, error } = useOverview()
  if (isLoading) return <Loading />
  if (error || !o) return <ErrorBox error={error} />
  return (
    <div className="space-y-4">
      {o.cash ? <UntilPayday c={o.cash} /> : <div className="card p-4 text-sm text-muted">Set where salary is paid in Plan → Income & salary to see cash until payday.</div>}
      <CashCalendarCard />
    </div>
  )
}

/** Home's line for payday: when the salary comes and the next top-up to make,
 *  recordable in one tap; the full runbook is on Plan → Cash. */
export function PaydayStrip({ c }: { c: CashPlan }) {
  const editor = useTxEditor()
  // The CFO number first: the day cash runs out or dips under the emergency
  // floor in the next 60 days, before the next top-up.
  const { data: cal } = useCashCalendar()
  if (!c.payday || !c.accounts.length) return null
  const when = (d: string) => (d <= todayISO() ? 'today' : dayLabel(d))
  const risk = cal?.first_short ? { tone: 'text-bad', text: <>Cash runs out {cal.first_short <= todayISO() ? '' : 'on '}<b>{when(cal.first_short)}</b></> }
    : cal?.first_below ? { tone: 'text-warn', text: <>{cal.first_below <= todayISO() ? <>Under your {eur(cal.floor)} floor <b>today</b></> : <>Under your {eur(cal.floor)} floor from <b>{dayLabel(cal.first_below)}</b></>} · lowest {eur(cal.low)} on {shortDate(cal.low_date)}</> }
    : null
  const days = Math.round((new Date(c.payday + 'T00:00:00').getTime() - new Date(todayISO() + 'T00:00:00').getTime()) / 86400000)
  const salary = c.accounts.find((a) => a.id === c.salary_account)?.name
  const moves = [...c.moves].sort((a, b) => a.by.localeCompare(b.by))
  const next = moves[0]
  const lc = (s: string) => s.charAt(0).toLowerCase() + s.slice(1)
  return (
    <section className="card">
      <Link to="/plan/cash" className="flex items-center gap-3 px-4 py-3 hover:bg-sunken/30">
        <IconTile name="calendar" color="var(--s7)" size={32} />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-semibold">Payday {days <= 0 ? 'today' : `in ${days} day${days === 1 ? '' : 's'}`}</span>
          <span className="block text-xs text-muted">Salary by {shortDate(c.payday)}{salary && <> into {salary}</>} · {eur(c.daily)}/day until then</span>
        </span>
        <span className={clsx('shrink-0 text-xs', c.short > 0 ? 'text-bad' : moves.length ? 'text-warn' : 'text-good')}>
          {c.short > 0 ? `${eur(c.short)} short` : moves.length ? `${moves.length} top-up${moves.length === 1 ? '' : 's'}` : 'all covered'}
        </span>
        <Icon name="chevronR" size={16} className="shrink-0 text-muted" />
      </Link>
      {risk && (
        <Link to="/plan/cash" className={clsx('flex items-start gap-2 border-t border-line px-4 py-2 text-xs hover:bg-sunken/30', risk.tone)}>
          <Icon name="alert" size={14} className="mt-0.5 shrink-0" /><span className="min-w-0">{risk.text}</span>
        </Link>
      )}
      {next && (
        <div className="flex items-center gap-3 border-t border-line px-4 py-2 text-sm">
          <span className={clsx('w-12 shrink-0 text-xs', next.by <= todayISO() ? 'font-semibold text-warn' : 'text-muted')}>by {next.by <= todayISO() ? 'today' : shortDate(next.by).replace(/ \d{2}$/, '')}</span>
          <span className="min-w-0 flex-1">{next.from_name} → {next.to_name}{next.for && <span className="block text-xs text-muted">{lc(next.for)}</span>}</span>
          <span className="shrink-0 font-medium tnum">{eur(next.amount)}</span>
          <button className="btn-ghost h-8 shrink-0 px-2 text-xs" onClick={() => editor.open({ kind: 'transfer', date: todayISO(), amount: next.amount, account_id: next.from, to_account_id: next.to, category: 'transfer.internal', note: `Top-up for ${lc(next.for)}` })}>Record</button>
        </div>
      )}
    </section>
  )
}

type Entry = { date: string; label: string; account?: string; amount: number; kind: CashItem['kind'] | 'move' | 'save'; move?: CashPlan['moves'][number] }

/** Everything that moves money until the next salary, in one dated list:
 *  obligations, bills, standing orders and the top-ups that keep every
 *  account above its buffer (each as late as is safe, so savings keep
 *  earning). Per-account low points and buffers sit below. Read-only advice:
 *  recording a move opens an ordinary transfer for review. */
export function UntilPayday({ c }: { c: CashPlan }) {
  const editor = useTxEditor()
  const [all, setAll] = useState(false)
  if (!c.payday || !c.accounts.length) return null
  const record = (m: CashPlan['moves'][number], note: string) =>
    editor.open({ kind: 'transfer', date: todayISO(), amount: m.amount, account_id: m.from, to_account_id: m.to, category: 'transfer.internal', note })
  const days = Math.round((new Date(c.payday + 'T00:00:00').getTime() - new Date(todayISO() + 'T00:00:00').getTime()) / 86400000)
  const salary = c.accounts.find((a) => a.id === c.salary_account)?.name
  const entries: Entry[] = [
    ...c.accounts.flatMap((a) => (a.items ?? []).filter((it) => it.kind !== 'spending' && !it.within).map((it) => ({ date: it.date, label: it.label, account: a.name, amount: it.amount, kind: it.kind }))),
    ...c.moves.map((m) => ({ date: m.by, label: `${m.from_name} → ${m.to_name}`, account: m.for ? `for ${m.for.charAt(0).toLowerCase()}${m.for.slice(1)}` : undefined, amount: m.amount, kind: 'move' as const, move: m })),
  ].sort((a, b) => a.date.localeCompare(b.date) || (a.kind === 'move' ? -1 : b.kind === 'move' ? 1 : 0))
  const shown = all ? entries : entries.slice(0, 7)
  const day = (d: string) => (d <= todayISO() ? 'Today' : shortDate(d).replace(/ \d{2}$/, ''))
  return (
    <section id="cash" className="card scroll-mt-16">
      <div className="flex items-start gap-3 px-4 pt-3">
        <IconTile name="calendar" color="var(--s7)" size={32} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">Until payday</div>
          <div className="text-xs text-muted">Salary by <b className="text-ink2">{shortDate(c.payday)}</b>{salary && <> into {salary}</>} · {days} days · day-to-day at {eur(c.daily)}/day</div>
        </div>
      </div>
      <div className="px-4 py-3">
        {c.moves.length === 0 && <div className="mb-2 flex items-center gap-1.5 text-sm text-good"><Icon name="check" size={16} />Every account covers what's due until payday</div>}
        {c.short > 0 && (
          <div className={clsx('mb-2 flex items-start gap-1.5 text-sm', c.cash >= c.short ? 'text-warn' : 'text-bad')}>
            <Icon name="alert" size={16} className="mt-0.5 shrink-0" />
            <span>Savings fall <b className="tnum">{eur(c.short)}</b> short — {c.cash >= c.short ? <>deposit that much of your {eur(c.cash)} cash, or lower a buffer below</> : 'lower a buffer below or spend less until payday'}.</span>
          </div>
        )}
        <div className="divide-y divide-line">
          {shown.map((e, i) => (
            <div key={i} className={clsx('flex items-center gap-3 py-1.5 text-sm', e.kind === 'move' && '-mx-2 rounded-lg border-0 bg-accent/10 px-2')}>
              <span className={clsx('w-12 shrink-0 text-xs', e.date <= todayISO() ? 'font-semibold text-warn' : 'text-muted')}>{e.kind === 'move' ? 'by ' : ''}{day(e.date)}</span>
              <span className="min-w-0 flex-1">
                <span className={clsx('block', e.kind === 'move' && 'font-medium')}>{e.label}</span>
                {e.account && <span className="block truncate text-xs text-muted">{e.account}</span>}
              </span>
              {e.move ? (
                <>
                  <span className="shrink-0 font-medium tnum">{eur(e.amount)}</span>
                  <button className="btn-ghost h-7 shrink-0 px-2 text-xs" onClick={() => record(e.move!, `Top-up for ${e.move!.for.charAt(0).toLowerCase()}${e.move!.for.slice(1)}`)} title="Made the transfer? Record it">Record</button>
                </>
              ) : (
                <span className={clsx('shrink-0 tnum', e.amount > 0 && 'text-good')}>{e.amount > 0 ? '+' : '−'}{eur(Math.abs(e.amount))}</span>
              )}
            </div>
          ))}
        </div>
        {entries.length > 7 && <button className="mt-1 text-xs text-accent" onClick={() => setAll(!all)}>{all ? 'Show less' : `Show all ${entries.length}`}</button>}
        {(c.to_savings ?? []).map((m, i) => (
          <div key={i} className="mt-2 flex items-start gap-3 rounded-lg border border-good/30 bg-good/5 px-3 py-2 text-sm">
            <Icon name="piggy" size={16} className="mt-0.5 shrink-0 text-good" />
            <span className="min-w-0 flex-1"><b className="tnum">{eur(m.amount)}</b> could earn in {m.to_name} — it {m.for}</span>
            <button className="btn-ghost h-7 shrink-0 px-2 text-xs" onClick={() => record(m, 'Idle cash to savings')}>Record</button>
          </div>
        ))}
      </div>
      <div className="divide-y divide-line border-t border-line">
        {c.accounts.map((a) => <CashAccountRow key={a.id} a={a} payday={c.payday} />)}
      </div>
    </section>
  )
}

function CashAccountRow({ a, payday }: { a: CashAccount; payday: string }) {
  const [open, setOpen] = useState(false)
  const items = a.items ?? []
  return (
    <div>
      <button onClick={() => setOpen(!open)} aria-expanded={open} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40">
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">{a.name}</span>
          <span className="block text-xs text-muted">
            now <span className="tnum text-ink2">{eur(a.balance)}</span> · buffer <span className="tnum text-ink2">{eur(a.buffer)}</span> · lowest <span className={clsx('tnum', a.low < 0 ? 'text-bad' : 'text-ink2')}>{a.low < 0 ? `−${eur(-a.low)}` : eur(a.low)}</span> on {shortDate(a.low_date)}
            {a.needed > 0 && <> · needs <b className="tnum text-warn">{eur(a.needed)}</b></>}
          </span>
        </span>
        <span className="text-xs text-muted">{items.length} {items.length === 1 ? 'item' : 'items'}</span>
        <Icon name="chevronD" size={14} className={clsx('shrink-0 text-muted transition', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-1 px-4 pb-3 text-sm">
          {items.map((it, i) => (
            <div key={i} className={clsx('flex items-baseline gap-2', it.within && 'text-muted')}>
              <span className="w-14 shrink-0 text-xs text-muted">{it.kind === 'spending' ? `to ${shortDate(payday).replace(/ \d{2}$/, '')}` : shortDate(it.date).replace(/ \d{2}$/, '')}</span>
              <span className="min-w-0 flex-1 truncate">{it.label}{it.within && <span className="text-xs"> · in day-to-day</span>}</span>
              <span className={clsx('tnum', it.amount > 0 && 'text-good')}>{it.amount > 0 ? '+' : '−'}{eur(Math.abs(it.amount))}</span>
            </div>
          ))}
          <BufferEditor a={a} />
          <div className="flex items-baseline gap-2 border-t border-line pt-1 text-xs text-muted">
            <span className="flex-1">Before payday, without top-ups</span>
            <span className={clsx('tnum', a.end < 0 ? 'text-bad' : 'text-ink2')}>{a.end < 0 ? `−${eur(-a.end)}` : eur(a.end)}</span>
          </div>
        </div>
      )}
    </div>
  )
}

/** The floor an everyday account is kept at — automatic from its history,
 *  or your own figure (saved in plan settings). */
function BufferEditor({ a }: { a: CashAccount }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [v, setV] = useState<string | null>(null)
  const save = async (value: number | null) => {
    try {
      // Buffers merge per account on the server; 0 puts one back to automatic.
      await api.patch('/plan/settings', { buffers: { [a.id]: value ?? 0 } })
      setV(null)
      qc.invalidateQueries({ queryKey: ['overview'] }); qc.invalidateQueries({ queryKey: ['plan-settings'] })
      toast(value === null ? 'Buffer back to automatic' : 'Buffer saved', 'good')
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  return (
    <div className="my-2 rounded-lg bg-sunken/50 px-3 py-2 text-xs text-ink2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex-1">Buffer <b className="tnum text-ink">{eur(a.buffer)}</b> — {a.buffer_why}</span>
        {v === null ? (
          <>
            <button className="text-accent" onClick={() => setV(String(a.buffer))}>Change</button>
            {!a.buffer_auto && <button className="text-accent" onClick={() => save(null)}>Automatic</button>}
          </>
        ) : (
          <span className="flex items-center gap-1.5">
            <input className="input h-7 w-24 text-xs tnum" inputMode="decimal" value={v} onChange={(e) => setV(e.target.value)} autoFocus />
            <button className="btn-primary h-7 px-2 text-xs" onClick={() => { const n = parseNum(v); if (n !== undefined && n >= 0) save(n) }}>Save</button>
            <button className="text-muted" onClick={() => setV(null)}>Cancel</button>
          </span>
        )}
      </div>
      <div className="mt-1 text-muted">Kept in the account through the month; anything above it until payday is better off earning in savings.</div>
    </div>
  )
}
