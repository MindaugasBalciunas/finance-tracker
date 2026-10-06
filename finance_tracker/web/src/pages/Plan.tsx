import { useState } from 'react'
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis } from 'recharts'
import { api } from '../lib/api'
import { usePlan, useRefresh } from '../lib/hooks'
import { useCats } from '../lib/categories'
import { addMonths, eur, eurc, monthLabel, shortDate, thisMonth, todayISO } from '../lib/format'
import type { Budget, PlanLine, PlanReport, TxList } from '../lib/types'
import { AskCFO, Card, Empty, ErrorBox, Field, Loading, Meter, NumberInput, PageHeader, Segmented, Sheet, Tabs, Toggle, useToast } from '../components/ui'
import { CategoryPicker, TagInput } from '../components/pickers'
import { TooltipBox } from '../components/charts'
import { Icon } from '../components/Icon'
import { TxRow, useTxEditor } from '../components/TxEditor'

export default function Plan() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.includes('/trips') ? 'trips' : loc.pathname.includes('/settings') ? 'settings' : 'month'
  return (
    <div>
      <PageHeader title="Plan" actions={<AskCFO q="Review my budget plan against the last 12 months: which lines are unrealistic, what should become a fund, and is the plan affordable on my income?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'month' ? '/plan' : `/plan/${v}`)}
        tabs={[{ value: 'month', label: 'Budget' }, { value: 'trips', label: 'Trips' }, { value: 'settings', label: 'Income & goals' }]} />
      <Routes>
        <Route path="/" element={<Month />} />
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
  if (isLoading) return <Loading />
  if (error || !r) return <ErrorBox error={error} />
  const groups: { kind: string; title: string; hint: string }[] = [
    { kind: 'fixed', title: 'Fixed obligations', hint: 'Committed every month — never counted against spending budgets' },
    { kind: 'saving', title: 'Saving & investing', hint: 'Transfers into investments, pension and debt principal' },
    { kind: 'spending', title: 'Spending', hint: 'Day-to-day choices' },
  ]
  const current = month === thisMonth()
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, -1))} aria-label="Previous month"><Icon name="chevronL" /></button>
        <div className="text-base font-semibold">{monthLabel(month, true)}</div>
        <button className="btn-ghost h-9 w-9 px-0" onClick={() => setMonth(addMonths(month, 1))} aria-label="Next month"><Icon name="chevronR" /></button>
      </div>
      <Summary r={r} current={current} />
      <YearGrid r={r} />
      {groups.map((g) => {
        const lines = r.lines.filter((l) => l.kind === g.kind)
        return (
          <Card key={g.kind} pad={false} title={<span>{g.title} <span className="font-normal text-muted">{eur(lines.reduce((a, l) => a + l.monthly_share, 0))}/mo</span></span>}
            action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setEdit({ kind: g.kind as any, period: 'monthly', categories: [], tag: '', amount: 0 })}><Icon name="plus" size={14} />Line</button>}>
            <div className="px-4 pb-2 -mt-1 text-xs text-muted">{g.hint}</div>
            {lines.length === 0 ? <div className="px-4 pb-4 text-sm text-muted">No lines yet.</div> : (
              <div className="divide-y divide-line border-t border-line">
                {lines.map((l) => <LineRow key={l.id} l={l} onEdit={() => setEdit(l)} onApply={(amount, fund) => applySuggestion.mutate({ line: l, amount, fund })} />)}
              </div>
            )}
          </Card>
        )
      })}
      {r.unbudgeted.length > 0 && (
        <Card pad={false} title="Not budgeted">
          <div className="px-4 pb-2 -mt-1 text-xs text-muted">Discretionary spending no line covers.</div>
          <div className="divide-y divide-line border-t border-line">
            {r.unbudgeted.map((u) => (
              <div key={u.category} className="flex items-center gap-3 px-4 py-2.5">
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{cats.name(u.category)}</div>
                  <div className="text-xs text-muted">{eur(u.spent)} this month · {eur(u.year_spent)} this year</div>
                </div>
                {u.suggestion && (
                  <button className="chip hover:bg-sunken" onClick={() => setEdit({ kind: 'spending', name: cats.name(u.category), categories: [u.category], amount: u.suggestion!.amount, fund: u.suggestion!.lumpy, start_month: month, period: 'monthly' })}>
                    + budget {eur(u.suggestion.amount)}{u.suggestion.lumpy ? ' fund' : ''}
                  </button>
                )}
              </div>
            ))}
          </div>
        </Card>
      )}
      {edit && <BudgetEditor b={edit} month={month} onClose={() => setEdit(null)} />}
    </div>
  )
}

function Summary({ r, current }: { r: PlanReport; current: boolean }) {
  const planned = r.fixed_planned + r.saving_planned + r.spending_planned
  const rows = [
    { label: 'Income base', value: r.income_base, sub: r.income_base_source },
    { label: 'Fixed', value: -r.fixed_planned },
    { label: 'Saving target', value: -r.saving_planned },
    { label: 'Spending budgets', value: -r.spending_planned },
  ]
  const unallocated = r.income_base - planned
  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
      <section className="card p-4 lg:col-span-2">
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <div className="shrink-0">
            <div className="text-sm text-ink2">{current ? 'Safe to spend this month' : 'Left after the plan'}</div>
            <div className={clsx('text-3xl font-semibold tracking-tight', r.safe_to_spend < 0 ? 'text-bad' : 'text-good')}>{eur(r.safe_to_spend)}</div>
          </div>
          <div className="text-xs text-muted sm:text-right">income − fixed − saving − fund set-asides − spent outside funds</div>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-4">
          {rows.map((x) => (
            <div key={x.label}>
              <div className="text-xs text-muted">{x.label}</div>
              <div className="tnum font-medium">{eur(x.value)}</div>
            </div>
          ))}
        </div>
        <div className={clsx('mt-3 text-xs', unallocated < 0 ? 'text-bad' : 'text-muted')}>
          {unallocated >= 0 ? `${eur(unallocated)} of the income base is not allocated to any line.` : `The plan allocates ${eur(-unallocated)} more than the income base.`}
        </div>
      </section>
      <section className="card p-4 text-sm">
        <div className="section-title mb-2">Actual so far</div>
        {[['Income received', r.income_actual], ['Fixed paid', r.fixed_spent], ['Saved & invested', r.saved_actual], ['Discretionary spent', r.discretionary_spent], ['…of which from funds', r.fund_spent]].map(([l, v]) => (
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
                    const c = cell(h.spent, h.budget, l.fund)
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

function LineRow({ l, onEdit, onApply }: { l: PlanLine; onEdit: () => void; onApply: (amount: number, fund?: boolean) => void }) {
  const cats = useCats()
  const [open, setOpen] = useState(false)
  const over = l.remaining < 0
  const what = l.tag ? `#${l.tag}` : l.categories.map((c) => cats.name(c)).join(', ')
  const pace = l.year ? l.year.elapsed : undefined
  return (
    <div className="px-4 py-3">
      <button className="w-full text-left" onClick={() => setOpen(!open)}>
        <div className="flex items-baseline justify-between gap-2">
          <div className="min-w-0">
            <span className="text-sm font-medium">{l.name}</span>
            {l.fund && <span className="ml-1.5 rounded-full bg-accent/10 px-1.5 py-0.5 text-[10px] font-medium text-accent">fund</span>}
            {l.period === 'yearly' && <span className="ml-1.5 rounded-full bg-sunken px-1.5 py-0.5 text-[10px] font-medium text-ink2">yearly</span>}
            <div className="truncate text-xs text-muted">{what}</div>
          </div>
          <div className="shrink-0 text-right">
            <div className="tnum text-sm"><b>{eur(l.spent)}</b> <span className="text-muted">/ {eur(l.budgeted)}</span></div>
            <div className={clsx('tnum text-xs', over && l.kind !== 'saving' ? 'text-bad' : 'text-muted')}>{l.kind === 'saving' ? (l.remaining <= 0 ? 'target reached' : `${eur(l.remaining)} to go`) : over ? `${eur(-l.remaining)} over` : `${eur(l.remaining)} ${l.fund ? 'in fund' : 'left'}`}</div>
          </div>
        </div>
        <Meter className="mt-2" value={l.spent} max={l.budgeted} pace={pace} goal={l.kind === 'saving'} />
      </button>
      {l.suggestion && (
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
          <span className="text-ink2">Last {l.suggestion.months} months suggest {eur(l.suggestion.amount)} — {l.suggestion.basis}</span>
          <button className="chip-on" onClick={() => onApply(l.suggestion!.amount)}>Apply from this month</button>
          {l.suggestion.lumpy && !l.fund && l.kind === 'spending' && <button className="chip hover:bg-sunken" onClick={() => onApply(l.suggestion!.amount, true)}>Make it a fund</button>}
        </div>
      )}
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
                <Bar dataKey="spent" name="Spent" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <button className="btn-outline h-8 text-xs" onClick={onEdit}><Icon name="edit" size={14} />Edit line</button>
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
  const { data, isLoading } = useQuery({ queryKey: ['trips'], queryFn: () => api.get<any>('/trips') })
  const cats = useCats()
  const refresh = useRefresh()
  const toast = useToast()
  const [names, setNames] = useState<Record<number, string>>({})
  const [open, setOpen] = useState<number | null>(null)
  // Rows left out of a suggestion before tagging (unticked).
  const [skipped, setSkipped] = useState<Record<number, Set<number>>>({})
  if (isLoading) return <Loading />
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
  return (
    <div className="space-y-4">
      {data?.suggestions?.length > 0 && (
        <Card title="Looks like a trip" pad={false}>
          <div className="divide-y divide-line">
            {data.suggestions.map((s: any, i: number) => {
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
        </Card>
      )}
      {!data?.trips?.length ? <Empty title="No trips yet" icon="plane">Tag travel transactions with trip:name to cost a trip as a whole.</Empty> : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {data.trips.map((t: any) => (
            <a key={t.tag} href={`#/ledger?period=all&tag=${encodeURIComponent(t.tag)}`} className="card block p-4 hover:bg-sunken/40">
              <div className="flex items-baseline justify-between">
                <div className="font-semibold capitalize">{t.name.replace(/-/g, ' ')}</div>
                <div className="tnum font-semibold">{eur(t.total)}</div>
              </div>
              <div className="text-xs text-muted">{shortDate(t.from)} – {shortDate(t.to)} · {t.count} rows{t.days <= 31 ? ` · ${eur(t.per_day)}/day` : ''}</div>
              {t.budget != null && <Meter className="mt-2" value={t.total} max={t.budget} />}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {t.by_category.slice(0, 4).map((c: any) => <span key={c.name} className="chip">{cats.name(c.name)} {eur(c.amount)}</span>)}
              </div>
            </a>
          ))}
        </div>
      )}
    </div>
  )
}

// ── settings ────────────────────────────────────────────────────────

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
              <Field label="Gross salary €/month"><NumberInput value={v.gross_salary || undefined} onChange={num('gross_salary')} /></Field>
              <Field label="Other deductions €/month"><NumberInput value={v.monthly_deductions || undefined} onChange={num('monthly_deductions')} /></Field>
              <div className="col-span-2 text-xs text-muted">Net after Lithuanian taxes ≈ {eur(data.net_from_gross)} (saved settings)</div>
            </div>
          )}
          {v.income_mode === 'manual' && <Field label="Monthly income €"><NumberInput value={v.manual_income || undefined} onChange={num('manual_income')} /></Field>}
          {(!v.income_mode || v.income_mode === 'median') && <div className="text-xs text-muted">Median of the last 12 complete months of income.</div>}
        </div>
      </Card>
      <Card title="Salary timing" action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setRules([...rules, { from: todayISO(), paid_by_day: 10 }])}><Icon name="plus" size={14} />New job</button>}>
        <div className="mb-3 text-xs text-muted">If your salary for a month arrives early the next month, count it in the month it pays for — otherwise one month looks like you saved nothing and the next like you saved everything. Add a rule when the pay schedule changes (a new job).</div>
        <div className="space-y-2">
          {sorted.map((r, n) => (
            <div key={r.i} className="rounded-xl border border-line p-3">
              <div className="flex flex-wrap items-end gap-3">
                <Field label={n === 0 && !r.from ? 'From' : 'From (payment date)'}>
                  {n === 0 && !r.from ? <div className="input flex items-center text-muted">the beginning</div>
                    : <input type="date" className="input" value={r.from} onChange={(e) => setRules(rules.map((x, j) => (j === r.i ? { ...x, from: e.target.value } : x)))} />}
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
