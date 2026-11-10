import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { useRefresh } from '../../lib/hooks'
import { addMonths, eur, monthLabel, shortDate, thisMonth, todayISO } from '../../lib/format'
import type { GrossChange, PlanSettings as Settings, SalaryRule } from '../../lib/types'
import { Card, Field, Loading, NumberInput, Segmented, Toggle, useToast } from '../../components/ui'
import { AccountSelect, SPEND_KINDS } from '../../components/pickers'
import { Icon } from '../../components/Icon'
import { DateInput } from '../../components/DateInput'

/** Gross salary as a timeline: what you earn now, then each change from
 *  its month (a raise, a new job) — the months before a change keep the old
 *  salary. All of it is saved with the form. */
function SalaryTimeline({ v, setS }: { v: Settings; setS: (s: Settings) => void }) {
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

/** Plan → Income & goals: the income base, salary timing and FI inputs. */
export function PlanSettings() {
  const { data, isLoading } = useQuery({ queryKey: ['plan-settings'], queryFn: () => api.get<{ settings: Settings; net_from_gross: number }>('/plan/settings') })
  const [s, setS] = useState<Settings | null>(null)
  const refresh = useRefresh()
  const toast = useToast()
  if (isLoading || !data) return <Loading />
  const v = s ?? data.settings
  const save = async () => {
    try {
      await api.put('/plan/settings', v)
      toast('Saved', 'good')
      refresh()
    } catch (e) { toast((e as Error).message || 'Could not save', 'bad') }
  }
  // Salary timing: when the salary for a month arrives early the next month.
  const rules: SalaryRule[] = v.salary_rules?.length ? v.salary_rules : [{ from: '', paid_by_day: 3 }]
  const setRules = (r: SalaryRule[]) => setS({ ...v, salary_rules: r })
  const sorted = [...rules].map((r, i) => ({ ...r, i })).sort((a, b) => a.from.localeCompare(b.from))
  const num = (k: keyof Settings) => (n: number | undefined) => setS({ ...v, [k]: n ?? 0 })
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
        <div className="mt-3"><Toggle checked={!v.emergency_exclude_cash} onChange={(on) => setS({ ...v, emergency_exclude_cash: !on })} label="Count the Cash account (notes and coins) in the emergency fund" /></div>
      </Card>
      <button className="btn-primary" onClick={save}>Save</button>
      <div className="text-xs text-muted">Savings rate = (income − spending) ÷ income; mortgage principal counts as saving, interest as spending.</div>
    </div>
  )
}
