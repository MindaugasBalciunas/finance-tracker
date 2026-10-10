import { Fragment, useEffect, useMemo, useState, type ReactNode } from 'react'
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
import { axisProps, Donut, GroupBar, gridProps, Legend, TooltipBox, type Slice } from '../../components/charts'
import { Icon, IconTile } from '../../components/Icon'
import { accountColors, accountIcon, bankOf, bankRank, brandColor, jitter, volatility } from '../../lib/brand'
import { cleanTerms, LoanFields } from './Loans'

export function Overview() {
  const [range, setRange] = usePeriod('wealth', '3y', RANGES.map((r) => r.value))
  // One chart, three cuts of the same money: asset groups, accounts or banks.
  const [view, setView] = usePeriod('wealth-view', 'groups', ['groups', 'accounts', 'banks'])
  const { prefs, set: setPrefs } = usePrefs()
  const liquidOnly = prefs.liquid_only
  const setLiquidOnly = (v: boolean) => setPrefs({ liquid_only: v })
  const { data: hist, isLoading } = useNetWorthHistory(rangeFrom(range), rangeStep(range))
  // Per-account history too, so switching to Accounts or Banks is instant.
  useNetWorthHistory(rangeFrom(range), rangeStep(range), true)
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
  // Steadiest group at the bottom of the stack, so jumpy cash doesn't lift the rest.
  const stackGroups = useMemo(() => [...groups].map((g) => ({ g, j: jitter((hist ?? []).map((h) => h.by_group[g.id] ?? 0)) })).sort((a, b) => a.j - b.j).map((x) => x.g), [groups, hist])
  const last = hist?.[hist.length - 1]
  const first = hist?.[0]
  const head = last ? (liquidOnly ? last.liquid : last.net_worth) : 0
  const assetSum = last ? groups.filter((g) => g.id !== 'debt').reduce((t, g) => t + Math.max(0, last.by_group[g.id] ?? 0), 0) : 0
  const change = last && first ? head - (liquidOnly ? first.liquid : first.net_worth) : 0

  const byGroup = useMemo(() => {
    const m: Record<string, Account[]> = {}
    for (const a of accounts ?? []) {
      // Hidden accounts leave the list — unless they still hold something
      // (hidden before hiding closed them at €0), so the groups add up.
      if (a.archived && !a.balance) continue
      ;(m[a.group] ||= []).push(a)
    }
    return m
  }, [accounts])

  return (
    <div className="space-y-4">
      <MoneyStrip byGroup={byGroup} liquidOnly={liquidOnly} />
      <section className="card p-4">
        {/* What: the headline and the liquid filter. How: one toolbar for the cut and the period. */}
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="text-sm text-ink2">{liquidOnly ? 'Liquid assets' : 'Net worth'}</div>
            <div className="text-3xl font-semibold tracking-tight">{eur(head)}</div>
            <div className="text-sm text-muted">{rangeLabel(range)} <Delta value={change} /></div>
          </div>
          <span className="pt-1 text-xs"><Toggle checked={liquidOnly} onChange={setLiquidOnly} label="Liquid only" /></span>
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3">
          <Segmented size="sm" value={view} onChange={setView} options={[{ value: 'groups', label: 'Groups' }, { value: 'accounts', label: 'Accounts' }, { value: 'banks', label: 'Banks' }]} />
          <div className="no-scrollbar -mx-4 max-w-[calc(100%+2rem)] overflow-x-auto overflow-y-hidden px-4 sm:mx-0 sm:max-w-none sm:px-0"><Segmented value={range} onChange={setRange} options={RANGES} size="sm" /></div>
        </div>
        {view === 'groups' ? <>
        <ChartFrame donut={last && <GroupDonut byGroup={last.by_group} liquidOnly={liquidOnly} />}>
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
                {stackGroups.map((g) => (
                  <Area key={g.id} type="monotone" dataKey={g.id} name={g.name} stackId="1" stroke="var(--chart-surface)" strokeWidth={1.5}
                    fill={`var(--s${g.slot})`} fillOpacity={0.85} isAnimationActive={false} />
                ))}
                <Line type="monotone" dataKey="net" name={liquidOnly ? 'Liquid' : 'Net worth'} stroke="rgb(var(--ink))" strokeWidth={2} dot={false} isAnimationActive={false} />
              </ComposedChart>
            </ResponsiveContainer>
          )}
        </ChartFrame>
        <div className="mt-3"><Legend items={[...groups.filter((g) => last?.by_group[g.id]).map((g) => ({ color: `var(--s${g.slot})`, label: g.name, value: last ? <>{eurk(last.by_group[g.id] ?? 0)}{g.id !== 'debt' && assetSum > 0 && <span className="text-muted"> · {pct((last.by_group[g.id] ?? 0) / assetSum)}</span>}</> : undefined })),
          { color: 'rgb(var(--ink))', label: liquidOnly ? 'Liquid (line)' : 'Net worth (line)', value: eurk(head) }]} /></div>
        </> : <div className="mt-4"><WhereMoneyIs from={rangeFrom(range)} range={range} liquidOnly={liquidOnly} accounts={accounts ?? []} view={view} /></div>}
      </section>

      <div className="flex items-center justify-between gap-2">
        <h2 className="text-base font-semibold">Accounts</h2>
        <div className="flex gap-2">
          <button className="btn-outline h-9 w-9 px-0 sm:w-auto sm:px-3.5" onClick={() => setNewAcct(true)} aria-label="New account"><Icon name="plus" size={16} /><span className="hidden sm:inline">Account</span></button>
          <button className="btn-primary h-9" onClick={() => setUpdateOpen(true)}><Icon name="refresh" size={16} />Update balances</button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {GROUPS.filter((g) => byGroup[g.id]?.length).map((g) => {
          const sum = byGroup[g.id].reduce((a, x) => a + (x.balance ?? 0), 0)
          // Same base as the bar above: liquid money when "Liquid only" is on.
          const base = stripTotal(byGroup, liquidOnly)
          const inBase = g.id !== 'debt' && (!liquidOnly || LIQUID_GROUPS.includes(g.id))
          return (
            <Card key={g.id} pad={false} title={<span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: `var(--s${g.slot})` }} />{g.name}</span>}
              action={<span className="flex items-baseline gap-2"><span className="tnum text-sm font-semibold">{eurc(sum)}</span>{inBase && base > 0 && sum > 0 && <span className="tnum text-xs text-muted">{pct(sum / base)}</span>}</span>}>
              <div className="divide-y divide-line border-t border-line">
                {byGroup[g.id].map((a) => <AccountRow key={a.id} a={a} share={g.id === 'debt' ? (sum ? Math.abs(a.balance ?? 0) / Math.abs(sum) : 0) : sum > 0 ? Math.max(0, a.balance ?? 0) / sum : 0} color={g.id === 'debt' ? 'rgb(var(--bad))' : brandColor(a) ?? `var(--s${g.slot})`} onClick={() => setAcct(a)} />)}
              </div>
            </Card>
          )
        })}
        {(accounts ?? []).some((a) => a.archived) && (
          <a href="#/settings/accounts" className="text-xs text-muted hover:text-accent lg:col-span-2">{(accounts ?? []).filter((a) => a.archived).length} hidden accounts — manage in Settings</a>
        )}
        {byGroup.other?.length > 0 && (
          <Card pad={false} title="Other"><div className="divide-y divide-line border-t border-line">{byGroup.other.map((a) => <AccountRow key={a.id} a={a} onClick={() => setAcct(a)} />)}</div></Card>
        )}
      </div>
      <Movement from={rangeFrom(range) || (hist?.[0]?.date ?? '')} label={rangeLabel(range)} />
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
function WhereMoneyIs({ from, range, liquidOnly, accounts, view }: { from: string; range: string; liquidOnly: boolean; accounts: Account[]; view: string }) {
  const { data: hist, isLoading } = useNetWorthHistory(from, rangeStep(range), true)
  const { prefs, set } = usePrefs()
  const off = new Set(prefs.hidden_accounts ?? [])
  const toggle = (ids: string[]) => {
    const next = new Set(off)
    const allOff = ids.every((id) => next.has(id))
    for (const id of ids) allOff ? next.delete(id) : next.add(id)
    set({ hidden_accounts: [...next] })
  }
  // Every account that held money at some point in the range — a closed or
  // archived one still shows for the months it had a balance, so the past
  // stays true; today's split only counts what is held now.
  const eligible = useMemo(() => {
    const held = new Set<string>()
    for (const h of hist ?? []) for (const [id, v] of Object.entries(h.by_account ?? {})) if ((v as number) > 0) held.add(id)
    return accounts.filter((a) => a.kind !== 'loan' && (!liquidOnly || LIQUID_GROUPS.includes(a.group)) && (((a.balance ?? 0) > 0 && !a.archived) || held.has(a.id)))
  }, [accounts, liquidOnly, hist])
  const colors = useMemo(() => accountColors(eligible), [eligible])

  // Series: one per account, or one per bank. Order by stability (steadiest
  // first = bottom of the stack); accounts grouped under their bank.
  const series = useMemo(() => {
    const vals = (ids: string[]) => (hist ?? []).map((h) => ids.reduce((t, id) => t + Math.max(0, h.by_account?.[id] ?? 0), 0))
    const banks = new Map<string, Account[]>()
    for (const a of eligible) banks.set(bankOf(a), [...(banks.get(bankOf(a)) ?? []), a])
    const bankList = [...banks.entries()].map(([name, accts]) => ({ name, accts, vol: volatility(vals(accts.map((x) => x.id))), jit: jitter(vals(accts.map((x) => x.id))) }))
      .sort((x, y) => bankRank(x.accts[0]) - bankRank(y.accts[0]) || x.vol - y.vol)
    if (view === 'banks') {
      // Steadiest bank at the bottom of the stack (and first in the legend).
      return [...bankList].sort((x, y) => x.jit - y.jit).map((b) => {
        const lead = [...b.accts].sort((x, y) => (y.balance ?? 0) - (x.balance ?? 0))[0]
        return { key: 'bank:' + b.name, name: b.name, ids: b.accts.map((x) => x.id), color: brandColor(lead) ?? colors[lead.id], value: b.accts.reduce((t, x) => t + (x.balance ?? 0), 0), bank: b.name, icon: accountIcon(lead), jit: b.jit }
      })
    }
    return bankList.flatMap((b) => b.accts.map((x) => ({ a: x, vol: volatility(vals([x.id])) })).sort((x, y) => x.vol - y.vol)
      .map(({ a }) => ({ key: a.id, name: a.name, ids: [a.id], color: colors[a.id], value: a.balance ?? 0, bank: b.name, icon: accountIcon(a), jit: jitter(vals([a.id])) })))
  }, [hist, eligible, colors, view])

  // Amounts count only switched-on accounts, so the total is the same in both views.
  const bal = useMemo(() => Object.fromEntries(eligible.map((a) => [a.id, Math.max(0, a.balance ?? 0)])), [eligible])
  const valueOf = (x: { ids: string[] }) => x.ids.filter((id) => !off.has(id)).reduce((t, id) => t + (bal[id] ?? 0), 0)
  const shown = series.filter((x) => !x.ids.every((id) => off.has(id)))
  // The table: one block per bank (accounts view) or one block of banks.
  const blocks = useMemo(() => {
    if (view !== 'accounts') { // banks: up to three even blocks, one per column
      const n = Math.ceil(series.length / 3) || 1
      return Array.from({ length: Math.ceil(series.length / n) }, (_, i) => ({ bank: String(i), items: series.slice(i * n, i * n + n) }))
    }
    const out: { bank: string; items: typeof series }[] = []
    for (const x of series) {
      if (out.length && out[out.length - 1].bank === x.bank) out[out.length - 1].items.push(x)
      else out.push({ bank: x.bank, items: [x] })
    }
    return out
  }, [series, view])
  // The stack is drawn steadiest-first (bottom), whatever order the legend uses.
  const stack = [...shown].sort((x, y) => x.jit - y.jit)
  const rows = useMemo(() => (hist ?? []).map((h) => {
    const row: any = { date: h.date }
    for (const x of shown) row[x.key] = x.ids.filter((id) => !off.has(id)).reduce((t, id) => t + Math.max(0, h.by_account?.[id] ?? 0), 0)
    return row
  }), [hist, shown, off])
  if (isLoading) return <ChartFrame><Loading /></ChartFrame>
  if (!eligible.length) return <ChartFrame><div className="grid h-full place-items-center text-sm text-muted">No account balances yet.</div></ChartFrame>
  const total = shown.reduce((t, x) => t + valueOf(x), 0)
  const slices: Slice[] = shown.map((x) => ({ key: x.key, label: x.name, value: valueOf(x), color: x.color })).filter((x) => x.value > 0)
  return (
    <>
      <ChartFrame donut={shown.length > 0 && <Donut slices={slices} center={eurk(total)} sub="today" height={200} legend={false} />}>
        {!shown.length ? <div className="grid h-full place-items-center text-sm text-muted">Everything is switched off — tap one below.</div> : (
          <ResponsiveContainer>
            <AreaChart data={rows} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid {...gridProps} />
              <XAxis dataKey="date" {...axisProps} tickFormatter={rangeTick(range)} minTickGap={40} />
              <YAxis {...axisProps} tickFormatter={eurk} width={48} />
              <Tooltip content={({ active, payload, label }) => active && payload?.length ? (
                <TooltipBox title={shortDate(label)} rows={[...stack.filter((x) => payload[0].payload[x.key] > 0).map((x) => ({ color: x.color, label: x.name, value: eur(payload[0].payload[x.key]) })).reverse(),
                  { label: 'Total', value: eur(shown.reduce((t, x) => t + (payload[0].payload[x.key] ?? 0), 0)), bold: true }]} />) : null} />
              {stack.map((x) => (
                <Area key={x.key} type="monotone" dataKey={x.key} name={x.name} stackId="a" stroke="var(--chart-surface)" strokeWidth={1.5} fill={x.color} fillOpacity={0.9} isAnimationActive={false} />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        )}
      </ChartFrame>
      <div className="mt-4 mb-2 text-xs text-muted"><b className="tnum text-ink">{eur(total)}</b> in {shown.reduce((t, x) => t + x.ids.filter((id) => !off.has(id) && bal[id] > 0).length, 0)} {liquidOnly ? 'liquid ' : ''}accounts{view === 'banks' ? ` in ${shown.length} ${shown.length === 1 ? 'place' : 'places'}` : ''} · tap one to leave it out of the chart</div>
      {/* The legend is the switchboard: a compact table of every account (or
          bank) — amount and share of what is shown; tap a row to leave it out. */}
      <div className="columns-1 gap-3 sm:columns-2 lg:columns-3">
        {blocks.map((blk) => (
          <div key={blk.bank + blk.items[0].key} className="mb-3 break-inside-avoid divide-y divide-line overflow-hidden rounded-lg border border-line">
            {view === 'accounts' && <div className="bg-sunken/40 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-muted">{blk.bank}</div>}
            {blk.items.map((x) => {
          const on = !x.ids.every((id) => off.has(id))
          const onCount = x.ids.filter((id) => !off.has(id)).length
          const v = on ? valueOf(x) : x.value
          return (
            <Fragment key={x.key}>
              <button type="button" onClick={() => toggle(x.ids)} aria-pressed={on}
                className={clsx('grid w-full grid-cols-[auto_1fr_auto_2.25rem] items-center gap-2 px-2.5 py-1.5 text-left text-xs transition hover:bg-sunken/50', !on && 'opacity-50')}>
                <span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: on ? x.color : 'var(--s-other)' }} />
                <span className={clsx('min-w-0 truncate', on ? 'text-ink' : 'text-muted line-through')}>
                  {x.name}{x.ids.every((id) => !(bal[id] > 0)) && <span className="text-muted"> · closed</span>}{x.ids.length > 1 && <span className="text-muted"> · {onCount < x.ids.length ? `${onCount} of ${x.ids.length}` : `${x.ids.length} accounts`}</span>}
                </span>
                <span className="tnum font-medium text-ink">{eurk(v)}</span>
                <span className="text-right tnum text-muted">{on && total > 0 ? `${Math.round((v / total) * 100)}%` : '—'}</span>
              </button>
            </Fragment>
          )
        })}
          </div>
        ))}
      </div>
    </>
  )
}

/** Same footprint in every view, so switching Groups / Accounts / Banks never
 *  moves the page: a fixed-height chart with the donut beside (below on phones). */
function ChartFrame({ children, donut }: { children: ReactNode; donut?: ReactNode }) {
  return (
    <div className="mt-4 grid grid-cols-1 items-center gap-4 lg:grid-cols-[1fr_300px]">
      <div className="h-64 sm:h-80">{children}</div>
      <div className="grid h-[200px] place-items-center">{donut}</div>
    </div>
  )
}

/** Where the money sits now, by asset group (debt is not a slice). */
function GroupDonut({ byGroup, liquidOnly }: { byGroup: Record<string, number>; liquidOnly: boolean }) {
  const slices: Slice[] = GROUPS.filter((g) => g.id !== 'debt' && (!liquidOnly || LIQUID_GROUPS.includes(g.id)))
    .map((g) => ({ key: g.id, label: g.name, value: Math.max(0, byGroup[g.id] ?? 0), color: `var(--s${g.slot})` })).filter((x) => x.value > 0)
  const total = slices.reduce((a, x) => a + x.value, 0)
  if (!total) return null
  return <Donut slices={slices} center={eurk(total)} sub={liquidOnly ? 'liquid today' : 'assets today'} height={200} legend={false} />
}

// Group totals as Home and net worth count them (a negative account lowers its group).
const stripTotal = (byGroup: Record<string, Account[]>, liquidOnly: boolean) =>
  GROUPS.filter((g) => g.id !== 'debt' && (!liquidOnly || LIQUID_GROUPS.includes(g.id)))
    .reduce((t, g) => t + Math.max(0, (byGroup[g.id] ?? []).reduce((a, x) => a + (x.balance ?? 0), 0)), 0)

/** Where the money is, at a glance — the same bar as on Home. The total
 *  is the chart's headline right below, so it isn't repeated here. */
function MoneyStrip({ byGroup, liquidOnly }: { byGroup: Record<string, Account[]>; liquidOnly: boolean }) {
  const parts = GROUPS.filter((g) => g.id !== 'debt' && (!liquidOnly || LIQUID_GROUPS.includes(g.id)))
    .map((g) => ({ g, v: Math.max(0, (byGroup[g.id] ?? []).reduce((a, x) => a + (x.balance ?? 0), 0)) })).filter((x) => x.v > 0)
    .sort((a, b) => b.v - a.v)
  const total = parts.reduce((t, x) => t + x.v, 0)
  const debt = liquidOnly ? 0 : Math.abs((byGroup.debt ?? []).reduce((a, x) => a + Math.min(0, x.balance ?? 0), 0))
  if (!total) return null
  return (
    <section className="card px-4 py-3 sm:px-6">
      <GroupBar parts={parts.map(({ g, v }) => ({ id: g.id, name: g.name, slot: g.slot, v }))} debt={debt} />
    </section>
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

function AccountRow({ a, onClick, share, color }: { a: Account; onClick: () => void; share?: number; color?: string }) {
  const age = staleDays(a.balance_date)
  const stale = age > 45 && !['property', 'vehicle', 'loan'].includes(a.kind) && !!a.balance
  return (
    <button onClick={onClick} className={clsx('flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/50', a.archived && 'opacity-50')}>
      <IconTile name={accountIcon(a)} color={brandColor(a) ?? `var(--s${GROUPS.find((g) => g.id === a.group)?.slot ?? 1})`} size={34} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{a.name}</div>
        <div className={clsx('truncate text-xs', stale ? 'text-warn' : 'text-muted')}>
          {[a.institution !== a.name && a.institution, a.quantity ? `${a.quantity} units` : '', a.balance_date ? (stale ? `${age} days old` : shortDate(a.balance_date)) : 'no balance', a.source === 'bank' ? 'from bank' : a.source === 'computed' ? 'estimated' : a.source === 'closed' ? 'closed' : '', a.archived && a.balance ? 'hidden but still counted — close it in Settings' : ''].filter(Boolean).join(' · ')}
        </div>
      </div>
      <div className="flex min-w-[6rem] shrink-0 flex-col items-end gap-1">
        <div className="whitespace-nowrap tnum text-sm font-semibold">{a.balance != null ? eurc(a.balance) : '—'}</div>
        {share != null && share > 0 && (
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-sunken" title={`${pct(share)} of the group`}>
            <div className="h-full rounded-full" style={{ width: `${Math.max(3, Math.min(100, share * 100))}%`, background: color }} />
          </div>
        )}
      </div>
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
            <span className="text-ink2">{p.date}{p.at && <span className="text-xs text-muted"> {new Date(p.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })}</span>} <span className="text-xs text-muted">{p.source}</span></span>
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
        {['checking', 'savings', 'cash', 'brokerage', 'other'].includes(v.kind ?? '') && (() => {
          const d = safeParse(details)
          return (
            <Field label="IBAN" hint="For accounts your bank doesn't sync (e.g. a savings account): transfers to and from it are then recognised as yours and keep its balance up to date.">
              <input className="input font-mono text-xs" value={d.iban ?? ''} placeholder="LT00 0000 0000 0000 0000" autoComplete="off" spellCheck={false}
                onChange={(e) => setDetails(JSON.stringify({ ...d, iban: e.target.value.replace(/\s/g, '').toUpperCase() || undefined }, null, 2))} />
            </Field>
          )
        })()}
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
