import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur, shortDate } from '../../lib/format'
import type { NextEuro, RunStep, Runbook } from '../../lib/types'
import { AskCFO, Card, Empty, ErrorBox, Loading, Meter, NumberInput } from '../../components/ui'
import { Icon } from '../../components/Icon'

/** Payday: pay yourself first as a checklist that ticks itself off from the
 *  bank, and where any spare money is best put next. */
export function PaydayView() {
  return (
    <div className="space-y-4">
      <RunbookCard />
      <NextEuroCard />
    </div>
  )
}

const STATUS: Record<RunStep['status'], { icon: string; tone: string; label: string }> = {
  done: { icon: 'check', tone: 'text-good', label: 'Done' },
  partial: { icon: 'alert', tone: 'text-warn', label: 'Part done' },
  todo: { icon: 'send', tone: 'text-muted', label: 'To do' },
}

function RunbookCard() {
  const { data, isLoading, error } = useQuery({ queryKey: ['runbook'], queryFn: () => api.get<Runbook>('/plan/payday') })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  const head = data.payday
    ? <>Salary of <b className="tnum">{eur(data.salary)}</b> came in {shortDate(data.payday)}.</>
    : <>The salary isn't in yet{data.expected && <> — expected by {shortDate(data.expected)}</>}. These are the steps for when it lands.</>
  return (
    <Card title="Pay yourself first" icon="send" action={<AskCFO q="Walk me through what's still to move after payday, and why." />}>
      <div className="text-sm text-ink2">{head}</div>
      {data.steps.length === 0 ? (
        <div className="mt-3"><Empty title="Nothing to move" icon="check">Add saving lines to the budget (investing, pension, savings) and they show up here as payday steps.</Empty></div>
      ) : <>
        <div className="mt-3 flex items-baseline justify-between text-sm">
          <span>{data.open === 0 ? <b className="text-good">All done for this payday</b> : <><b>{data.open}</b> of {data.steps.length} steps left</>}</span>
          <span className="tnum text-ink2">{eur(data.done_total)} of {eur(data.total)}</span>
        </div>
        <Meter value={data.done_total} max={data.total} goal className="mt-1.5" />
        <ul className="mt-3 divide-y divide-line">
          {data.steps.map((s) => <Step key={s.key} s={s} />)}
        </ul>
        <p className="mt-2 text-xs text-muted">Steps tick themselves off as the bank shows each transfer — nothing here moves money.</p>
      </>}
    </Card>
  )
}

function Step({ s }: { s: RunStep }) {
  const st = STATUS[s.status]
  const route = s.from_name || s.to_name ? [s.from_name, s.to_name].filter(Boolean).join(' → ') : ''
  const body = (
    <div className="flex items-start gap-3 py-2.5">
      <span className={clsx('mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-full bg-sunken', st.tone)} aria-label={st.label}><Icon name={st.icon} size={15} /></span>
      <div className="min-w-0 flex-1">
        <div className={clsx('text-sm font-medium', s.status === 'done' && 'text-ink2')}>{s.title}</div>
        <div className="text-xs text-ink2">
          {route}{route && (s.detail || s.by) ? ' · ' : ''}{s.by ? `by ${shortDate(s.by)}` : ''}{s.by && s.detail ? ' · ' : ''}{s.detail}
        </div>
      </div>
      <div className="text-right">
        <div className="tnum text-sm font-semibold">{eur(s.amount)}</div>
        {s.status === 'partial' && <div className="tnum text-xs text-warn">{eur(s.amount - s.done)} left</div>}
      </div>
    </div>
  )
  return <li>{s.link && s.link !== '/' ? <Link to={s.link} className="block rounded-lg hover:bg-sunken/50">{body}</Link> : body}</li>
}

function NextEuroCard() {
  const [amount, setAmount] = useState<number | undefined>(500)
  const sum = amount && amount > 0 ? amount : 500
  const { data, isLoading, error } = useQuery({
    queryKey: ['next-euro', sum], queryFn: () => api.get<NextEuro>(`/plan/next-euro?amount=${sum}`), placeholderData: (p) => p,
  })
  return (
    <Card title="Where the next euros go" icon="target" action={<AskCFO q={`Where should my next €${sum} go, and why?`} />}>
      <label className="flex items-center gap-2 text-sm">
        <span className="text-ink2">If I had</span>
        <NumberInput value={amount} onChange={setAmount} className="input tnum w-28" aria-label="Amount to place, in euros" />
        <span className="text-ink2">to spare</span>
      </label>
      {isLoading && !data ? <Loading /> : error || !data ? <ErrorBox error={error} /> : <>
        <ol className="mt-3 space-y-2">
          {data.options.map((o, i) => (
            <li key={o.key} className={clsx('rounded-xl border p-3', o.amount > 0 ? 'border-accent/40 bg-accent/5' : 'border-line')}>
              <div className="flex items-baseline justify-between gap-2">
                <div className="min-w-0 text-sm font-medium"><span className="text-muted">{i + 1}.</span> {o.title}{o.to_name && <span className="text-ink2"> · {o.to_name}</span>}</div>
                <div className={clsx('tnum shrink-0 text-sm font-semibold', o.amount > 0 ? 'text-ink' : 'text-muted')}>{o.amount > 0 ? eur(o.amount) : '—'}</div>
              </div>
              <div className="mt-0.5 text-xs text-ink2">{o.why}</div>
              <div className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-muted">
                {o.key === 'buffer' ? <span>safety first</span> : o.key.startsWith('goal:') ? <span>a date to keep</span>
                  : <span className="tnum">{o.return.toFixed(1)}% a year {o.sure ? '· certain' : '· expected'}</span>}
                {o.room != null && <span className="tnum">room {eur(o.room)}</span>}
                {o.link && <Link to={o.link} className="text-accent">open</Link>}
              </div>
            </li>
          ))}
        </ol>
        {data.notes.map((n) => <p key={n} className="mt-2 text-xs text-warn">{n}</p>)}
        <p className="mt-2 text-xs text-muted">A ranking in your own numbers, not advice from a licensed adviser. Investing is scored 2 points under its expected return because it isn't certain.</p>
      </>}
    </Card>
  )
}
