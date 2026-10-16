import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { api } from '../../lib/api'
import { AskCFO, Loading } from '../../components/ui'
import { Icon } from '../../components/Icon'

type Item = { label: string; value: string; sub?: string; n?: number; from?: string; to?: string; top?: boolean }
export type Fact = { key: string; icon: string; title: string; value: string; detail?: string; items?: Item[] | null }

const S = ['var(--s1)', 'var(--s2)', 'var(--s3)', 'var(--s4)', 'var(--s5)', 'var(--s6)', 'var(--s7)', 'var(--s8)']
const reduced = () => typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

export const useFunFacts = () => useQuery({ queryKey: ['fun-facts'], queryFn: () => api.get<Fact[]>('/insights/fun'), staleTime: 3_600_000 })

/** One-line "did you know?" stories from the facts — shared with Home. */
export function funLines(facts: Fact[]): string[] {
  const by = Object.fromEntries(facts.map((f) => [f.key, f])) as Record<string, Fact>
  const out: string[] = []
  const habit = (label: string) => by.habits?.items?.find((i) => i.label === label)
  if (by.employers?.items?.[0]) out.push(`${by.employers.value} paid you ${by.employers.items[0].value} — more than any other employer.`)
  if (by.first_pay) out.push(`Your first pay cheque was ${by.first_pay.value}. ${by.first_pay.detail?.split('. ').slice(1).join('. ') ?? ''}`.trim())
  if (by.visits?.items?.[0]) out.push(`You've ordered from ${by.visits.value} ${by.visits.items[0].value.replace('×', '')} times.`)
  const ff = habit('Fast-food stops')
  if (ff) out.push(`${ff.value} fast-food stops so far — ${ff.sub?.split(' · ')[0]} in all.`)
  const fuel = habit('Fuel fill-ups')
  if (fuel) out.push(`${fuel.value} trips to the pump, ${fuel.sub?.split(' · ')[0]} of fuel.`)
  if (by.this_year) out.push(`${by.this_year.value} is this year's favourite: ${by.this_year.detail?.split(' — ')[1] ?? by.this_year.detail}`)
  if (by.weekday) out.push(`${by.weekday.value} is your big spending day.`)
  if (by.biggest) out.push(`The biggest single purchase: ${by.biggest.value} — ${by.biggest.detail?.split(' · ')[0]}.`)
  return out.filter(Boolean)
}

function useCountUp(target: number, ms = 1100) {
  const [v, setV] = useState(reduced() ? target : 0)
  useEffect(() => {
    if (reduced()) { setV(target); return }
    let raf = 0
    const t0 = performance.now()
    const tick = (t: number) => {
      const p = Math.min(1, (t - t0) / ms)
      setV(target * (1 - Math.pow(1 - p, 3)))
      if (p < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [target, ms])
  return v
}

const fmt = (n: number, euro = false) => (euro ? '€' : '') + Math.round(n).toLocaleString('en-GB')

function Count({ n, euro }: { n: number; euro?: boolean }) {
  return <>{fmt(useCountUp(n), euro)}</>
}

const glow = (c: string): CSSProperties => ({ background: `linear-gradient(135deg, color-mix(in oklab, ${c} 22%, rgb(var(--surface))), rgb(var(--surface)) 70%)` })
const gradText: CSSProperties = { backgroundImage: 'linear-gradient(90deg, var(--s1), var(--s7))', WebkitBackgroundClip: 'text', backgroundClip: 'text', color: 'transparent' }

/** The stories in the whole history, drawn: a career timeline, favourite
 *  shops as bars, habits as big numbers, the week and the years as charts.
 *  Obligations (alimony, the mortgage, family support) are left out. */
export function FunView() {
  const { data, isLoading } = useFunFacts()
  if (isLoading || !data) return <Loading />
  const by = Object.fromEntries(data.map((f) => [f.key, f])) as Record<string, Fact>
  const rest = data.filter((f) => f.key !== 'lifetime')
  return (
    <div className="space-y-4">
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

function Hero({ f, lines }: { f: Fact; lines: string[] }) {
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
      <div className="text-xs font-medium uppercase tracking-widest text-ink2">Your money story{it('years')?.from ? ` since ${it('years')!.from}` : ''}</div>
      {/* Phones: the earned total gets its own row; three big numbers don't fit side by side. */}
      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3">
        {[['earned', 'earned', true], ['years', 'years tracked', false], ['transactions', 'transactions', false]].map(([k, label, euro]) => (
          <div key={k as string} className={clsx('min-w-0', k === 'earned' && 'col-span-2 sm:order-2 sm:col-span-1', k === 'years' && 'sm:order-1', k === 'transactions' && 'sm:order-3')}>
            <div className="text-2xl font-bold tracking-tight tnum sm:text-4xl" style={gradText}><Count n={it(k as string)?.n ?? 0} euro={euro as boolean} /></div>
            <div className="text-xs text-muted sm:text-sm">{label as string}</div>
          </div>
        ))}
      </div>
      {lines.length > 0 && (
        <div className="mt-5 flex items-center gap-3 rounded-xl bg-raised/70 px-3 py-2.5 backdrop-blur">
          <Icon name="spark" size={18} className="shrink-0 text-accent" />
          <div className="min-w-0 flex-1 text-sm"><span className="text-muted">Did you know? </span><span key={i} className="animate-[fadein_.5s_ease]">{lines[i]}</span></div>
          {lines.length > 1 && <button className="btn-ghost h-8 w-8 shrink-0 px-0" aria-label="Next fact" onClick={() => setI((x) => (x + 1) % lines.length)}><Icon name="chevronR" size={16} /></button>}
        </div>
      )}
    </section>
  )
}

function Shell({ f, color, children, big }: { f: Fact; color: string; children?: ReactNode; big?: boolean }) {
  return (
    <section className="overflow-hidden rounded-2xl border border-line p-4" style={glow(color)}>
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide" style={{ color }}>
        <span className="grid h-7 w-7 place-items-center rounded-lg" style={{ background: `color-mix(in oklab, ${color} 20%, transparent)` }}><Icon name={f.icon} size={15} /></span>
        {f.title}
      </div>
      <div className={clsx('mt-2 font-bold tracking-tight', big ? 'text-4xl' : 'text-2xl')}>{f.value}</div>
      {f.detail && <p className="mt-1 text-sm text-ink2">{f.detail}</p>}
      {children}
    </section>
  )
}

function FactCard({ f, color }: { f: Fact; color: string }) {
  const items = f.items ?? []
  const max = Math.max(...items.map((i) => i.n ?? 0), 1)
  switch (f.key) {
    case 'employers': {
      const years = items.flatMap((i) => [Number(i.from), Number(i.to)]).filter(Boolean)
      const lo = Math.min(...years), hi = Math.max(...years) + 1
      const pos = (y: number) => `${((y - lo) / (hi - lo)) * 100}%`
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 space-y-2">
            {items.map((it, i) => (
              <div key={it.label}>
                <div className="flex items-baseline justify-between text-xs"><span className="truncate font-medium">{it.label}</span><span className="tnum text-ink2">{it.value}</span></div>
                <div className="relative mt-1 h-2.5 rounded-full bg-sunken">
                  <div className="absolute inset-y-0 rounded-full" title={it.sub}
                    style={{ left: pos(Number(it.from)), width: `calc(${pos(Number(it.to) + 1)} - ${pos(Number(it.from))})`, background: S[i % S.length], opacity: 0.35 + 0.65 * ((it.n ?? 0) / max) }} />
                </div>
              </div>
            ))}
            <div className="flex justify-between text-[10px] text-muted tnum"><span>{lo}</span><span>{hi - 1}</span></div>
          </div>
        </Shell>
      )
    }
    case 'visits':
    case 'spent_most':
    case 'trips':
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 space-y-1.5">
            {items.map((it) => (
              <div key={it.label} className="relative overflow-hidden rounded-lg px-2.5 py-1.5 text-sm">
                <div className="absolute inset-y-0 left-0 rounded-lg" style={{ width: `${((it.n ?? 0) / max) * 100}%`, background: `color-mix(in oklab, ${color} 22%, transparent)` }} />
                <div className="relative flex items-baseline gap-2">
                  <span className="min-w-0 flex-1 truncate">{f.key === 'trips' && <Icon name="plane" size={12} className="mr-1 inline text-muted" />}{it.label}</span>
                  <span className="text-xs text-muted">{it.sub}</span>
                  <span className="font-semibold tnum">{it.value}</span>
                </div>
              </div>
            ))}
          </div>
        </Shell>
      )
    case 'habits':
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 grid grid-cols-2 gap-2">
            {items.map((it, i) => (
              <div key={it.label} className="rounded-xl bg-raised/60 p-2.5">
                <div className="text-2xl font-bold tracking-tight tnum" style={{ color: S[(i + 2) % S.length] }}>{it.value}</div>
                <div className="text-xs font-medium">{it.label}</div>
                <div className="mt-0.5 text-[11px] leading-snug text-muted">{it.sub}</div>
              </div>
            ))}
          </div>
        </Shell>
      )
    case 'weekday':
    case 'years': {
      const many = items.length > 8
      return (
        <Shell f={f} color={color}>
          <div className="mt-3 flex h-28 items-end gap-1">
            {items.map((it) => (
              <div key={it.label} className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1" title={`${it.label}: ${it.value}`}>
                {it.top && <span className="text-[10px] font-semibold tnum" style={{ color }}>{it.value}</span>}
                <div className="w-full rounded-t-md" style={{ height: `${Math.max(4, ((it.n ?? 0) / max) * (it.top ? 80 : 96))}%`, background: it.top ? color : `color-mix(in oklab, ${color} 35%, transparent)` }} />
              </div>
            ))}
          </div>
          <div className="mt-1 flex gap-1">
            {items.map((it, i) => <span key={it.label} className="min-w-0 flex-1 text-center text-[10px] text-muted">{!many || i % 3 === 0 || it.top ? (many ? `’${it.label.slice(2)}` : it.label) : ''}</span>)}
          </div>
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
        <div className="text-sm">{lines[i % lines.length]}</div>
      </div>
      <button className="btn-ghost h-8 w-8 shrink-0 px-0" aria-label="Another fact" onClick={() => setI((x) => x + 1)}><Icon name="refresh" size={15} /></button>
      <Link to="/insights/fun" className="shrink-0 text-xs font-medium text-accent">More</Link>
    </section>
  )
}
