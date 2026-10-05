import { Fragment, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { Area, AreaChart, CartesianGrid, ComposedChart, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api } from '../../lib/api'
import { useAccounts, useNetWorthHistory, usePeriod, usePrefs, useRefresh } from '../../lib/hooks'
import { GROUPS, LIQUID_GROUPS } from '../../lib/categories'
import { eur, eurc, eurk, parseNum, pct, shortDate, todayISO } from '../../lib/format'
import { RANGES, rangeFrom, rangeLabel, rangeStep, rangeTick } from '../../lib/periods'
import type { Account } from '../../lib/types'
import { Card, Delta, ErrorBox, Field, Loading, NumberInput, Segmented, Sheet, Toggle, useToast } from '../../components/ui'
import { axisProps, Donut, gridProps, Legend, TooltipBox, type Slice } from '../../components/charts'
import { Icon, IconTile } from '../../components/Icon'
import { accountColors, accountIcon, bankOf, bankRank, brandColor, volatility } from '../../lib/brand'
import { cleanTerms, LoanFields } from './Loans'

export function Overview() {
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
        {['property', 'vehicle'].includes(v.kind ?? '') && (() => {
          const d = safeParse(details)
          const put = (k: string, val: unknown) => setDetails(JSON.stringify({ ...d, [k]: val === '' || val === undefined ? undefined : val }, null, 2))
          return (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <Field label="Purchased on"><input type="date" className="input" value={d.purchase_date ?? ''} onChange={(e) => put('purchase_date', e.target.value)} /></Field>
                <Field label="Purchase price €"><NumberInput value={d.purchase_price} onChange={(n) => put('purchase_price', n)} /></Field>
              </div>
              <Field label={v.kind === 'vehicle' ? 'Description' : 'Address'}><input className="input" value={d.address ?? ''} onChange={(e) => put('address', e.target.value)} /></Field>
            </div>
          )
        })()}
        <ErrorBox error={save.error || del.error} />
      </div>
    </Sheet>
  )
}
