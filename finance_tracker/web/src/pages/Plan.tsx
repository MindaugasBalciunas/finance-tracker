import { useState, type ReactNode } from 'react'
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { ledgerLink, monthEnd } from '../lib/links'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis } from 'recharts'
import { api } from '../lib/api'
import { useOverview, usePlan, useRefresh, useWide } from '../lib/hooks'
import { useCats } from '../lib/categories'
import { addMonths, eur, eurc, monthLabel, shortDate, thisMonth, todayISO } from '../lib/format'
import type { Budget, PlanLine, PlanReport, Runbook, Tx, TxList } from '../lib/types'
import { AskCFO, Card, CountUp, Empty, ErrorBox, Field, hue, Loading, Meter, NumberInput, PageHeader, Segmented, Sheet, Tabs, Toggle, useToast } from '../components/ui'
import { AccountSelect, CategoryPicker, SPEND_KINDS, TagInput } from '../components/pickers'
import { TooltipBox } from '../components/charts'
import { Icon } from '../components/Icon'
import { MerchantMark } from '../components/MerchantLogo'
import { TxRow, useTxEditor } from '../components/TxEditor'
import { Wishlist } from './plan/Wishlist'
import { ForecastView } from './plan/Forecast'
import { RecurringView } from './insights/Recurring'
import { DateInput } from '../components/DateInput'

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
        action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setEdit({ kind: g.kind as any, period: 'monthly', categories: [], tag: '', amount: 0 })}><Icon name="plus" size={14} />Line</button>}>
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
      {current && rebalance ? (
        <Rebalance r={r} onAddLine={(u) => setEdit({ kind: 'spending', name: cats.name(u.category), categories: [u.category], amount: u.suggestion!.amount, fund: u.suggestion!.lumpy, start_month: month, period: 'monthly' })}>
          {suggestions(true)}
        </Rebalance>
      ) : suggestions(false)}
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
  const rows = [
    { label: 'Income base', value: r.income_base, sub: r.income_base_source },
    { label: 'Fixed', value: -r.fixed_planned },
    { label: 'Saving target', value: -r.saving_planned },
    { label: 'Spending budgets', value: -r.spending_planned },
  ]
  const unallocated = r.income_base - planned
  // Over-allocated: offer to take the excess off the biggest monthly spending budget.
  const trim = unallocated < 0 ? r.lines.filter((l) => l.kind === 'spending' && l.period === 'monthly' && !l.fund && l.monthly_share > -unallocated).sort((a, b) => b.monthly_share - a.monthly_share)[0] : undefined
  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
      <section className="hero p-4 lg:col-span-2">
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <div className="shrink-0">
            <div className="text-sm text-ink2">{current ? 'Left to spend this month' : 'Left after the plan'}</div>
            <div className={clsx('text-3xl font-semibold tracking-tight', r.safe_to_spend < 0 ? 'text-bad' : heading ? 'text-warn' : 'text-good')}><CountUp value={r.safe_to_spend} format={eur} /></div>
            {heading && r.safe_to_spend >= 0 && <div className="text-xs text-warn">heading for {eur(-o!.plan.projected_left)} over by month end</div>}
          </div>
          <div className="text-xs text-muted sm:text-right">income − fixed − saving − fund set-asides − spent outside funds (net of {(r.refunds ?? 0) > 0.005 ? <span className="text-good">{eur(r.refunds!)} refunds</span> : 'refunds'}){(r.fund_overdrawn ?? 0) > 0.005 && <> − <span className="text-bad">{eur(r.fund_overdrawn!)} paid beyond what funds held</span></>}{(r.unreviewed ?? 0) > 0.005 && <> · incl. <span className="text-ink2">{eur(r.unreviewed!)} in the bank inbox, not reviewed yet</span></>}</div>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-4">
          {rows.map((x) => (
            <div key={x.label}>
              <div className="text-xs text-muted">{x.label}</div>
              <div className="tnum font-medium">{eur(x.value)}</div>
            </div>
          ))}
        </div>
        <div className={clsx('mt-3 flex flex-wrap items-center gap-2 text-xs', unallocated < 0 ? 'text-bad' : 'text-muted')}>
          <span>{unallocated >= 0 ? `${eur(unallocated)} of the income base is not allocated to any line.` : `The plan allocates ${eur(-unallocated)} more than the income base.`}</span>
          {trim && current && <button className="chip hover:bg-sunken" onClick={() => onTrim(trim, Math.floor((trim.monthly_share + unallocated) * 100) / 100)}>Take it off {trim.name} ({eur(trim.monthly_share)} → {eur(Math.floor((trim.monthly_share + unallocated) * 100) / 100)})</button>}
        </div>
      </section>
      <section className="card p-4 text-sm">
        <div className="section-title mb-2">Actual so far</div>
        {([['Income received', r.income_actual], ['Fixed paid', r.fixed_spent], ['Saved & invested', r.saved_actual], ['Discretionary spent', r.discretionary_spent], ['…of which from funds', r.fund_spent]] as [string, number][])
          .concat((r.refunds ?? 0) > 0.005 ? [['…after refunds of', r.refunds!]] : []).map(([l, v]) => (
          <div key={l as string} className="flex justify-between py-0.5"><span className="text-ink2">{l}</span><span className="tnum">{eur(v as number)}</span></div>
        ))}
      </section>
    </div>
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
            {l.fund && <span className="ml-1.5 rounded-full bg-accent/10 px-1.5 py-0.5 text-[10px] font-medium text-accent">{l.fund_state ? 'fund' : `fund from ${monthLabel(l.start_month)}`}</span>}
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
                <Bar dataKey="spent" name="Spent" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={(d: any) => d?.month && nav(txs(d.month))} />
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

// ── trips ───────────────────────────────────────────────────────────

function Trips() {
  const nav = useNavigate()
  const { data, isLoading } = useQuery({ queryKey: ['trips'], queryFn: () => api.get<any>('/trips') })
  const cats = useCats()
  const refresh = useRefresh()
  const toast = useToast()
  const [names, setNames] = useState<Record<number, string>>({})
  const [open, setOpen] = useState<number | null>(null)
  // Rows left out of a suggestion before tagging (unticked).
  const [skipped, setSkipped] = useState<Record<number, Set<number>>>({})
  const [showHistory, setShowHistory] = useState(false)
  const qc = useQueryClient()
  const setDone = async (tag: string, done: boolean) => {
    try {
      await api.put('/trips/history', { tag, done })
      toast(done ? 'Moved to history' : 'Back on the list', 'good')
      qc.invalidateQueries({ queryKey: ['trips'] })
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  if (isLoading) return <Loading />
  // Over and done with: no more dates or mapping prompts, kept apart below.
  const current = (data?.trips ?? []).filter((t: any) => !t.done)
  const past = (data?.trips ?? []).filter((t: any) => t.done)
  // Untagged travel from long ago isn't worth mapping any more either.
  const old = (s: any) => s.to < addMonths(thisMonth(), -2) + '-01'
  const suggestions = (data?.suggestions ?? []).map((s: any, i: number) => ({ s, i }))
  const recentSugg = suggestions.filter((x: any) => !old(x.s))
  const oldSugg = suggestions.filter((x: any) => old(x.s))
  const dates: Record<string, { tag: string; start: string; end: string }> = Object.fromEntries((data?.dates ?? []).map((d: any) => [d.tag, d]))
  const tag = async (i: number, ids: number[]) => {
    const name = names[i]
    if (!name) return
    const keep = ids.filter((id) => !skipped[i]?.has(id))
    if (!keep.length) return
    await api.post('/trips/tag', { name, ids: keep })
    toast(`Tagged ${keep.length} transactions`, 'good')
    setSkipped({ ...skipped, [i]: new Set() })
    setOpen(null)
    refresh()
  }
  const toggleSkip = (i: number, id: number) => {
    const s = new Set(skipped[i] ?? [])
    s.has(id) ? s.delete(id) : s.add(id)
    setSkipped({ ...skipped, [i]: s })
  }
  const suggestionList = (list: any[]) => (
          <div className="divide-y divide-line">
            {list.map(({ s, i }: any) => {
              const kept = s.tx_ids.length - (skipped[i]?.size ?? 0)
              return (
                <div key={i}>
                  <div className="flex flex-wrap items-center gap-2 px-4 py-3">
                    <button className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={() => setOpen(open === i ? null : i)} aria-expanded={open === i}>
                      <Icon name="chevronD" size={16} className={clsx('shrink-0 text-muted transition', open === i && 'rotate-180')} />
                      <span className="min-w-0">
                        <span className="block text-sm font-medium">{shortDate(s.from)} – {shortDate(s.to)} · {eur(s.total)}</span>
                        <span className="block truncate text-xs text-muted">{open === i ? `${kept} of ${s.count} selected` : `${s.count} untagged travel rows · ${s.top?.join(', ')}`}</span>
                      </span>
                    </button>
                    <input className="input h-8 w-36 text-xs" placeholder="trip name" value={names[i] ?? ''} onChange={(e) => setNames({ ...names, [i]: e.target.value })} />
                    <button className="btn-primary h-8 text-xs" onClick={() => tag(i, s.tx_ids)} disabled={!names[i] || kept === 0}>Tag{open === i ? ` ${kept}` : ''}</button>
                  </div>
                  {open === i && <SuggestionRows s={s} skipped={skipped[i] ?? new Set()} onToggle={(id) => toggleSkip(i, id)} />}
                </div>
              )
            })}
          </div>
  )
  return (
    <div className="space-y-4">
      {/* Trips ahead are saved for on the wish list, funded with everything else. */}
      <Card title="Planned trips" pad={false} action={<button className="btn-ghost h-8 px-2.5 text-xs text-accent" onClick={() => nav('/plan/wishlist?trip=1')}><Icon name="plus" size={14} />Plan a trip</button>}>
        {!data?.planned?.length ? (
          <div className="border-t border-line px-4 py-3 text-sm text-muted">No trip planned. Plan one and save for it on the wish list — what's tagged to it later counts against what you saved.</div>
        ) : (
          <div className="divide-y divide-line border-t border-line">
            {data.planned.map((g: any) => (
              <div key={g.id} className="px-4 py-3">
              <button onClick={() => nav('/plan/wishlist')} className="block w-full text-left hover:opacity-80">
                <div className="flex items-baseline justify-between gap-2 text-sm">
                  <span className="truncate font-medium"><Icon name="plane" size={14} className="mr-1.5 inline text-muted" />{g.name}{g.target_date && <span className="font-normal text-muted"> · {shortDate(g.target_date)}</span>}</span>
                  <span className="tnum">{eur(g.saved)} <span className="text-muted">of {eur(g.target)}</span></span>
                </div>
                <Meter value={g.saved} max={g.target} goal className="mt-2" />
                {g.spent > 0 && <div className="mt-1 text-xs text-muted tnum">{eur(g.spent)} already spent under {g.tag}</div>}
              </button>
              {g.tag && <TripDatesRow tag={g.tag} dates={dates[g.tag]} guess={{ start: g.target_date?.slice(0, 10) ?? '', end: g.target_date?.slice(0, 10) ?? '' }} />}
              </div>
            ))}
          </div>
        )}
      </Card>
      {recentSugg.length > 0 && <Card title="Looks like a trip" pad={false}>{suggestionList(recentSugg)}</Card>}
      {!current.length ? (past.length ? <div className="text-sm text-muted">No trip on the list — past ones are in History below.</div> : <Empty title="No trips yet" icon="plane">Tag travel transactions with trip:name to cost a trip as a whole.</Empty>) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {current.map((t: any) => (
            <div key={t.tag} className="card p-4">
              <a href={`#/ledger?period=all&tag=${encodeURIComponent(t.tag)}`} className="block hover:opacity-80">
                <div className="flex items-baseline justify-between">
                  <div className="font-semibold capitalize">{t.name.replace(/-/g, ' ')}</div>
                  <div className="tnum font-semibold">{eur(t.total)}</div>
                </div>
                <div className="text-xs text-muted">{t.count} rows{t.days <= 31 ? ` · ${eur(t.per_day)}/day` : ''}{t.booked_from && ` · booked from ${shortDate(t.booked_from)}`}</div>
              </a>
              {t.budget != null && <Meter className="mt-2" value={t.total} max={t.budget} />}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {t.by_category.slice(0, 4).map((c: any) => <span key={c.name} className="chip">{cats.name(c.name)} {eur(c.amount)}</span>)}
              </div>
              <TripDatesRow tag={t.tag} dates={dates[t.tag]} guess={{ start: t.from, end: t.to }} />
              <button className="btn-ghost mt-1 h-8 px-2 text-xs text-muted" onClick={() => setDone(t.tag, true)}><Icon name="check" size={14} />Trip's over — move to history</button>
            </div>
          ))}
        </div>
      )}
      {(past.length > 0 || oldSugg.length > 0) && (
        <Card pad={false} icon="calendar" color="var(--s-other)" title={past.length ? `History · ${past.length} ${past.length === 1 ? 'trip' : 'trips'}` : 'History'}
          action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowHistory(!showHistory)} aria-expanded={showHistory}>{showHistory ? 'Hide' : 'Show'}</button>}>
          <div className="-mt-1 px-4 pb-2 text-xs text-muted">Trips that are over — a month after they end, or moved here by hand. Their costs stay in the Ledger.</div>
          {showHistory && (
            <div className="divide-y divide-line border-t border-line">
              {past.map((t: any) => (
                <div key={t.tag} className="flex items-center gap-3 px-4 py-2.5">
                  <Link to={ledgerLink({ tag: t.tag })} className="min-w-0 flex-1 hover:text-accent">
                    <div className="truncate text-sm font-medium capitalize">{t.name.replace(/-/g, ' ')}</div>
                    <div className="text-xs text-muted">{shortDate(dates[t.tag]?.start ?? t.from)} – {shortDate(dates[t.tag]?.end ?? t.to)} · {t.count} rows</div>
                  </Link>
                  <span className="tnum text-sm font-semibold">{eur(t.total)}</span>
                  <button className="btn-ghost h-8 px-2 text-xs text-muted" onClick={() => setDone(t.tag, false)}>Bring back</button>
                </div>
              ))}
              {oldSugg.length > 0 && (
                <div>
                  <div className="px-4 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wide text-muted">Older untagged travel</div>
                  {suggestionList(oldSugg)}
                </div>
              )}
            </div>
          )}
        </Card>
      )}
    </div>
  )
}

/** A trip's real dates — set once, its spending tags itself from then on —
 *  and a sweep for what it missed: untagged spending inside the dates
 *  (ticked) and travel booked in the nine months before (to confirm). */
function TripDatesRow({ tag, dates, guess }: { tag: string; dates?: { start: string; end: string }; guess: { start: string; end: string } }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState(false)
  const [start, setStart] = useState(dates?.start ?? guess.start)
  const [end, setEnd] = useState(dates?.end ?? guess.end)
  const [sweep, setSweep] = useState(false)
  const save = async (clear = false) => {
    try {
      if (clear) await api.del('/trips/dates', { tag })
      else await api.put('/trips/dates', { tag, start, end })
      qc.invalidateQueries({ queryKey: ['trips'] }); qc.invalidateQueries({ queryKey: ['tag-suggest'] })
      toast(clear ? 'Dates cleared' : 'Dates saved — spending inside them tags itself', 'good')
      setEdit(false)
      if (!clear) setSweep(true)
    } catch (e) { toast((e as Error).message, 'bad') }
  }
  return (
    <div className="mt-3 border-t border-line pt-2 text-xs">
      {dates ? (
        <div className="flex flex-wrap items-center gap-2">
          <Icon name="calendar" size={14} className="text-accent" />
          <span className="font-medium">{shortDate(dates.start)} – {shortDate(dates.end)}</span>
          <span className="text-muted">· spending inside tags itself</span>
          <span className="ml-auto flex gap-2">
            <button className="text-accent" onClick={() => setSweep(true)}>Find untagged</button>
            <button className="text-muted" onClick={() => setEdit(true)}>Edit</button>
          </span>
        </div>
      ) : (
        <button className="flex items-center gap-1.5 text-accent" onClick={() => setEdit(true)}><Icon name="calendar" size={14} />Set the trip's dates — then its spending tags itself</button>
      )}
      {edit && (
        <Sheet open onClose={() => setEdit(false)} title={`Dates · ${tag.slice(5).replace(/-/g, ' ')}`} footer={<>
          {dates && <button className="btn-ghost text-bad" onClick={() => save(true)}>Clear</button>}
          <button className="btn-primary" disabled={!start || !end || end < start} onClick={() => save()}>Save</button>
        </>}>
          <div className="grid grid-cols-2 gap-3">
            <label><span className="label">First day</span><DateInput value={start} onChange={(v) => setStart(v)} /></label>
            <label><span className="label">Last day</span><DateInput value={end} min={start} onChange={(v) => setEnd(v)} /></label>
          </div>
          <p className="mt-3 text-sm text-muted">New and bank-synced expenses dated inside get <b>{tag}</b> automatically — bills, obligations and transfers aside. Flights and hotels booked earlier are suggested, not added.</p>
        </Sheet>
      )}
      {sweep && <TripSweep tag={tag} onClose={() => setSweep(false)} />}
    </div>
  )
}

function TripSweep({ tag, onClose }: { tag: string; onClose: () => void }) {
  const qc = useQueryClient()
  const toast = useToast()
  const { data, isLoading, error } = useQuery({ queryKey: ['trip-untagged', tag], queryFn: () => api.get<{ in_dates: Tx[]; maybe: Tx[]; bookings: Tx[] }>('/trips/untagged', { tag }) })
  const [pick, setPick] = useState<Set<number> | null>(null)
  const chosen = pick ?? new Set((data?.in_dates ?? []).map((t) => t.id!))
  const toggle = (id: number) => { const n = new Set(chosen); n.has(id) ? n.delete(id) : n.add(id); setPick(n) }
  const apply = async () => {
    await api.post('/transactions/bulk', { ids: [...chosen], add_tags: [tag] })
    qc.invalidateQueries()
    toast(`Tagged ${chosen.size} with ${tag}`, 'good')
    onClose()
  }
  const rows = (title: string, list: Tx[], hint: string) => list.length > 0 && (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted">{title} <span className="font-normal normal-case">· {hint}</span></div>
      <div className="divide-y divide-line rounded-xl border border-line">
        {list.map((t) => (
          <label key={t.id} className="flex items-center gap-3 px-3 py-2 text-sm">
            <input type="checkbox" className="h-4 w-4 accent-[rgb(var(--accent))]" checked={chosen.has(t.id!)} onChange={() => toggle(t.id!)} />
            <span className="min-w-0 flex-1"><span className="block truncate">{t.merchant || t.note || '—'}</span><span className="block text-xs text-muted">{shortDate(t.date)} · {t.category}</span></span>
            <span className="tnum">{eur(t.amount)}</span>
          </label>
        ))}
      </div>
    </div>
  )
  return (
    <Sheet open onClose={onClose} title={`Untagged · ${tag.slice(5).replace(/-/g, ' ')}`} footer={<button className="btn-primary" disabled={!chosen.size} onClick={apply}>Tag {chosen.size}</button>}>
      {isLoading ? <Loading /> : error ? <ErrorBox error={error} /> : !data?.in_dates.length && !data?.maybe.length && !data?.bookings.length ? (
        <Empty title="Nothing missed" icon="check">Every expense in the trip's dates already carries a trip tag.</Empty>
      ) : (
        <div className="space-y-4">
          {rows('In the trip\'s dates', data!.in_dates, 'places you don\'t pay at home — ticked')}
          {rows('Maybe', data!.maybe, 'in the dates too, but a place you pay at home, kids\' costs, or no merchant — tick the trip\'s')}
          {rows('Booked before', data!.bookings, 'flights and stays in the nine months before — tick the ones for this trip')}
        </div>
      )}
    </Sheet>
  )
}

// ── settings ────────────────────────────────────────────────────────

type GrossChange = { from: string; gross: number }

/** Gross salary as a timeline: what you earn now, then each change from
 *  its month (a raise, a new job) — the months before a change keep the old
 *  salary. All of it is saved with the form. */
function SalaryTimeline({ v, setS }: { v: any; setS: (s: any) => void }) {
  const changes: GrossChange[] = [...(v.gross_changes ?? [])].sort((a, b) => a.from.localeCompare(b.from))
  const set = (cs: GrossChange[]) => setS({ ...v, gross_changes: cs })
  const [from, setFrom] = useState(addMonths(thisMonth(), 1))
  const [gross, setGross] = useState<number | undefined>()
  const months = Array.from({ length: 19 }, (_, i) => addMonths(thisMonth(), i))
  return (
    <div className="col-span-2 space-y-2">
      <div className="text-sm font-medium">Gross salary €/month</div>
      <div className="divide-y divide-line rounded-xl border border-line">
        <div className="flex items-center gap-3 px-3 py-2">
          <span className="min-w-0 flex-1 text-sm">{changes.length ? <>Until {monthLabel(addMonths(changes[0].from, -1), true)}</> : 'Now'}</span>
          <NumberInput aria-label="Gross salary" className="input w-28 tnum" value={v.gross_salary || undefined} onChange={(n) => setS({ ...v, gross_salary: n ?? 0 })} />
          <span className="w-9" />
        </div>
        {changes.map((c, i) => (
          <div key={c.from} className="flex items-center gap-3 px-3 py-2">
            <span className="min-w-0 flex-1 text-sm">From {monthLabel(c.from, true)}</span>
            <NumberInput aria-label={`Gross salary from ${monthLabel(c.from, true)}`} className="input w-28 tnum" value={c.gross || undefined}
              onChange={(n) => set(changes.map((x, j) => (j === i ? { ...x, gross: n ?? 0 } : x)))} />
            <button className="btn-ghost h-9 w-9 px-0 text-muted hover:text-bad" onClick={() => set(changes.filter((_, j) => j !== i))} aria-label={`Remove the change from ${monthLabel(c.from, true)}`}><Icon name="trash" size={16} /></button>
          </div>
        ))}
        <div className="flex flex-wrap items-center gap-2 px-3 py-2">
          <span className="text-sm text-muted">New salary from</span>
          <select className="input w-40 min-w-0" aria-label="New salary from" value={from} onChange={(e) => setFrom(e.target.value)}>
            {months.map((m) => <option key={m} value={m}>{monthLabel(m, true)}</option>)}
          </select>
          <NumberInput aria-label="New gross salary" placeholder="gross €" className="input w-28 tnum" value={gross} onChange={setGross} />
          <button className="btn-outline h-10" disabled={!gross || gross <= 0}
            onClick={() => { set([...changes.filter((x) => x.from !== from), { from, gross: gross! }].sort((a, b) => a.from.localeCompare(b.from))); setGross(undefined) }}>
            <Icon name="plus" size={14} />Add
          </button>
        </div>
      </div>
      <div className="text-xs text-muted">A raise or a new job: add it from the month it starts, then Save.</div>
    </div>
  )
}

function PlanSettings() {
  const { data, isLoading } = useQuery({ queryKey: ['plan-settings'], queryFn: () => api.get<any>('/plan/settings') })
  const [s, setS] = useState<any>(null)
  const refresh = useRefresh()
  const toast = useToast()
  if (isLoading || !data) return <Loading />
  const v = s ?? data.settings
  const save = async () => {
    try {
      await api.put('/plan/settings', v)
      toast('Saved', 'good')
      refresh()
    } catch (e: any) { toast(e?.message ?? 'Could not save', 'bad') }
  }
  // Salary timing: when the salary for a month arrives early the next month.
  type Rule = { from: string; paid_by_day: number }
  const rules: Rule[] = v.salary_rules?.length ? v.salary_rules : [{ from: '', paid_by_day: 3 }]
  const setRules = (r: Rule[]) => setS({ ...v, salary_rules: r })
  const sorted = [...rules].map((r, i) => ({ ...r, i })).sort((a, b) => a.from.localeCompare(b.from))
  const num = (k: string) => (n: number | undefined) => setS({ ...v, [k]: n ?? 0 })
  return (
    <div className="max-w-xl space-y-4">
      <Card title="Income base">
        <div className="space-y-3">
          <Segmented value={v.income_mode || 'median'} onChange={(m) => setS({ ...v, income_mode: m })}
            options={[{ value: 'median', label: 'From history' }, { value: 'gross', label: 'Gross salary' }, { value: 'manual', label: 'Fixed amount' }]} />
          {v.income_mode === 'gross' && (
            <div className="grid grid-cols-2 gap-3">
              <SalaryTimeline v={v} setS={setS} />
              <Field label="Other deductions €/month"><NumberInput value={v.monthly_deductions || undefined} onChange={num('monthly_deductions')} /></Field>
              <div className="col-span-2 text-xs text-muted">This month ≈ <b className="tnum text-ink">{eur(data.net_from_gross)}</b> net after Lithuanian taxes (saved settings). Months already paid use the salary that actually arrived, so nothing here rewrites them.</div>
            </div>
          )}
          {v.income_mode === 'manual' && <Field label="Monthly income €"><NumberInput value={v.manual_income || undefined} onChange={num('manual_income')} /></Field>}
          {(!v.income_mode || v.income_mode === 'median') && <div className="text-xs text-muted">Median of the last 12 complete months of income.</div>}
        </div>
      </Card>
      <Card title="Salary timing" action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setRules([...rules, { from: todayISO(), paid_by_day: 10 }])}><Icon name="plus" size={14} />New job</button>}>
        <div className="mb-3 text-xs text-muted">If your salary for a month arrives early the next month, count it in the month it pays for — otherwise one month looks like you saved nothing and the next like you saved everything. Add a rule when the pay schedule changes (a new job).</div>
        <Field label="Paid into" hint="Where salary lands — Cash until payday counts it there. Empty = the account the last salary went to.">
          <AccountSelect value={v.salary_account ?? ''} onChange={(id) => setS({ ...v, salary_account: id })} kinds={SPEND_KINDS} placeholder="Automatic (last salary's account)" />
        </Field>
        <div className="mt-3 space-y-2">
          {sorted.map((r, n) => (
            <div key={r.i} className="rounded-xl border border-line p-3">
              <div className="flex flex-wrap items-end gap-3">
                <Field label={n === 0 && !r.from ? 'From' : 'From (payment date)'}>
                  {n === 0 && !r.from ? <div className="input flex items-center text-muted">the beginning</div>
                    : <DateInput value={r.from} onChange={(v) => setRules(rules.map((x, j) => (j === r.i ? { ...x, from: v } : x)))} />}
                </Field>
                <Field label="Counts for last month if paid by day">
                  <NumberInput integer className="input w-24 tnum" value={r.paid_by_day} onChange={(d) => setRules(rules.map((x, j) => (j === r.i ? { ...x, paid_by_day: Math.max(0, Math.min(15, d ?? 0)) } : x)))} />
                </Field>
                {rules.length > 1 && <button className="btn-ghost h-10 w-10 px-0 text-muted hover:text-bad" onClick={() => setRules(rules.filter((_, j) => j !== r.i))} aria-label="Remove rule"><Icon name="trash" size={16} /></button>}
              </div>
              <div className="mt-2 text-xs text-ink2">
                {r.paid_by_day > 0
                  ? <>A salary paid on the 1st–{r.paid_by_day}{r.paid_by_day === 1 ? 'st' : r.paid_by_day === 2 ? 'nd' : r.paid_by_day === 3 ? 'rd' : 'th'} counts for the previous month{r.from ? ` (payments from ${shortDate(r.from)})` : ''}.</>
                  : <>Salary counts in the month it arrives{r.from ? ` (from ${shortDate(r.from)})` : ''}.</>}
              </div>
            </div>
          ))}
        </div>
      </Card>
      <Card title="Financial independence">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Birth year"><NumberInput value={v.birth_year || undefined} onChange={num('birth_year')} /></Field>
          <Field label="Target age"><NumberInput value={v.target_age || undefined} onChange={num('target_age')} /></Field>
          <Field label="Spending in FI €/month" hint="Empty = last 12 months"><NumberInput value={v.fi_monthly_spend || undefined} onChange={num('fi_monthly_spend')} /></Field>
          <Field label="Withdrawal rate %"><NumberInput value={v.withdrawal_rate || undefined} onChange={num('withdrawal_rate')} /></Field>
          <Field label="Real return % p.a."><NumberInput value={v.expected_return} onChange={num('expected_return')} /></Field>
          <Field label="Emergency fund (months)"><NumberInput value={v.emergency_months || undefined} onChange={num('emergency_months')} /></Field>
        </div>
      </Card>
      <button className="btn-primary" onClick={save}>Save</button>
      <div className="text-xs text-muted">Savings rate = (income − spending) ÷ income; mortgage principal counts as saving, interest as spending.</div>
    </div>
  )
}

/** The transactions a trip suggestion would tag, each with a tick box. */
function SuggestionRows({ s, skipped, onToggle }: { s: any; skipped: Set<number>; onToggle: (id: number) => void }) {
  const ids = new Set<number>(s.tx_ids)
  const { data, isLoading } = useQuery({
    queryKey: ['trip-suggestion', s.from, s.to],
    queryFn: () => api.get<TxList>('/transactions', { from: s.from, to: s.to, limit: 500, sort: '+date' }),
  })
  const editor = useTxEditor()
  if (isLoading) return <div className="px-4 pb-3"><Loading /></div>
  const rows = (data?.items ?? []).filter((t) => ids.has(t.id))
  return (
    <div className="border-t border-line bg-sunken/30">
      <div className="divide-y divide-line">
        {rows.map((t) => <TxRow key={t.id} t={t} showDate selected={!skipped.has(t.id)} onSelect={() => onToggle(t.id)} onClick={() => editor.open(t)} />)}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2 text-xs text-muted">
        <span>Untick anything that wasn't part of the trip; tap a row to edit it.</span>
        <a className="text-accent" href={`#/ledger?from=${s.from}&to=${s.to}`}>Everything in these dates in the Ledger →</a>
      </div>
    </div>
  )
}
