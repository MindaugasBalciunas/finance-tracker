import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur, shortDate, todayISO } from '../../lib/format'
import { useCats } from '../../lib/categories'
import type { Decision, DecisionRecord, Impact } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Field, Loading, NumberInput, Segmented, useToast } from '../../components/ui'
import { CategoryPicker } from '../../components/pickers'
import { Icon } from '../../components/Icon'

type Journal = { items: Decision[]; record: DecisionRecord }

/** The decision journal: write a big purchase down, see what it would do in
 *  your own numbers, sleep on it, decide — and a couple of months after
 *  buying, say whether it was worth it. */
export function DecisionsView() {
  const { data, isLoading, error } = useQuery({ queryKey: ['decisions'], queryFn: () => api.get<Journal>('/plan/decisions') })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  const thinking = data.items.filter((x) => x.status === 'thinking')
  const done = data.items.filter((x) => x.status !== 'thinking')
  return (
    <div className="space-y-4">
      <NewDecision />
      <Card title="Sleeping on it" icon="calendar">
        {thinking.length === 0 ? <p className="text-sm text-ink2">Nothing waiting. Write down the next big purchase before you buy it.</p>
          : <ul className="divide-y divide-line">{thinking.map((x) => <Row key={x.id} x={x} />)}</ul>}
      </Card>
      {done.length > 0 && (
        <Card title="Decided" icon="check" action={<AskCFO q="What does my decision journal say about what I value?" />}>
          <ul className="divide-y divide-line">{done.map((x) => <Row key={x.id} x={x} />)}</ul>
        </Card>
      )}
      <RecordCard r={data.record} />
    </div>
  )
}

function useRefresh() {
  const qc = useQueryClient()
  // Home's "needs you" list carries decision reminders.
  return () => { qc.invalidateQueries({ queryKey: ['decisions'] }); qc.invalidateQueries({ queryKey: ['overview'] }) }
}

function NewDecision() {
  const [title, setTitle] = useState('')
  const [amount, setAmount] = useState<number | undefined>()
  const [category, setCategory] = useState('')
  const [note, setNote] = useState('')
  // Ask for the impact once typing settles, not on every keystroke.
  const [settled, setSettled] = useState<number | undefined>()
  useEffect(() => { const t = setTimeout(() => setSettled(amount), 400); return () => clearTimeout(t) }, [amount])
  const impact = useQuery({
    queryKey: ['decision-impact', settled], enabled: !!settled && settled > 0,
    queryFn: () => api.get<Impact>(`/plan/decisions/impact?amount=${settled}`), placeholderData: (p) => p,
  })
  const refresh = useRefresh()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => api.post<Decision>('/plan/decisions', { title, amount, category, note }),
    onSuccess: (d) => {
      refresh()
      toast(d.cool_until > todayISO() ? `Written down — decide after ${shortDate(d.cool_until)}` : 'Written down', 'good')
      setTitle(''); setAmount(undefined); setCategory(''); setNote('')
    },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  return (
    <Card title="Thinking of buying…" icon="bag">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="What"><input className="input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Robot vacuum" maxLength={120} /></Field>
        <Field label="Price"><NumberInput value={amount} onChange={setAmount} placeholder="€" /></Field>
        <Field label="Category"><CategoryPicker value={category} onChange={(id) => setCategory(id)} kind="expense" /></Field>
        <Field label="Why (optional)"><input className="input" value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} /></Field>
      </div>
      {impact.data && amount && amount > 0 && (
        <ul className="mt-3 space-y-1.5">
          {impact.data.lines.map((l) => (
            <li key={l.key} className="flex items-start gap-2 text-sm">
              <Icon name={l.tone === 'good' ? 'check' : 'alert'} size={15} className={clsx('mt-0.5 shrink-0', l.tone === 'good' ? 'text-good' : l.tone === 'warn' ? 'text-warn' : 'text-bad')} />
              <span>{l.text}</span>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-3 flex items-center justify-between gap-3">
        <span className="text-xs text-muted">{amount && amount >= 100 ? `Sleep on it: ${amount < 500 ? 'a weekend' : amount < 2000 ? 'a week' : 'a month'}.` : 'Under €100 needs no cooling-off.'}</span>
        <button className="btn-primary" disabled={!title.trim() || !amount || amount <= 0 || save.isPending} onClick={() => save.mutate()}>Write it down</button>
      </div>
    </Card>
  )
}

function Row({ x }: { x: Decision }) {
  const cats = useCats()
  const refresh = useRefresh()
  const toast = useToast()
  const today = todayISO()
  const put = useMutation({
    mutationFn: (p: Partial<Decision>) => api.put(`/plan/decisions/${x.id}`, { ...x, ...p }),
    onSuccess: refresh, onError: (e) => toast((e as Error).message, 'bad'),
  })
  const del = useMutation({ mutationFn: () => api.del(`/plan/decisions/${x.id}`), onSuccess: refresh, onError: (e) => toast((e as Error).message, 'bad') })
  const cooling = x.status === 'thinking' && x.cool_until > today
  const reviewDue = x.status === 'bought' && !x.worth && x.review_on && x.review_on <= today
  return (
    <li className="py-3">
      <div className="flex items-baseline justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">{x.title}</div>
          <div className="text-xs text-ink2">
            {x.category && <>{cats.path(x.category)} · </>}
            {x.status === 'thinking' ? (cooling ? `decide after ${shortDate(x.cool_until)}` : 'cooling-off over — your call')
              : x.status === 'skipped' ? `let go ${shortDate(x.decided_on)}` : `bought ${shortDate(x.decided_on)}`}
            {x.note && <> · {x.note}</>}
          </div>
        </div>
        <div className="tnum shrink-0 text-sm font-semibold">{eur(x.amount)}</div>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {x.status === 'thinking' && <>
          <button className="btn-outline h-9 px-3 text-sm" onClick={() => put.mutate({ status: 'bought' })} disabled={put.isPending}>Bought it</button>
          <button className="btn-outline h-9 px-3 text-sm" onClick={() => put.mutate({ status: 'skipped' })} disabled={put.isPending}>Let it go</button>
        </>}
        {x.status === 'bought' && (x.worth || reviewDue
          ? <div className="flex items-center gap-2">
              <span className="text-xs text-ink2">{x.worth ? 'Worth it?' : 'Was it worth it?'}</span>
              <Segmented size="sm" value={x.worth || ('' as any)} onChange={(v) => put.mutate({ worth: v as Decision['worth'] })}
                options={[{ value: 'yes', label: 'Yes' }, { value: 'meh', label: 'Meh' }, { value: 'no', label: 'No' }]} />
            </div>
          : <span className="text-xs text-muted">Ask again {shortDate(x.review_on)}</span>)}
        <button className="btn-ghost ml-auto h-9 w-9 px-0" aria-label={`Delete ${x.title}`} onClick={() => { if (confirm(`Delete “${x.title}”?`)) del.mutate() }}><Icon name="trash" size={16} /></button>
      </div>
    </li>
  )
}

function RecordCard({ r }: { r: DecisionRecord }) {
  const cats = useCats()
  if (r.bought === 0 && r.skipped === 0) return null
  const rated = r.yes + r.meh + r.no
  return (
    <Card title="What the journal shows" icon="chart">
      <div className="grid grid-cols-2 gap-3 text-sm">
        <div><div className="text-xs text-ink2">Kept by letting go</div><div className="tnum text-lg font-semibold text-good">{eur(r.skipped_sum)}</div><div className="text-xs text-muted">{r.skipped} decision{r.skipped === 1 ? '' : 's'}</div></div>
        <div><div className="text-xs text-ink2">Bought and regretted</div><div className={clsx('tnum text-lg font-semibold', r.regretted_sum > 0 && 'text-bad')}>{eur(r.regretted_sum)}</div>
          <div className="text-xs text-muted">{rated ? `${r.yes} of ${rated} rated worth it` : 'none rated yet'}</div></div>
      </div>
      {r.by_category.length > 0 && (
        <ul className="mt-3 space-y-1 text-sm">
          {r.by_category.map((c) => (
            <li key={c.category} className="flex justify-between"><span className="text-ink2">{c.category ? cats.path(c.category) : 'No category'}</span><span className="tnum">{c.worth} of {c.rated} worth it</span></li>
          ))}
        </ul>
      )}
    </Card>
  )
}
