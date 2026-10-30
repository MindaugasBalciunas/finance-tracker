import { NextEuroCard } from './NextEuro'
import { DecisionRow, NewDecisionSheet, RecordCard, SleepingOnIt, useJournal } from './Decisions'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useAccounts, useRefresh, useTags } from '../../lib/hooks'
import { addMonths, eur, monthLabel, shortDate, thisMonth } from '../../lib/format'
import { AskCFO, Card, Empty, ErrorBox, Field, hue, Loading, Meter, NumberInput, Sheet, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { DateInput } from '../../components/DateInput'
import { SuggestInput } from '../../components/pickers'

type Goal = { id: number; name: string; target: number; priority: number; target_date: string; note: string; url: string; tag: string; spent?: number; status: 'active' | 'bought' | 'dropped'; done_on: string
  saved: number; remaining: number; eta?: string; per_month?: number; outlook?: { tone: 'good' | 'warn' | 'bad'; text: string; gap?: number }
  held?: { account_id: string; name: string; amount: number }[] | null }
/** A savings account and what the wish list has set aside in it. */
type Pot = { account_id: string; name: string; balance: number; set_aside: number; free: number }
type MonthFlow = { month: string; income: number; invested: number; spending: number; left_over: number; funded: number; available: number; complete: boolean }
type Alloc = { goal_id: number; name?: string; amount: number }
type WishPlan = { goals: Goal[]; funding: MonthFlow; proposal: Alloc[]; history: MonthFlow[]; avg_left_over: number; total_saved: number; total_needed: number; pots?: Pot[] | null }

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
  const [thinking, setThinking] = useState(false)
  const { data: journal } = useJournal()
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
      {/* Wide screens: the month that funds the list beside where its money sits. */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
        <LeftOver p={p} month={month} setMonth={setMonth} onFund={() => setFund(true)} />
        <Pots pots={p.pots ?? []} />
      </div>

      {/* Phones: the title, then the two ways in on their own row — "not sure
          yet" (the decision journal) and "want it" (a goal). */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start justify-between gap-2">
          <div>
          <h2 className="text-base font-semibold">Wish list</h2>
          {active.length > 0 && (() => {
            const unpriced = active.filter((g) => !(g.target > 0)).length
            return <div className="text-xs text-muted tnum">{unpriced === active.length ? `${active.length} goals · ${unpriced} need a price` : `${eur(p.total_saved)} saved · ${eur(p.total_needed)} to go${unpriced ? ` · ${unpriced} without a price` : ''} · top first`}</div>
          })()}
          </div>
          <div className="sm:hidden"><AskCFO q="Look at my wish list and the last months' left over: is the order sensible, are the prices realistic, and when will each be funded?" label="Review" /></div>
        </div>
        <div className="flex gap-2">
          <span className="hidden sm:inline-flex"><AskCFO q="Look at my wish list and the last months' left over: is the order sensible, are the prices realistic, and when will each be funded?" label="Review" /></span>
          <button className="btn-outline h-10 flex-1 whitespace-nowrap sm:h-9 sm:flex-none" onClick={() => setThinking(true)} title="Not sure yet? Write it down and sleep on it"><Icon name="bag" size={16} />Thinking of buying…</button>
          <button className="btn-primary h-10 flex-1 whitespace-nowrap sm:h-9 sm:flex-none" onClick={() => setEdit({})}><Icon name="plus" size={16} />Goal</button>
        </div>
      </div>
      <SleepingOnIt items={(journal?.items ?? []).filter((x) => x.status === 'thinking')} />
      {!active.length ? (
        <Card><Empty title="Nothing on the wish list yet" icon="target">Add what you're saving for — a 3D printer, a sauna — and what's left at month end funds it, top of the list first.</Empty></Card>
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {active.map((g, i) => (
            <GoalCard key={g.id} g={g} rank={i + 1} first={i === 0} last={i === active.length - 1} onUp={() => reorder(i, -1)} onDown={() => reorder(i, 1)} onOpen={() => setEdit(g)} />
          ))}
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
        <div className="space-y-4">
          {/* One "done" list: journal decisions (with their worth-it rating) and
              goals bought or dropped that didn't come from the journal — a goal
              saved for from a decision shows once, as the decision. */}
          {(() => {
            const decided = (journal?.items ?? []).filter((x) => x.status === 'bought' || x.status === 'skipped')
            const linked = new Set((journal?.items ?? []).map((x) => x.goal_id).filter(Boolean))
            const goalsDone = done.filter((g) => !linked.has(g.id))
            if (!decided.length && !goalsDone.length) return null
            return (
              <Card pad={false} color="var(--hue-good)" title="Done" action={decided.length ? <AskCFO q="What does my decision journal say about what I value?" /> : undefined}>
                {decided.length > 0 && <ul className="divide-y divide-line border-t border-line px-4">{decided.map((x) => <DecisionRow key={`d${x.id}`} x={x} />)}</ul>}
                {goalsDone.length > 0 && (
                  <div className="divide-y divide-line border-t border-line">
                    {goalsDone.map((g) => (
                      <button key={g.id} onClick={() => setEdit(g)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm hover:bg-sunken/50">
                        <Icon name={g.status === 'bought' ? 'check' : 'x'} size={16} className={g.status === 'bought' ? 'text-good' : 'text-muted'} />
                        <span className="min-w-0 flex-1 truncate">{g.name}</span>
                        <span className="text-xs text-muted">{g.status === 'bought' ? (g.tag ? 'travelled' : 'bought') : 'dropped'}{g.done_on ? ` ${shortDate(g.done_on)}` : ''}</span>
                        <span className="tnum text-ink2">{eur(g.target)}</span>
                      </button>
                    ))}
                  </div>
                )}
              </Card>
            )
          })()}
          {journal && <RecordCard r={journal.record} />}
        </div>
        <div className="space-y-4">
          <NextEuroCard />
        </div>
      </div>
      {edit && <GoalSheet g={edit} pots={p.pots ?? []} onClose={() => setEdit(null)} />}
      {fund && <FundSheet p={p} onClose={() => setFund(false)} />}
      {thinking && <NewDecisionSheet onClose={() => setThinking(false)} />}
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
    <section className="card p-4" style={hue(f.left_over < 0 ? 'var(--hue-bad)' : 'var(--hue-good)')}>
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
              <span className="min-w-0 text-ink2">{r.label}{r.sub && <span className="hidden text-xs text-muted xl:inline"> · {r.sub}</span>}</span>
              {/* An amount never splits from its sign. */}
              <span className={clsx('tnum shrink-0 whitespace-nowrap', r.cls)}>{r.v < 0 ? '−' : ''}{eur(Math.abs(r.v))}</span>
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

/** Savings accounts and what of each is set aside for the wish list — the
 *  rest of the balance is free to set aside or spend. */
function Pots({ pots }: { pots: Pot[] }) {
  if (!pots.length) return null
  return (
    <Card pad={false} icon="piggy" color="var(--s6)" title="Savings set aside">
      <div className="divide-y divide-line border-t border-line">
        {pots.map((x) => (
          <div key={x.account_id} className="px-4 py-2.5">
            <div className="flex items-baseline justify-between gap-2 text-sm">
              <span className="truncate font-medium">{x.name}</span>
              <span className="tnum text-ink2">{eur(x.set_aside)} <span className="text-muted">of {eur(x.balance)}</span></span>
            </div>
            <Meter value={x.set_aside} max={Math.max(x.balance, x.set_aside, 1)} goal className="mt-1.5" />
            <div className="mt-1 text-xs text-muted tnum">{x.set_aside > x.balance ? <span className="text-bad">{eur(x.set_aside - x.balance)} more set aside than the account holds</span> : `${eur(x.free)} free`}</div>
          </div>
        ))}
      </div>
    </Card>
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
        {!!g.held?.length && (
          <div className="mt-0.5 text-[11px] text-muted"><Icon name="piggy" size={11} className="mr-0.5 inline" />{g.held.map((h) => `${eur(h.amount)} in ${h.name}`).join(' · ')}</div>
        )}
        {g.tag && (g.spent ?? 0) !== 0 && (
          <div className="mt-0.5 text-[11px] text-muted">spent so far <b className="tnum text-ink2">{eur(g.spent ?? 0)}</b> · {g.saved >= (g.spent ?? 0) ? `${eur(g.saved - (g.spent ?? 0))} of the savings left` : `${eur((g.spent ?? 0) - g.saved)} more than saved`}</div>
        )}
        {priced && !full && g.outlook ? (
          <div className={clsx('mt-1 text-xs', g.outlook.tone === 'good' ? 'text-good' : g.outlook.tone === 'warn' ? 'text-warn' : 'text-bad')}>{g.outlook.text}</div>
        ) : priced && !full && g.target_date && (
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

function GoalSheet({ g, pots, onClose }: { g: Partial<Goal>; pots: Pot[]; onClose: () => void }) {
  const [v, setV] = useState<Partial<Goal>>(g)
  const [delta, setDelta] = useState<number | undefined>()
  const [why, setWhy] = useState('')
  // Where the money is set aside: the account it already sits in, else none
  // (plain saving) — pick a savings account to set its money aside there.
  const [acct, setAcct] = useState(() => g.held?.[0]?.account_id ?? '')
  const { data: accounts } = useAccounts()
  const acctName = (id: string) => accounts?.find((a) => a.id === id)?.name ?? id
  const pot = pots.find((x) => x.account_id === acct)
  const inAcct = acct ? (g.held ?? []).find((h) => h.account_id === acct)?.amount ?? 0 : (g.saved ?? 0) - (g.held ?? []).reduce((t, h) => t + h.amount, 0)
  const refresh = useRefresh()
  const toast = useToast()
  const { data: moves } = useQuery({ queryKey: ['goal-moves', g.id], queryFn: () => api.get<any[]>(`/goals/${g.id}/moves`), enabled: !!g.id })
  useEffect(() => setV(g), [g])
  const done = (msg: string) => { refresh(); toast(msg, 'good'); onClose() }
  const fail = (e: unknown) => toast((e as Error).message, 'bad')
  const save = useMutation({ mutationFn: (body: Partial<Goal>) => (g.id ? api.put(`/goals/${g.id}`, body) : api.post('/goals', body)), onError: fail })
  const put = useMutation({ mutationFn: (amount: number) => api.post(`/goals/${g.id}/moves`, { amount, note: why, account_id: acct }), onSuccess: () => done('Saved'), onError: fail })
  const del = useMutation({ mutationFn: () => api.del(`/goals/${g.id}`), onSuccess: () => done('Removed'), onError: fail })
  // Trip tags already in use, so a planned trip joins the one you're tagging.
  const { data: tagList } = useTags()
  const tripTags = (tagList ?? []).filter((x) => x.tag.startsWith('trip:')).map((x) => ({ value: x.tag, count: x.count }))
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
          <Field label="Wanted by" hint="optional"><DateInput clearable value={v.target_date ?? ''} onChange={(d) => setV({ ...v, target_date: d })} /></Field>
        </div>
        <div className="rounded-xl border border-line p-3">
          <Toggle checked={v.tag != null && v.tag !== ''} onChange={(on) => setV({ ...v, tag: on ? (v.tag || 'trip:' + (v.name ?? '').toLowerCase().trim().replace(/[^a-z0-9ąčęėįšųūž]+/g, '-').replace(/^-|-$/g, '')) : '' })} label="It's a trip" />
          {!!v.tag && (
            <div className="mt-2">
              <Field label="Trip tag" hint="Tag its flights, hotel and spending with this — they count against what you saved, and it shows on the Trips tab.">
                <SuggestInput value={v.tag} onChange={(x) => setV({ ...v, tag: x })} placeholder="trip:rome-2027" options={tripTags} />
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
            {!!g.held?.length && <div className="mt-0.5 text-xs text-muted">{g.held.map((h) => `${eur(h.amount)} in ${h.name}`).join(' · ')}</div>}
            <Field label="Set aside in">
              <select className="input" value={acct} onChange={(e) => setAcct(e.target.value)}>
                <option value="">Not in a particular account</option>
                {pots.map((x) => <option key={x.account_id} value={x.account_id}>{x.name} · {eur(x.free)} free</option>)}
              </select>
            </Field>
            <div className="mt-2 grid grid-cols-[7rem_1fr] gap-2">
              <NumberInput value={delta} onChange={setDelta} placeholder="€" className="input tnum" />
              <input className="input" value={why} onChange={(e) => setWhy(e.target.value)} placeholder="why (a bonus, a repair…)" />
            </div>
            {pot && <div className="mt-1 text-xs text-muted">{pot.name}: {eur(pot.balance)}, {eur(pot.set_aside)} already set aside{inAcct > 0 && ` (${eur(inAcct)} for this)`}</div>}
            <div className="mt-2 flex flex-wrap gap-2">
              <button className="btn-outline h-8 text-xs" disabled={put.isPending || !delta || delta <= 0 || (!!pot && delta > pot.free)} onClick={() => put.mutate(delta!)}>{pot ? `Set aside${delta && delta > pot.free ? ` — only ${eur(pot.free)} free` : ''}` : 'Put in'}</button>
              <button className="btn-outline h-8 text-xs" disabled={put.isPending || !delta || delta <= 0 || delta > inAcct} onClick={() => put.mutate(-delta!)}>{pot ? 'Release' : 'Take out'}</button>
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
                  <span className="text-ink2">{monthLabel(m.month)} · {m.kind === 'funding' ? 'month-end funding' : m.note || 'by hand'}{m.account_id && <span className="text-muted"> · {acctName(m.account_id)}</span>}</span>
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
