import { useMemo } from 'react'
import { useAccounts, useAccountName } from '../hooks/useAccounts'
import { ACCOUNT_LABELS, type Account, type AccountGroup, type Balance } from '../types'

// One field of a balance snapshot, placed under its bank. Built-in accounts
// read their column, added ones their extra value, and the two BTC holdings
// (not accounts — priced live) sit under Revolut as crypto.
export type SnapshotField = {
  key: string
  label: string
  group: AccountGroup
  institution: string
  kind: 'eur' | 'btc'
  builtin: boolean
  archived: boolean
  // EUR value in a snapshot (BTC fields: the live EUR value).
  value: (b: Balance) => number
}

export type BankSection = { institution: string; fields: SnapshotField[] }

const GROUP_ORDER: AccountGroup[] = ['cash', 'investments', 'pensions', 'crypto', 'other']
// Banks first in a familiar order, anything else alphabetically, cash in
// hand last, unassigned at the very end.
const BANK_ORDER = ['SEB', 'Swedbank', 'Revolut', 'IBKR', 'Artea', 'Luminor']

const DEFAULT_INSTITUTION: Record<string, string> = {
  seb: 'SEB', seb_pen: 'SEB', swed: 'Swedbank', swed_etf: 'Swedbank',
  rev_m: 'Revolut', rev_r: 'Revolut', rev_stocks: 'Revolut',
  ibkr_stocks: 'IBKR', art: 'Artea', cash: 'Cash', luminor: 'Luminor',
}
const DEFAULT_GROUP: Record<string, AccountGroup> = {
  seb: 'cash', swed: 'cash', cash: 'cash', rev_m: 'cash', rev_r: 'cash',
  swed_etf: 'investments', rev_stocks: 'investments', ibkr_stocks: 'investments',
  seb_pen: 'pensions', art: 'pensions', luminor: 'other',
}
const BUILTIN_KEYS = Object.keys(ACCOUNT_LABELS) as (keyof typeof ACCOUNT_LABELS)[]

export function bankRank(name: string): [number, string] {
  const i = BANK_ORDER.findIndex((b) => b.toLowerCase() === name.toLowerCase())
  if (i >= 0) return [i, name]
  if (name === 'Cash') return [900, name]
  if (name === '') return [999, name]
  return [100, name.toLowerCase()]
}

export function buildBankSections(accounts: Account[], name: (key: string, fallback?: string) => string): BankSection[] {
  const byKey = new Map(accounts.map((a) => [a.key, a]))
  const fields: SnapshotField[] = []
  for (const key of BUILTIN_KEYS) {
    const a = byKey.get(key)
    fields.push({
      key, label: name(key), kind: 'eur', builtin: true, archived: false,
      group: a?.group ?? DEFAULT_GROUP[key],
      institution: a?.institution || DEFAULT_INSTITUTION[key],
      value: (b) => (b as any)[key] ?? 0,
    })
  }
  for (const a of accounts.filter((x) => !x.builtin)) {
    fields.push({
      key: a.key, label: a.label, kind: 'eur', builtin: false, archived: a.archived,
      group: a.group, institution: a.institution ?? '',
      value: (b) => b.extra?.[a.key] ?? 0,
    })
  }
  fields.push(
    { key: 'r_btc', label: 'R BTC', kind: 'btc', builtin: true, archived: false, group: 'crypto', institution: 'Revolut', value: (b) => b.r_btc_eur ?? 0 },
    { key: 'm_btc', label: 'M BTC', kind: 'btc', builtin: true, archived: false, group: 'crypto', institution: 'Revolut', value: (b) => b.m_btc_eur ?? 0 },
  )
  const order = new Map(accounts.map((a) => [a.key, a.sort_order]))
  const banks = new Map<string, SnapshotField[]>()
  for (const f of fields) banks.set(f.institution, [...(banks.get(f.institution) ?? []), f])
  return [...banks.entries()]
    .sort(([a], [b]) => {
      const [ra, na] = bankRank(a)
      const [rb, nb] = bankRank(b)
      return ra - rb || na.localeCompare(nb)
    })
    .map(([institution, fs]) => ({
      institution,
      fields: fs.sort((x, y) =>
        GROUP_ORDER.indexOf(x.group) - GROUP_ORDER.indexOf(y.group)
        || (x.kind === y.kind ? 0 : x.kind === 'eur' ? -1 : 1)
        || (order.get(x.key) ?? 50) - (order.get(y.key) ?? 50)),
    }))
}

// useBankSections groups every snapshot field by bank, in a stable order.
export function useBankSections(): BankSection[] {
  const { data } = useAccounts()
  const name = useAccountName()
  return useMemo(() => buildBankSections(data ?? [], name), [data, name])
}
