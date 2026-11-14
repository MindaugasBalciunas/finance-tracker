import { useState, type ReactNode } from 'react'
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { ledgerLink, monthEnd } from '../lib/links'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis } from 'recharts'
import { api } from '../lib/api'
import { useOverview, usePlan, useRefresh, useWide } from '../lib/hooks'
import { useCats } from '../lib/categories'
import { addMonths, eur, eurc, monthLabel, shortDate, thisMonth, todayISO } from '../lib/format'
import type { Budget, PlanLine, PlanReport, Runbook } from '../lib/types'
import { AskCFO, Card, CountUp, ErrorBox, Field, hue, Loading, Meter, NumberInput, PageHeader, Segmented, Sheet, Tabs, Toggle, useToast } from '../components/ui'
import { CategoryPicker, TagInput } from '../components/pickers'
import { TooltipBox } from '../components/charts'
import { MonthTimeline } from '../components/MonthChart'
import { Icon } from '../components/Icon'
import { MerchantMark } from '../components/MerchantLogo'
import { Wishlist } from './plan/Wishlist'
import { ForecastView } from './plan/Forecast'
import { Trips } from './plan/Trips'
import { PlanSettings } from './plan/Settings'
import { RecurringView } from './insights/Recurring'
import { FreeBudgetCard } from './home/FreeBudget'

export default function Plan() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.includes('/recurring') ? 'recurring' : loc.pathname.includes('/forecast') ? 'forecast' : loc.pathname.includes('/trips') ? 'trips' : loc.pathname.includes('/wishlist') || loc.pathname.includes('/decisions') ? 'wishlist' : loc.pathname.includes('/settings') ? 'settings' : 'month'
  return (
    <div>
      <PageHeader title="Plan" actions={<AskCFO q="Review my budget plan against the last 12 months: which lines are unrealistic, what should become a fund, and is the plan affordable on my income?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'month' ? '/plan' : `/plan/${v}`)}
        tabs={[{ value: 'month', label: 'Budget' }, { value: 'recurring', label: 'Recurring' }, { value: 'forecast', label: 'Forecast' }, { value: 'wishlist', label: 'Wish list' }, { value: 'trips', label: 'Trips' }, { value: 'settings', label: 'Income & goals' }]} />
      <Routes>
        <Route path="/" element={<Month />} />
        {/* Payday's checklist now lives in the budget's saving and fixed lines. */}
        <Route path="/payday" element={<Navigate to="/plan" replace />} />
        {/* The decision journal lives on the wish list. */}
        <Route path="/decisions" element={<Navigate to="/plan/wishlist" replace />} />
        <Route path="/recurring" element={<RecurringView />} />
        <Route path="/forecast" element={<ForecastView />} />
        <Route path="/wishlist" element={<Wishlist />} />
        <Route path="/trips" element={<Trips />} />
        <Route path="/settings" element={<PlanSettings />} />
      </Routes>
    </div>
  )
}

function Month() {
  const [month, setMonth] = useState(thisMonth())
  const { data: r, isLoading, error } = usePlan(month)
  const [edit, setEdit] = useState<Partial<Budget> | null>(null)
  const refresh = useRefresh()
  const toast = useToast()
  const cats = useCats()
  const applySuggestion = useMutation({
    mutationFn: ({ line, amount, fund }: { line: PlanLine; amount: number; fund?: boolean }) =>
      api.put(`/budgets/${line.id}`, { ...line, amount, fund: fund ?? line.fund, start_month: fund && !line.fund ? month : line.start_month, from_month: month }),
    onSuccess: () => { refresh(); toast('Budget updated', 'good') },
  })
  const applyAll = useMutation({
    mutationFn: async (ls: PlanLine[]) => {
      for (const l of ls) await api.put(`/budgets/${l.id}`, { ...l, amount: l.suggestion!.amount, from_month: month })
    },
    onSuccess: (_, ls) => { refresh(); toast(`${ls.length} budgets updated`, 'good') },
  })
  const { data: o } = useOverview()
  const current = month === thisMonth()
  const wide = useWide()
  // This month's payday checklist: where each saving line's money goes and
  // whether the bank shows it moved (the same engine as Home's reminder).
  const { data: rb } = useQuery({ queryKey: ['runbook'], queryFn: () => api.get<Runbook>('/plan/payday?moves=0'), enabled: current })
  if (isLoading) return <Loading />
  if (error || !r) return <ErrorBox error={error} />
  const rebalance = needsRebalance(o?.plan.track ?? [])
  // The "set lines to what is real" list: inside the rebalance card when the
  // plan keeps missing, a card of its own otherwise.
  const suggestions = (bare: boolean) => (
    <Suggestions bare={bare} lines={r.lines.filter((l) => l.suggestion && l.kind !== 'saving')} unallocated={r.income_base - r.fixed_planned - r.saving_planned - r.spending_planned} busy={applySuggestion.isPending || applyAll.isPending}
      onApply={(l, fund) => applySuggestion.mutate({ line: l, amount: l.suggestion!.amount, fund })} onApplyAll={(ls) => applyAll.mutate(ls)} />
  )
  const groups: { kind: string; title: string; hint: string }[] = [
    { kind: 'fixed', title: 'Fixed obligations', hint: 'Committed every month — never counted against spending budgets' },
    { kind: 'saving', title: 'Saving & investing', hint: 'Transfers into investments, pension and debt principal' },
    { kind: 'spending', title: 'Spending', hint: 'Day-to-day choices' },
  ]
  // What each line still needs this month, for the current month only.
  const extra = (l: PlanLine): LineExtra | undefined => {
    if (!current) return undefined
    if (l.kind === 'saving') {
      const st = rb?.steps.find((x) => x.key === `saving:${l.id}`)
      if (!st) return undefined
      const route = [st.from_name, st.to_name].filter(Boolean).join(' → ')
      return { route, done: st.status === 'done' }
    }
    if (l.kind === 'fixed') {
      const f = o?.plan.fixed?.find((x) => x.name === l.name)
      if (!f) return undefined
      if (f.spent >= f.budgeted && f.paid) return { note: `paid ${shortDate(f.paid)}`, done: true }
      if (f.due) return { note: `due ${shortDate(f.due)}`, warn: f.due <= todayISO() }
    }
    return undefined
  }
  // Discretionary spending no line covers.
  const notBudgeted = r.unbudgeted.length > 0 && (

      <Card pad={false} title="Not budgeted">
        <div className="px-4 pb-2 -mt-1 text-xs text-muted">Discretionary spending no line covers.</div>
        <div className="divide-y divide-line border-t border-line">
          {r.unbudgeted.map((u) => (
            <div key={u.category} className="flex items-center gap-3 px-4 py-2.5">
              <Link to={ledgerLink({ category: u.category, month })} className="min-w-0 flex-1 hover:text-accent">
                <div className="text-sm font-medium">{cats.name(u.category)}</div>
                <div className="text-xs text-muted">{eur(u.spent)} this month · {eur(u.year_spent)} this year</div>
              </Link>
              {u.suggestion && (
                <button className="chip hover:bg-sunken" onClick={() => setEdit({ kind: 'spending', name: cats.name(u.category), categories: [u.category], amount: u.suggestion!.amount, fund: u.suggestion!.lumpy, start_month: month, period: 'monthly' })}>
                  + budget {eur(u.suggestion.amount)}{u.suggestion.lumpy ? ' fund' : ''}
                </button>
              )}
            </div>
          ))}
        </div>
      </Card>
  )
  // One card per kind of line: fixed, saving, spending.
  const groupCard = (g: { kind: string; title: string; hint: string }) => {
    const lines = r.lines.filter((l) => l.kind === g.kind)
    return (
      <div key={g.kind} id={`plan-${g.kind}`} className="scroll-mt-16"><Card pad={false} color={KIND_HUE[g.kind]} title={<span>{g.title} <span className="font-normal text-muted">{eur(lines.reduce((a, l) => a + l.monthly_share, 0))}/mo</span></span>}
        action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setEdit({ kind: g.kind as Budget['kind'], period: 'monthly', categories: [], tag: '', amount: 0 })}><Icon name="plus" size={14} />Line</button>}>
        <div className="px-4 pb-2 -mt-1 text-xs text-muted">{g.hint}</div>
        {g.kind === 'saving' && current && rb && rb.steps.some((x) => x.kind === 'saving') && (() => {
          const steps = rb.steps.filter((x) => x.kind === 'saving')
          const left = steps.filter((x) => x.status !== 'done').length
          return (
            <div className={clsx('mx-4 mb-2 flex items-center gap-2 rounded-lg px-3 py-2 text-xs', left ? 'bg-sunken/60 text-ink2' : 'bg-good/10 text-good')}>
              <Icon name={left ? 'send' : 'check'} size={14} className="shrink-0" />
              <span>{rb.payday ? <>Salary in {shortDate(rb.payday)} · </> : <>Salary not in yet{rb.expected && <> — expected {shortDate(rb.expected)}</>} · </>}
                {left ? `${steps.length - left} of ${steps.length} moved — the rest ticks off when the bank shows it` : 'all moved this month'}</span>
            </div>
          )
        })()}
        {lines.length === 0 ? <div className="px-4 pb-4 text-sm text-muted">No lines yet.</div> : (
          <div className="divide-y divide-line border-t border-line">
            {lines.map((l) => <LineRow key={l.id} l={l} month={month} monthPace={current ? monthProgress() : undefined} extra={extra(l)} onEdit={() => setEdit(l)} />)}
          </div>
        )}
      </Card></div>
    )
  }
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, -1))} aria-label="Previous month"><Icon name="chevronL" /></button>
        <div className="text-base font-semibold">{monthLabel(month, true)}</div>
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, 1))} aria-label="Next month"><Icon name="chevronR" /></button>
      </div>
      {/* The month's number first, then why and what to change, then the lines; the slower views close the page. */}
      <Summary r={r} current={current} onTrim={(l, amount) => applySuggestion.mutate({ line: l, amount })} />
      {/* Home's spending budget card, for whichever month is open. */}
      {r.free && <FreeBudgetCard f={r.free} month={monthLabel(month, true)} unreviewed={current ? r.unreviewed : 0} link={false} />}
      {current && <EmergencyFund />}
      {/* Home's month chart, full width: spending so far against the plan's
          pace through the month's free money, and where this rhythm ends up. */}
      {current && (o?.plan.days?.length ?? 0) >= 28 && (
        <Card icon="trend" color="var(--s1)" title="The month day by day"
          action={<span className="text-xs text-muted">{o!.plan.safe_to_spend > 0 ? <>≈ <b className="tnum text-ink">{eur(o!.plan.per_day_left)}</b>/day · {o!.plan.days_left} days left</> : <span className="text-bad">over budget</span>}</span>}>
          <MonthTimeline p={o!.plan} big />
        </Card>
      )}
      {current && rebalance ? (
        <Rebalance r={r} onAddLine={(u) => setEdit({ kind: 'spending', name: cats.name(u.category), categories: [u.category], amount: u.suggestion!.amount, fund: u.suggestion!.lumpy, start_month: month, period: 'monthly' })}>
          {suggestions(true)}
        </Rebalance>
      ) : current && suggestions(false)}
      {/* Desktop: what goes out first (fixed, saving) and what no line covers on
          the left; day-to-day spending and once-a-year costs on the right.
          Phones keep the reading order. */}
      {wide ? (
        <div className="grid grid-cols-2 items-start gap-4">
          <div className="space-y-4">{groups.filter((g) => g.kind !== 'spending').map(groupCard)}{notBudgeted}</div>
          <div className="space-y-4">{groups.filter((g) => g.kind === 'spending').map(groupCard)}{current && <OnceAYear r={r} onEdit={setEdit} month={month} />}</div>
        </div>
      ) : <>
        {groups.map(groupCard)}
        {notBudgeted}
        {current && <OnceAYear r={r} onEdit={setEdit} month={month} />}
      </>}
      <YearGrid r={r} />
      {edit && <BudgetEditor b={edit} month={month} onClose={() => setEdit(null)} />}
    </div>
  )
}

/** When recent months keep ending over the plan, the case for changing it
 *  comes right under the month's number:
 *  the track record, the gap in a usual month, and the three levers that
 *  close it — each with its amount and one tap to act. Budget lines don't
 *  change left to spend on their own (it is income − obligations − saving −
 *  funds − what is spent), so the saving target and funds are named too. */
function needsRebalance(t: { left: number }[]) {
  return t.length >= 3 && t.filter((m) => m.left < 0).length >= 2
}

function Rebalance({ r, onAddLine, children }: { r: PlanReport; onAddLine: (u: PlanReport['unbudgeted'][number]) => void; children?: ReactNode }) {
  const { data: o } = useOverview()
  const cats = useCats()
  const t = o?.plan.track ?? []
  const overs = t.filter((m) => m.left < 0).length
  if (!needsRebalance(t)) return null
  const s = [...t.map((m) => m.spent)].sort((a, b) => a - b)
  const usualSpent = s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2
  const free = o?.plan.free_planned ?? 0
  const gap = usualSpent - free
  // The same lines the suggestions list shows, so the two never disagree.
  const off = r.lines.filter((l) => l.suggestion && l.kind !== 'saving')
  const unb = r.unbudgeted.filter((u) => u.suggestion).sort((a, b) => b.suggestion!.amount - a.suggestion!.amount)
  const unbTotal = unb.reduce((a, u) => a + u.suggestion!.amount, 0)
  const saving = r.lines.filter((l) => l.kind === 'saving')
  const scrollTo = (id: string) => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  return (
    <section className="card p-4" style={hue('var(--hue-warn)')}>
      <div className="flex items-start gap-3">
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-warn/15 text-warn"><Icon name="target" /></span>
        <div className="min-w-0 flex-1">
          <div className="font-semibold">This plan ended over in {overs} of the last {t.length} months</div>
          <div className="mt-0.5 text-sm text-ink2">
            A usual month spends <b className="tnum text-ink">{eur(usualSpent)}</b> day to day; the plan leaves <b className="tnum text-ink">{eur(free)}</b> — a gap of <b className="tnum text-warn">{eur(gap)}</b> a month.
          </div>
        </div>
      </div>
      <div className="mt-2 flex gap-1" aria-label={t.map((m) => `${monthLabel(m.month)} ${m.left < 0 ? 'over' : 'left'} ${eur(Math.abs(m.left))}`).join(', ')}>
        {t.map((m) => (
          <span key={m.month} className={clsx('flex-1 rounded px-1 py-0.5 text-center text-[10px] tnum', m.left < 0 ? 'bg-bad/15 text-bad' : 'bg-good/15 text-good')}>
            {monthLabel(m.month).split(' ')[0]} {m.left < 0 ? '−' : '+'}{eur(Math.round(Math.abs(m.left)))}
          </span>
        ))}
      </div>
      <div className="mt-3 divide-y divide-line border-t border-line text-sm">
        <div className="flex items-center gap-3 py-2.5">
          <span className="min-w-0 flex-1">
            <b>1. Fund what you really spend on</b>
            <span className="block text-xs text-muted">{unb.length ? <>{unb.slice(0, 3).map((u) => cats.name(u.category)).join(', ')}{unb.length > 3 && ` +${unb.length - 3}`} have no line — about <span className="tnum">{eur(unbTotal)}</span>/mo. A fund for lumpy ones keeps them from tipping a month.</> : 'Every category you spend on has a line.'}</span>
          </span>
          {unb[0] && <button className="chip shrink-0 hover:bg-sunken" onClick={() => onAddLine(unb[0])}>+ {cats.name(unb[0].category)} {eur(unb[0].suggestion!.amount)}{unb[0].suggestion!.lumpy ? ' fund' : ''}</button>}
        </div>
        <div className="py-2.5">
          <b>2. Set lines to what is real</b>
          {!off.length && <span className="block text-xs text-muted">Every line matches the last 12 months.</span>}
          {children}
        </div>
        <div className="flex items-center gap-3 py-2.5">
          <span className="min-w-0 flex-1">
            <b>3. Decide the saving target</b>
            <span className="block text-xs text-muted">The plan saves <span className="tnum">{eur(r.saving_planned)}</span> first{o?.usual_saved != null && <>; a usual month has left <span className="tnum">{eur(Math.round(o.usual_saved))}</span> from income</>}. Keep it and spend <span className="tnum">{eur(gap)}</span> less, or lower it to what the month funds.</span>
          </span>
          {saving[0] && <button className="chip shrink-0 hover:bg-sunken" onClick={() => scrollTo('plan-saving')}>Saving lines</button>}
        </div>
      </div>
      <div className="mt-2 flex justify-end"><AskCFO q={`My plan ended over in ${overs} of the last ${t.length} months; a usual month spends ${eur(usualSpent)} day to day and the plan leaves ${eur(free)}. Propose a rebalanced plan: which lines to change, which to add, and whether my ${eur(r.saving_planned)} saving target is realistic.`} label="Ask CFO for a rebalanced plan" /></div>
    </section>
  )
}

type YearlyCost = { merchant: string; category: string; month: number; typical: number; last: number; next: string; monthly: number; fund?: string; fund_id?: number; covered: boolean; shortfall?: number }

/** Costs that come once a year — the car service every October — and
 *  whether a fund sets money aside for them, so they stop tipping the month
 *  they land in. One tap opens the fund line, filled in. */
function OnceAYear({ r, onEdit, month }: { r: PlanReport; onEdit: (b: Partial<Budget>) => void; month: string }) {
  const cats = useCats()
  const { data } = useQuery({ queryKey: ['plan-yearly'], queryFn: () => api.get<YearlyCost[]>('/plan/yearly'), staleTime: 300_000 })
  if (!data?.length) return null
  const open = data.filter((y) => !y.covered)
  return (
    <Card icon="calendar" color="var(--s4)" title="Once a year" action={<span className="text-xs text-muted">{open.length ? `${open.length} not set aside for` : 'all set aside for'}</span>}>
      <div className="-mt-1 mb-2 text-xs text-muted">Costs that come around in the same month every year. Set aside monthly, they stop tipping the month they land in.</div>
      <div className="divide-y divide-line border-t border-line">
        {data.map((y) => {
          const line = y.fund_id ? r.lines.find((l) => l.id === y.fund_id) : undefined
          return (
            <div key={y.merchant} className="flex items-center gap-3 py-2.5">
              <MerchantMark merchant={y.merchant} category={y.category} size={32} />
              <div className="min-w-0 flex-1">
                <div className="text-sm"><span className="font-medium">{y.merchant}</span> <span className="text-muted">· {cats.name(y.category)}</span></div>
                <div className="text-xs text-muted">
                  every {monthLabel(y.next, true).split(' ')[0]} · about <span className="tnum text-ink2">{eur(y.typical)}</span>{y.last > y.typical * 1.15 && <> (last time <span className="tnum text-warn">{eur(y.last)}</span>)</>} · <span className="tnum">{eur(y.monthly)}</span>/mo
                </div>
                <div className={clsx('text-xs', y.covered ? 'text-good' : 'text-warn')}>
                  {y.covered ? `Covered by the ${y.fund} fund` : y.fund ? `The ${y.fund} fund is ${eur(y.shortfall!)}/mo short` : 'No fund sets money aside for it'}
                </div>
              </div>
              {!y.covered && (line
                ? <button className="chip shrink-0 hover:bg-sunken" onClick={() => onEdit({ ...line, amount: Math.ceil(line.monthly_share + (y.shortfall ?? 0)) })}>Raise to {eur(Math.ceil(line.monthly_share + (y.shortfall ?? 0)))}</button>
                : <button className="chip shrink-0 hover:bg-sunken" onClick={() => onEdit({ kind: 'spending', name: cats.name(y.category), categories: [y.category], amount: Math.ceil(y.monthly), fund: true, start_month: month, period: 'monthly' })}>+ fund {eur(Math.ceil(y.monthly))}/mo</button>)}
            </div>
          )
        })}
      </div>
    </Card>
  )
}

/** Lines that differ from what the last 12 months suggest, one tap each or
 *  all at once. `bare`: drawn inside the rebalance card, without its own frame. */
function Suggestions({ lines, unallocated, busy, onApply, onApplyAll, bare = false }: { lines: PlanLine[]; unallocated: number; busy: boolean; onApply: (l: PlanLine, fund?: boolean) => void; onApplyAll: (ls: PlanLine[]) => void; bare?: boolean }) {
  const [open, setOpen] = useState(false)
  if (lines.length === 0) return null
  const delta = lines.reduce((a, l) => a + l.suggestion!.amount - l.monthly_share, 0)
  return (
    <section id="plan-suggestions" className={clsx('scroll-mt-16', bare ? 'mt-1' : 'card p-4')}>
      <div className="flex flex-wrap items-center gap-3">
        {!bare && <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-accent/10 text-accent"><Icon name="spark" /></span>}
        <div className={clsx('min-w-0 flex-1', bare ? 'text-xs text-muted' : 'text-sm')}>
          <b>{lines.length} {lines.length === 1 ? 'budget differs' : 'budgets differ'}</b> from what the last 12 months suggest
          <div className="text-xs text-muted">Applying {lines.length === 1 ? 'it' : 'all'} changes budgets by <span className="whitespace-nowrap tnum">{delta >= 0 ? '+' : '−'}{eur(Math.abs(delta))}/mo</span>
            {unallocated - delta < 0 && <span className="text-warn"> — the plan would then exceed the income base by <span className="whitespace-nowrap tnum">{eur(delta - unallocated)}</span></span>}</div>
        </div>
        <div className="flex w-full justify-end gap-1.5 sm:w-auto">
          <button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Review'}</button>
          {lines.length > 1 && <button className="btn-primary h-8 px-2.5 text-xs" disabled={busy} onClick={() => onApplyAll(lines)}>Apply all</button>}
        </div>
      </div>
      {open && (
        <div className="mt-3 divide-y divide-line border-t border-line">
          {lines.map((l) => (
            <div key={l.id} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 py-2.5">
              <div className="min-w-0 flex-1 basis-[calc(100%-3rem)] sm:basis-0">
                <div className="text-sm"><span className="font-medium">{l.name}</span> <span className="tnum text-muted">{eur(l.monthly_share)} →</span> <b className="tnum">{eur(l.suggestion!.amount)}</b></div>
                <div className="text-xs text-muted">{l.suggestion!.basis}</div>
              </div>
              <div className="flex w-full justify-end gap-1.5 sm:w-auto">
                {l.suggestion!.lumpy && !l.fund && l.kind === 'spending' && <button className="chip hover:bg-sunken" disabled={busy} onClick={() => onApply(l, true)}>Make it a fund</button>}
                <button className="chip-on" disabled={busy} onClick={() => onApply(l)}>Apply</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function Summary({ r, current, onTrim }: { r: PlanReport; current: boolean; onTrim: (l: PlanLine, amount: number) => void }) {
  // Same colour as Home: amber when the month is heading over at the usual rhythm.
  const { data: o } = useOverview()
  const heading = current && (o?.plan.projected_left ?? 0) < 0
  const planned = r.fixed_planned + r.saving_planned + r.spending_planned
  const unallocated = r.income_base - planned
  // Over-allocated: offer to take the excess off the biggest monthly spending budget.
  const trim = unallocated < 0 ? r.lines.filter((l) => l.kind === 'spending' && l.period === 'monthly' && !l.fund && l.monthly_share > -unallocated).sort((a, b) => b.monthly_share - a.monthly_share)[0] : undefined
  // A finished month: one card with how it went — no plan-basis figure
  // beside the real one, no second set of bars.
  if (!current) return <MonthResult r={r} />
  return (
    // Side by side, equal, only where both fit (xl); stacked below that.
    <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
      <section className="hero flex flex-col p-4">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <div className="shrink-0">
            <div className="text-sm text-ink2">{current ? 'Left to spend this month' : 'Left after the plan'}</div>
            <div className={clsx('text-3xl font-semibold tracking-tight', r.safe_to_spend < 0 ? 'text-bad' : heading ? 'text-warn' : 'text-good')}><CountUp value={r.safe_to_spend} format={eur} /></div>
            {heading && r.safe_to_spend >= 0 && <div className="text-xs text-warn">heading for {eur(-o!.plan.projected_left)} over by month end</div>}
          </div>
          {current && o && o.plan.days_left > 0 && r.safe_to_spend > 0 && (
            <div className="rounded-xl bg-sunken/70 px-3 py-2 text-right">
              <div className="text-lg font-semibold tnum">≈ {eur(o.plan.per_day_left)}<span className="text-sm font-normal text-muted"> a day</span></div>
              <div className="text-xs text-muted">for the {o.plan.days_left} days left · you average {eur(o.plan.avg_day)}</div>
            </div>
          )}
        </div>
        <Allocation r={r} />
        {/* The legend shows what's not allocated; over-allocation needs words and the fix. */}
        {unallocated < 0 && (
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-bad">
            <span>The plan allocates {eur(-unallocated)} more than the income base.</span>
            {trim && current && <button className="chip hover:bg-sunken" onClick={() => onTrim(trim, Math.floor((trim.monthly_share + unallocated) * 100) / 100)}>Take it off {trim.name} ({eur(trim.monthly_share)} → {eur(Math.floor((trim.monthly_share + unallocated) * 100) / 100)})</button>}
          </div>
        )}
        <div className="mt-auto pt-3 text-[11px] text-muted">Left to spend = income − fixed − saving − fund set-asides − spent outside funds (net of {(r.refunds ?? 0) > 0.005 ? <span className="text-good">{eur(r.refunds!)} refunds</span> : 'refunds'}){(r.fund_overdrawn ?? 0) > 0.005 && <> − <span className="text-bad">{eur(r.fund_overdrawn!)} paid beyond what funds held</span></>}{(r.unreviewed ?? 0) > 0.005 && <> · incl. <span className="text-ink2">{eur(r.unreviewed!)} in the bank inbox, not reviewed yet</span></>}</div>
      </section>
      <ActualSoFar r={r} current={current} />
    </div>
  )
}

/** The emergency fund the plan goes by: what an emergency could use — the
 *  bank (and the Cash account, unless left out) less what leaves before
 *  payday and the wish list's set-asides — against months of essentials.
 *  The payday checklist funds the wish list only above this line. */
function EmergencyFund() {
  const { data: o } = useOverview()
  const e = o?.emergency_plan
  if (!e || e.monthly_essential <= 0) return null
  const short = e.gap > 0
  const scale = Math.max(e.target * 1.5, e.available, 1)
  const rows: [string, number, string?][] = [
    ['In the bank', e.bank],
    ...(e.cash > 0 ? [[e.counts_cash ? 'Cash account (notes and coins)' : 'Cash account — not counted', e.counts_cash ? e.cash : 0, e.counts_cash ? undefined : eur(e.cash)] as [string, number, string?]] : []),
    ...(e.committed > 0 ? [[`Leaving before payday${e.payday ? ` (${shortDate(e.payday)})` : ''}`, -e.committed] as [string, number]] : []),
    ...(e.set_aside > 0 ? [['Set aside for the wish list', -e.set_aside] as [string, number]] : []),
  ]
  return (
    <Card icon="umbrella" color={short ? 'var(--hue-warn)' : 'var(--hue-good)'} title="Emergency fund"
      action={<span className={clsx('text-xs font-medium', short ? 'text-warn' : 'text-good')}>{e.months.toFixed(1)} of {e.target_months} months</span>}>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)] md:items-center">
        <div>
          <div className="flex flex-wrap items-baseline justify-between gap-x-3">
            <div><span className={clsx('text-2xl font-semibold tnum', e.available < 0 && 'text-bad')}>{eur(e.available)}</span> <span className="text-sm text-muted">an emergency could use</span></div>
            <div className={clsx('text-sm font-medium', short ? 'text-warn' : 'text-good')}>{short ? `${eur(e.gap)} short` : `${eur(e.spare)} above the floor`}</div>
          </div>
          <div className="relative mt-2 h-3 rounded-full bg-sunken" role="meter" aria-label="Emergency fund" aria-valuenow={Math.max(0, Math.min(100, Math.round((e.available / Math.max(e.target, 1)) * 100)))} aria-valuemin={0} aria-valuemax={100}>
            <div className={clsx('h-full rounded-full', short ? 'bg-warn' : 'bg-good')} style={{ width: `${Math.max(0, Math.min(1, e.available / scale)) * 100}%` }} />
            <div className="absolute -top-1 h-5 w-0.5 rounded bg-ink" style={{ left: `${(e.target / scale) * 100}%` }} title={`${e.target_months} months: ${eur(e.target)}`} />
          </div>
          <div className="mt-1 flex justify-between text-xs text-muted">
            <span>{e.target_months} months of essentials ({eur(e.monthly_essential)}/mo) = <b className="text-ink2 tnum">{eur(e.target)}</b></span>
          </div>
          <p className={clsx('mt-2 text-xs', short ? 'text-warn' : 'text-muted')}>
            {e.available < 0 ? <>What leaves before payday is {eur(-e.available)} more than you hold — top up before the wish list gets anything.</>
              : short ? <>The payday checklist keeps last month's leftovers here before the wish list gets any.</> : <>The wish list can take up to {eur(e.spare)} without going under the floor.</>}
          </p>
        </div>
        <ul className="divide-y divide-line rounded-xl border border-line text-sm">
          {rows.map(([l, v, note]) => (
            <li key={l} className="flex justify-between gap-3 px-3 py-1.5"><span className="text-ink2">{l}</span><span className={clsx('tnum', v < 0 && 'text-ink2')}>{note ?? (v < 0 ? `−${eur(-v)}` : eur(v))}</span></li>
          ))}
          <li className="flex justify-between gap-3 bg-sunken/50 px-3 py-1.5 font-medium"><span>Emergency fund</span><span className="tnum">{eur(e.available)}</span></li>
        </ul>
      </div>
    </Card>
  )
}

/** A finished month, in one card: what was left over (or how far spending
 *  ran past the income) as the headline, the income and where it went, then
 *  the plan beside what happened, group by group. */
function MonthResult({ r }: { r: PlanReport }) {
  const savingLines = r.lines.filter((l) => l.kind === 'saving').reduce((a, l) => a + l.spent, 0)
  const left = r.income_actual - r.fixed_spent - savingLines - r.discretionary_spent
  const plannedLeft = r.income_base - r.fixed_planned - r.saving_planned - r.spending_planned
  const principal = Math.max(0, r.fixed_spent - (r.fixed_expense ?? r.fixed_spent))
  // vs plan, coloured by what is good: more income and saving, less spending.
  const rows: { label: string; plan: number; actual: number; goodUp: boolean | null; note?: string }[] = [
    { label: 'Income', plan: r.income_base, actual: r.income_actual, goodUp: true },
    { label: 'Fixed', plan: r.fixed_planned, actual: r.fixed_spent, goodUp: null, note: principal > 0.005 ? `incl. ${eur(principal)} mortgage principal` : undefined },
    { label: 'Saving', plan: r.saving_planned, actual: savingLines, goodUp: true },
    { label: 'Spending', plan: r.spending_planned, actual: r.discretionary_spent, goodUp: false },
    { label: 'Left over', plan: plannedLeft, actual: left, goodUp: true },
  ]
  return (
    <section className="hero p-4 sm:p-5" style={hue(left < 0 ? 'var(--hue-bad)' : 'var(--hue-good)')}>
      <div className="text-sm text-ink2">How {monthLabel(r.month, true)} went</div>
      <div className={clsx('text-3xl font-semibold tracking-tight', left < 0 ? 'text-bad' : 'text-good')}>
        <CountUp value={Math.abs(left)} format={eur} /> <span className="text-lg font-medium">{left < 0 ? 'more went out than came in' : 'left over'}</span>
      </div>
      <div className="grid grid-cols-1 gap-x-8 gap-y-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] lg:items-center">
        <IncomeUsed r={r} brief />
        <div className="lg:mt-4">
          <div className="mb-1.5 text-xs text-muted">Against the plan</div>
          <table className="w-full text-[13px] tnum sm:text-sm [&_td:not(:first-child)]:whitespace-nowrap [&_td:not(:first-child)]:pl-2 [&_th:not(:first-child)]:pl-2">
            <thead className="text-xs text-muted"><tr className="[&>th]:pb-1 [&>th]:font-medium"><th className="text-left" /><th className="text-right">Planned</th><th className="text-right">Actual</th><th className="text-right">vs plan</th></tr></thead>
            <tbody className="divide-y divide-line">
              {rows.map((x) => {
                const d = x.actual - x.plan
                const tone = Math.abs(d) < 0.5 || x.goodUp == null ? 'text-muted' : (d > 0) === x.goodUp ? 'text-good' : 'text-bad'
                return (
                  <tr key={x.label} className={clsx('[&>td]:py-1.5', x.label === 'Left over' && 'font-semibold')}>
                    <td className="text-left">{x.label}{x.note && <span className="block text-[11px] font-normal text-muted">{x.note}</span>}</td>
                    <td className="text-right text-ink2">{eur(x.plan)}</td>
                    <td className="text-right">{eur(x.actual)}</td>
                    <td className={clsx('text-right text-xs', tone)}>{Math.abs(d) < 0.5 ? '—' : `${d > 0 ? '+' : '−'}${eur(Math.abs(d))}`}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {plannedLeft < -0.5 && <p className="mt-2 text-xs text-muted">The plan allocated {eur(-plannedLeft)} more than its {eur(r.income_base)} income base ({r.income_base_source}).</p>}
        </div>
      </div>
    </section>
  )
}

/** A finished month: the income that actually came in, and where it went —
 *  obligations, saved and invested, spending, and what was left (or how far
 *  spending ran past it). Obligations leave the mortgage principal to saving. */
export function IncomeUsed({ r, brief = false }: { r: PlanReport; brief?: boolean }) {
  const income = r.income_actual
  if (income <= 0) return <div className="mt-4 text-sm text-muted">No income recorded for {monthLabel(r.month, true)}.</div>
  const obligations = r.fixed_expense ?? r.fixed_spent
  const parts = [
    { label: 'Obligations', value: obligations, color: 'var(--hue-fixed)' },
    { label: 'Saved & invested', value: Math.max(0, r.saved_actual), color: 'var(--hue-saving)' },
    { label: 'Spending', value: r.discretionary_spent, color: 'var(--hue-spending)' },
  ]
  const used = parts.reduce((a, x) => a + x.value, 0)
  const left = income - used
  const spent = obligations + r.discretionary_spent
  const total = Math.max(income, used, 1)
  return (
    <div className="mt-4">
      <div className="text-sm">
        <span className="text-ink2">Of the <b className="tnum text-ink">{eur(income)}</b> that came in, you spent </span>
        <b className="tnum">{eur(spent)}</b> <span className="text-muted tnum">({Math.round((spent / income) * 100)}%)</span>
        {brief
          ? <span className="text-ink2"> and put <b className="tnum text-ink">{eur(Math.max(0, r.saved_actual))}</b> into saving and the mortgage.</span>
          : <span className="text-ink2"> and {left >= 0 ? <>had <b className="tnum text-good">{eur(left)}</b> left over</> : <>went <b className="tnum text-bad">{eur(-left)}</b> past it</>}.</span>}
      </div>
      <div className="mt-2 flex h-5 overflow-hidden rounded-full bg-sunken" role="img"
        aria-label={`${parts.map((x) => `${x.label} ${eur(x.value)}`).join(', ')}, ${left >= 0 ? `left over ${eur(left)}` : `over by ${eur(-left)}`}, of ${eur(income)} received`}>
        {parts.map((x) => x.value > 0 && <div key={x.label} className="h-full border-r-2 border-[rgb(var(--surface))]" style={{ width: `${(x.value / total) * 100}%`, background: x.color }} />)}
        {left > 0 && <div className="h-full bg-good/70" style={{ width: `${(left / total) * 100}%` }} />}
      </div>
      {left < 0 && <div className="relative h-0"><div className="absolute -top-5 h-5 w-0.5 bg-ink" style={{ left: `${(income / total) * 100}%` }} title="income received" /></div>}
      <div className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2.5 text-sm sm:grid-cols-4">
        {[...parts, { label: left >= 0 ? 'Left over' : 'Past the income', value: Math.abs(left), color: left >= 0 ? 'rgb(var(--good))' : 'rgb(var(--bad))' }].map((x) => (
          <div key={x.label}>
            <div className="flex items-center gap-1.5 text-xs text-muted"><span className="h-2 w-2 rounded-full" style={{ background: x.color }} />{x.label}</div>
            <div className="tnum font-medium">{eur(x.value)} <span className="text-xs font-normal text-muted">{Math.round((x.value / income) * 100)}%</span></div>
          </div>
        ))}
      </div>
    </div>
  )
}

/** Where the income base goes: one bar split into fixed, saving, spending
 *  budgets and what no line takes — the plan at a glance. */
function Allocation({ r }: { r: PlanReport }) {
  const parts = [
    { label: 'Fixed', value: r.fixed_planned, color: 'var(--hue-fixed)' },
    { label: 'Saving', value: r.saving_planned, color: 'var(--hue-saving)' },
    { label: 'Spending budgets', value: r.spending_planned, color: 'var(--hue-spending)' },
  ]
  const planned = parts.reduce((a, x) => a + x.value, 0)
  const free = r.income_base - planned
  const total = Math.max(r.income_base, planned, 1)
  return (
    <div className="mt-4">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 text-xs text-muted">
        <span>Where your <b className="tnum text-ink">{eur(r.income_base)}</b> income base goes</span>
        <span>{r.income_base_source}</span>
      </div>
      <div className="mt-2 flex h-4 overflow-hidden rounded-full bg-sunken" role="img" aria-label={parts.map((x) => `${x.label} ${eur(x.value)}`).join(', ') + (free > 0 ? `, not allocated ${eur(free)}` : '')}>
        {parts.map((x) => x.value > 0 && <div key={x.label} className="h-full border-r-2 border-[rgb(var(--surface))] last:border-r-0" style={{ width: `${(x.value / total) * 100}%`, background: x.color }} />)}
        {free < 0 && <div className="h-full bg-bad" style={{ width: `${(-free / total) * 100}%` }} />}
      </div>
      <div className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2.5 text-sm sm:grid-cols-4">
        {parts.map((x) => (
          <div key={x.label}>
            <div className="flex items-center gap-1.5 text-xs text-muted"><span className="h-2 w-2 rounded-full" style={{ background: x.color }} />{x.label}</div>
            <div className="tnum font-medium">{eur(x.value)} <span className="text-xs font-normal text-muted">{r.income_base > 0 ? `${Math.round((x.value / r.income_base) * 100)}%` : ''}</span></div>
          </div>
        ))}
        <div>
          <div className="flex items-center gap-1.5 text-xs text-muted"><span className={clsx('h-2 w-2 rounded-full', free < 0 ? 'bg-bad' : 'bg-sunken ring-1 ring-line')} />{free < 0 ? 'Over the base' : 'Not allocated'}</div>
          <div className={clsx('tnum font-medium', free < 0 && 'text-bad')}>{eur(Math.abs(free))}</div>
        </div>
      </div>
    </div>
  )
}

/** Actual so far, each against its part of the plan: a bar per line with
 *  how much of it is used, and — this month — a tick on spending for how
 *  far through the month we are, so it reads as ahead of or behind the pace. */
function ActualSoFar({ r, current }: { r: PlanReport; current: boolean }) {
  const pace = current ? monthProgress() : 1
  const day = new Date().getDate()
  const rows: { label: string; used: number; of: number; tone: 'goal' | 'paid' | 'spend'; note?: string }[] = [
    { label: 'Income received', used: r.income_actual, of: r.income_base, tone: 'goal' },
    { label: 'Fixed paid', used: r.fixed_spent, of: r.fixed_planned, tone: 'paid' },
    { label: 'Saved & invested', used: r.saved_actual, of: r.saving_planned, tone: 'goal' },
    { label: 'Spending', used: r.discretionary_spent, of: r.spending_planned, tone: 'spend',
      note: [r.fund_spent > 0.005 && `${eur(r.fund_spent)} from funds`, (r.refunds ?? 0) > 0.005 && `after ${eur(r.refunds!)} refunds`].filter(Boolean).join(' · ') || undefined },
  ]
  return (
    <section className="card p-4 text-sm">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <div className="section-title">Actual so far</div>
        {current && <span className="text-xs text-muted">day {day} · {Math.round(pace * 100)}% of the month</span>}
      </div>
      <ul className="space-y-3">
        {rows.map((x) => {
          const ratio = x.of > 0 ? x.used / x.of : x.used > 0 ? 1 : 0
          const over = x.tone === 'spend' && ratio > 1
          const ahead = x.tone === 'spend' && current && !over && ratio > pace + 0.1
          const done = x.tone !== 'spend' && x.of > 0 && ratio >= 0.995
          const fill = over ? 'bg-bad' : ahead ? 'bg-warn' : done ? 'bg-good' : x.tone === 'spend' ? 'bg-accent' : x.tone === 'paid' ? 'bg-[var(--s1)]' : 'bg-[var(--s3)]'
          const left = x.of - x.used
          const status = x.of <= 0 ? (x.used > 0 ? 'nothing planned' : '—')
            : x.tone === 'spend' ? (over ? `${eur(-left)} over` : `${eur(left)} left${current ? ahead ? ' · ahead of the month' : ' · on pace' : ''}`)
            : done ? (x.tone === 'paid' ? 'all paid' : 'done') : `${eur(left)} to go`
          return (
            <li key={x.label}>
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-ink2">{x.label}</span>
                <span className="tnum"><b className="font-semibold">{eur(x.used)}</b><span className="text-muted"> of {eur(x.of)}</span></span>
              </div>
              <div className="relative mt-1 h-2.5 overflow-hidden rounded-full bg-sunken" role="meter" aria-label={x.label} aria-valuenow={Math.round(ratio * 100)} aria-valuemin={0} aria-valuemax={100}>
                <div className={clsx('h-full rounded-full', fill)} style={{ width: `${Math.max(x.used > 0 ? 2 : 0, Math.min(ratio, 1) * 100)}%` }} />
                {/* Income and bills come on their own days; only spending runs with the month. */}
                {current && pace < 1 && x.tone === 'spend' && <div className="absolute top-0 h-full w-0.5 bg-ink/50" style={{ left: `${pace * 100}%` }} title="today" />}
              </div>
              <div className={clsx('mt-0.5 flex justify-between gap-2 text-xs', over ? 'text-bad' : ahead ? 'text-warn' : 'text-muted')}>
                <span className="tnum">{x.of > 0 ? `${Math.round(ratio * 100)}%` : ''}{x.note && <span className="text-muted">{x.of > 0 ? ' · ' : ''}{x.note}</span>}</span>
                <span>{status}</span>
              </div>
            </li>
          )
        })}
      </ul>
      {current && <div className="mt-3 flex items-center gap-1.5 text-[11px] text-muted"><span className="inline-block h-2.5 w-0.5 bg-ink/50" />today — spending past the line is ahead of the month</div>}
    </section>
  )
}

/** Twelve months × every line: how much of each month's share was used. */
function YearGrid({ r }: { r: PlanReport }) {
  const [open, setOpen] = useState(false)
  const lines = r.lines.filter((l) => l.kind !== 'saving')
  const cell = (spent: number, budget: number, fund: boolean) => {
    if (budget <= 0) return { bg: 'rgb(var(--sunken))', label: '' }
    const ratio = spent / budget
    if (ratio > 1 && !fund) return { bg: 'color-mix(in oklab, rgb(var(--bad)) 75%, rgb(var(--surface)))', label: 'over' }
    return { bg: `color-mix(in oklab, var(--s1) ${Math.round(12 + 70 * Math.min(ratio, 1))}%, rgb(var(--surface)))`, label: '' }
  }
  return (
    <Card title="The year at a glance" action={<button className="btn-ghost h-8 text-xs" onClick={() => setOpen(!open)}>{open ? 'Hide' : 'Show'}</button>}>
      {open && (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead><tr><th className="pb-1 pr-2 text-left font-medium text-muted">Line</th>{r.months.map((m) => <th key={m} className="pb-1 font-medium text-muted">{monthLabel(m).slice(0, 3)}</th>)}</tr></thead>
            <tbody>
              {lines.map((l) => (
                <tr key={l.id}>
                  <td className="max-w-[8rem] truncate py-0.5 pr-2">{l.name}</td>
                  {l.history.map((h) => {
                    const c = cell(h.spent, h.budget, l.fund && (!l.start_month || h.month >= l.start_month))
                    return <td key={h.month} className="p-0.5"><div title={`${monthLabel(h.month, true)}: ${eur(h.spent)} of ${eur(h.budget)}`} className="h-6 min-w-6 rounded" style={{ background: c.bg }} /></td>
                  })}
                </tr>
              ))}
            </tbody>
          </table>
          <div className="mt-2 flex items-center gap-3 text-[11px] text-muted"><span>Darker = more of the month's share used</span><span className="inline-flex items-center gap-1"><span className="h-3 w-3 rounded" style={{ background: 'color-mix(in oklab, rgb(var(--bad)) 75%, rgb(var(--surface)))' }} />over budget</span></div>
        </div>
      )}
    </Card>
  )
}

// How far through the current month we are (today counts as begun).
function monthProgress() {
  const d = new Date()
  return d.getDate() / new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()
}

// Budget groups by meaning: obligations, what you keep, what you spend.
const KIND_HUE: Record<string, string> = { fixed: 'var(--hue-fixed)', saving: 'var(--hue-saving)', spending: 'var(--hue-spending)' }

type LineExtra = { route?: string; note?: string; done?: boolean; warn?: boolean }

export function LineRow({ l, month, onEdit, monthPace, extra }: { l: PlanLine; month: string; onEdit: () => void; monthPace?: number; extra?: LineExtra }) {
  const cats = useCats()
  const nav = useNavigate()
  // The line's transactions, matched as the line matches them: its tag and
  // categories, the kind its lines count (spending: expenses, saving:
  // transfers), and for a yearly line the year so far.
  const kind = l.kind === 'spending' ? 'expense' as const : l.kind === 'saving' ? 'transfer' as const : undefined
  const txs = (m: string) => ledgerLink({ tag: l.tag || undefined, category: l.categories, kind,
    ...(l.period === 'yearly' && m === month ? { from: `${m.slice(0, 4)}-01-01`, to: monthEnd(m) } : { month: m }) })
  const [open, setOpen] = useState(false)
  const over = l.remaining < 0
  const what = l.tag ? `#${l.tag}` : l.categories.map((c) => cats.name(c)).join(', ')
  // The tick shows where spending would be today at an even pace: the year
  // for yearly lines, the month for monthly spending (funds are lumpy by design).
  const pace = l.year ? l.year.elapsed : l.kind === 'spending' && !l.fund_state && l.period === 'monthly' ? monthPace : undefined
  return (
    <div className="px-4 py-3">
      <button className="w-full text-left" onClick={() => setOpen(!open)}>
        <div className="flex items-baseline justify-between gap-2">
          <div className="min-w-0">
            <span className="text-sm font-medium">{l.name}</span>
            {l.fund && (l.fund_state || !l.start_month || l.start_month <= month) && <span className="ml-1.5 rounded-full bg-accent/10 px-1.5 py-0.5 text-[10px] font-medium text-accent">{l.fund_state ? 'fund' : `fund from ${monthLabel(l.start_month)}`}</span>}
            {l.period === 'yearly' && <span className="ml-1.5 rounded-full bg-sunken px-1.5 py-0.5 text-[10px] font-medium text-ink2">yearly</span>}
            <div className="truncate text-xs text-muted">{what}</div>
            {extra && (extra.route || extra.note) && (
              <div className={clsx('mt-0.5 flex items-center gap-1 text-xs', extra.done ? 'text-good' : extra.warn ? 'text-warn' : 'text-ink2')}>
                {extra.done && <Icon name="check" size={12} className="shrink-0" />}
                <span className="truncate">{[extra.route, extra.note].filter(Boolean).join(' · ')}</span>
              </div>
            )}
          </div>
          <div className="shrink-0 text-right">
            <div className="tnum text-sm"><b>{eur(l.spent)}</b> <span className="text-muted">/ {eur(l.budgeted)}</span></div>
            {/* A fixed line paid in full says so beside its name; "€0 left" would repeat it. */}
            {!(l.kind === 'fixed' && extra?.done && l.remaining === 0) && (
              <div className={clsx('tnum text-xs', over && l.kind !== 'saving' ? 'text-bad' : 'text-muted')}>{l.kind === 'saving' ? (l.remaining <= 0 ? 'target reached' : `${eur(l.remaining)} to go`) : over ? `${eur(-l.remaining)} over` : `${eur(l.remaining)} ${l.fund_state ? 'in fund' : 'left'}`}</div>
            )}
          </div>
        </div>
        {/* An obligation paid in full is done, not a warning; paying more still shows red. */}
        <Meter className="mt-2" value={l.spent} max={l.budgeted} pace={pace} goal={l.kind === 'saving' || (l.kind === 'fixed' && !over)} />
      </button>
      {open && (
        <div className="mt-3 space-y-2">
          {l.fund_state && (
            <div className="grid grid-cols-3 gap-2 text-xs">
              <div><div className="text-muted">Carried in</div><div className="tnum">{eur(l.fund_state.opening)}</div></div>
              <div><div className="text-muted">Added this month</div><div className="tnum">{eur(l.fund_state.contribution)}</div></div>
              <div><div className="text-muted">Since {monthLabel(l.fund_state.start_month)}</div><div className="tnum">{eur(l.fund_state.contributed)} in · {eur(l.fund_state.drawn)} out</div></div>
            </div>
          )}
          {l.year && <div className="text-xs text-muted">Year to date {eur(l.year.spent)} of {eur(l.year.budget)} · on pace for {eur(l.year.projected)}</div>}
          <div className="h-24">
            <ResponsiveContainer>
              <BarChart data={l.history} margin={{ top: 4, right: 0, bottom: 0, left: 0 }} barCategoryGap={3}>
                <XAxis dataKey="month" tickFormatter={(m) => monthLabel(m).slice(0, 3)} tick={{ fill: 'var(--chart-text)', fontSize: 10 }} tickLine={false} axisLine={false} interval={1} />
                <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length ? (
                  <TooltipBox title={monthLabel(payload[0].payload.month, true)} rows={[{ label: 'Spent', value: eurc(payload[0].payload.spent), bold: true }, { label: 'Budget', value: eurc(payload[0].payload.budget) }]} />) : null} />
                <Bar dataKey="spent" name="Spent" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={(d: { month?: string }) => d?.month && nav(txs(d.month))} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="flex flex-wrap gap-2">
            <Link className="btn-outline h-8 text-xs" to={txs(month)}><Icon name="list" size={14} />{l.period === 'yearly' ? `${month.slice(0, 4)} so far` : monthLabel(month)} transactions</Link>
            <button className="btn-outline h-8 text-xs" onClick={onEdit}><Icon name="edit" size={14} />Edit line</button>
          </div>
          <div className="text-[11px] text-muted">Tap a month's bar for its transactions.</div>
        </div>
      )}
    </div>
  )
}

function BudgetEditor({ b: init, month, onClose }: { b: Partial<Budget>; month: string; onClose: () => void }) {
  const [b, setB] = useState<Partial<Budget>>({ fund: false, period: 'monthly', categories: [], tag: '', ...init })
  const [fromNow, setFromNow] = useState(!!init.id)
  const [matchBy, setMatchBy] = useState<'category' | 'tag'>(init.tag ? 'tag' : 'category')
  const refresh = useRefresh()
  const toast = useToast()
  const cats = useCats()
  const save = useMutation({
    mutationFn: () => (b.id ? api.put(`/budgets/${b.id}`, { ...b, from_month: fromNow ? month : '' }) : api.post('/budgets', b)),
    onSuccess: () => { refresh(); toast('Saved', 'good'); onClose() },
  })
  const del = useMutation({ mutationFn: () => api.del(`/budgets/${b.id}`), onSuccess: () => { refresh(); onClose() } })
  return (
    <Sheet open onClose={onClose} title={b.id ? 'Edit budget line' : 'New budget line'} footer={<>
      {b.id && <button className="btn-danger mr-auto" onClick={() => confirm('Delete this line?') && del.mutate()}>Delete</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={save.isPending}>Save</button>
    </>}>
      <div className="space-y-4">
        <Field label="Name"><input className="input" value={b.name ?? ''} onChange={(e) => setB({ ...b, name: e.target.value })} /></Field>
        <div>
          <div className="label">Kind</div>
          <Segmented value={b.kind ?? 'spending'} onChange={(k) => setB({ ...b, kind: k })} options={[{ value: 'fixed', label: 'Fixed' }, { value: 'saving', label: 'Saving' }, { value: 'spending', label: 'Spending' }]} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label={b.period === 'yearly' ? 'Amount per year (€)' : 'Amount per month (€)'}><NumberInput value={b.amount} onChange={(v) => setB({ ...b, amount: v })} /></Field>
          <div><div className="label">Period</div><Segmented value={b.period ?? 'monthly'} onChange={(p) => setB({ ...b, period: p })} options={[{ value: 'monthly', label: 'Monthly' }, { value: 'yearly', label: 'Yearly' }]} /></div>
        </div>
        <div>
          <div className="label">Counts transactions by</div>
          <Segmented value={matchBy} onChange={(m) => { setMatchBy(m); setB({ ...b, categories: m === 'tag' ? [] : b.categories, tag: m === 'category' ? '' : b.tag }) }} options={[{ value: 'category', label: 'Category' }, { value: 'tag', label: 'Tag' }]} />
        </div>
        {matchBy === 'category' ? (
          <div className="space-y-2">
            <div className="flex flex-wrap gap-1.5">
              {(b.categories ?? []).map((c) => <button key={c} className="chip-on" onClick={() => setB({ ...b, categories: b.categories!.filter((x) => x !== c) })}>{cats.path(c)}<Icon name="x" size={12} /></button>)}
            </div>
            <CategoryPicker value="" placeholder="Add a category" onChange={(id) => !b.categories?.includes(id) && setB({ ...b, categories: [...(b.categories ?? []), id] })} />
          </div>
        ) : (
          <TagInput value={b.tag ? [b.tag] : []} onChange={(v) => setB({ ...b, tag: v[v.length - 1] ?? '' })} placeholder="e.g. trip:rome" />
        )}
        {b.kind === 'spending' && (
          <div className="space-y-2">
            <Toggle checked={!!b.fund} onChange={(v) => setB({ ...b, fund: v, start_month: v && !b.start_month ? month : b.start_month })} label="Sinking fund — unspent money carries over" />
            {b.fund && <Field label="Fund starts"><input type="month" className="input" value={b.start_month ?? ''} onChange={(e) => setB({ ...b, start_month: e.target.value })} /></Field>}
          </div>
        )}
        {b.id && <Toggle checked={fromNow} onChange={setFromNow} label={`New amount applies from ${monthLabel(month, true)} only (keeps history)`} />}
        <ErrorBox error={save.error} />
      </div>
    </Sheet>
  )
}
