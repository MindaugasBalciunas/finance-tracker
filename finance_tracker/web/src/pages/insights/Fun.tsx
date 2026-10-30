import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { api } from '../../lib/api'
import { shortDate } from '../../lib/format'
import { usePeriod } from '../../lib/hooks'
import { PeriodPicker, pickerRange } from '../../components/PeriodPicker'
import { AskCFO, hue, Loading, useCountUp } from '../../components/ui'
import { Icon } from '../../components/Icon'

type Item = { label: string; value: string; sub?: string; n?: number; from?: string; to?: string; top?: boolean; years?: number[]; link?: string }
export type Fact = { key: string; icon: string; title: string; value: string; detail?: string; items?: Item[] | null; link?: string }
/** A "did you know?" line and the transactions behind it. */
export type FunLine = { text: string; link?: string }

const S = ['var(--s1)', 'var(--s2)', 'var(--s3)', 'var(--s4)', 'var(--s5)', 'var(--s6)', 'var(--s7)', 'var(--s8)']
const reduced = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

export const useFunFacts = (from?: string, to?: string) =>
  useQuery({ queryKey: ['fun-facts', from ?? '', to ?? ''], queryFn: () => api.get<Fact[]>('/insights/fun', { from, to }), staleTime: 3_600_000, placeholderData: (prev) => prev })

/** One-line "did you know?" stories from the facts — shared with Home —
 *  each with the transactions behind it. */
export function funLines(facts: Fact[]): FunLine[] {
  const by = Object.fromEntries(facts.map((f) => [f.key, f])) as Record<string, Fact>
  const out: FunLine[] = []
  const add = (text: string | false | undefined, link?: string) => { if (text) out.push({ text: text.trim(), link }) }
  const habit = (label: string) => by.habits?.items?.find((i) => i.label === label)
  if (by.employers?.items?.[0]) add(`${by.employers.value} paid you ${by.employers.items[0].value} — more than any other employer.`, by.employers.items[0].link)
  if (by.first_pay) add(`Your first pay cheque was ${by.first_pay.value}. ${by.first_pay.detail?.split('. ').slice(1).join('. ') ?? ''}`, by.first_pay.link)
  if (by.visits?.items?.[0]) add(`You've ordered from ${by.visits.value} ${by.visits.items[0].value.replace('×', '')} times.`, by.visits.items[0].link)
  const ff = habit('Fast-food stops')
  if (ff) add(`${ff.value} fast-food stops so far — ${ff.sub?.split(' · ')[0]} in all.`, ff.link)
  const fuel = habit('Fuel fill-ups')
  if (fuel) add(`${fuel.value} trips to the pump, ${fuel.sub?.split(' · ')[0]} of fuel.`, fuel.link)
  if (by.this_year) add(`${by.this_year.value} is this year's favourite: ${by.this_year.detail?.split(' — ')[1] ?? by.this_year.detail}`, by.this_year.link)
  if (by.weekday) add(`${by.weekday.value} is your big spending day.`)
  if (by.biggest) add(`The biggest single purchase: ${by.biggest.value} — ${by.biggest.detail?.split(' · ')[0]}.`, by.biggest.link)
  if (by.signature) add(`${by.signature.value} is your signature price — ${by.signature.detail?.replace(/^Paid exactly that /, 'paid exactly that ')}`)
  if (by.rhythm?.detail?.includes('Longest break')) add(`Longest break from spending: ${by.rhythm.detail.split('Longest break: ')[1]}`)
  if (by.new_places) add(`${by.new_places.value.replace(" you'd never paid before", '')} new places — ${by.new_places.items?.slice(0, 2).map((i) => i.label).join(' and ')} among them.`, by.new_places.items?.[0]?.link)
  if (by.vs_before) add(`${by.vs_before.value} spent than in the period before.`)
  if (by.trips?.items?.[0]) add(`${by.trips.items[0].label} was your priciest trip: ${by.trips.items[0].value}.`, by.trips.items[0].link)
  return out
}

/** A line that opens its transactions when it has them. */
export function FunLineText({ line }: { line: FunLine }) {
  if (!line.link) return <>{line.text}</>
  return <Link to={line.link} className="underline decoration-accent/40 underline-offset-2 hover:decoration-accent">{line.text}</Link>
}

/** A row or bar that opens its transactions (a plain block without a link). */
function Linked({ to, className, title, children }: { to?: string; className?: string; title?: string; children: ReactNode }) {
  if (!to) return <div className={className} title={title}>{children}</div>
  return <Link to={to} className={clsx(className, 'block transition hover:brightness-95 focus-visible:outline-2')} title={title} aria-label={title}>{children}</Link>
}

const fmt = (n: number, euro = false) => (euro ? '€' : '') + Math.round(n).toLocaleString('en-GB')

function Count({ n, euro }: { n: number; euro?: boolean }) {
  return <>{fmt(useCountUp(n, 1100), euro)}</>
}

const gradText: CSSProperties = { backgroundImage: 'linear-gradient(90deg, var(--s1), var(--s7))', WebkitBackgroundClip: 'text', backgroundClip: 'text', color: 'transparent' }

/** The stories in the whole history, drawn: a career timeline, favourite
 *  shops as bars, habits as big numbers, the week and the years as charts.
 *  Obligations (alimony, the mortgage, family support) are left out. */
export function FunView() {
  const [period, setPeriod] = usePeriod('fun', 'all')
  const { from, to } = pickerRange(period)
  const { data, isLoading } = useFunFacts(from, to)
  if (isLoading || !data) return <Loading />
  const by = Object.fromEntries(data.map((f) => [f.key, f])) as Record<string, Fact>
  const rest = data.filter((f) => f.key !== 'lifetime')
  return (
    <div className="space-y-4">
      <PeriodPicker value={period} onChange={setPeriod} />
      {by.lifetime && <Hero f={by.lifetime} lines={funLines(data)} />}
      <div className="columns-1 gap-4 sm:columns-2 xl:columns-3">
        {rest.map((f, i) => (
          <div key={f.key} className="mb-4 break-inside-avoid">
            <FactCard f={f} color={S[i % S.length]} />
          </div>
        ))}
      </div>
      <div className="flex justify-center"><AskCFO q="Look at my fun facts (get_fun_facts) and tell me the three most surprising things in my spending history, with a light touch." label="Ask CFO for more surprises" /></div>
    </div>
  )
}

function Hero({ f, lines }: { f: Fact; lines: FunLine[] }) {
  const it = (k: string) => f.items?.find((x) => x.label === k)
  const [i, setI] = useState(0)
  useEffect(() => {
    if (reduced() || lines.length < 2) return
    const h = setInterval(() => setI((x) => (x + 1) % lines.length), 6000)
    return () => clearInterval(h)
  }, [lines.length])
  return (
    <section className="relative overflow-hidden rounded-2xl border border-line p-5 sm:p-7"
      style={{ background: 'radial-gradient(120% 140% at 0% 0%, color-mix(in oklab, var(--s1) 35%, transparent), transparent 55%), radial-gradient(120% 140% at 100% 100%, color-mix(in oklab, var(--s7) 35%, transparent), transparent 55%), rgb(var(--surface))' }}>
      <div className="text-xs font-medium uppercase tracking-widest text-ink2">Your money story{it('days') ? ` · ${shortDate(it('days')!.from!)} – ${shortDate(it('days')!.to!)}` : it('years')?.from ? ` since ${it('years')!.from}` : ''}</div>
      {/* Phones: the earned total gets its own row; three big numbers don't fit side by side. */}
      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3">
        {[['earned', 'earned', true], it('days') ? ['days', 'days', false] : ['years', 'years tracked', false], ['transactions', 'transactions', false]].map(([k, label, euro]) => (
          <div key={k as string} className={clsx('min-w-0', k === 'earned' && 'col-span-2 sm:order-2 sm:col-span-1', (k === 'years' || k === 'days') && 'sm:order-1', k === 'transactions' && 'sm:order-3')}>
            <div className="text-2xl font-bold tracking-tight tnum sm:text-4xl" style={gradText}>
              {k === 'earned' && f.link ? <Link to={f.link} aria-label="Everything earned — the transactions"><Count n={it(k as string)?.n ?? 0} euro={euro as boolean} /></Link> : <Count n={it(k as string)?.n ?? 0} euro={euro as boolean} />}
            </div>
            <div className="text-xs text-muted sm:text-sm">{label as string}</div>
          </div>
        ))}
      </div>
      {lines.length > 0 && (
        <div className="mt-5 flex items-center gap-3 rounded-xl bg-raised/70 px-3 py-2.5 backdrop-blur">
          <Icon name="spark" size={18} className="shrink-0 text-accent" />
          <div className="min-w-0 flex-1 text-sm"><span className="text-muted">Did you know? </span><span key={i} className="animate-[fadein_.5s_ease]"><FunLineText line={lines[i % lines.length]} /></span></div>
          {lines.length > 1 && <button className="btn-ghost h-8 w-8 shrink-0 px-0" aria-label="Next fact" onClick={() => setI((x) => (x + 1) % lines.length)}><Icon name="chevronR" size={16} /></button>}
        </div>
      )}
    </section>
  )
}

function Shell({ f, color, children, big }: { f: Fact; color: string; children?: ReactNode; big?: boolean }) {
  return (
    <section className="card overflow-hidden p-4" style={hue(color)}>
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide" style={{ color }}>
        <span className="grid h-7 w-7 place-items-center rounded-lg" style={{ background: `color-mix(in oklab, ${color} 20%, transparent)` }}><Icon name={f.icon} size={15} /></span>
        {f.title}
      </div>
      <div className={clsx('mt-2 font-bold tracking-tight', big ? 'text-4xl' : 'text-2xl')}>
        {f.link ? <Link to={f.link} className="inline-flex items-center gap-1 hover:text-accent">{f.value}<Icon name="chevronR" size={18} className="text-muted" /></Link> : f.value}
      </div>
      {f.detail && <p className="mt-1 text-sm text-ink2">{f.detail}</p>}
      {children}
    </section>
  )
}

// Consecutive years as [first, last] runs: 2016,2017,…,2020,2024 → [2016,2020],[2024,2024].
function runs(years: number[]): [number, number][] {
  const out: [number, number][] = []
  for (const y of [...years].sort((a, b) => a - b)) {
    const last = out[out.length - 1]
    if (last && y === last[1] + 1) last[1] = y
    else out.push([y, y])
  }
  return out
}

function FactCard({ f, color }: { f: Fact; color: string }) {
  const items = f.items ?? []
  const max = Math.max(...items.map((i) => i.n ?? 0), 1)
  switch (f.key) {
    case 'employers': {
      // A timeline: each bar runs from an employer's first payment year to
      // its last, against a year axis with a line per year.
      const years = items.flatMap((i) => [Number(i.from), Number(i.to)]).filter(Boolean)
      const lo = Math.min(...years), hi = Math.max(...years) + 1
      const span = hi - lo
      const pos = (y: number) => `${((y - lo) / span) * 100}%`
      const step = span > 10 ? 3 : span > 6 ? 2 : 1
      const ticks = Array.from({ length: span }, (_, k) => lo + k)
      return (
        <Shell f={f} color={color}>
          <div className="mt-3">
            <div className="relative ml-0 h-4 text-[10px] text-muted tnum">
              {ticks.filter((y) => (y - lo) % step === 0).map((y) => (
                <span key={y} className="absolute -translate-x-1/2" style={{ left: `calc(${pos(y)} + ${50 / span}%)` }}>’{String(y).slice(2)}</span>
              ))}
            </div>
            <div className="space-y-2.5">
              {items.map((it, i) => (
                <Linked key={it.label} to={it.link} className="rounded-md">
                  <div className="flex items-baseline gap-2 text-xs">
                    <span className="min-w-0 truncate font-medium">{it.label}</span>
                    <span className="shrink-0 text-muted tnum">{runs(it.years ?? [Number(it.from)]).map(([a, b]) => (a === b ? `${a}` : `${a}–${b}`)).join(', ')}</span>
                    <span className="ml-auto shrink-0 font-semibold tnum">{it.value}</span>
                    {it.link && <Icon name="chevronR" size={12} className="shrink-0 self-center text-muted" />}
                  </div>
                  <div className="relative mt-1 h-2.5 rounded-full bg-sunken">
                    {ticks.slice(1).map((y) => <span key={y} className="absolute inset-y-0 w-px bg-line" style={{ left: pos(y) }} />)}
                    {/* One block per run of consecutive years paid — a gap stays a gap. */}
                    {runs(it.years ?? [Number(it.from)]).map(([a, b]) => (
                      <div key={a} className="absolute inset-y-0 rounded-full" title={`${it.label}: ${a === b ? a : `${a}–${b}`}`}
                        style={{ left: pos(a), width: `calc(${pos(b + 1)} - ${pos(a)})`, background: S[i % S.length] }} />
                    ))}
                  </div>
                </Linked>
              ))}
            </div>
            <div className="mt-2 text-[11px] text-muted">Each block is a year the employer paid you · ranked by total paid</div>
          </div>
        </Shell>
      )
    }
    case 'visits':
    case 'spent_most':
    case 'trips':
    case 'new_places':
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 space-y-1.5">
            {items.map((it) => (
              <Linked key={it.label} to={it.link} className="relative overflow-hidden rounded-lg px-2.5 py-1.5 text-sm">
                <div className="absolute inset-y-0 left-0 rounded-lg" style={{ width: `${((it.n ?? 0) / max) * 100}%`, background: `color-mix(in oklab, ${color} 22%, transparent)` }} />
                <div className="relative flex items-baseline gap-2">
                  <span className="min-w-0 flex-1 truncate">{f.key === 'trips' && <Icon name="plane" size={12} className="mr-1 inline text-muted" />}{it.label}</span>
                  <span className="text-xs text-muted">{it.sub}</span>
                  <span className="font-semibold tnum">{it.value}</span>
                  {it.link && <Icon name="chevronR" size={12} className="shrink-0 self-center text-muted" />}
                </div>
              </Linked>
            ))}
          </div>
        </Shell>
      )
    case 'habits':
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 grid grid-cols-2 gap-2">
            {items.map((it, i) => (
              <Linked key={it.label} to={it.link} className="rounded-xl bg-raised/60 p-2.5">
                <div className="text-2xl font-bold tracking-tight tnum" style={{ color: S[(i + 2) % S.length] }}>{it.value}</div>
                <div className="flex items-center gap-0.5 text-xs font-medium">{it.label}{it.link && <Icon name="chevronR" size={12} className="text-muted" />}</div>
                <div className="mt-0.5 text-[11px] leading-snug text-muted">{it.sub}</div>
              </Linked>
            ))}
          </div>
        </Shell>
      )
    case 'weekday':
    case 'months':
    case 'years': {
      const many = items.length > 8
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 flex h-28 items-end gap-1">
            {items.map((it) => (
              <Linked key={it.label} to={it.link} className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1" title={`${it.label}: ${it.value}`}>
                {it.top && <span className="text-[10px] font-semibold tnum" style={{ color }}>{it.value}</span>}
                <div className="w-full rounded-t-md" style={{ height: `${Math.max(4, ((it.n ?? 0) / max) * (it.top ? 80 : 96))}%`, background: it.top ? color : `color-mix(in oklab, ${color} 35%, transparent)` }} />
              </Linked>
            ))}
          </div>
          <div className="mt-1 flex gap-1">
            {items.map((it, i) => <span key={it.label} className="min-w-0 flex-1 text-center text-[10px] text-muted">{!many || i % (f.key === 'months' ? 2 : 3) === 0 || it.top ? (many && f.key === 'years' ? `’${it.label.slice(2)}` : it.label) : ''}</span>)}
          </div>
          {items.some((it) => it.link) && <div className="mt-1 text-[11px] text-muted">Tap a bar for its transactions</div>}
        </Shell>
      )
    }
    default:
      return <Shell f={f} color={color} big={['first_pay', 'biggest'].includes(f.key)} />
  }
}

/** Home's teaser: one fact at a time, linking to the page. */
export function FunTeaser() {
  const { data } = useFunFacts()
  const lines = useMemo(() => (data ? funLines(data) : []), [data])
  const [i, setI] = useState(() => Math.floor(Math.random() * 1000))
  if (!lines.length) return null
  return (
    <section className="flex items-center gap-3 overflow-hidden rounded-2xl border border-line px-4 py-3"
      style={{ background: 'linear-gradient(90deg, color-mix(in oklab, var(--s1) 18%, rgb(var(--surface))), color-mix(in oklab, var(--s7) 18%, rgb(var(--surface))))' }}>
      <Icon name="spark" size={20} className="shrink-0 text-accent" />
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-medium uppercase tracking-wide text-ink2">Did you know?</div>
        <div className="text-sm"><FunLineText line={lines[i % lines.length]} /></div>
      </div>
      <button className="btn-ghost h-8 w-8 shrink-0 px-0" aria-label="Another fact" onClick={() => setI((x) => x + 1)}><Icon name="refresh" size={15} /></button>
      <Link to="/insights/fun" className="shrink-0 text-xs font-medium text-accent">More</Link>
    </section>
  )
}
