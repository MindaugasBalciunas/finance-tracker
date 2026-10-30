import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur } from '../../lib/format'
import type { NextEuro } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Loading, NumberInput } from '../../components/ui'

/** Where spare money is best put next, ranked in the owner's own numbers:
 *  the buffer while short, the 3rd-pillar refund while it has room, a goal
 *  about to miss its date, then the loan's rate against investing. */
export function NextEuroCard() {
  const [amount, setAmount] = useState<number | undefined>(500)
  const sum = amount && amount > 0 ? amount : 500
  const { data, isLoading, error } = useQuery({
    queryKey: ['next-euro', sum], queryFn: () => api.get<NextEuro>(`/plan/next-euro?amount=${sum}`), placeholderData: (p) => p,
  })
  return (
    <Card title="Where the next euros go" icon="target" color="var(--hue-info)" action={<AskCFO q={`Where should my next €${sum} go, and why?`} />}>
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
                  : <span className="tnum">{o.return.toFixed(o.sure ? 2 : 1)}% a year {o.sure ? '· certain' : '· expected'}</span>}
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
