import { useEffect, useMemo, useState } from 'react'
import { Route, Routes, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../lib/api'
import { useAccounts, useNetWorthHistory, usePeriod, usePrefs, useRefresh } from '../lib/hooks'
import { GROUPS, LIQUID_GROUPS } from '../lib/categories'
import { eur, eurc, eurk, pct, shortDate, todayISO } from '../lib/format'
import type { Account } from '../lib/types'
import { AskCFO, Card, Delta, Empty, ErrorBox, Field, Loading, PageHeader, Segmented, Sheet, Tabs, Toggle, useToast } from '../components/ui'
import { axisProps, Donut, foldSlices, gridProps, Legend, TooltipBox, type Slice } from '../components/charts'
import { Icon } from '../components/Icon'
import { AccountSelect } from '../components/pickers'

export default function Wealth() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'overview'
  return (
    <div>
      <PageHeader title="Wealth" actions={<AskCFO q="Review my balance sheet: allocation across cash, investments, pension, crypto and property, against my framework. What should I change?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'overview' ? '/wealth' : `/wealth/${v}`)}
        tabs={[{ value: 'overview', label: 'Net worth' }, { value: 'investments', label: 'Investments' }, { value: 'loans', label: 'Loans' }]} />
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/investments" element={<Investments />} />
        <Route path="/loans" element={<Loans />} />
      </Routes>
    </div>
  )
}

const RANGES = [{ value: '3m', label: '3M' }, { value: '6m', label: '6M' }, { value: 'ytd', label: 'YTD' }, { value: '1y', label: '1Y' }, { value: '3y', label: '3Y' }, { value: '5y', label: '5Y' }, { value: 'all', label: 'All' }]
const SHORT = ['3m', '6m', 'ytd']
function rangeFrom(r: string) {
  if (r === 'all') return ''
  const d = new Date()
  if (r === 'ytd') return `${d.getFullYear() - 1}-12-31`
  if (r.endsWith('m')) d.setMonth(d.getMonth() - Number(r.slice(0, -1)))
  else d.setFullYear(d.getFullYear() - Number(r.slice(0, -1)))
  return d.toISOString().slice(0, 10)
}
const rangeLabel = (r: string) => (r === 'all' ? 'since records began' : r === 'ytd' ? 'this year' : `over ${r.toUpperCase()}`)
const rangeTick = (r: string) => (d: string) => (r === '5y' || r === 'all' ? d.slice(0, 4) : new Date(d).toLocaleDateString('en-GB', SHORT.includes(r) ? { day: 'numeric', month: 'short' } : { month: 'short', year: '2-digit' }))

function Overview() {
  const [range, setRange] = usePeriod('wealth', '3y', RANGES.map((r) => r.value))
  const { prefs, set: setPrefs } = usePrefs()
  const liquidOnly = prefs.liquid_only
  const setLiquidOnly = (v: boolean) => setPrefs({ liquid_only: v })
  const { data: hist, isLoading } = useNetWorthHistory(rangeFrom(range), SHORT.includes(range) ? 'week' : 'month')
  const { data: accounts } = useAccounts()
  const [sp, setSp] = useSearchParams()
  const [updateOpen, setUpdateOpen] = useState(sp.get('update') === '1')
  const [acct, setAcct] = useState<Account | null>(null)
  const [newAcct, setNewAcct] = useState(false)
  useEffect(() => {
    if (sp.get('update') === '1') { setUpdateOpen(true); sp.delete('update'); setSp(sp, { replace: true }) }
  }, [sp, setSp])

  const groups = liquidOnly ? GROUPS.filter((g) => LIQUID_GROUPS.includes(g.id)) : GROUPS
  const data = useMemo(() => (hist ?? []).map((h) => {
    const row: any = { date: h.date, net: liquidOnly ? h.liquid : h.net_worth }
    for (const g of groups) row[g.id] = h.by_group[g.id] ?? 0
    return row
  }), [hist, groups, liquidOnly])
  const last = hist?.[hist.length - 1]
  const first = hist?.[0]
  const head = last ? (liquidOnly ? last.liquid : last.net_worth) : 0
  const change = last && first ? head - (liquidOnly ? first.liquid : first.net_worth) : 0

  const byGroup = useMemo(() => {
    const m: Record<string, Account[]> = {}
    for (const a of accounts ?? []) {
      if (a.archived) continue
      ;(m[a.group] ||= []).push(a)
    }
    return m
  }, [accounts])

  return (
    <div className="space-y-4">
      <section className="card p-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-sm text-ink2">{liquidOnly ? 'Liquid assets' : 'Net worth'}</div>
            <div className="text-3xl font-semibold tracking-tight">{eur(head)}</div>
            <div className="text-sm text-muted">{rangeLabel(range)} <Delta value={change} /></div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Toggle checked={liquidOnly} onChange={setLiquidOnly} label="Liquid only" />
            <Segmented value={range} onChange={setRange} options={RANGES} size="sm" />
          </div>
        </div>
        <div className="mt-4 h-64 sm:h-80">
          {isLoading ? <Loading /> : (
            <ResponsiveContainer>
              <ComposedChart data={data} stackOffset="sign" margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} />
                <ReferenceLine y={0} stroke="var(--chart-axis)" />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={shortDate(label)} rows={[...groups.filter((g) => payload[0].payload[g.id]).map((g) => ({ color: `var(--s${g.slot})`, label: g.name, value: eur(payload[0].payload[g.id]) })),
                    { label: liquidOnly ? 'Liquid' : 'Net worth', value: eur(payload[0].payload.net), bold: true }]} />) : null} />
                {groups.map((g) => (
                  <Area key={g.id} type="monotone" dataKey={g.id} name={g.name} stackId="1" stroke="var(--chart-surface)" strokeWidth={1.5}
                    fill={`var(--s${g.slot})`} fillOpacity={0.85} isAnimationActive={false} />
                ))}
                <Line type="monotone" dataKey="net" name={liquidOnly ? 'Liquid' : 'Net worth'} stroke="rgb(var(--ink))" strokeWidth={2} dot={false} isAnimationActive={false} />
              </ComposedChart>
            </ResponsiveContainer>
          )}
        </div>
        <div className="mt-3"><Legend items={[...groups.map((g) => ({ color: `var(--s${g.slot})`, label: g.name, value: last ? eurk(last.by_group[g.id] ?? 0) : undefined })),
          { color: 'rgb(var(--ink))', label: liquidOnly ? 'Liquid (line)' : 'Net worth (line)', value: eurk(head) }]} /></div>
        {last && <Allocation byGroup={last.by_group} liquidOnly={liquidOnly} />}
      </section>
      <WhereMoneyIs from={rangeFrom(range)} range={range} liquidOnly={liquidOnly} accounts={accounts ?? []} />
      <Movement from={rangeFrom(range) || (hist?.[0]?.date ?? '')} label={rangeLabel(range)} />

      <div className="flex items-center justify-between gap-2">
        <h2 className="text-base font-semibold">Accounts</h2>
        <div className="flex gap-2">
          <button className="btn-outline h-9 w-9 px-0 sm:w-auto sm:px-3.5" onClick={() => setNewAcct(true)} aria-label="New account"><Icon name="plus" size={16} /><span className="hidden sm:inline">Account</span></button>
          <button className="btn-primary h-9" onClick={() => setUpdateOpen(true)}><Icon name="refresh" size={16} />Update balances</button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {GROUPS.filter((g) => byGroup[g.id]?.length).map((g) => (
          <Card key={g.id} pad={false} title={<span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: `var(--s${g.slot})` }} />{g.name}</span>}
            action={<span className="tnum text-sm font-semibold">{eur(byGroup[g.id].reduce((a, x) => a + (x.balance ?? 0), 0))}</span>}>
            <div className="divide-y divide-line border-t border-line">
              {byGroup[g.id].map((a) => <AccountRow key={a.id} a={a} onClick={() => setAcct(a)} />)}
            </div>
          </Card>
        ))}
        {(accounts ?? []).some((a) => a.archived) && (
          <a href="#/settings/accounts" className="text-xs text-muted hover:text-accent lg:col-span-2">{(accounts ?? []).filter((a) => a.archived).length} hidden accounts — manage in Settings</a>
        )}
        {byGroup.other?.length > 0 && (
          <Card pad={false} title="Other"><div className="divide-y divide-line border-t border-line">{byGroup.other.map((a) => <AccountRow key={a.id} a={a} onClick={() => setAcct(a)} />)}</div></Card>
        )}
      </div>
      {updateOpen && <UpdateBalances accounts={accounts ?? []} onClose={() => setUpdateOpen(false)} />}
      {acct && <AccountSheet a={acct} onClose={() => setAcct(null)} />}
      {newAcct && <AccountEditor onClose={() => setNewAcct(false)} />}
    </div>
  )
}

/** Where the money is, account by account: stacked balances over the chosen
 *  range plus today's split, with a switch per account. */
function WhereMoneyIs({ from, range, liquidOnly, accounts }: { from: string; range: string; liquidOnly: boolean; accounts: Account[] }) {
  const { data: hist, isLoading } = useNetWorthHistory(from, SHORT.includes(range) ? 'week' : 'month', true)
  const { prefs, set } = usePrefs()
  const off = new Set(prefs.hidden_accounts ?? [])
  const toggle = (id: string) => {
    const next = new Set(off)
    next.has(id) ? next.delete(id) : next.add(id)
    set({ hidden_accounts: [...next] })
  }
  // Colours are ranked within the view (all assets, or liquid only) so the
  // seven hues go to what is on screen; switching an account off keeps them.
  const { colors, eligible } = useMemo(() => {
    const eligible = accounts.filter((a) => a.kind !== 'loan' && !a.archived && (a.balance ?? 0) > 0 && (!liquidOnly || LIQUID_GROUPS.includes(a.group)))
      .sort((a, b) => (b.balance ?? 0) - (a.balance ?? 0))
    const colors: Record<string, string> = {}
    eligible.slice(0, 7).forEach((a, i) => { colors[a.id] = `var(--s${i + 1})` })
    return { colors, eligible }
  }, [accounts, liquidOnly])
  const shown = eligible.filter((a) => !off.has(a.id))
  const named = shown.filter((a) => colors[a.id])
  const others = shown.filter((a) => !colors[a.id])
  const series = useMemo(() => (hist ?? []).map((h) => {
    const row: any = { date: h.date }
    for (const a of named) row[a.id] = Math.max(0, h.by_account?.[a.id] ?? 0)
    row.other = others.reduce((t, a) => t + Math.max(0, h.by_account?.[a.id] ?? 0), 0)
    return row
  }), [hist, named, others])
  if (isLoading) return <Card title="Where my money is"><Loading /></Card>
  if (!eligible.length) return null
  const slices: Slice[] = foldSlices([...named.map((a) => ({ key: a.id, label: a.name, value: a.balance ?? 0, color: colors[a.id] })),
    ...(others.length ? [{ key: 'other', label: `Other (${others.length})`, value: others.reduce((t, a) => t + (a.balance ?? 0), 0), color: 'var(--s-other)' }] : [])])
  const keys = [...named.map((a) => ({ id: a.id, name: a.name, color: colors[a.id] })), ...(others.length ? [{ id: 'other', name: 'Other', color: 'var(--s-other)' }] : [])]
  const total = shown.reduce((t, a) => t + (a.balance ?? 0), 0)
  return (
    <Card title="Where my money is" action={<span className="text-xs text-muted">{liquidOnly ? 'liquid' : 'all assets'} · {shown.length} of {eligible.length} accounts</span>}>
      {/* One switch per account: tap to leave it out of both charts. */}
      <div className="no-scrollbar -mx-4 mb-3 flex gap-1.5 overflow-x-auto overflow-y-hidden px-4 sm:mx-0 sm:flex-wrap sm:px-0">
        {eligible.map((a) => {
          const on = !off.has(a.id)
          return (
            <button key={a.id} type="button" onClick={() => toggle(a.id)} aria-pressed={on} className={clsx('chip shrink-0', on ? 'text-ink' : 'opacity-50 line-through')}>
              <span className="h-2 w-2 rounded-[3px]" style={{ background: on ? colors[a.id] ?? 'var(--s-other)' : 'transparent', border: on ? undefined : '1px solid rgb(var(--axis))' }} />
              {a.name}
            </button>
          )
        })}
      </div>
      {!shown.length ? <div className="py-8 text-center text-sm text-muted">Every account is switched off — tap one above.</div> : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_minmax(0,320px)]">
          <div className="h-56 sm:h-64">
            <ResponsiveContainer>
              <AreaChart data={series} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={shortDate(label)} rows={[...keys.filter((k) => payload[0].payload[k.id] > 0).map((k) => ({ color: k.color, label: k.name, value: eur(payload[0].payload[k.id]) })).reverse(),
                    { label: 'Total', value: eur(keys.reduce((t, k) => t + (payload[0].payload[k.id] ?? 0), 0)), bold: true }]} />) : null} />
                {keys.map((k) => (
                  <Area key={k.id} type="monotone" dataKey={k.id} name={k.name} stackId="a" stroke="var(--chart-surface)" strokeWidth={1.5} fill={k.color} fillOpacity={0.85} isAnimationActive={false} />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <Donut slices={slices} center={eurk(total)} sub="today" height={180} />
        </div>
      )}
    </Card>
  )
}

/** Where the money sits now: one composition bar of the positive groups. */
function Allocation({ byGroup, liquidOnly }: { byGroup: Record<string, number>; liquidOnly: boolean }) {
  const parts = GROUPS.filter((g) => g.id !== 'debt' && (!liquidOnly || LIQUID_GROUPS.includes(g.id)))
    .map((g) => ({ ...g, v: Math.max(0, byGroup[g.id] ?? 0) })).filter((g) => g.v > 0)
  const total = parts.reduce((a, g) => a + g.v, 0)
  if (!total) return null
  return (
    <div className="mt-4">
      <div className="section-title mb-1.5">Allocation</div>
      <div className="flex h-3 w-full gap-0.5 overflow-hidden rounded-full">
        {parts.map((g) => <div key={g.id} title={`${g.name} ${pct(g.v / total, 1)}`} style={{ width: `${(g.v / total) * 100}%`, background: `var(--s${g.slot})` }} />)}
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink2">
        {parts.map((g) => <span key={g.id}>{g.name} <b className="tnum text-ink">{pct(g.v / total)}</b></span>)}
      </div>
    </div>
  )
}

/** Which accounts drove the change over the selected range. */
function Movement({ from, label }: { from: string; label: string }) {
  const { data } = useQuery({ queryKey: ['movement', from], queryFn: () => api.get<any[]>('/networth/movement', { from }), enabled: !!from })
  if (!data?.length) return null
  const max = Math.max(...data.map((m) => Math.abs(m.change)), 1)
  return (
    <Card title={`What moved ${label}`}>
      <div className="space-y-2">
        {data.filter((m) => Math.abs(m.change) >= 1).slice(0, 10).map((m) => (
          <div key={m.account_id} className="grid grid-cols-[1fr_auto] items-center gap-x-3 text-sm">
            <span className="truncate">{m.name} <span className="text-xs text-muted tnum">{eurk(m.start)} → {eurk(m.end)}</span></span>
            <span className={clsx('tnum', m.change >= 0 ? 'text-good' : 'text-bad')}>{m.change >= 0 ? '+' : '−'}{eur(Math.abs(m.change))}</span>
            <div className="col-span-2 h-1 rounded-full bg-sunken"><div className={clsx('h-full rounded-full', m.change >= 0 ? 'bg-good' : 'bg-bad')} style={{ width: `${(Math.abs(m.change) / max) * 100}%` }} /></div>
          </div>
        ))}
      </div>
    </Card>
  )
}

function staleDays(d?: string) {
  if (!d) return 0
  return Math.floor((Date.now() - new Date(d + 'T00:00:00').getTime()) / 86400000)
}

function AccountRow({ a, onClick }: { a: Account; onClick: () => void }) {
  const age = staleDays(a.balance_date)
  const stale = age > 45 && !['property', 'vehicle', 'loan'].includes(a.kind) && !!a.balance
  return (
    <button onClick={onClick} className={clsx('flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/50', a.archived && 'opacity-50')}>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{a.name}</div>
        <div className={clsx('truncate text-xs', stale ? 'text-warn' : 'text-muted')}>
          {[a.institution !== a.name && a.institution, a.quantity ? `${a.quantity} units` : '', a.balance_date ? (stale ? `${age} days old` : shortDate(a.balance_date)) : 'no balance', a.source === 'bank' ? 'from bank' : a.source === 'computed' ? 'estimated' : ''].filter(Boolean).join(' · ')}
        </div>
      </div>
      <div className="tnum text-sm font-semibold">{a.balance != null ? eur(a.balance) : '—'}</div>
    </button>
  )
}

function UpdateBalances({ accounts, onClose }: { accounts: Account[]; onClose: () => void }) {
  const editable = accounts.filter((a) => !a.archived && a.kind !== 'loan')
  // House, car and solar are revalued once a year at most: tucked away.
  const isValuation = (a: Account) => a.kind === 'property' || a.kind === 'vehicle'
  const frequent = editable.filter((a) => !isValuation(a))
  const valuations = editable.filter(isValuation)
  const oldest = valuations.map((a) => a.balance_date).filter(Boolean).sort()[0]
  const [showVal, setShowVal] = useState(false)
  const [date, setDate] = useState(todayISO())
  const [vals, setVals] = useState<Record<string, string>>({})
  const [qty, setQty] = useState<Record<string, string>>({})
  const [price, setPrice] = useState('')
  const refresh = useRefresh()
  const toast = useToast()
  useEffect(() => {
    // Prefill today's BTC price if we can.
    api.get<any>('/market/quote/BTC-EUR').then((q) => q?.price && setPrice(String(Math.round(q.price)))).catch(() => {})
  }, [])
  const save = useMutation({
    mutationFn: () => api.post<{ saved: number }>('/balances', {
      date,
      values: editable.flatMap((a): { account_id: string; value?: number; quantity?: number; price?: number }[] => {
        if (a.kind === 'crypto' && qty[a.id]) return [{ account_id: a.id, quantity: Number(qty[a.id]), price: Number(price) }]
        if (vals[a.id] !== undefined && vals[a.id] !== '') return [{ account_id: a.id, value: Number(vals[a.id]) }]
        return []
      }),
    }),
    onSuccess: (r) => { toast(`${r.saved} balances saved`, 'good'); refresh(); onClose() },
  })
  const row = (a: Account) => (
    <div key={a.id} className="flex items-center gap-3 py-2">
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{a.name}</div>
        <div className="text-xs text-muted">now {a.balance != null ? eurc(a.balance) : '—'}{a.balance_date ? ` · ${shortDate(a.balance_date)}` : ''}</div>
      </div>
      {a.kind === 'crypto' ? (
        <input className="input h-9 w-32 tnum" inputMode="decimal" placeholder={a.quantity ? String(a.quantity) : 'units'} value={qty[a.id] ?? ''} onChange={(e) => setQty({ ...qty, [a.id]: e.target.value })} />
      ) : (
        <input className="input h-9 w-32 tnum" inputMode="decimal" placeholder={a.balance != null ? String(a.balance) : '€'} value={vals[a.id] ?? ''} onChange={(e) => setVals({ ...vals, [a.id]: e.target.value })} />
      )}
    </div>
  )
  return (
    <Sheet open onClose={onClose} title="Update balances" footer={<>
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={save.isPending}>Save</button>
    </>}>
      <div className="mb-3 grid grid-cols-2 gap-3">
        <Field label="As of"><input type="date" className="input" value={date} onChange={(e) => setDate(e.target.value)} /></Field>
        <Field label="BTC price €"><input className="input tnum" value={price} onChange={(e) => setPrice(e.target.value)} /></Field>
      </div>
      <div className="text-xs text-muted mb-2">Leave a field empty to keep it. Bank-synced accounts update themselves on sync.</div>
      <div className="divide-y divide-line">
        {frequent.map(row)}
      </div>
      {valuations.length > 0 && (
        <div className="mt-3 rounded-xl border border-line">
          <button type="button" onClick={() => setShowVal(!showVal)} aria-expanded={showVal} className="flex w-full items-center gap-3 px-3 py-2.5 text-left">
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium">Property & car <span className="font-normal text-muted">· {valuations.length}</span></div>
              <div className="truncate text-xs text-muted">{eurc(valuations.reduce((t, a) => t + (a.balance ?? 0), 0))} · valued {oldest ? `since ${shortDate(oldest)}` : '—'} · change rarely</div>
            </div>
            <Icon name="chevronD" size={16} className={clsx('text-muted transition', showVal && 'rotate-180')} />
          </button>
          {showVal && <div className="divide-y divide-line border-t border-line px-3">{valuations.map(row)}</div>}
        </div>
      )}
      <ErrorBox error={save.error} />
    </Sheet>
  )
}

function AccountSheet({ a, onClose }: { a: Account; onClose: () => void }) {
  const { data: pts, isLoading } = useQuery({ queryKey: ['balances', a.id], queryFn: () => api.get<any[]>('/balances', { account: a.id }) })
  const [editing, setEditing] = useState(false)
  const refresh = useRefresh()
  const del = async (date: string) => {
    if (!confirm(`Delete the ${date} value?`)) return
    await api.del('/balances', { account: a.id, date })
    refresh()
  }
  if (editing) return <AccountEditor a={a} onClose={onClose} />
  const series = (pts ?? []).map((p) => ({ date: p.date, value: p.value }))
  return (
    <Sheet open onClose={onClose} title={a.name} wide footer={<button className="btn-outline" onClick={() => setEditing(true)}><Icon name="edit" size={16} />Edit account</button>}>
      <div className="mb-3 flex items-baseline justify-between">
        <div className="text-2xl font-semibold">{a.balance != null ? eur(a.balance) : '—'}</div>
        <div className="text-sm text-muted">{[a.institution, a.kind, a.liquid ? 'liquid' : 'not liquid'].filter(Boolean).join(' · ')}</div>
      </div>
      {isLoading ? <Loading /> : series.length > 1 && (
        <div className="h-48">
          <ResponsiveContainer>
            <LineChart data={series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="date" {...axisProps} tickFormatter={(d) => d.slice(0, 7)} minTickGap={40} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={['auto', 'auto']} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={shortDate(label)} rows={[{ label: 'Balance', value: eurc(payload[0].value as number), bold: true }]} /> : null} />
              <Line type="stepAfter" dataKey="value" stroke="var(--s1)" strokeWidth={2} dot={false} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
      <div className="mt-4 section-title">Recorded values</div>
      <div className="mt-1 max-h-72 divide-y divide-line overflow-y-auto">
        {[...(pts ?? [])].reverse().slice(0, 200).map((p) => (
          <div key={p.date} className="flex items-center justify-between py-1.5 text-sm">
            <span className="text-ink2">{p.date} <span className="text-xs text-muted">{p.source}</span></span>
            <span className="flex items-center gap-2"><span className="tnum">{eurc(p.value)}</span>
              <button className="text-muted hover:text-bad" onClick={() => del(p.date)} aria-label="Delete value"><Icon name="trash" size={14} /></button></span>
          </div>
        ))}
      </div>
    </Sheet>
  )
}

const safeParse = (s: string) => { try { return JSON.parse(s || '{}') } catch { return {} } }

const KINDS = ['checking', 'savings', 'cash', 'brokerage', 'pension', 'crypto', 'property', 'vehicle', 'loan', 'other']

function AccountEditor({ a, onClose }: { a?: Account; onClose: () => void }) {
  const [v, setV] = useState<Partial<Account>>(a ?? { kind: 'checking', liquid: true, name: '', institution: '' })
  const [details, setDetails] = useState(JSON.stringify(a?.details ?? {}, null, 2))
  const refresh = useRefresh()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => {
      let d = {}
      try { d = JSON.parse(details || '{}') } catch { throw new Error('Details must be valid JSON') }
      if (v.kind === 'loan') d = cleanTerms(d)
      const body = { ...v, details: d }
      return a ? api.put(`/accounts/${a.id}`, body) : api.post('/accounts', body)
    },
    onSuccess: () => { refresh(); toast('Saved', 'good'); onClose() },
  })
  const del = useMutation({ mutationFn: () => api.del(`/accounts/${a!.id}`), onSuccess: () => { refresh(); onClose() } })
  return (
    <Sheet open onClose={onClose} title={a ? `Edit ${a.name}` : 'New account'} footer={<>
      {a && <button className="btn-danger mr-auto" onClick={() => confirm('Delete this account? Only possible when no transaction uses it.') && del.mutate()}>Delete</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()}>Save</button>
    </>}>
      <div className="space-y-4">
        <Field label="Name"><input className="input" value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} /></Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Institution"><input className="input" value={v.institution} onChange={(e) => setV({ ...v, institution: e.target.value })} /></Field>
          <Field label="Kind"><select className="input select-pad" value={v.kind} onChange={(e) => setV({ ...v, kind: e.target.value })}>{KINDS.map((k) => <option key={k}>{k}</option>)}</select></Field>
        </div>
        <div className="flex flex-wrap gap-4">
          <Toggle checked={!!v.liquid} onChange={(x) => setV({ ...v, liquid: x })} label="Liquid" />
          <Toggle checked={!!v.archived} onChange={(x) => setV({ ...v, archived: x })} label="Archived" />
        </div>
        <Field label="Notes"><textarea className="input h-20 py-2" value={v.notes ?? ''} onChange={(e) => setV({ ...v, notes: e.target.value })} /></Field>
        {v.kind === 'loan' && <LoanFields d={safeParse(details)} onChange={(d) => setDetails(JSON.stringify(d, null, 2))} />}
        {['property', 'vehicle'].includes(v.kind ?? '') && (
          <Field label="Details (JSON)" hint="purchase_date, purchase_price, address">
            <textarea className="input h-36 py-2 font-mono text-xs" value={details} onChange={(e) => setDetails(e.target.value)} />
          </Field>
        )}
        <ErrorBox error={save.error || del.error} />
      </div>
    </Sheet>
  )
}

// ── investments ─────────────────────────────────────────────────────

function Investments() {
  const { data: p, isLoading, error } = useQuery({ queryKey: ['portfolio'], queryFn: () => api.get<any>('/portfolio'), staleTime: 300_000 })
  const { data: trades } = useQuery({ queryKey: ['trades'], queryFn: () => api.get<any[]>('/trades') })
  const [pos, setPos] = useState<any>(null)
  const [trade, setTrade] = useState<any>(null)
  if (isLoading) return <Loading label="Fetching live prices…" />
  if (error) return <ErrorBox error={error} />
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div className="card p-3.5"><div className="text-xs text-ink2">Market value</div><div className="text-xl font-semibold">{eur(p.value_eur)}</div></div>
        <div className="card p-3.5"><div className="text-xs text-ink2">Cost basis</div><div className="text-xl font-semibold">{eur(p.cost_eur)}</div></div>
        <div className="card p-3.5"><div className="text-xs text-ink2">Unrealised</div><div className={clsx('text-xl font-semibold', p.gain_eur >= 0 ? 'text-good' : 'text-bad')}>{eur(p.gain_eur)}</div><div className="text-xs text-muted">{pct(p.cost_eur ? p.gain_eur / p.cost_eur : 0, 1)}</div></div>
        <div className="card p-3.5"><div className="text-xs text-ink2">Realised</div><div className="text-xl font-semibold">{eur(p.realized_eur)}</div></div>
      </div>
      <ScenarioCard />
      <Card pad={false} title="Positions" action={<button className="btn-primary h-8 text-xs" onClick={() => setTrade({ action: 'buy', date: todayISO(), currency: 'USD', account_id: 'ibkr' })}><Icon name="plus" size={14} />Trade</button>}>
        {!p.holdings.length ? <Empty title="No open positions" /> : (
          <div className="divide-y divide-line border-t border-line">
            {p.holdings.map((h: any) => (
              <button key={h.ticker} onClick={() => setPos(h)} className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/50">
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold">{h.ticker} <span className="text-xs font-normal text-muted">{h.account_id}</span></div>
                  <div className="text-xs text-muted tnum">{+h.shares.toFixed(4)} × {h.price != null ? h.price.toFixed(2) : '—'} {h.currency} · avg {h.avg_cost.toFixed(2)} · {pct(h.cost_share)} of book</div>
                </div>
                <div className="text-right">
                  <div className="tnum text-sm font-semibold">{h.value_eur != null ? eur(h.value_eur) : eur(h.cost_eur)}</div>
                  {h.gain_pct != null ? <div className={clsx('tnum text-xs', h.gain_pct >= 0 ? 'text-good' : 'text-bad')}>{h.gain_pct >= 0 ? '+' : ''}{pct(h.gain_pct, 1)}</div> : <div className="text-xs text-muted">no quote</div>}
                </div>
              </button>
            ))}
          </div>
        )}
      </Card>
      <Card pad={false} title="Trades">
        <div className="max-h-96 divide-y divide-line overflow-y-auto border-t border-line">
          {[...(trades ?? [])].reverse().map((t) => (
            <button key={t.id} onClick={() => setTrade(t)} className="flex w-full items-center justify-between px-4 py-2 text-left text-sm hover:bg-sunken/50">
              <span><span className={t.action === 'buy' ? 'text-good' : 'text-bad'}>{t.action}</span> <b>{t.ticker}</b> <span className="text-muted">{shortDate(t.date)} · {t.account_id}</span></span>
              <span className="tnum">{+t.shares.toFixed(4)} × {t.price} {t.currency}</span>
            </button>
          ))}
        </div>
      </Card>
      {pos && <PositionSheet h={pos} onClose={() => setPos(null)} />}
      {trade && <TradeEditor t={trade} onClose={() => setTrade(null)} />}
    </div>
  )
}

/** The book at analyst low / mean / high targets (52-week range for ETFs). */
function ScenarioCard() {
  const { data: s, isLoading } = useQuery({ queryKey: ['scenarios'], queryFn: () => api.get<any>('/portfolio/scenarios'), staleTime: 3_600_000 })
  if (isLoading) return <Card title="Where analysts see it"><Loading label="Fetching targets…" /></Card>
  if (!s?.value_eur) return null
  const rows = [{ label: 'Bear (low targets)', v: s.low_eur }, { label: 'Base (mean targets)', v: s.mean_eur }, { label: 'Bull (high targets)', v: s.high_eur }]
  return (
    <Card title="Where analysts see it" action={<span className="text-xs text-muted">{pct(s.covered)} of the book has targets</span>}>
      <div className="grid grid-cols-3 gap-3 text-sm">
        {rows.map((x) => (
          <div key={x.label}>
            <div className="text-xs text-muted">{x.label}</div>
            <div className="font-semibold tnum">{eur(x.v)}</div>
            <div className={clsx('text-xs tnum', x.v >= s.value_eur ? 'text-good' : 'text-bad')}>{x.v >= s.value_eur ? '+' : '−'}{pct(Math.abs(x.v / s.value_eur - 1))}</div>
          </div>
        ))}
      </div>
      <div className="mt-2 text-xs text-muted">Today {eur(s.value_eur)}. Positions without targets count at today's value in every scenario. Targets are opinions, not forecasts.</div>
    </Card>
  )
}

function PositionSheet({ h, onClose }: { h: any; onClose: () => void }) {
  const [range, setRange] = usePeriod('position', '1y', ['1mo', '6mo', 'ytd', '1y', '5y'])
  const { data: hist } = useQuery({ queryKey: ['hist', h.ticker, range], queryFn: () => api.get<any[]>(`/market/history/${h.ticker}`, { range }) })
  const { data: an } = useQuery({ queryKey: ['analyst', h.ticker], queryFn: () => api.get<any>(`/market/analyst/${h.ticker}`), retry: false })
  return (
    <Sheet open onClose={onClose} title={h.ticker} wide>
      <div className="mb-3 grid grid-cols-3 gap-3 text-sm">
        <div><div className="text-xs text-muted">Value</div><div className="font-semibold">{h.value_eur != null ? eur(h.value_eur) : '—'}</div></div>
        <div><div className="text-xs text-muted">Gain</div><div className={clsx('font-semibold', (h.gain ?? 0) >= 0 ? 'text-good' : 'text-bad')}>{h.gain != null ? `${h.gain.toFixed(0)} ${h.currency}` : '—'}</div></div>
        <div><div className="text-xs text-muted">52-week</div><div className="font-semibold tnum">{h.week52_low ? `${h.week52_low.toFixed(0)}–${h.week52_high.toFixed(0)}` : '—'}</div></div>
      </div>
      <Segmented value={range} onChange={setRange} size="sm" options={[{ value: '1mo', label: '1M' }, { value: '6mo', label: '6M' }, { value: 'ytd', label: 'YTD' }, { value: '1y', label: '1Y' }, { value: '5y', label: '5Y' }]} />
      <div className="mt-2 h-52">
        <ResponsiveContainer>
          <LineChart data={hist ?? []} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="date" {...axisProps} minTickGap={40} tickFormatter={(d) => d.slice(2, 7)} />
            <YAxis {...axisProps} domain={['auto', 'auto']} width={48} />
            <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={label} rows={[{ label: 'Close', value: `${(payload[0].value as number).toFixed(2)} ${h.currency}`, bold: true }]} /> : null} />
            <ReferenceLine y={h.avg_cost} stroke="var(--s2)" strokeDasharray="0" label={{ value: 'your avg', fill: 'var(--chart-text)', fontSize: 10, position: 'insideTopLeft' }} />
            <Line type="monotone" dataKey="close" stroke="var(--s1)" strokeWidth={2} dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>
      {an && (
        <div className="mt-3 rounded-xl bg-sunken p-3 text-sm">
          <div className="section-title mb-1">Analyst targets {an.num_analysts ? `(${an.num_analysts})` : '(from 52-week range)'}</div>
          <div className="grid grid-cols-3 gap-2 tnum">
            <div><div className="text-xs text-muted">Low</div>{an.target_low?.toFixed(2)}</div>
            <div><div className="text-xs text-muted">Mean</div>{an.target_mean?.toFixed(2)}</div>
            <div><div className="text-xs text-muted">High</div>{an.target_high?.toFixed(2)}</div>
          </div>
          {an.recommendation && <div className="mt-1 text-xs text-muted">Consensus: {an.recommendation}</div>}
        </div>
      )}
    </Sheet>
  )
}

function TradeEditor({ t: init, onClose }: { t: any; onClose: () => void }) {
  const [t, setT] = useState(init)
  const refresh = useRefresh()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => (t.id ? api.put(`/trades/${t.id}`, t) : api.post('/trades', t)),
    onSuccess: () => { refresh(); toast('Saved', 'good'); onClose() },
  })
  const del = useMutation({ mutationFn: () => api.del(`/trades/${t.id}`), onSuccess: () => { refresh(); onClose() } })
  return (
    <Sheet open onClose={onClose} title={t.id ? 'Edit trade' : 'New trade'} footer={<>
      {t.id && <button className="btn-danger mr-auto" onClick={() => confirm('Delete this trade?') && del.mutate()}>Delete</button>}
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()}>Save</button>
    </>}>
      <div className="space-y-4">
        <Segmented value={t.action} onChange={(a) => setT({ ...t, action: a })} options={[{ value: 'buy', label: 'Buy' }, { value: 'sell', label: 'Sell' }]} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="Ticker"><input className="input uppercase" value={t.ticker ?? ''} onChange={(e) => setT({ ...t, ticker: e.target.value.toUpperCase() })} /></Field>
          <Field label="Date"><input type="date" className="input" value={t.date} onChange={(e) => setT({ ...t, date: e.target.value })} /></Field>
          <Field label="Shares"><input className="input tnum" inputMode="decimal" value={t.shares ?? ''} onChange={(e) => setT({ ...t, shares: Number(e.target.value) })} /></Field>
          <Field label="Price per share"><input className="input tnum" inputMode="decimal" value={t.price ?? ''} onChange={(e) => setT({ ...t, price: Number(e.target.value) })} /></Field>
          <Field label="Currency"><select className="input select-pad" value={t.currency} onChange={(e) => setT({ ...t, currency: e.target.value })}>{['USD', 'EUR', 'GBP'].map((c) => <option key={c}>{c}</option>)}</select></Field>
          <Field label="Account"><select className="input select-pad" value={t.account_id ?? ''} onChange={(e) => setT({ ...t, account_id: e.target.value })}><option value="ibkr">IBKR</option><option value="revolut_stocks">Revolut Stocks</option><option value="">—</option></select></Field>
        </div>
        <Field label="Notes"><input className="input" value={t.notes ?? ''} onChange={(e) => setT({ ...t, notes: e.target.value })} /></Field>
        <ErrorBox error={save.error} />
      </div>
    </Sheet>
  )
}

// ── loans ───────────────────────────────────────────────────────────

/** Structured loan terms (replaces hand-edited JSON). */
type Num = number | string // raw text while typing ("3." must survive), a number once saved
type LoanTerms = {
  lender?: string; asset_id?: string; base_rate_name?: string; base_rate?: Num; margin?: Num; rate_reset_date?: string
  monthly_payment?: Num; payment_day?: Num; start_date?: string; start_principal?: Num; end_date?: string
}
const LOAN_NUMS = ['base_rate', 'margin', 'monthly_payment', 'payment_day', 'start_principal'] as const
const toNum = (v?: Num) => (v == null || v === '' ? undefined : Number(String(v).replace(',', '.')))

/** Numbers as numbers; rejects anything that isn't one. */
function cleanTerms(d: LoanTerms): LoanTerms {
  const out: LoanTerms = { ...d }
  for (const k of LOAN_NUMS) {
    const n = toNum(d[k])
    if (n !== undefined && !Number.isFinite(n)) throw new Error(`${k.replace('_', ' ')} must be a number`)
    out[k] = n
  }
  const p = out.payment_day as number | undefined
  if (p != null && (p < 1 || p > 31 || !Number.isInteger(p))) throw new Error('Payment day must be 1–31')
  return out
}

function LoanFields({ d, onChange }: { d: LoanTerms; onChange: (d: LoanTerms) => void }) {
  const num = (k: keyof LoanTerms) => ({
    className: 'input tnum', inputMode: 'decimal' as const, value: d[k] ?? '',
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...d, [k]: e.target.value === '' ? undefined : e.target.value }),
  })
  const txt = (k: keyof LoanTerms, type = 'text') => ({
    className: 'input', type, value: (d[k] as string) ?? '',
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...d, [k]: e.target.value || undefined }),
  })
  const rate = (toNum(d.base_rate) ?? 0) + (toNum(d.margin) ?? 0)
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="Lender"><input {...txt('lender')} placeholder="SEB" /></Field>
        <Field label="Secured on"><AccountSelect value={d.asset_id ?? ''} onChange={(v) => onChange({ ...d, asset_id: v || undefined })} placeholder="Nothing" kinds={['property', 'vehicle']} /></Field>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <Field label="Base rate"><input {...txt('base_rate_name')} placeholder="6M EURIBOR" /></Field>
        <Field label="Base %"><input {...num('base_rate')} placeholder="2.10" /></Field>
        <Field label="Margin %"><input {...num('margin')} placeholder="1.85" /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Rate resets on" hint={rate > 0 ? `Rate now ${rate.toFixed(2)}%` : undefined}><input {...txt('rate_reset_date', 'date')} /></Field>
        <Field label="Monthly payment €" hint="Empty = computed from the end date"><input {...num('monthly_payment')} /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Started"><input {...txt('start_date', 'date')} /></Field>
        <Field label="Ends"><input {...txt('end_date', 'date')} /></Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Payment day"><input {...num('payment_day')} placeholder="17" /></Field>
        <Field label="Original principal €"><input {...num('start_principal')} /></Field>
      </div>
    </div>
  )
}

function LoanEditor({ id, onClose }: { id: string; onClose: () => void }) {
  const { data: accounts } = useAccounts()
  const a = accounts?.find((x) => x.id === id)
  const [d, setD] = useState<LoanTerms | null>(null)
  const [owed, setOwed] = useState('')
  const [owedDate, setOwedDate] = useState(todayISO())
  const refresh = useRefresh()
  const toast = useToast()
  useEffect(() => { if (a && !d) setD({ ...(a.details ?? {}) }) }, [a, d])
  const save = useMutation({
    mutationFn: async () => {
      if (!a || !d) return
      await api.put(`/accounts/${a.id}`, { ...a, details: { ...(a.details ?? {}), ...cleanTerms(d) } })
      if (owed.trim()) await api.post('/balances', { date: owedDate, values: [{ account_id: a.id, value: -Math.abs(Number(owed.replace(',', '.'))) }] })
    },
    onSuccess: () => { refresh(); toast('Loan updated', 'good'); onClose() },
  })
  return (
    <Sheet open onClose={onClose} title={a ? `Edit ${a.name}` : 'Edit loan'} footer={<>
      <button className="btn-ghost" onClick={onClose}>Cancel</button>
      <button className="btn-primary" onClick={() => save.mutate()} disabled={!d || save.isPending}>Save</button>
    </>}>
      {!d ? <Loading /> : (
        <div className="space-y-4">
          <LoanFields d={d} onChange={setD} />
          <div className="rounded-xl bg-sunken p-3">
            <div className="section-title mb-2">Balance owed</div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Owed €" hint={a?.balance != null ? `now ${eurc(Math.abs(a.balance))}` : undefined}><input className="input tnum" inputMode="decimal" value={owed} onChange={(e) => setOwed(e.target.value)} placeholder="leave empty to keep" /></Field>
              <Field label="As of"><input type="date" className="input" value={owedDate} onChange={(e) => setOwedDate(e.target.value)} /></Field>
            </div>
          </div>
          <ErrorBox error={save.error} />
        </div>
      )}
    </Sheet>
  )
}

function Loans() {
  const [editing, setEditing] = useState<string | null>(null)
  const { data, isLoading } = useQuery({ queryKey: ['loans'], queryFn: () => api.get<any[]>('/loans') })
  const { data: hist } = useNetWorthHistory(rangeFrom('5y'), 'month', true)
  if (isLoading) return <Loading />
  if (!data?.length) return <Empty title="No loans">Add a loan account to track its balance, interest and payoff.</Empty>
  return (
    <div className="space-y-4">
      {data.map((l) => {
        const series = (hist ?? []).map((h) => ({ date: h.date, balance: -(h.by_account?.[l.account_id] ?? 0), equity: (h.by_account?.[l.details.asset_id] ?? 0) + (h.by_account?.[l.account_id] ?? 0) })).filter((x) => x.balance > 0)
        return (
          <Card key={l.account_id} title={`${l.name}${l.details.lender ? ` · ${l.details.lender}` : ''}`} action={<button className="btn-ghost h-8 px-2.5 text-xs" onClick={() => setEditing(l.account_id)}><Icon name="edit" size={15} />Edit</button>}>
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
              <div><div className="text-xs text-muted">Owed</div><div className="text-xl font-semibold">{eur(l.balance)}</div><div className="text-xs text-muted">as of {shortDate(l.balance_date)}</div></div>
              <div><div className="text-xs text-muted">Rate</div><div className="text-xl font-semibold">{l.rate.toFixed(2)}%</div><div className="text-xs text-muted">{l.details.base_rate_name} {l.details.base_rate}% + {l.details.margin}%</div></div>
              <div><div className="text-xs text-muted">Home equity</div><div className="text-xl font-semibold">{eur(l.equity)}</div><div className="text-xs text-muted">LTV {pct(l.ltv)}</div></div>
              <div><div className="text-xs text-muted">Paid off</div><div className="text-xl font-semibold">{l.payoff_date}</div><div className="text-xs text-muted">{l.months_left} payments left</div></div>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-3 rounded-xl bg-sunken p-3 text-sm sm:grid-cols-4">
              <div><div className="text-xs text-muted">Next payment</div><div className="tnum">{eur(l.next_interest + l.next_principal)}</div></div>
              <div><div className="text-xs text-muted">…interest / principal</div><div className="tnum">{eur(l.next_interest)} / {eur(l.next_principal)}</div></div>
              <div><div className="text-xs text-muted">Last 12 months</div><div className="tnum">{eur(l.paid_interest_12m)} interest · {eur(l.paid_principal_12m)} principal</div></div>
              <div><div className="text-xs text-muted">Interest still to pay</div><div className="tnum">{eur(l.total_interest_left)}</div></div>
            </div>
            {l.days_to_reset > 0 && <div className="mt-3 flex items-center gap-1.5 text-sm text-warn"><Icon name="alert" size={16} />Rate resets in {l.days_to_reset} days ({l.details.rate_reset_date})</div>}
            {series.length > 1 && (
              <div className="mt-4">
                <div className="h-52">
                  <ResponsiveContainer>
                    <LineChart data={series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
                      <CartesianGrid {...gridProps} />
                      <XAxis dataKey="date" {...axisProps} minTickGap={40} tickFormatter={(d) => d.slice(0, 4)} />
                      <YAxis {...axisProps} tickFormatter={eurk} width={48} />
                      <Tooltip content={({ active, payload, label }) => active && payload?.length ? <TooltipBox title={shortDate(label)} rows={payload.map((p: any) => ({ color: p.stroke, label: p.name, value: eur(p.value) }))} /> : null} />
                      <Line dataKey="balance" name="Owed" stroke="var(--s7)" strokeWidth={2} dot={false} isAnimationActive={false} />
                      <Line dataKey="equity" name="Equity" stroke="var(--s3)" strokeWidth={2} dot={false} isAnimationActive={false} />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
                <div className="mt-2"><Legend items={[{ color: 'var(--s7)', label: 'Owed' }, { color: 'var(--s3)', label: 'Equity' }]} /></div>
                <div className="mt-1 text-xs text-muted">Balances before the bank split principal and interest are reconstructed from payments.</div>
              </div>
            )}
          </Card>
        )
      })}
      {editing && <LoanEditor id={editing} onClose={() => setEditing(null)} />}
    </div>
  )
}


