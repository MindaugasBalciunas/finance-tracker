import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { api } from '../../lib/api'
import { eur, monthLabel } from '../../lib/format'
import { AskCFO, Card, ErrorBox, Loading } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'

type Item = { label: string; sub?: string; amount: number; tone?: 'bad' | 'good' }
type Finding = { key: string; tone: 'bad' | 'warn' | 'info' | 'good'; title: string; detail: string; amount: number; link?: string; items?: Item[] }
type Summary = {
  from: string; to: string; months: number
  flow: { income: number; fixed: number; everyday: number; one_offs: number; saved: number; invested: number }
  findings: Finding[]
}

// Each finding's look and where it sends you.
const TONE = {
  bad: { icon: 'alert', color: 'rgb(var(--bad))', label: 'Act on this' },
  warn: { icon: 'alert', color: 'rgb(var(--warn))', label: 'Worth a look' },
  info: { icon: 'chart', color: 'var(--s1)', label: 'Good to know' },
  good: { icon: 'check', color: 'rgb(var(--good))', label: 'Going well' },
} as const
const ICON: Record<string, string> = { plan_fit: 'target', one_offs: 'bolt', unbudgeted: 'list', lines_off: 'rule', saving: 'piggy', rising: 'trend', bills: 'repeat', emergency: 'umbrella' }
const LINK_LABEL: Record<string, string> = { '/plan': 'Open the plan', '/insights/spending': 'See spending', '/insights/recurring': 'See recurring', '/insights/cashflow': 'See cash flow', '/wealth': 'See wealth' }

/** Insights' first page: what a typical month does with the money, then the
 *  findings worth acting on — worst first, each with its numbers and a way
 *  to act. Everything is the app's own figures, nothing estimated by AI. */
export function SummaryView() {
  const { data, isLoading, error } = useQuery({ queryKey: ['insights-summary'], queryFn: () => api.get<Summary>('/insights/summary'), staleTime: 60_000 })
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  if (!data.months) return <Card title="Not enough history yet">Insights need at least one complete month of transactions.</Card>
  const act = data.findings.filter((f) => f.tone === 'bad' || f.tone === 'warn').length
  return (
    <div className="space-y-4">
      <div className="text-sm text-muted">
        {monthLabel(data.from, true)} – {monthLabel(data.to, true)} · {data.months} complete months
        {act > 0 && <> · <span className="text-warn">{act} {act === 1 ? 'thing' : 'things'} worth acting on</span></>}
      </div>
      <TypicalMonth f={data.flow} />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {data.findings.map((f) => <FindingCard key={f.key} f={f} />)}
      </div>
    </div>
  )
}

/** One month's income as one bar: fixed costs, everyday spending, one-offs
 *  and what was left to save — the parts add up to the income. */
function TypicalMonth({ f }: { f: Summary['flow'] }) {
  const parts = [
    { key: 'fixed', label: 'Fixed obligations', sub: 'alimony, mortgage interest', v: f.fixed, color: 'var(--s-other)' },
    { key: 'everyday', label: 'Everyday spending', sub: 'food, home, kids, transport…', v: f.everyday, color: 'var(--s1)' },
    { key: 'one_offs', label: 'One-offs of €250+', sub: 'trips, electronics, repairs', v: f.one_offs, color: 'var(--s2)' },
    { key: 'saved', label: f.saved >= 0 ? 'Left to save' : 'Spent beyond income', sub: 'income − spending', v: f.saved, color: f.saved >= 0 ? 'var(--s3)' : 'rgb(var(--bad))' },
  ]
  const total = Math.max(f.income, parts.reduce((a, p) => a + Math.max(p.v, 0), 0), 1)
  return (
    <Card icon="pie" color="var(--s1)" title="A typical month" action={<span className="text-xs text-muted">monthly average</span>}>
      <div className="flex items-baseline justify-between">
        <span className="text-sm text-ink2">Income</span>
        <span className="text-xl font-semibold tracking-tight tnum">{eur(f.income)}</span>
      </div>
      <div className="mt-2 flex h-3 w-full gap-px overflow-hidden rounded-full bg-sunken" role="img"
        aria-label={parts.map((p) => `${p.label} ${eur(p.v)}`).join(', ')}>
        {parts.filter((p) => p.v > 0).map((p) => <div key={p.key} className="h-full" title={`${p.label} ${eur(p.v)}`} style={{ width: `${(p.v / total) * 100}%`, background: p.color }} />)}
      </div>
      <div className="mt-3 space-y-1.5">
        {parts.map((p) => (
          <div key={p.key} className="flex items-baseline gap-2 text-sm">
            <span className="h-2.5 w-2.5 shrink-0 translate-y-px rounded-sm" style={{ background: p.color }} />
            <span className="min-w-0 flex-1">{p.label} <span className="hidden text-xs text-muted sm:inline">· {p.sub}</span></span>
            <span className="w-10 shrink-0 text-right text-xs text-muted tnum">{f.income > 0 ? `${Math.round((p.v / f.income) * 100)}%` : ''}</span>
            <span className={clsx('w-20 shrink-0 text-right tnum', p.v < 0 && 'text-bad')}>{eur(p.v)}</span>
          </div>
        ))}
      </div>
      {f.invested > 0 && (
        <div className="mt-3 border-t border-line pt-2 text-xs text-muted">
          Moved into investments, pension and loan principal: <b className="tnum text-ink">{eur(f.invested)}</b>/month
          {f.invested > Math.max(f.saved, 0) + 1 && <> — <span className="tnum">{eur(f.invested - Math.max(f.saved, 0))}</span> more than income left, so it came from savings or one-off money</>}.
        </div>
      )}
    </Card>
  )
}

function FindingCard({ f }: { f: Finding }) {
  const t = TONE[f.tone]
  const items = f.items ?? []
  return (
    <section className="card flex flex-col p-4" style={{ backgroundImage: `linear-gradient(135deg, color-mix(in oklab, ${t.color} 16%, transparent), transparent 60%)` }}>
      <div className="flex items-start gap-3">
        <IconTile name={ICON[f.key] ?? t.icon} color={t.color} size={32} />
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-medium uppercase tracking-wide" style={{ color: t.color }}>{t.label}</div>
          <h3 className="mt-0.5 font-semibold leading-snug">{f.title}</h3>
        </div>
      </div>
      {f.detail && <p className="mt-2 text-sm text-ink2">{f.detail}</p>}
      {items.length > 0 && (
        <div className="mt-3 divide-y divide-line border-y border-line">
          {items.map((it, i) => (
            <div key={i} className="flex items-baseline gap-2 py-1.5 text-sm">
              <span className="min-w-0 flex-1">
                <span className="block truncate">{it.label}</span>
                {it.sub && <span className="block truncate text-xs text-muted">{it.sub}</span>}
              </span>
              {/* Whole euros: these are summary figures. A sign shows only where it means over/under. */}
              <span className={clsx('shrink-0 tnum', it.tone === 'bad' ? 'text-bad' : it.tone === 'good' ? 'text-good' : '')}>
                {it.tone ? (it.amount < 0 ? '−' : '+') : ''}{eur(Math.round(Math.abs(it.amount)))}
              </span>
            </div>
          ))}
        </div>
      )}
      <div className="mt-auto flex flex-wrap items-center gap-2 pt-3">
        {f.link && (
          <Link to={f.link} className="inline-flex items-center gap-1 rounded-lg bg-accent/10 px-2.5 py-1.5 text-xs font-medium text-accent hover:bg-accent/15">
            {LINK_LABEL[f.link] ?? 'Open'}<Icon name="chevronR" size={12} />
          </Link>
        )}
        {f.tone !== 'good' && <AskCFO q={`${f.title}. ${f.detail} What should I change, concretely?`} label="Ask CFO" />}
      </div>
    </section>
  )
}
