import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { eur, eurk, monthLabel, pct, shortDate, todayISO } from '../../lib/format'
import { useCats } from '../../lib/categories'
import { ledgerLink, monthEnd } from '../../lib/links'
import type { OptionalReport, OptTx } from '../../lib/types'
import { Card, Empty, ErrorBox, Loading } from '../../components/ui'
import { Icon } from '../../components/Icon'
import { MerchantMark } from '../../components/MerchantLogo'
import { axisProps, gridProps, Legend, TooltipBox } from '../../components/charts'
import { CuttingCard, JudgeCard } from './Judge'

const ESSENTIAL = 'var(--s1)'
const OPTIONAL = 'var(--s7)'
// Yearly figures in whole euros: they are estimates, not receipts.
const eur0 = (v: number) => eur(Math.round(v))

/** Optional spending: where the money for things you could do without goes,
 *  and the patterns that usually mean it leaks rather than buys a better
 *  life. "Optional" is the Free money card's split — every figure opens the
 *  same payments in the Ledger. */
export function OptionalView() {
  const { data: r, isLoading, error } = useQuery({ queryKey: ['insights-optional'], queryFn: () => api.get<OptionalReport>('/insights/optional') })
  if (isLoading) return <Loading />
  if (error || !r) return <ErrorBox error={error} />
  // The twelve complete months the "last 12 months" figures cover.
  const from = `${r.months[0].month}-01`, last = r.months[r.months.length - 2]?.month
  const to = last ? monthEnd(last) : todayISO()
  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
      <div className="space-y-4">
        <Headline r={r} />
        <JudgeCard r={r} />
        <MonthsChart r={r} />
        <WhereItGoes r={r} from={from} to={to} />
      </div>
      <div className="space-y-4">
        <CuttingCard r={r} />
        <Leaks r={r} />
        <div className="px-1 text-xs text-muted">
          Optional means an expense no fixed line covers, under a category not marked essential — change which are essential in{' '}
          <Link to="/settings/categories" className="text-accent">Settings → Categories</Link>. Yearly figures are estimates at the recent pace.
        </div>
      </div>
    </div>
  )
}

function Headline({ r }: { r: OptionalReport }) {
  const vs = r.usual > 0 ? r.spent / r.usual - 1 : 0
  return (
    <Card icon="pie" color={OPTIONAL} title={`Optional · ${monthLabel(r.month, true)}`}>
      <Link to={ledgerLink({ month: r.month, need: 'optional' })} className="block">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-2xl font-semibold tracking-tight tnum">{eur(r.spent)}</span>
          <span className="text-sm text-ink2">so far{r.free > 0 && <> · {pct(r.spent / r.free)} of this month's free money</>}</span>
        </div>
      </Link>
      <div className="mt-2 grid grid-cols-2 gap-2 text-sm">
        <div className="rounded-lg border border-line px-2.5 py-1.5"><div className="text-xs text-muted">Usual month</div><div className="font-semibold tnum">{eur(r.usual)}{r.usual > 0 && r.spent > 0 && <span className={clsx('pl-1.5 text-xs font-normal', vs > 0 ? 'text-warn' : 'text-muted')}>{vs > 0 ? `${pct(vs)} over so far` : 'not reached yet'}</span>}</div></div>
        <div className="rounded-lg border border-line px-2.5 py-1.5"><div className="text-xs text-muted">Last 12 months</div><div className="font-semibold tnum">{eur0(r.year)}</div></div>
      </div>
    </Card>
  )
}

function MonthsChart({ r }: { r: OptionalReport }) {
  const nav = useNavigate()
  return (
    <Card icon="chart" color={OPTIONAL} title="Essential and optional, month by month">
      <div className="h-48">
        <ResponsiveContainer>
          <BarChart data={r.months} margin={{ top: 8, right: 4, bottom: 0, left: 0 }} barCategoryGap="18%">
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="month" {...axisProps} tickFormatter={(m) => monthLabel(m)} minTickGap={20} />
            <YAxis {...axisProps} tickFormatter={eurk} width={44} />
            <Tooltip cursor={{ fill: 'rgb(var(--sunken))' }} content={({ active, payload, label: l }) => active && payload?.length ? (
              <TooltipBox title={monthLabel(String(l), true)} rows={[...payload].reverse().map((p) => ({ color: p.fill, label: p.dataKey === 'optional' ? 'Optional' : 'Essential', value: eur(Number(p.value)) }))} />) : null} />
            <Bar dataKey="essential" stackId="s" fill={ESSENTIAL} isAnimationActive={false} cursor="pointer" onClick={(d: { month?: string }) => d.month && nav(ledgerLink({ month: d.month, need: 'essential' }))} />
            <Bar dataKey="optional" stackId="s" fill={OPTIONAL} radius={[4, 4, 0, 0]} isAnimationActive={false} cursor="pointer" onClick={(d: { month?: string }) => d.month && nav(ledgerLink({ month: d.month, need: 'optional' }))} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-2"><Legend items={[{ color: ESSENTIAL, label: 'Essential' }, { color: OPTIONAL, label: 'Optional' }]} /></div>
      <div className="mt-1 text-xs text-muted">Fixed lines and saving left out. Tap a bar for its payments.</div>
    </Card>
  )
}

function WhereItGoes({ r, from, to }: { r: OptionalReport; from: string; to: string }) {
  const cats = useCats()
  if (!r.categories.length && !r.merchants.length) return null
  return (
    <Card pad={false} icon="list" color={OPTIONAL} title="Where it goes · last 12 months">
      <div className="divide-y divide-line border-t border-line">
        {r.categories.map((c) => (
          <LinkRow key={'c' + c.key} to={ledgerLink({ category: c.key, need: 'optional', from, to })} lead={<span className="h-2 w-2 rounded-full" style={{ background: OPTIONAL }} />}
            label={cats.path(c.key)} sub={`${eur(c.usual)}/mo on average · ${eur(c.month)} this month`} value={eur0(c.year)} />
        ))}
      </div>
      {r.merchants.length > 0 && <>
        <div className="border-t border-line px-4 pb-1 pt-3 text-xs font-semibold uppercase tracking-wide text-muted">Places</div>
        <div className="divide-y divide-line">
          {r.merchants.map((m) => (
            <LinkRow key={'m' + m.key} to={ledgerLink({ merchant: m.key, need: 'optional', from, to })} lead={<MerchantMark merchant={m.key} size={28} />}
              label={m.key} sub={`${m.count} payment${m.count === 1 ? '' : 's'} · ${eur(m.month)} this month`} value={eur0(m.year)} />
          ))}
        </div>
      </>}
    </Card>
  )
}

function Leaks({ r }: { r: OptionalReport }) {
  const l = r.leaks
  const ninety = new Date(Date.now() - 90 * 86400000).toLocaleDateString('sv')
  const lastComplete = r.months[r.months.length - 2]?.month
  const cards: ReactNode[] = []
  if (l.while_over.months.length) cards.push(
    <Leak key="over" icon="alert" title="Spent after the money ran out" figure={eur0(l.while_over.year + l.while_over.this_month)} figureNote={`since ${monthLabel(r.months[0].month)}`}
      why="Optional payments after the day a month had used up its free money — money that month didn't have. Costs paid from a fund don't count here, though each month's list in the Ledger shows them too.">
      {/* Newest first; six months say enough. */}
      {[...l.while_over.months].reverse().slice(0, 6).map((m) => (
        <LinkRow key={m.month} to={ledgerLink({ need: 'optional', from: `${m.month}-${String(m.from).padStart(2, '0')}`, to: monthEnd(m.month) })}
          label={monthLabel(m.month, true)} sub={`from the ${ordinal(m.from)}`} value={eur(m.amount)} />
      ))}
      {l.while_over.months.length > 6 && <div className="px-4 py-2 text-xs text-muted">and {l.while_over.months.length - 6} earlier month{l.while_over.months.length === 7 ? '' : 's'}</div>}
      <Items items={l.while_over.items} />
    </Leak>)
  if (l.small_often.length) cards.push(
    <Leak key="small" icon="coins" title="Small and often" figure={`≈ ${eur0(l.small_often.reduce((a, s) => a + s.yearly, 0))}/yr`}
      why="Under €15, six or more times in 90 days at one place. Each one is nothing; a year of them isn't.">
      {l.small_often.map((s) => (
        <LinkRow key={s.merchant} to={ledgerLink({ merchant: s.merchant, need: 'optional', from: ninety, max: 14.99 })} lead={<MerchantMark merchant={s.merchant} size={28} />}
          label={s.merchant} sub={`${s.count}× in 90 days · ${eur(s.spent)}`} value={`≈ ${eur0(s.yearly)}/yr`} />
      ))}
    </Leak>)
  if (l.payday && l.payday.extra > 0) cards.push(
    <Leak key="payday" icon="send" title="Payday splurge" figure={`≈ ${eur0(l.payday.extra)}/yr`}
      why={`On payday and the two days after it you spend ${eur(l.payday.after)} on optional things; a usual three days is ${eur(l.payday.usual)} (last ${l.payday.months} paydays).`} />)
  if (l.creeping.length) cards.push(
    <Leak key="creep" icon="trend" title="Creeping up" figure={`+${eur0(l.creeping.reduce((a, c) => a + c.yearly, 0))}/yr`}
      why="Places you go to regularly and now spend at least half as much again as three months before.">
      {l.creeping.map((c) => (
        <LinkRow key={c.merchant} to={ledgerLink({ merchant: c.merchant, need: 'optional', from: `${r.months[r.months.length - 7]?.month}-01`, to: lastComplete ? monthEnd(lastComplete) : undefined })}
          lead={<MerchantMark merchant={c.merchant} size={28} />} label={c.merchant} sub={`${eur(c.before)} → ${eur(c.now)} a month`} value={`+${eur0(c.yearly)}/yr`} />
      ))}
    </Leak>)
  if (l.convenience) cards.push(
    <Leak key="conv" icon="utensils" title="Convenience food" figure={`≈ ${eur0(l.convenience.yearly)}/yr`}
      why={`Delivery and fast food ${eur(l.convenience.convenience)} a month against ${eur(l.convenience.groceries)} on groceries (last 3 months).`}>
      <LinkRow to={ledgerLink({ category: ['food.delivery', 'food.fast_food'], need: 'optional', from: `${r.months[r.months.length - 4]?.month}-01`, to: lastComplete ? monthEnd(lastComplete) : undefined })}
        label="See the payments" value="" />
    </Leak>)
  if (l.one_offs.items.length) cards.push(
    <Leak key="big" icon="bag" title="Big one-offs" figure={eur0(l.one_offs.year)} figureNote={`since ${monthLabel(r.months[0].month)}`}
      why="Optional payments of €250 or more that no fund or trip covered — worth a second look before the next one.">
      <Items items={l.one_offs.items} />
    </Leak>)
  return (
    <div className="space-y-4">
      <div className="px-1">
        <div className="text-sm font-semibold">Possible leaks</div>
        <div className="text-xs text-muted">Patterns that often mean money slipping away rather than buying a better life. Only you can say which is which.</div>
      </div>
      {cards.length ? cards : <Card><Empty title="No leak signals" icon="check">Nothing here looks like money slipping away.</Empty></Card>}
    </div>
  )
}

function Leak({ icon, title, figure, figureNote, why, children }: { icon: string; title: string; figure: string; figureNote?: string; why: string; children?: ReactNode }) {
  return (
    <Card pad={false} icon={icon} color="var(--hue-warn)" title={title} action={<span className="text-sm font-semibold tnum">{figure}{figureNote && <span className="pl-1 text-xs font-normal text-muted">{figureNote}</span>}</span>}>
      <div className={clsx('px-4 text-xs text-muted', children ? 'pb-2' : 'pb-4')}>{why}</div>
      {children && <div className="divide-y divide-line border-t border-line">{children}</div>}
    </Card>
  )
}

function LinkRow({ to, lead, label, sub, value }: { to: string; lead?: ReactNode; label: string; sub?: string; value: string }) {
  return (
    <Link to={to} className="flex min-h-11 items-center gap-3 px-4 py-2 hover:bg-sunken/40">
      {lead && <span className="grid w-7 shrink-0 place-items-center">{lead}</span>}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm">{label}</span>
        {sub && <span className="block truncate text-xs text-muted">{sub}</span>}
      </span>
      {value && <span className="shrink-0 tnum text-sm font-semibold">{value}</span>}
      <Icon name="chevronR" size={14} className="shrink-0 text-muted" />
    </Link>
  )
}

function Items({ items }: { items: OptTx[] }) {
  const cats = useCats()
  return <>{items.map((t) => (
    <div key={t.id} className="flex items-center gap-3 px-4 py-2">
      <MerchantMark merchant={t.merchant} category={t.category} size={28} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm">{t.merchant || cats.name(t.category)}</span>
        <span className="block truncate text-xs text-muted">{shortDate(t.date)} · {cats.path(t.category)}</span>
      </span>
      <span className="shrink-0 tnum text-sm">{eur(t.amount)}</span>
    </div>
  ))}</>
}

const ordinal = (n: number) => `${n}${n % 10 === 1 && n !== 11 ? 'st' : n % 10 === 2 && n !== 12 ? 'nd' : n % 10 === 3 && n !== 13 ? 'rd' : 'th'}`
