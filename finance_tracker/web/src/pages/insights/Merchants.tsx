import { useState } from 'react'
import { Link } from 'react-router-dom'
import clsx from 'clsx'
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useMerchantInsights, usePeriod } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import { ledgerLink } from '../../lib/links'
import { eur, eurk, monthLabel, pct, shortDate } from '../../lib/format'
import type { MerchantReport, MerchantStat } from '../../lib/types'
import { Card, Empty, ErrorBox, Loading, Segmented, Sheet, Stat } from '../../components/ui'
import { axisProps, gridProps, ShareBar, TooltipBox } from '../../components/charts'
import { MerchantMark } from '../../components/MerchantLogo'
import { Icon } from '../../components/Icon'

// ── merchants: who was paid ─────────────────────────────────────────

const PRESETS = [{ value: 'month', label: 'This month' }, { value: 'last_month', label: 'Last month' }, { value: '3m', label: '3 months' }, { value: '6m', label: '6 months' }, { value: 'ytd', label: 'This year' }, { value: '12m', label: '12 months' }, { value: 'last_year', label: 'Last year' }, { value: 'all', label: 'All time' }]
const SHOWN = 20

/** The trend against the equal window before, as the list shows it:
 *  more spent reads bad, less good; "new" for a first visit, nothing when
 *  there is no earlier window to compare with. */
export function trendLabel(m: Pick<MerchantStat, 'previous' | 'change' | 'new'>, compare = true): { text: string; tone: 'good' | 'bad' | 'muted' } | null {
  if (m.new) return { text: 'new', tone: 'muted' }
  if (!compare || m.previous <= 0) return null
  const tone = m.change > 0.1 ? 'bad' : m.change < -0.1 ? 'good' : 'muted'
  return { text: `${m.change > 0 ? '+' : ''}${pct(m.change)}`, tone }
}

export function MerchantsView() {
  const [preset, setPreset] = usePeriod('merchants', '12m', PRESETS.map((p) => p.value))
  const [show, setShow] = useState<'all' | 'new'>('all')
  const [more, setMore] = useState(false)
  const [open, setOpen] = useState<MerchantStat | null>(null)
  const { data, isLoading, error } = useMerchantInsights(preset)
  const compare = preset !== 'all'
  return (
    <div className="space-y-4">
      <div className="no-scrollbar -mx-4 flex gap-1.5 overflow-x-auto overflow-y-hidden px-4 sm:mx-0 sm:px-0">
        {PRESETS.map((p) => <button key={p.value} className={preset === p.value ? 'chip-on' : 'chip'} onClick={() => { setPreset(p.value); setMore(false) }}>{p.label}</button>)}
      </div>
      <ErrorBox error={error} />
      {isLoading || !data ? (error ? null : <Loading />) : !data.merchants.length ? (
        <Card><Empty title="No merchants in this period" icon="bag">Spending with a merchant on it shows here — bank-synced card payments carry one.</Empty></Card>
      ) : <>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat label="Spent at merchants" value={eur(data.total)} sub={`${shortDate(data.from)} – ${shortDate(data.to)}`} icon="bag" />
          <Stat label="Places" value={data.count} sub={`about ${eur(data.total / Math.max(data.count, 1))} each`} icon="list" />
          {compare
            ? <Stat label="New places" value={data.new_count} sub={data.new_count ? `${eur(data.new_total)} spent there` : 'none'} icon="spark" />
            : <Stat label="Biggest" value={data.merchants[0].name} sub={`${eur(data.merchants[0].total)} · ${pct(data.merchants[0].share)}`} icon="spark" />}
          <Stat label="No merchant" value={eur(data.unnamed)} sub="spending without one" icon="dots" />
        </div>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <MerchantList data={data} show={show} setShow={(v) => { setShow(v); setMore(false) }} more={more} setMore={setMore} compare={compare} onOpen={setOpen} />
          <div className="space-y-4">
            <Lapsed data={data} />
          </div>
        </div>
      </>}
      {open && data && <MerchantSheet m={open} data={data} compare={compare} onClose={() => setOpen(null)} />}
    </div>
  )
}

function MerchantList({ data, show, setShow, more, setMore, compare, onOpen }: {
  data: MerchantReport; show: 'all' | 'new'; setShow: (v: 'all' | 'new') => void; more: boolean; setMore: (v: boolean) => void; compare: boolean; onOpen: (m: MerchantStat) => void
}) {
  const cats = useCats()
  const fresh = data.merchants.filter((m) => m.new)
  const rows = show === 'new' && compare ? fresh : data.merchants
  const max = Math.max(...rows.map((m) => m.total), 1)
  const shown = more ? rows : rows.slice(0, SHOWN)
  return (
    <Card pad={false} icon="bag" title="Top merchants" className="lg:col-span-2"
      action={compare && <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'all', label: 'All' }, { value: 'new', label: `New (${fresh.length})` }]} />}>
      {!rows.length ? <p className="border-t border-line px-4 py-6 text-center text-sm text-muted">No new places in this period.</p> : (
        <div className="divide-y divide-line border-t border-line">
          {shown.map((m) => {
            const tr = trendLabel(m, compare)
            return (
              <button key={m.name} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40" onClick={() => onOpen(m)}>
                <MerchantMark merchant={m.name} category={m.category} size={34} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="min-w-0 truncate text-sm font-medium">{m.name}</span>
                    <span className="tnum shrink-0 text-sm font-semibold">{eur(m.total)}</span>
                  </div>
                  <div className="mt-1 flex items-center gap-3">
                    <ShareBar value={m.total} max={max} />
                    <span className={clsx('w-12 shrink-0 text-right text-xs tnum', tr?.tone === 'bad' ? 'text-bad' : tr?.tone === 'good' ? 'text-good' : 'text-muted')}>{tr?.text ?? ''}</span>
                  </div>
                  <div className="mt-0.5 truncate text-xs text-muted">×{m.count} · {eur(m.average)} a visit · {cats.name(m.category)}</div>
                </div>
              </button>
            )
          })}
          {rows.length > SHOWN && (
            <button className="w-full px-4 py-2.5 text-center text-sm text-accent hover:bg-sunken/40" onClick={() => setMore(!more)}>
              {more ? 'Show fewer' : `Show all ${rows.length}`}
            </button>
          )}
        </div>
      )}
      {data.count > data.merchants.length && <p className="border-t border-line px-4 py-2 text-xs text-muted">The {data.merchants.length} biggest of {data.count} places.</p>}
    </Card>
  )
}

function Lapsed({ data }: { data: MerchantReport }) {
  const cats = useCats()
  return (
    <Card pad={false} icon="undo" title="Stopped going">
      {!data.lapsed.length ? <p className="border-t border-line px-4 py-3 text-sm text-ink2">Every regular from the half-year before is still being paid.</p> : <>
        <div className="divide-y divide-line border-t border-line">
          {data.lapsed.slice(0, 10).map((l) => (
            <Link key={l.name} to={ledgerLink({ merchant: l.name })} className="flex items-center gap-3 px-4 py-2 hover:bg-sunken/40">
              <MerchantMark merchant={l.name} category={l.category} size={30} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm">{l.name}</div>
                <div className="truncate text-xs text-muted">{cats.name(l.category)} · last {shortDate(l.last)}</div>
              </div>
              <span className="tnum shrink-0 text-xs text-ink2">{eur(l.monthly)}/mo before</span>
            </Link>
          ))}
        </div>
        <p className="border-t border-line px-4 py-2 text-xs text-muted">Paid in at least 3 of the 6 months before this period, nothing for two months since. Moved shop, or a cancelled bill still worth checking?</p>
      </>}
    </Card>
  )
}

function MerchantSheet({ m, data, compare, onClose }: { m: MerchantStat; data: MerchantReport; compare: boolean; onClose: () => void }) {
  const cats = useCats()
  const tr = trendLabel(m, compare)
  return (
    <Sheet open onClose={onClose} title={<span className="flex items-center gap-2"><MerchantMark merchant={m.name} category={m.category} size={28} />{m.name}</span>}
      footer={<>
        <Link to={ledgerLink({ merchant: m.name })} className="btn-outline">All time</Link>
        <Link to={ledgerLink({ merchant: m.name, from: data.from, to: data.to })} className="btn-primary">Transactions<Icon name="chevronR" size={14} /></Link>
      </>}>
      <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
        <div><div className="text-xs text-muted">Spent</div><div className="tnum text-xl font-semibold">{eur(m.total)}</div><div className="text-xs text-muted">{pct(m.share)} of merchant spend</div></div>
        <div><div className="text-xs text-muted">Visits</div><div className="tnum text-xl font-semibold">{m.count}</div><div className="tnum text-xs text-muted">{eur(m.average)} a visit</div></div>
        <div>
          <div className="text-xs text-muted">Before</div>
          <div className="tnum">{compare ? eur(m.previous) : '—'} {tr && <span className={clsx('text-xs', tr.tone === 'bad' ? 'text-bad' : tr.tone === 'good' ? 'text-good' : 'text-muted')}>{tr.text}</span>}</div>
          {compare && <div className="text-xs text-muted">{shortDate(data.prev_from)} – {shortDate(data.prev_to)}</div>}
        </div>
        <div><div className="text-xs text-muted">Usually</div><div className="truncate">{cats.name(m.category)}</div></div>
        <div><div className="text-xs text-muted">First paid</div><div className="tnum">{shortDate(m.first)}</div></div>
        <div><div className="text-xs text-muted">Last paid</div><div className="tnum">{shortDate(m.last)}</div></div>
      </div>
      <div className="mt-5 text-xs font-semibold uppercase tracking-wide text-muted">Twelve months to {monthLabel(data.to.slice(0, 7))}</div>
      <div className="mt-2 h-44">
        <ResponsiveContainer>
          <BarChart data={m.months} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="month" {...axisProps} tickFormatter={(v) => monthLabel(v).slice(0, 3)} interval={1} />
            <YAxis {...axisProps} tickFormatter={eurk} width={44} />
            <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload }) => active && payload?.length
              ? <TooltipBox title={monthLabel(payload[0].payload.month, true)} rows={[{ color: 'var(--s1)', label: 'Spent', value: eur(payload[0].payload.amount) }]} /> : null} />
            <Bar dataKey="amount" fill="var(--s1)" radius={[4, 4, 0, 0]} isAnimationActive={false}>
              {m.months.map((x) => <Cell key={x.month} fillOpacity={x.month >= data.from.slice(0, 7) ? 1 : 0.35} />)}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </Sheet>
  )
}
