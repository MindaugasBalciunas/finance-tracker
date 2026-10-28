import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { api } from '../../lib/api'
import { eur, shortDate } from '../../lib/format'
import { AskCFO, Card, Loading, Segmented } from '../../components/ui'
import { Icon } from '../../components/Icon'

type Item = { key: string; kind: 'due' | 'refund' | 'exempt' | 'info'; title: string; detail: string; amount: number; link?: string }
type TaxYear = { year: number; opens: string; deadline: string; items: Item[]; due: number; refund: number; pillar3_before_2025: boolean | null }

const KIND = {
  due: { label: 'to pay', cls: 'text-bad', icon: 'alert' },
  refund: { label: 'back', cls: 'text-good', icon: 'piggy' },
  exempt: { label: 'tax-free', cls: 'text-good', icon: 'check' },
  info: { label: '', cls: 'text-muted', icon: 'dots' },
} as const

/** The income declaration ahead (GPM311, due 1 May): what the ledger says
 *  it will show — estimates, each with what it is based on. VMI's
 *  pre-filled declaration is what counts. */
export function TaxView() {
  const now = new Date().getFullYear()
  const [year, setYear] = useState(String(now))
  const qc = useQueryClient()
  const { data: t, isLoading } = useQuery({ queryKey: ['tax', year], queryFn: () => api.get<TaxYear>('/tax', { year }) })
  if (isLoading || !t) return <Loading />
  const net = t.due - t.refund
  const answer = async (v: boolean) => { await api.put('/tax/settings', { pillar3_before_2025: v }); qc.invalidateQueries({ queryKey: ['tax'] }); qc.invalidateQueries({ queryKey: ['forecast'] }) }
  return (
    <div className="space-y-4">
      <Segmented value={year} onChange={setYear} options={[{ value: String(now - 1), label: String(now - 1) }, { value: String(now), label: `${now} so far` }]} />
      <section className="hero p-4">
        <div className="text-xs text-ink2">Income declaration for {t.year} · opens {shortDate(t.opens)} (VMI pre-fills it) · due by {shortDate(t.deadline)}</div>
        <div className={clsx('mt-1 text-2xl font-semibold tracking-tight tnum', net > 0 ? 'text-warn' : net < 0 ? 'text-good' : '')}>
          {net > 0 ? `About ${eur(net)} to pay` : net < 0 ? `About ${eur(-net)} back` : 'Nothing extra expected'}
        </div>
        <div className="mt-1 text-xs text-muted">{t.due > 0 && t.refund > 0 ? `${eur(t.due)} to pay, ${eur(t.refund)} back. ` : ''}Estimates from your ledger and gross salary — your pre-filled declaration is what counts.{net > 0 && ' The forecast sets it aside for May.'}</div>
      </section>
      {t.pillar3_before_2025 == null && t.items.some((i) => i.key === 'pillar3') && (
        <section className="card border-accent/40 p-4 text-sm">
          <div className="flex items-start gap-3">
            <Icon name="piggy" size={18} className="mt-0.5 shrink-0 text-accent" />
            <span>Was your 3rd-pillar (Artea) contract signed <b>before 2025</b>? Only those contracts still earn the 20% refund.</span>
          </div>
          <div className="mt-3 flex flex-wrap justify-end gap-1.5"><button className="chip-on" onClick={() => answer(true)}>Yes, before 2025</button><button className="chip hover:bg-sunken" onClick={() => answer(false)}>No, 2025 or later</button></div>
        </section>
      )}
      <Card pad={false} title="What it will show" action={<AskCFO q={`Check my ${t.year} income declaration estimate (GPM311): what will I owe or get back, and is there anything I can still do before year end?`} label="Ask CFO" />}>
        <div className="divide-y divide-line border-t border-line">
          {t.items.map((it) => {
            const k = KIND[it.kind]
            return (
              <div key={it.key} className="flex items-start gap-3 px-4 py-3">
                <Icon name={k.icon} size={16} className={clsx('mt-0.5 shrink-0', k.cls)} />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{it.title}</div>
                  <div className="text-xs text-muted">{it.detail}</div>
                  {it.link && <Link to={it.link} className="mt-1 inline-flex items-center gap-0.5 text-xs text-accent">Transactions<Icon name="chevronR" size={12} /></Link>}
                </div>
                {it.amount !== 0 && it.kind !== 'info' && (
                  <div className={clsx('shrink-0 text-right text-sm tnum', k.cls)}>{eur(Math.abs(it.amount))}<div className="text-[10px]">{k.label}</div></div>
                )}
              </div>
            )
          })}
        </div>
      </Card>
      <p className="text-xs text-muted">Rules used ({t.year}): 20% income tax, 25% on yearly income above €83,237 (from 2026); interest tax-free up to €500 a year; gains from selling investments tax-free up to €500, 15% above; dividends 15%; the 3rd-pillar 20% refund on up to €1,500 only for contracts signed before 2025.</p>
    </div>
  )
}
