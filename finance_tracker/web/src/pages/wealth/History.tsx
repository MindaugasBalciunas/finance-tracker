import { Fragment, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useAccounts, usePrefs } from '../../lib/hooks'
import { LIQUID_GROUPS } from '../../lib/categories'
import { eurc } from '../../lib/format'
import { Card, Loading, Sheet, Spinner, Toggle } from '../../components/ui'
import { Icon, IconTile } from '../../components/Icon'
import { accountIcon, brandColor } from '../../lib/brand'

// ── balance history ─────────────────────────────────────────────────

const SOURCE_COLOR: Record<string, string> = { bank: 'rgb(var(--accent))', manual: 'var(--s6)', import: 'var(--s-other)', computed: 'var(--s4)', market: 'var(--s3)', broker: 'var(--s5)' }
const hhmm = (iso?: string) => (iso ? new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }) : '')

/** Every account on every snapshot date, 50 dates a page, newest first.
 *  Recorded values in full ink with their source; carried-forward values grey. */
export function BalanceHistory() {
  const [page, setPage] = useState(1)
  const [picking, setPicking] = useState(false)
  const [open, setOpen] = useState<Set<string>>(new Set())
  const flip = (d: string) => setOpen((o) => { const n = new Set(o); n.has(d) ? n.delete(d) : n.add(d); return n })
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
          <span><b className="text-ink">Bold</b> = recorded that day (with the time when known) · <span className="opacity-60">grey</span> = carried forward · tap a day with several readings to see each</span>
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
                // Readings of the columns on screen (hidden accounts would only
                // add empty rows).
                const readings: any[] = (r.readings ?? []).filter((x: any) => cols.some((a) => a.id === x.account_id))
                const latest = Object.values(r.cells as Record<string, any>).map((c) => c.at).filter(Boolean).sort().pop()
                const expanded = open.has(r.date)
                return (
                  <Fragment key={r.date}>
                  <tr className="hover:bg-sunken/40">
                    <td className="sticky left-0 z-10 whitespace-nowrap border-t border-line bg-surface px-3 py-1.5 text-ink2">
                      {readings.length > 1 ? (
                        <button className="inline-flex items-center gap-1 text-left" onClick={() => flip(r.date)} aria-expanded={expanded}>
                          <Icon name={expanded ? 'chevronD' : 'chevronR'} size={12} />
                          <span>{r.date}<span className="block text-[10px] text-muted">{readings.length} readings · last {hhmm(latest)}</span></span>
                        </button>
                      ) : (
                        <span>{r.date}{latest && <span className="block text-[10px] text-muted">at {hhmm(latest)}</span>}</span>
                      )}
                    </td>
                    {cols.map((a) => {
                      const c = r.cells[a.id]
                      return (
                        <td key={a.id} className={clsx('whitespace-nowrap border-t border-line px-2 py-1.5 text-right', c?.recorded ? 'font-medium text-ink' : 'text-muted/60')}
                          title={c ? `${byId[a.id]?.name}: ${eurc(c.value)} — ${c.recorded ? (c.source || 'recorded') + (c.at ? ` at ${hhmm(c.at)}` : '') : 'carried forward'}` : ''}>
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
                  {expanded && (() => {
                    // One row per moment (same minute), newest first, each value
                    // under its own account column with the change since that
                    // account's previous reading.
                    const prevOf = new Map<number, number>()
                    const last: Record<string, number> = {}
                    readings.forEach((x, j) => { if (x.account_id in last) prevOf.set(j, last[x.account_id]); last[x.account_id] = x.value })
                    const groups: { at: string; items: { x: any; d: number | null }[] }[] = []
                    readings.forEach((x, j) => {
                      const key = hhmm(x.at)
                      const g = groups.find((g) => hhmm(g.at) === key)
                      const item = { x, d: prevOf.has(j) ? x.value - prevOf.get(j)! : null }
                      if (g && !g.items.some((i) => i.x.account_id === x.account_id)) g.items.push(item)
                      else groups.push({ at: x.at, items: [item] })
                    })
                    return groups.reverse().map((g, gi) => (
                      <tr key={gi} className="bg-sunken/30 text-[11px]">
                        <td className="sticky left-0 z-10 whitespace-nowrap bg-sunken px-3 py-1 pl-7 text-muted">{hhmm(g.at)}</td>
                        {cols.map((a) => {
                          const it = g.items.find((i) => i.x.account_id === a.id)
                          return (
                            <td key={a.id} className="whitespace-nowrap px-2 py-1 text-right" title={it ? `${a.name} at ${hhmm(g.at)} · ${it.x.source}` : ''}>
                              {it && (
                                <span className="inline-flex flex-col items-end leading-tight">
                                  <span className="inline-flex items-center gap-1 text-ink"><span className="h-1.5 w-1.5 rounded-full" style={{ background: SOURCE_COLOR[it.x.source] ?? 'var(--s-other)' }} />{eurc(it.x.value)}</span>
                                  {it.d != null && Math.abs(it.d) >= 0.005 && <span className={it.d > 0 ? 'text-good' : 'text-bad'}>{it.d > 0 ? '+' : '−'}{eurc(Math.abs(it.d))}</span>}
                                </span>
                              )}
                            </td>
                          )
                        })}
                        <td colSpan={3} />
                      </tr>
                    ))
                  })()}
                  </Fragment>
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
