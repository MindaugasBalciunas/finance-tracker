import { Fragment, useEffect, useMemo, useState } from 'react'
import { Route, Routes, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../lib/api'
import { useAccounts, useNetWorthHistory, usePeriod, usePrefs, useRefresh } from '../lib/hooks'
import { GROUPS, LIQUID_GROUPS } from '../lib/categories'
import { eur, eurc, eurk, parseNum, pct, shortDate, todayISO } from '../lib/format'
import { RANGES, rangeFrom, rangeLabel, rangeStep, rangeTick } from '../lib/periods'
import type { Account } from '../lib/types'
import { AskCFO, Card, Delta, Empty, ErrorBox, Field, Loading, NumberInput, PageHeader, Segmented, Sheet, Spinner, Tabs, Toggle, useToast } from '../components/ui'
import { axisProps, Donut, foldSlices, gridProps, Legend, TooltipBox, type Slice } from '../components/charts'
import { Icon, IconTile } from '../components/Icon'
import { accountColors, accountIcon, bankOf, bankRank, brandColor, volatility } from '../lib/brand'
import { AccountSelect } from '../components/pickers'

export default function Wealth() {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = loc.pathname.split('/')[2] || 'overview'
  return (
    <div>
      <PageHeader title="Wealth" actions={<AskCFO q="Review my balance sheet: allocation across cash, investments, pension, crypto and property, against my framework. What should I change?" />} />
      <Tabs value={tab} onChange={(v) => nav(v === 'overview' ? '/wealth' : `/wealth/${v}`)}
        tabs={[{ value: 'overview', label: 'Net worth' }, { value: 'investments', label: 'Investments' }, { value: 'loans', label: 'Loans' }, { value: 'history', label: 'History' }]} />
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/investments" element={<Investments />} />
        <Route path="/loans" element={<Loans />} />
        <Route path="/history" element={<BalanceHistory />} />
      </Routes>
    </div>
  )
}

function Overview() {
  const [range, setRange] = usePeriod('wealth', '3y', RANGES.map((r) => r.value))
  const { prefs, set: setPrefs } = usePrefs()
  const liquidOnly = prefs.liquid_only
  const setLiquidOnly = (v: boolean) => setPrefs({ liquid_only: v })
  const { data: hist, isLoading } = useNetWorthHistory(rangeFrom(range), rangeStep(range))
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
            action={<span className="tnum text-sm font-semibold">{eurc(byGroup[g.id].reduce((a, x) => a + (x.balance ?? 0), 0))}</span>}>
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

/** Where the money is: stacked balances over the chosen range plus today's
 *  split, by account or by bank. Bank colours; Swedbank, SEB and Revolut form
 *  the base in that order, other holdings follow steadiest-first, and each
 *  bank's accounts sit together (steadiest first within the bank). */
function WhereMoneyIs({ from, range, liquidOnly, accounts }: { from: string; range: string; liquidOnly: boolean; accounts: Account[] }) {
  const { data: hist, isLoading } = useNetWorthHistory(from, rangeStep(range), true)
  const { prefs, set } = usePrefs()
  const [view, setView] = usePeriod('wmi-view', 'accounts', ['accounts', 'banks'])
  const [allShown, setAllShown] = useState(false)
  const off = new Set(prefs.hidden_accounts ?? [])
  const toggle = (ids: string[]) => {
    const next = new Set(off)
    const allOff = ids.every((id) => next.has(id))
    for (const id of ids) allOff ? next.delete(id) : next.add(id)
    set({ hidden_accounts: [...next] })
  }
  const eligible = useMemo(() => accounts.filter((a) => a.kind !== 'loan' && !a.archived && (a.balance ?? 0) > 0 && (!liquidOnly || LIQUID_GROUPS.includes(a.group))), [accounts, liquidOnly])
  const colors = useMemo(() => accountColors(eligible), [eligible])

  // Series: one per account, or one per bank. Order by stability (steadiest
  // first = bottom of the stack); accounts grouped under their bank.
  const series = useMemo(() => {
    const vals = (ids: string[]) => (hist ?? []).map((h) => ids.reduce((t, id) => t + Math.max(0, h.by_account?.[id] ?? 0), 0))
    const banks = new Map<string, Account[]>()
    for (const a of eligible) banks.set(bankOf(a), [...(banks.get(bankOf(a)) ?? []), a])
    const bankList = [...banks.entries()].map(([name, accts]) => ({ name, accts, vol: volatility(vals(accts.map((x) => x.id))) })).sort((x, y) => bankRank(x.accts[0]) - bankRank(y.accts[0]) || x.vol - y.vol)
    if (view === 'banks') {
      return bankList.map((b) => {
        const lead = [...b.accts].sort((x, y) => (y.balance ?? 0) - (x.balance ?? 0))[0]
        return { key: 'bank:' + b.name, name: b.name, ids: b.accts.map((x) => x.id), color: brandColor(lead) ?? colors[lead.id], value: b.accts.reduce((t, x) => t + (x.balance ?? 0), 0), bank: b.name, icon: accountIcon(lead) }
      })
    }
    return bankList.flatMap((b) => b.accts.map((x) => ({ a: x, vol: volatility(vals([x.id])) })).sort((x, y) => x.vol - y.vol)
      .map(({ a }) => ({ key: a.id, name: a.name, ids: [a.id], color: colors[a.id], value: a.balance ?? 0, bank: b.name, icon: accountIcon(a) })))
  }, [hist, eligible, colors, view])

  // Amounts count only switched-on accounts, so the total is the same in both views.
  const bal = useMemo(() => Object.fromEntries(eligible.map((a) => [a.id, a.balance ?? 0])), [eligible])
  const valueOf = (x: { ids: string[] }) => x.ids.filter((id) => !off.has(id)).reduce((t, id) => t + (bal[id] ?? 0), 0)
  const shown = series.filter((x) => !x.ids.every((id) => off.has(id)))
  const rows = useMemo(() => (hist ?? []).map((h) => {
    const row: any = { date: h.date }
    for (const x of shown) row[x.key] = x.ids.filter((id) => !off.has(id)).reduce((t, id) => t + Math.max(0, h.by_account?.[id] ?? 0), 0)
    return row
  }), [hist, shown, off])
  if (isLoading) return <Card title="Where my money is"><Loading /></Card>
  if (!eligible.length) return null
  const total = shown.reduce((t, x) => t + valueOf(x), 0)
  const slices: Slice[] = shown.map((x) => ({ key: x.key, label: x.name, value: valueOf(x), color: x.color }))
  const limit = view === 'banks' ? 99 : 8
  const visible = allShown ? series : series.slice(0, limit)
  return (
    <Card title="Where my money is" action={<Segmented size="sm" value={view} onChange={setView} options={[{ value: 'accounts', label: 'Accounts' }, { value: 'banks', label: 'Banks' }]} />}>
      <div className="mb-2 text-xs text-muted"><b className="tnum text-ink">{eur(total)}</b> in {shown.reduce((t, x) => t + x.ids.filter((id) => !off.has(id)).length, 0)} {liquidOnly ? 'liquid ' : ''}accounts · same total in both views · tap to leave one out</div>
      {/* The legend is the switchboard: amount and share of what is shown. */}
      <div className="mb-3 grid grid-cols-1 gap-1.5 sm:grid-cols-2 lg:grid-cols-3">
        {visible.map((x, i) => {
          const on = !x.ids.every((id) => off.has(id))
          const onCount = x.ids.filter((id) => !off.has(id)).length
          const v = on ? valueOf(x) : x.value
          const newBank = view === 'accounts' && (i === 0 || visible[i - 1].bank !== x.bank)
          return (
            <Fragment key={x.key}>
              {newBank && <div className="col-span-full mt-1 text-[11px] font-semibold uppercase tracking-wide text-muted first:mt-0">{x.bank}</div>}
              <button type="button" onClick={() => toggle(x.ids)} aria-pressed={on}
                className={clsx('flex min-w-0 items-center gap-2 rounded-lg border border-line px-2 py-1.5 text-left text-xs transition hover:bg-sunken/50', !on && 'opacity-50')}>
                <IconTile name={x.icon} color={on ? x.color : 'var(--s-other)'} size={24} />
                <span className={clsx('min-w-0 flex-1 truncate', on ? 'text-ink' : 'text-muted line-through')}>
                  {x.name}{x.ids.length > 1 && <span className="text-muted"> · {onCount < x.ids.length ? `${onCount} of ${x.ids.length}` : x.ids.length}</span>}
                </span>
                <span className="tnum font-medium text-ink">{eurk(v)}</span>
                <span className="w-9 text-right tnum text-muted">{on && total > 0 ? `${Math.round((v / total) * 100)}%` : '—'}</span>
              </button>
            </Fragment>
          )
        })}
        {series.length > limit && (
          <button type="button" className="btn-ghost h-8 justify-start px-2.5 text-xs" onClick={() => setAllShown(!allShown)}>
            <Icon name="chevronD" size={14} className={clsx('transition', allShown && 'rotate-180')} />{allShown ? 'Fewer' : `+${series.length - limit} more`}
          </button>
        )}
      </div>
      {!shown.length ? <div className="py-8 text-center text-sm text-muted">Everything is switched off — tap one above.</div> : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_minmax(0,320px)]">
          <div className="h-56 sm:h-64">
            <ResponsiveContainer>
              <AreaChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={shortDate(label)} rows={[...shown.filter((x) => payload[0].payload[x.key] > 0).map((x) => ({ color: x.color, label: x.name, value: eur(payload[0].payload[x.key]) })).reverse(),
                    { label: 'Total', value: eur(shown.reduce((t, x) => t + (payload[0].payload[x.key] ?? 0), 0)), bold: true }]} />) : null} />
                {shown.map((x) => (
                  <Area key={x.key} type="monotone" dataKey={x.key} name={x.name} stackId="a" stroke="var(--chart-surface)" strokeWidth={1.5} fill={x.color} fillOpacity={0.9} isAnimationActive={false} />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <Donut slices={slices} center={eurk(total)} sub="today" height={200} legend={false} />
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
      <IconTile name={accountIcon(a)} color={brandColor(a) ?? `var(--s${GROUPS.find((g) => g.id === a.group)?.slot ?? 1})`} size={34} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{a.name}</div>
        <div className={clsx('truncate text-xs', stale ? 'text-warn' : 'text-muted')}>
          {[a.institution !== a.name && a.institution, a.quantity ? `${a.quantity} units` : '', a.balance_date ? (stale ? `${age} days old` : shortDate(a.balance_date)) : 'no balance', a.source === 'bank' ? 'from bank' : a.source === 'computed' ? 'estimated' : ''].filter(Boolean).join(' · ')}
        </div>
      </div>
      <div className="tnum text-sm font-semibold">{a.balance != null ? eurc(a.balance) : '—'}</div>
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
        if (a.kind === 'crypto' && qty[a.id]) {
          const q = parseNum(qty[a.id]), p = parseNum(price)
          if (q === undefined || p === undefined) throw new Error(`${a.name}: quantity and BTC price must be numbers`)
          return [{ account_id: a.id, quantity: q, price: p }]
        }
        if (vals[a.id] !== undefined && vals[a.id] !== '') {
          const v = parseNum(vals[a.id])
          if (v === undefined) throw new Error(`${a.name}: "${vals[a.id]}" is not a number`)
          return [{ account_id: a.id, value: v }]
        }
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
  const { data: sc } = useQuery({ queryKey: ['scenarios'], queryFn: () => api.get<any>('/portfolio/scenarios'), staleTime: 3_600_000 })
  const [range, setRange] = usePeriod('stocks', '1y', RANGES.map((r) => r.value))
  const { data: perf, isLoading: perfLoading } = useQuery({ queryKey: ['portfolio-history', range], queryFn: () => api.get<any[]>('/portfolio/history', { range }), staleTime: 600_000 })
  const [sort, setSort] = usePeriod('stocks-sort', 'value', ['value', 'gain', 'day'])
  const [split, setSplit] = usePeriod('stocks-split', 'positions', ['positions', 'brokers', 'currency'])
  const [pos, setPos] = useState<any>(null)
  const [trade, setTrade] = useState<any>(null)
  const [showTrades, setShowTrades] = useState(false)
  const [mode, setMode] = usePeriod('stocks-chart', 'worth', ['worth', 'gain'])
  if (isLoading) return <Loading label="Fetching live prices…" />
  if (error) return <ErrorBox error={error} />
  const hs: any[] = p.holdings
  // One colour per position (largest first), shared by the donut and the rows.
  const color: Record<string, string> = {}
  ;[...hs].sort((a, b) => (b.value_eur ?? b.cost_eur ?? 0) - (a.value_eur ?? a.cost_eur ?? 0)).forEach((h, i) => { color[h.ticker] = i < 7 ? `var(--s${i + 1})` : 'var(--s-other)' })
  const val = (h: any) => h.value_eur ?? h.cost_eur ?? 0
  const gainEUR = (h: any) => (h.value_eur != null && h.cost_eur != null ? h.value_eur - h.cost_eur : null)
  const upside: Record<string, number> = {}
  // Only real analyst coverage; ETFs fall back to their 52-week range, which isn't a target.
  for (const x of sc?.positions ?? []) if (x.analysts > 0 && x.mean_eur && x.value_eur) upside[x.ticker] = x.mean_eur / x.value_eur - 1
  const sorted = [...hs].sort((a, b) => sort === 'gain' ? (b.gain_pct ?? -9) - (a.gain_pct ?? -9) : sort === 'day' ? (b.day_pct ?? -9) - (a.day_pct ?? -9) : val(b) - val(a))
  const slices: Slice[] = split === 'positions'
    ? foldSlices(hs.map((h) => ({ key: h.ticker, label: h.ticker, value: val(h), color: color[h.ticker] })))
    : (() => {
      const m = new Map<string, number>()
      for (const h of hs) { const k = split === 'brokers' ? (h.account_id || 'other') : h.currency; m.set(k, (m.get(k) ?? 0) + val(h)) }
      return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([k, v], i) => ({ key: k, label: split === 'brokers' ? brokerName(k) : k, value: v, color: split === 'brokers' ? (brandColor({ institution: brokerName(k), name: k, kind: 'brokerage' }) ?? `var(--s${i + 1})`) : `var(--s${i + 1})` }))
    })()
  const gainPct = p.cost_eur ? p.gain_eur / p.cost_eur : 0
  const dayPct = p.value_eur - p.day_change_eur > 0 ? p.day_change_eur / (p.value_eur - p.day_change_eur) : 0
  const last = perf?.[perf.length - 1], first = perf?.[0]
  return (
    <div className="space-y-4">
      {/* Headline + money in vs worth now */}
      <section className="card p-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-sm text-ink2">Stocks & ETFs</div>
            <div className="text-3xl font-semibold tracking-tight">{eur(p.value_eur)}</div>
            <div className="mt-0.5 flex flex-wrap gap-x-3 text-sm text-muted">
              <span>Today <Delta value={p.day_change_eur} /> <span className="tnum">{p.day_change_eur ? `(${dayPct >= 0 ? '+' : ''}${pct(dayPct, 2)})` : ''}</span></span>
              <span>Unrealised <Delta value={p.gain_eur} /> <span className="tnum">({gainPct >= 0 ? '+' : ''}{pct(gainPct, 1)})</span></span>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4 text-sm sm:text-right">
            <div><div className="text-xs text-muted">Cost basis</div><div className="font-semibold tnum">{eur(p.cost_eur)}</div></div>
            <div><div className="text-xs text-muted">Realised</div><div className={clsx('font-semibold tnum', p.realized_eur >= 0 ? 'text-good' : 'text-bad')}>{eur(p.realized_eur)}</div></div>
            <div><div className="text-xs text-muted">Positions</div><div className="font-semibold tnum">{hs.length}</div></div>
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-muted">Worth vs money in {last && first ? <>· {rangeLabel(range)} <Delta value={(last.value_eur - last.cost_eur) - (first.value_eur - first.cost_eur)} /></> : null}</span>
          <div className="flex flex-wrap items-center gap-2">
            <Segmented size="sm" value={mode} onChange={setMode} options={[{ value: 'worth', label: 'Worth' }, { value: 'gain', label: 'Gain' }]} />
            <Segmented size="sm" value={range} onChange={setRange} options={RANGES} />
          </div>
        </div>
        <div className="mt-2 h-56 sm:h-64">
          {perfLoading ? <Loading label="Replaying trades…" /> : !perf?.length ? <div className="grid h-full place-items-center text-sm text-muted">No trades in this period</div> : (
            <ResponsiveContainer>
              <ComposedChart data={mode === 'gain' ? perf.map((x: any) => ({ ...x, gain: x.value_eur - x.cost_eur, up: Math.max(0, x.value_eur - x.cost_eur), down: Math.min(0, x.value_eur - x.cost_eur) })) : perf} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
                <defs><linearGradient id="stk" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--s1)" stopOpacity={0.28} /><stop offset="100%" stopColor="var(--s1)" stopOpacity={0.02} /></linearGradient></defs>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
                <YAxis {...axisProps} tickFormatter={eurk} width={48} domain={['auto', 'auto']} />
                <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                  <TooltipBox title={shortDate(label)} rows={[
                    { color: 'var(--s1)', label: 'Worth', value: eur(payload[0].payload.value_eur), bold: true },
                    { color: 'var(--s2)', label: 'Money in (cost)', value: eur(payload[0].payload.cost_eur) },
                    { label: 'Gain', value: <span className={payload[0].payload.value_eur >= payload[0].payload.cost_eur ? 'text-good' : 'text-bad'}>{eur(payload[0].payload.value_eur - payload[0].payload.cost_eur)}</span> },
                  ]} />) : null} />
                {mode === 'gain' ? <>
                  <ReferenceLine y={0} stroke="var(--chart-axis)" />
                  <Area type="monotone" dataKey="up" name="Gain" stroke="rgb(var(--good))" strokeWidth={0} fill="rgb(var(--good))" fillOpacity={0.18} isAnimationActive={false} />
                  <Area type="monotone" dataKey="down" name="Loss" stroke="rgb(var(--bad))" strokeWidth={0} fill="rgb(var(--bad))" fillOpacity={0.18} isAnimationActive={false} />
                  <Line type="monotone" dataKey="gain" name="Gain" stroke="rgb(var(--ink))" strokeWidth={2} dot={false} isAnimationActive={false} />
                </> : <>
                  <Area type="monotone" dataKey="value_eur" name="Worth" stroke="var(--s1)" strokeWidth={2} fill="url(#stk)" isAnimationActive={false} />
                  <Line type="stepAfter" dataKey="cost_eur" name="Money in" stroke="var(--s2)" strokeWidth={2} dot={false} isAnimationActive={false} />
                </>}
              </ComposedChart>
            </ResponsiveContainer>
          )}
        </div>
        <div className="mt-2">{mode === 'gain'
          ? <Legend items={[{ color: 'rgb(var(--ink))', label: 'Unrealised gain (worth − money in)', value: last ? eur(last.value_eur - last.cost_eur) : undefined }]} />
          : <Legend items={[{ color: 'var(--s1)', label: 'Worth', value: last ? eurk(last.value_eur) : undefined }, { color: 'var(--s2)', label: 'Money in (cost of open positions)', value: last ? eurk(last.cost_eur) : undefined }]} />}</div>
        <div className="mt-1 text-[11px] text-muted">From your trades and daily/weekly closes at today's exchange rates; fund accounts without trades (Revolut ETF, Swedbank funds) are on Net worth.</div>
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
        <Card icon="pie" color="var(--s1)" title="Allocation" action={<Segmented size="sm" value={split} onChange={setSplit} options={[{ value: 'positions', label: 'Positions' }, { value: 'brokers', label: 'Brokers' }, { value: 'currency', label: 'Currency' }]} />}>
          <Donut slices={slices} center={eurk(p.value_eur)} sub="market value" height={180} stacked />
        </Card>

        <Card pad={false} icon="trend" color="var(--s7)" title="Positions" action={
          <button className="btn-primary h-8 px-2.5 text-xs" onClick={() => setTrade({ action: 'buy', date: todayISO(), currency: 'USD', account_id: 'ibkr' })}><Icon name="plus" size={14} />Trade</button>}>
          <div className="flex items-center justify-between gap-2 px-4 pb-2.5">
            <span className="text-xs text-muted">Sort by</span>
            <Segmented size="sm" value={sort} onChange={setSort} options={[{ value: 'value', label: 'Value' }, { value: 'gain', label: 'Gain' }, { value: 'day', label: 'Today' }]} />
          </div>
          {!hs.length ? <Empty title="No open positions" /> : (
            <div className="divide-y divide-line border-t border-line">
              {sorted.map((h) => {
                const g = gainEUR(h)
                const up = upside[h.ticker]
                return (
                  <button key={h.ticker} onClick={() => setPos(h)} className="block w-full px-4 py-3 text-left hover:bg-sunken/50">
                    <div className="flex items-center gap-3">
                      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl text-[10px] font-bold tracking-tight"
                        style={{ background: `color-mix(in oklab, ${color[h.ticker]} var(--tint), transparent)`, color: `color-mix(in oklab, ${color[h.ticker]} 80%, rgb(var(--ink)))` }}>{h.ticker.replace(/\..*$/, '').slice(0, 5)}</span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-baseline gap-1.5"><span className="text-sm font-semibold">{h.ticker}</span><span className="truncate text-xs text-muted">{h.name ?? brokerName(h.account_id)}</span></div>
                        <div className="truncate text-xs text-muted tnum">{+h.shares.toFixed(4)} × {h.price != null ? h.price.toFixed(2) : '—'} {h.currency} · avg {h.avg_cost.toFixed(2)}</div>
                      </div>
                      <div className="text-right">
                        <div className="tnum text-sm font-semibold">{eur(val(h))}</div>
                        <div className="flex items-center justify-end gap-1.5 text-xs tnum">
                          {h.day_pct != null && <span className={h.day_pct >= 0 ? 'text-good' : 'text-bad'}>{h.day_pct >= 0 ? '▲' : '▼'}{pct(Math.abs(h.day_pct), 1)}</span>}
                          {g != null ? <span className={clsx('rounded px-1', g >= 0 ? 'bg-good/10 text-good' : 'bg-bad/10 text-bad')}>{g >= 0 ? '+' : '−'}{eurk(Math.abs(g))} · {pct(Math.abs(h.gain_pct ?? 0), 0)}</span> : <span className="text-muted">no quote</span>}
                        </div>
                      </div>
                    </div>
                    <div className="mt-2 grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 pl-12">
                      <div className="h-1.5 rounded-full bg-sunken"><div className="h-full rounded-full" style={{ width: `${Math.max(2, h.weight * 100)}%`, background: color[h.ticker] }} /></div>
                      <span className="w-24 text-right text-[11px] text-muted tnum">{pct(h.weight, 1)} of book</span>
                      {h.week52_high > h.week52_low && h.price != null && <Range52 lo={h.week52_low} hi={h.week52_high} price={h.price} avg={h.avg_cost} />}
                      {h.week52_high > h.week52_low && h.price != null && <span className="w-24 text-right text-[11px] text-muted">{up != null ? <span className={up >= 0 ? 'text-good' : 'text-bad'}>target {up >= 0 ? '+' : '−'}{pct(Math.abs(up), 0)}</span> : '52-week range'}</span>}
                    </div>
                  </button>
                )
              })}
            </div>
          )}
        </Card>
      </div>

      <ScenarioCard />

      {p.closed?.length > 0 && (
        <Card pad={false} icon="check" color="var(--s6)" title="Closed positions" action={<span className="text-xs text-muted">realised {eur(p.realized_eur)}</span>}>
          <div className="divide-y divide-line border-t border-line">
            {p.closed.map((h: any) => (
              <div key={h.ticker} className="flex items-center justify-between px-4 py-2 text-sm">
                <span><b>{h.ticker}</b> <span className="text-xs text-muted">{brokerName(h.account_id)} · first bought {shortDate(h.first_buy)}</span></span>
                <span className={clsx('tnum', h.realized >= 0 ? 'text-good' : 'text-bad')}>{h.realized >= 0 ? '+' : '−'}{Math.abs(h.realized).toFixed(2)} {h.currency}</span>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card pad={false} icon="list" color="var(--s2)" title={`Trades (${trades?.length ?? 0})`} action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setShowTrades(!showTrades)}>{showTrades ? 'Hide' : 'Show all'}</button>}>
        <div className="divide-y divide-line border-t border-line">
          {[...(trades ?? [])].reverse().slice(0, showTrades ? undefined : 6).map((t) => (
            <button key={t.id} onClick={() => setTrade(t)} className="flex w-full items-center gap-3 px-4 py-2 text-left text-sm hover:bg-sunken/50">
              <span className={clsx('w-10 shrink-0 rounded-md py-0.5 text-center text-[10px] font-semibold uppercase', t.action === 'buy' ? 'bg-good/10 text-good' : 'bg-bad/10 text-bad')}>{t.action}</span>
              <span className="min-w-0 flex-1 truncate"><b>{t.ticker}</b> <span className="text-xs text-muted">{shortDate(t.date)} · {brokerName(t.account_id)}</span></span>
              <span className="text-right tnum"><span className="text-xs text-muted">{+t.shares.toFixed(4)} × {t.price}</span> <b>{(t.shares * t.price).toFixed(2)} {t.currency}</b></span>
            </button>
          ))}
        </div>
      </Card>
      {pos && <PositionSheet h={pos} onClose={() => setPos(null)} />}
      {trade && <TradeEditor t={trade} onClose={() => setTrade(null)} />}
    </div>
  )
}

const brokerName = (id?: string) => ({ ibkr: 'IBKR', revolut_stocks: 'Revolut', revolut_etf: 'Revolut', swed_etf: 'Swedbank' } as Record<string, string>)[id ?? ''] ?? (id || '—')

/** 52-week range: where today's price sits, with your average cost marked. */
function Range52({ lo, hi, price, avg }: { lo: number; hi: number; price: number; avg: number }) {
  const at = (v: number) => `${Math.min(100, Math.max(0, ((v - lo) / (hi - lo)) * 100))}%`
  return (
    <div className="relative h-3" title={`52-week ${lo.toFixed(2)} – ${hi.toFixed(2)} · price ${price.toFixed(2)} · your avg ${avg.toFixed(2)}`}>
      <div className="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 rounded-full bg-sunken" />
      <div className="absolute top-0 h-3 w-0.5 -translate-x-1/2 rounded bg-axis" style={{ left: at(avg) }} />
      <div className="absolute top-1/2 h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface bg-accent" style={{ left: at(price) }} />
    </div>
  )
}

/** The book at analyst low / mean / high targets (52-week range for ETFs). */
function ScenarioCard() {
  const { data: s, isLoading } = useQuery({ queryKey: ['scenarios'], queryFn: () => api.get<any>('/portfolio/scenarios'), staleTime: 3_600_000 })
  if (isLoading) return <Card icon="target" color="var(--s4)" title="Scenarios"><Loading label="Fetching targets…" /></Card>
  if (!s?.value_eur) return null
  // Honest labels: analyst targets where Yahoo has coverage, otherwise each
  // holding's 52-week low / middle / high.
  const analysts = s.covered > 0
  const basis = analysts ? 'targets' : '52-week'
  const rows = [{ label: `Bear (${analysts ? 'low targets' : '52-week low'})`, v: s.low_eur }, { label: `Base (${analysts ? 'mean targets' : '52-week middle'})`, v: s.mean_eur }, { label: `Bull (${analysts ? 'high targets' : '52-week high'})`, v: s.high_eur }]
  return (
    <Card icon="target" color="var(--s4)" title={analysts ? 'Where analysts see it' : 'Range scenarios'}
      action={<span className="text-xs text-muted">{analysts ? `${pct(s.covered)} of the book has analyst targets` : 'analyst targets unavailable'}</span>}>
      <div className="grid grid-cols-3 gap-3 text-sm">
        {rows.map((x) => (
          <div key={x.label}>
            <div className="text-xs text-muted">{x.label}</div>
            <div className="font-semibold tnum">{eur(x.v)}</div>
            <div className={clsx('text-xs tnum', x.v >= s.value_eur ? 'text-good' : 'text-bad')}>{x.v >= s.value_eur ? '+' : '−'}{pct(Math.abs(x.v / s.value_eur - 1))}</div>
          </div>
        ))}
      </div>
      <div className="mt-2 text-xs text-muted">Today {eur(s.value_eur)}. {analysts
        ? 'Positions without targets use their 52-week range. Targets are opinions, not forecasts.'
        : `Yahoo isn't returning analyst targets right now, so each holding is valued at its 52-week low, middle and high — a range, not a ${basis === '52-week' ? 'forecast' : 'target'}.`}</div>
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
          <Field label="Shares"><NumberInput value={t.shares} onChange={(v) => setT({ ...t, shares: v })} /></Field>
          <Field label="Price per share"><NumberInput value={t.price} onChange={(v) => setT({ ...t, price: v })} /></Field>
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
const toNum = (v?: Num) => (v == null || v === '' ? undefined : parseNum(String(v)) ?? NaN)

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
      if (owed.trim()) await api.post('/balances', { date: owedDate, values: [{ account_id: a.id, value: -Math.abs(parseNum(owed) ?? NaN) }] })
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



// ── balance history ─────────────────────────────────────────────────

const SOURCE_COLOR: Record<string, string> = { bank: 'rgb(var(--accent))', manual: 'var(--s6)', import: 'var(--s-other)', computed: 'var(--s4)' }

/** Every account on every snapshot date, 50 dates a page, newest first.
 *  Recorded values in full ink with their source; carried-forward values grey. */
function BalanceHistory() {
  const [page, setPage] = useState(1)
  const [picking, setPicking] = useState(false)
  const { data: accounts } = useAccounts()
  const { prefs, set } = usePrefs()
  const { data: t, isLoading, isFetching } = useQuery({ queryKey: ['balance-table', page], queryFn: () => api.get<any>('/balances/table', { page, size: 50 }), placeholderData: (p) => p })
  if (isLoading || !t) return <Loading />
  const byId = Object.fromEntries((accounts ?? []).map((a) => [a.id, a]))
  const inTable = (accounts ?? []).filter((a) => t.accounts.includes(a.id))
  // Columns you chose to hide; never configured → valuations (house, car, solar)
  // and hidden accounts are off, since they rarely change.
  const hidden = new Set(prefs.history_hidden ?? inTable.filter((a) => a.kind === 'property' || a.kind === 'vehicle' || a.archived).map((a) => a.id))
  const cols = inTable.filter((a) => !hidden.has(a.id) && t.rows.some((r: any) => r.cells[a.id]))
  const toggle = (id: string) => { const n = new Set(hidden); n.has(id) ? n.delete(id) : n.add(id); set({ history_hidden: [...n] }) }
  const from = (t.page - 1) * t.size + 1, to = from + t.rows.length - 1
  // Numbered pages: first, last, and two either side of the current one.
  const nums = [...new Set([1, t.pages, ...[-2, -1, 0, 1, 2].map((d) => t.page + d)])].filter((n) => n >= 1 && n <= t.pages).sort((a, b) => a - b)
  return (
    <div className="space-y-3">
      <Card pad={false} className="full-bleed" icon="calendar" color="var(--s7)" title="Balance history" action={
        <div className="flex items-center gap-1">
          <span className="mr-1 hidden text-xs text-muted tnum sm:inline">{from}–{to} of {t.dates}</span>
          {isFetching && <Spinner />}
          <button className="btn-ghost h-8 w-8 px-0" disabled={page <= 1} onClick={() => setPage(page - 1)} aria-label="Newer"><Icon name="chevronL" size={16} /></button>
          <button className="btn-ghost h-8 w-8 px-0" disabled={page >= t.pages} onClick={() => setPage(page + 1)} aria-label="Older"><Icon name="chevronR" size={16} /></button>
          <button className="btn-outline h-8 px-2.5 text-xs" onClick={() => setPicking(true)}><Icon name="filter" size={14} />Columns</button>
        </div>}>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 pb-2.5 text-[11px] text-muted">
          <span><b className="text-ink">Bold</b> = recorded that day · <span className="opacity-60">grey</span> = carried forward</span>
          {Object.entries(SOURCE_COLOR).map(([k, c]) => <span key={k} className="inline-flex items-center gap-1"><span className="h-1.5 w-1.5 rounded-full" style={{ background: c }} />{k === 'import' ? 'imported' : k}</span>)}
          {hidden.size > 0 && <button className="ml-auto text-accent" onClick={() => setPicking(true)}>{hidden.size} column{hidden.size === 1 ? '' : 's'} hidden</button>}
        </div>
        <div className="overflow-x-auto border-t border-line">
          <table className="w-full border-separate border-spacing-0 text-xs tnum">
            <thead>
              <tr className="text-left text-muted">
                <th className="sticky left-0 z-10 bg-surface px-3 py-2 font-medium">Date</th>
                {cols.map((a) => (
                  <th key={a.id} className="whitespace-nowrap px-2 py-2 text-right font-medium">
                    <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-[3px]" style={{ background: brandColor(a) ?? 'var(--s-other)' }} />{a.name.length > 16 ? a.name.slice(0, 15) + '…' : a.name}</span>
                  </th>
                ))}
                <th className="whitespace-nowrap px-2 py-2 text-right font-medium">Liquid</th>
                <th className="whitespace-nowrap px-3 py-2 text-right font-medium">Net worth</th>
                <th className="whitespace-nowrap px-3 py-2 text-right font-medium">Change</th>
              </tr>
            </thead>
            <tbody>
              {t.rows.map((r: any, i: number) => {
                const older = t.rows[i + 1]
                const change = older ? r.net_worth - older.net_worth : null
                return (
                  <tr key={r.date} className="hover:bg-sunken/40">
                    <td className="sticky left-0 z-10 whitespace-nowrap border-t border-line bg-surface px-3 py-1.5 text-ink2">{r.date}</td>
                    {cols.map((a) => {
                      const c = r.cells[a.id]
                      return (
                        <td key={a.id} className={clsx('whitespace-nowrap border-t border-line px-2 py-1.5 text-right', c?.recorded ? 'font-medium text-ink' : 'text-muted/60')}
                          title={c ? `${byId[a.id]?.name}: ${eurc(c.value)} — ${c.recorded ? c.source || 'recorded' : 'carried forward'}` : ''}>
                          {c ? <span className="inline-flex items-center gap-1">{c.recorded && <span className="h-1.5 w-1.5 rounded-full" style={{ background: SOURCE_COLOR[c.source] ?? 'var(--s-other)' }} />}{eurc(c.value)}</span> : ''}
                        </td>
                      )
                    })}
                    <td className="whitespace-nowrap border-t border-line px-2 py-1.5 text-right text-ink2">{eurc(r.liquid)}</td>
                    <td className="whitespace-nowrap border-t border-line px-3 py-1.5 text-right font-semibold">{eurc(r.net_worth)}</td>
                    <td className={clsx('whitespace-nowrap border-t border-line px-3 py-1.5 text-right', change == null ? '' : change > 0 ? 'text-good' : change < 0 ? 'text-bad' : 'text-muted')}>
                      {change == null ? '' : Math.abs(change) < 0.005 ? '±0' : `${change > 0 ? '+' : '−'}${eurc(Math.abs(change))}`}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        {t.pages > 1 && (
          <nav className="flex items-center justify-center gap-1 border-t border-line px-4 py-2.5" aria-label="Pages">
            <button className="btn-ghost h-8 w-8 px-0" disabled={page <= 1} onClick={() => setPage(page - 1)} aria-label="Newer"><Icon name="chevronL" size={16} /></button>
            {nums.map((n, i) => (
              <Fragment key={n}>
                {i > 0 && n - nums[i - 1] > 1 && <span className="px-1 text-xs text-muted">…</span>}
                <button onClick={() => setPage(n)} aria-current={n === t.page ? 'page' : undefined}
                  className={clsx('h-8 min-w-8 rounded-lg px-2 text-xs tnum transition', n === t.page ? 'bg-accent text-white' : 'text-ink2 hover:bg-sunken')}>{n}</button>
              </Fragment>
            ))}
            <button className="btn-ghost h-8 w-8 px-0" disabled={page >= t.pages} onClick={() => setPage(page + 1)} aria-label="Older"><Icon name="chevronR" size={16} /></button>
          </nav>
        )}
      </Card>
      {picking && (
        <Sheet open onClose={() => setPicking(false)} title="Columns" footer={<>
          <button className="btn-ghost mr-auto" onClick={() => set({ history_hidden: inTable.filter((a) => a.kind === 'property' || a.kind === 'vehicle' || a.archived).map((a) => a.id) })}>Reset</button>
          <button className="btn-primary" onClick={() => setPicking(false)}>Done</button>
        </>}>
          <div className="mb-3 flex gap-2">
            <button className="btn-outline h-8 text-xs" onClick={() => set({ history_hidden: [] })}>Show all</button>
            <button className="btn-outline h-8 text-xs" onClick={() => set({ history_hidden: inTable.filter((a) => !LIQUID_GROUPS.includes(a.group)).map((a) => a.id) })}>Liquid only</button>
          </div>
          <div className="divide-y divide-line">
            {inTable.map((a) => (
              <label key={a.id} className="flex cursor-pointer items-center gap-3 py-2">
                <IconTile name={accountIcon(a)} color={brandColor(a) ?? 'var(--s-other)'} size={28} />
                <span className="min-w-0 flex-1"><span className="block truncate text-sm">{a.name}</span>{a.archived && <span className="text-xs text-muted">hidden account</span>}</span>
                <Toggle checked={!hidden.has(a.id)} onChange={() => toggle(a.id)} />
              </label>
            ))}
          </div>
        </Sheet>
      )}
    </div>
  )
}
