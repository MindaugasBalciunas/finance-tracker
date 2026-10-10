import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { addMonths, eur, monthLabel, shortDate, thisMonth } from '../../lib/format'
import { AskCFO, Card, Empty, ErrorBox, Field, Loading, Meter, NumberInput, Sheet, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

type Goal = { id: number; name: string; target: number; priority: number; target_date: string; note: string; url: string; tag: string; spent?: number; status: 'active' | 'bought' | 'dropped'; done_on: string
  saved: number; remaining: number; eta?: string; per_month?: number }
type MonthFlow = { month: string; income: number; invested: number; spending: number; left_over: number; funded: number; available: number; complete: boolean }
type Alloc = { goal_id: number; name?: string; amount: number }
type WishPlan = { goals: Goal[]; funding: MonthFlow; proposal: Alloc[]; history: MonthFlow[]; avg_left_over: number; total_saved: number; total_needed: number }

const lastDone = () => addMonths(thisMonth(), -1)

/** The wish list: things saved up for, funded at month end from what is left
 *  after paying yourself first and every obligation — in priority order. */
export function Wishlist() {
  const [month, setMonth] = useState(lastDone())
  const { data: p, isLoading, error } = useQuery({ queryKey: ['goals', month], queryFn: () => api.get<WishPlan>('/goals', { month }) })
  // "Plan a trip" on the Trips tab lands here with a trip goal open.
  const [sp, setSp] = useSearchParams()
  const [edit, setEdit] = useState<Partial<Goal> | null>(() => (sp.get('trip') ? { tag: 'trip:' } : null))
  useEffect(() => { if (sp.get('trip')) setSp({}, { replace: true }) }, [sp, setSp])
  const [fund, setFund] = useState(false)
  const refresh = useRefresh()
  const toast = useToast()
  const move = useMutation({
    mutationFn: (ids: number[]) => api.post('/goals/order', { ids }),
    onSuccess: () => refresh(),
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  if (isLoading) return <Loading />
  if (error || !p) return <ErrorBox error={error} />
  const active = p.goals.filter((g) => g.status === 'active')
  const done = p.goals.filter((g) => g.status !== 'active')
  const reorder = (i: number, dir: -1 | 1) => {
    const ids = active.map((g) => g.id)
    const j = i + dir
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    move.mutate(ids)
  }
  return (
    <div className="space-y-4">
      <LeftOver p={p} month={month} setMonth={setMonth} onFund={() => setFund(true)} />

      <div className="flex items-center justify-between gap-2">
        <div>
          <h2 className="text-base font-semibold">Wish list</h2>
          {active.length > 0 && (() => {
            const unpriced = active.filter((g) => !(g.target > 0)).length
            return <div className="text-xs text-muted tnum">{unpriced === active.length ? `${active.length} goals · ${unpriced} need a price` : `${eur(p.total_saved)} saved · ${eur(p.total_needed)} to go${unpriced ? ` · ${unpriced} without a price` : ''} · top first`}</div>
          })()}
        </div>
        <div className="flex gap-2">
          <AskCFO q="Look at my wish list and the last months' left over: is the order sensible, are the prices realistic, and when will each be funded?" label="Review" />
          <button className="btn-primary h-9" onClick={() => setEdit({})}><Icon name="plus" size={16} />Goal</button>
        </div>
      </div>
      {!active.length ? (
        <Card><Empty title="Nothing on the wish list yet" icon="target">Add what you're saving for — a 3D printer, a sauna — and what's left at month end funds it, top of the list first.</Empty></Card>
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {active.map((g, i) => (
            <GoalCard key={g.id} g={g} rank={i + 1} first={i === 0} last={i === active.length - 1} onUp={() => reorder(i, -1)} onDown={() => reorder(i, 1)} onOpen={() => setEdit(g)} />
          ))}
        </div>
      )}
      {done.length > 0 && (
        <Card pad={false} title="Done">
          <div className="divide-y divide-line border-t border-line">
            {done.map((g) => (
              <button key={g.id} onClick={() => setEdit(g)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm hover:bg-sunken/50">
                <Icon name={g.status === 'bought' ? 'check' : 'x'} size={16} className={g.status === 'bought' ? 'text-good' : 'text-muted'} />
                <span className="min-w-0 flex-1 truncate">{g.name}</span>
                <span className="text-xs text-muted">{g.status === 'bought' ? (g.tag ? 'travelled' : 'bought') : 'dropped'}{g.done_on ? ` ${shortDate(g.done_on)}` : ''}</span>
                <span className="tnum text-ink2">{eur(g.target)}</span>
              </button>
            ))}
          </div>
        </Card>
      )}
      {edit && <GoalSheet g={edit} onClose={() => setEdit(null)} />}
      {fund && <FundSheet p={p} onClose={() => setFund(false)} />}
    </div>
  )
}

/** The month's waterfall: income, paid yourself first, obligations and
 *  spending — what is left funds the wish list. */
function LeftOver({ p, month, setMonth, onFund }: { p: WishPlan; month: string; setMonth: (m: string) => void; onFund: () => void }) {
  const f = p.funding
  const rows = [
    { label: 'Income', v: f.income, cls: 'text-ink' },
    { label: 'Paid yourself first', sub: 'investing, pensions, mortgage principal', v: -f.invested, cls: 'text-ink2' },
    { label: 'Obligations & spending', v: -f.spending, cls: 'text-ink2' },
  ]
  const max = Math.max(1, ...p.history.map((h) => Math.abs(h.left_over)))
  return (
    <section className="card p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1">
          <button className="btn-ghost h-8 w-8 px-0" onClick={() => setMonth(addMonths(month, -1))} aria-label="Previous month"><Icon name="chevronD" size={16} className="rotate-90" /></button>
          <h3 className="text-sm font-semibold">{monthLabel(month, true)}</h3>
          <button className="btn-ghost h-8 w-8 px-0 disabled:opacity-30" disabled={month >= lastDone()} onClick={() => setMonth(addMonths(month, 1))} aria-label="Next month"><Icon name="chevronD" size={16} className="-rotate-90" /></button>
        </div>
        {f.available > 0 && f.complete && p.proposal.length > 0 && (
          <button className="btn-primary h-9" onClick={onFund}><Icon name="target" size={16} />Fund goals · {eur(f.available)}</button>
        )}
      </div>
      <div className="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-[1fr_auto]">
        <div className="space-y-1 text-sm">
          {rows.map((r) => (
            <div key={r.label} className="flex items-baseline justify-between gap-3">
              <span className="text-ink2">{r.label}{r.sub && <span className="hidden text-xs text-muted sm:inline"> · {r.sub}</span>}</span>
              <span className={clsx('tnum', r.cls)}>{r.v < 0 ? '−' : ''}{eur(Math.abs(r.v))}</span>
            </div>
          ))}
          <div className="mt-1 flex items-baseline justify-between gap-3 border-t border-line pt-1.5">
            <span className="font-semibold">Left over</span>
            <span className={clsx('tnum text-base font-semibold', f.left_over >= 0 ? 'text-good' : 'text-bad')}>{f.left_over < 0 ? '−' : ''}{eur(Math.abs(f.left_over))}</span>
          </div>
          <div className="text-xs text-muted">
            {!f.complete ? 'Month still running — fund it once it ends.'
              : f.left_over <= 0 ? 'Nothing left for the wish list this month.'
              : f.available > 0 ? `${eur(f.funded)} in goals · ${eur(f.available)} still to put in`
              : `All of it is in goals.`}
          </div>
        </div>
        {/* Last six months' left over: the pace the ETAs run on. */}
        <div className="sm:w-56">
          <div className="flex h-16 items-end gap-1">
            {p.history.map((h) => (
              <button key={h.month} onClick={() => setMonth(h.month)} title={`${monthLabel(h.month)} ${eur(h.left_over)}`}
                className={clsx('flex-1 rounded-t', h.left_over >= 0 ? 'bg-good/70' : 'bg-bad/60', h.month === month && 'ring-2 ring-accent')}
                style={{ height: `${Math.max(4, (Math.abs(h.left_over) / max) * 100)}%` }} />
            ))}
          </div>
          <div className="mt-1 flex justify-between text-[10px] text-muted">{p.history.map((h) => <span key={h.month} className="flex-1 text-center">{monthLabel(h.month).slice(0, 3)}</span>)}</div>
          <div className="mt-1 text-xs text-muted">avg left over <b className="tnum text-ink">{eur(p.avg_left_over)}</b>/month</div>
        </div>
      </div>
    </section>
  )
}

function GoalCard({ g, rank, first, last, onUp, onDown, onOpen }: { g: Goal; rank: number; first: boolean; last: boolean; onUp: () => void; onDown: () => void; onOpen: () => void }) {
  const priced = g.target > 0
  const full = priced && g.remaining <= 0
  return (
    <section className="card flex items-stretch gap-2 p-3 pl-2">
      <div className="flex flex-col items-center justify-between">
        <button className="btn-ghost h-7 w-7 px-0 disabled:opacity-20" disabled={first} onClick={onUp} aria-label="Fund earlier"><Icon name="chevronD" size={14} className="rotate-180" /></button>
        <span className="text-xs font-semibold text-muted tnum">{rank}</span>
        <button className="btn-ghost h-7 w-7 px-0 disabled:opacity-20" disabled={last} onClick={onDown} aria-label="Fund later"><Icon name="chevronD" size={14} /></button>
      </div>
      <button onClick={onOpen} className="min-w-0 flex-1 text-left">
        <div className="flex items-baseline justify-between gap-2">
          <span className="truncate font-medium">{g.tag && <Icon name="plane" size={14} className="mr-1.5 inline text-muted" />}{g.name}{g.url && <Icon name="link" size={13} className="ml-1 inline text-muted" />}</span>
          <span className={clsx('tnum text-sm font-semibold', !priced && 'font-normal text-muted')}>{priced ? eur(g.target) : 'no price'}</span>
        </div>
        {!priced ? (
          <div className="mt-1.5 text-xs text-accent">Tap to set a price{g.saved > 0 && <span className="text-muted tnum"> · {eur(g.saved)} saved</span>}</div>
        ) : (<>
        <Meter value={g.saved} max={g.target} goal className="mt-2 h-2" />
        <div className="mt-1.5 flex flex-wrap items-baseline justify-between gap-x-3 text-xs">
          <span className="tnum text-ink2">{eur(g.saved)} saved{!full && <span className="text-muted"> · {eur(g.remaining)} to go</span>}</span>
          <span className={clsx('tnum', full ? 'text-good' : 'text-muted')}>
            {full ? 'ready to buy' : g.eta ? `funded ≈ ${monthLabel(g.eta)}` : 'needs a month with money left'}
          </span>
        </div>
        </>)}
        {g.tag && (g.spent ?? 0) !== 0 && (
          <div className="mt-0.5 text-[11px] text-muted">spent so far <b className="tnum text-ink2">{eur(g.spent ?? 0)}</b> · {g.saved >= (g.spent ?? 0) ? `${eur(g.saved - (g.spent ?? 0))} of the savings left` : `${eur((g.spent ?? 0) - g.saved)} more than saved`}</div>
        )}
        {priced && !full && g.target_date && (
          <div className="mt-0.5 text-[11px] text-muted">by {shortDate(g.target_date)}: <b className="tnum text-ink2">{eur(g.per_month ?? 0)}</b>/month</div>
        )}
      </button>
    </section>
  )
}

/** Fund a finished month: the suggested split (top of the list first), each
 *  amount editable, never more than the month left over. */
function FundSheet({ p, onClose }: { p: WishPlan; onClose: () => void }) {
  const active = p.goals.filter((g) => g.status === 'active' && g.remaining > 0)
  const [amt, setAmt] = useState<Record<number, number | undefined>>(() => Object.fromEntries(p.proposal.map((a) => [a.goal_id, a.amount])))
  const total = active.reduce((t, g) => t + (amt[g.id] ?? 0), 0)
  const over = total > p.funding.available + 0.005
  const refresh = useRefresh()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => api.post('/goals/fund', { month: p.funding.month, allocations: active.filter((g) => (amt[g.id] ?? 0) > 0).map((g) => ({ goal_id: g.id, amount: amt[g.id] })) }),
    onSuccess: () => { refresh(); toast(`${eur(total)} put into goals`, 'good'); onClose() },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  return (
    <Sheet open onClose={onClose} title={`Fund goals from ${monthLabel(p.funding.month, true)}`} footer={<>
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" disabled={over || total <= 0 || save.isPending} onClick={() => save.mutate()}>Put {eur(total)} in goals</button>
    </>}>
      <p className="text-sm text-ink2">{eur(p.funding.available)} left after paying yourself first and every obligation. Top of the list first — change any amount.</p>
      <div className="mt-3 space-y-2">
        {active.map((g) => (
          <div key={g.id} className="grid grid-cols-[1fr_7rem] items-center gap-3">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{g.name}</div>
              <div className="text-xs text-muted tnum">needs {eur(g.remaining)}</div>
            </div>
            <NumberInput value={amt[g.id]} onChange={(v) => setAmt({ ...amt, [g.id]: v == null ? undefined : Math.max(0, Math.min(v, g.remaining)) })} placeholder="0" className="input tnum text-right" />
          </div>
        ))}
      </div>
      <div className={clsx('mt-3 flex justify-between border-t border-line pt-2 text-sm', over ? 'text-bad' : 'text-ink2')}>
        <span>{over ? 'More than the month left over' : 'Left unassigned'}</span>
        <span className="tnum font-semibold">{eur(Math.abs(p.funding.available - total))}</span>
      </div>
    </Sheet>
  )
}

function GoalSheet({ g, onClose }: { g: Partial<Goal>; onClose: () => void }) {
  const [v, setV] = useState<Partial<Goal>>(g)
  const [delta, setDelta] = useState<number | undefined>()
  const [why, setWhy] = useState('')
  const refresh = useRefresh()
  const toast = useToast()
  const { data: moves } = useQuery({ queryKey: ['goal-moves', g.id], queryFn: () => api.get<any[]>(`/goals/${g.id}/moves`), enabled: !!g.id })
  useEffect(() => setV(g), [g])
  const done = (msg: string) => { refresh(); toast(msg, 'good'); onClose() }
  const fail = (e: unknown) => toast((e as Error).message, 'bad')
  const save = useMutation({ mutationFn: (body: Partial<Goal>) => (g.id ? api.put(`/goals/${g.id}`, body) : api.post('/goals', body)), onError: fail })
  const put = useMutation({ mutationFn: (amount: number) => api.post(`/goals/${g.id}/moves`, { amount, note: why }), onSuccess: () => done('Saved'), onError: fail })
  const del = useMutation({ mutationFn: () => api.del(`/goals/${g.id}`), onSuccess: () => done('Removed'), onError: fail })
  const body = { name: v.name, target: v.target ?? 0, target_date: v.target_date ?? '', note: v.note ?? '', url: v.url ?? '', tag: !v.tag ? '' : v.tag.trim() === 'trip:' ? `trip:${v.name ?? ''}` : v.tag, status: v.status ?? 'active' }
  return (
    <Sheet open onClose={onClose} title={g.id ? g.name : g.tag ? 'Plan a trip' : 'New goal'} footer={<>
      {g.id && <button className="btn-danger mr-auto" onClick={() => confirm(`Delete ${g.name} and its history?`) && del.mutate()}>Delete</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" disabled={save.isPending} onClick={() => save.mutate(body, { onSuccess: () => done(g.id ? 'Saved' : 'Added to the wish list') })}>{g.id ? 'Save' : 'Add'}</button>
    </>}>
      <div className="space-y-3">
        <Field label="What"><input className="input" value={v.name ?? ''} onChange={(e) => setV({ ...v, name: e.target.value })} placeholder="e.g. Bambu Lab X2D" autoFocus={!g.id} /></Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Price" hint="leave empty if you don't know yet"><NumberInput value={v.target || undefined} onChange={(x) => setV({ ...v, target: x })} placeholder="€" /></Field>
          <Field label="Wanted by" hint="optional"><input type="date" className="input" value={v.target_date ?? ''} onChange={(e) => setV({ ...v, target_date: e.target.value })} /></Field>
        </div>
        <div className="rounded-xl border border-line p-3">
          <Toggle checked={v.tag != null && v.tag !== ''} onChange={(on) => setV({ ...v, tag: on ? (v.tag || 'trip:' + (v.name ?? '').toLowerCase().trim().replace(/[^a-z0-9ąčęėįšųūž]+/g, '-').replace(/^-|-$/g, '')) : '' })} label="It's a trip" />
          {!!v.tag && (
            <div className="mt-2">
              <Field label="Trip tag" hint="Tag its flights, hotel and spending with this — they count against what you saved, and it shows on the Trips tab.">
                <input className="input" value={v.tag} onChange={(e) => setV({ ...v, tag: e.target.value })} placeholder="trip:rome-2027" />
              </Field>
              {!!g.id && (g.spent ?? 0) > 0 && <div className="mt-1 text-xs text-muted">{eur(g.spent ?? 0)} spent under it so far</div>}
            </div>
          )}
        </div>
        <Field label="Link" hint="optional — the shop or product page"><input className="input" value={v.url ?? ''} onChange={(e) => setV({ ...v, url: e.target.value })} placeholder="https://" /></Field>
        <Field label="Note"><input className="input" value={v.note ?? ''} onChange={(e) => setV({ ...v, note: e.target.value })} placeholder="optional" /></Field>
        {g.id && g.status === 'active' && (
          <div className="rounded-xl border border-line p-3">
            <div className="text-sm font-medium">Saved <span className="tnum">{eur(g.saved ?? 0)}</span> of <span className="tnum">{eur(g.target ?? 0)}</span></div>
            <div className="mt-2 grid grid-cols-[7rem_1fr] gap-2">
              <NumberInput value={delta} onChange={setDelta} placeholder="€" className="input tnum" />
              <input className="input" value={why} onChange={(e) => setWhy(e.target.value)} placeholder="why (a bonus, a repair…)" />
            </div>
            <div className="mt-2 flex flex-wrap gap-2">
              <button className="btn-outline h-8 text-xs" disabled={!delta || delta <= 0} onClick={() => put.mutate(delta!)}>Put in</button>
              <button className="btn-outline h-8 text-xs" disabled={!delta || delta <= 0 || delta > (g.saved ?? 0)} onClick={() => put.mutate(-delta!)}>Take out</button>
              <button className="btn-outline ml-auto h-8 text-xs text-good" onClick={() => save.mutate({ ...body, status: 'bought' }, { onSuccess: () => done(g.tag ? `${g.name} — done 🎉` : `${g.name} bought 🎉`) })}><Icon name="check" size={14} />{g.tag ? 'Travelled' : 'Bought'}</button>
              <button className="btn-ghost h-8 text-xs" onClick={() => save.mutate({ ...body, status: 'dropped' }, { onSuccess: () => done('Dropped — its money is free again') })}>Drop</button>
            </div>
          </div>
        )}
        {g.id && g.status !== 'active' && (
          <button className="btn-outline" onClick={() => save.mutate({ ...body, status: 'active' }, { onSuccess: () => done('Back on the wish list') })}>Put back on the wish list</button>
        )}
        {!!moves?.length && (
          <div>
            <div className="label">History</div>
            <div className="divide-y divide-line text-xs">
              {moves.map((m) => (
                <div key={m.id} className="flex justify-between gap-2 py-1.5">
                  <span className="text-ink2">{monthLabel(m.month)} · {m.kind === 'funding' ? 'month-end funding' : m.note || 'by hand'}</span>
                  <span className={clsx('tnum', m.amount < 0 ? 'text-bad' : 'text-good')}>{m.amount < 0 ? '−' : '+'}{eur(Math.abs(m.amount))}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </Sheet>
  )
}
