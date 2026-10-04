import type { Account, AccountGroup, Balance } from '../types'

export function freeCash(b: Balance): number {
  if (b.groups) return b.groups.cash ?? 0
  return b.seb + b.swed + b.cash + b.rev_m + b.rev_r + (b.extra_groups?.cash ?? 0)
}

export function investments(b: Balance): number {
  if (b.groups) return b.groups.investments ?? 0
  return b.swed_etf + b.rev_stocks + b.ibkr_stocks + (b.extra_groups?.investments ?? 0)
}

export function pensions(b: Balance): number {
  if (b.groups) return b.groups.pensions ?? 0
  return b.seb_pen + b.art + (b.extra_groups?.pensions ?? 0)
}

export function cryptoEur(b: Balance): number {
  const accounts = b.groups ? b.groups.crypto ?? 0 : b.extra_groups?.crypto ?? 0
  return (b.r_btc_eur ?? 0) + (b.m_btc_eur ?? 0) + accounts
}

export interface AccountDef {
  key: string
  label: string
  value: (b: Balance) => number
}

export interface GroupDef {
  key: string
  group: AccountGroup
  color: string
  total: (b: Balance) => number
  accounts: AccountDef[]
}

// The single source of truth for the group palette and account membership —
// the Dashboard hero bar and the Balances allocation cards all draw from it.
// Pensions is violet-700 (#6d28d9), not purple-500: adjacent to the
// investments blue in stacked bars, purple-500 was indistinguishable under
// deuteranopia (ΔE 0.9); the darker step separates by lightness (ΔE 10.9).
export const GROUPS: GroupDef[] = [
  {
    key: 'Free cash', group: 'cash', color: '#22c55e', total: freeCash,
    accounts: [
      { key: 'seb', label: 'SEB', value: (b) => b.seb },
      { key: 'swed', label: 'Swedbank', value: (b) => b.swed },
      { key: 'cash', label: 'Cash', value: (b) => b.cash },
      { key: 'rev_m', label: 'Revolut M', value: (b) => b.rev_m },
      { key: 'rev_r', label: 'Revolut R', value: (b) => b.rev_r },
    ],
  },
  {
    key: 'Investments', group: 'investments', color: '#3b82f6', total: investments,
    accounts: [
      { key: 'ibkr_stocks', label: 'IBKR stocks', value: (b) => b.ibkr_stocks },
      { key: 'swed_etf', label: 'Swedbank ETF', value: (b) => b.swed_etf },
      { key: 'rev_stocks', label: 'Rev M stocks', value: (b) => b.rev_stocks },
    ],
  },
  {
    key: 'Pensions', group: 'pensions', color: '#6d28d9', total: pensions,
    accounts: [
      { key: 'seb_pen', label: 'SEB pension', value: (b) => b.seb_pen },
      { key: 'art', label: 'Artea pension', value: (b) => b.art },
    ],
  },
  {
    key: 'Crypto', group: 'crypto', color: '#f59e0b', total: cryptoEur,
    accounts: [
      { key: 'm_btc', label: 'M BTC (€)', value: (b) => b.m_btc_eur ?? 0 },
      { key: 'r_btc', label: 'R BTC (€)', value: (b) => b.r_btc_eur ?? 0 },
    ],
  },
]

// Accounts outside every group (currently only Luminor) fold into a gray
// "Other" bucket, mirroring the hero bar's leftover segment.
export const OTHER_COLOR = '#9ca3af'

// One color per account type, everywhere a type is shown.
export const GROUP_COLORS: Record<AccountGroup, string> = {
  cash: '#22c55e',
  investments: '#3b82f6',
  pensions: '#6d28d9',
  crypto: '#f59e0b',
  other: OTHER_COLOR,
}
export const OTHER_ACCOUNTS: AccountDef[] = [
  { key: 'luminor', label: 'Luminor', value: (b) => b.luminor },
]

export function cryptoSubtitle(b: Balance, btcPrice: number | null): string {
  const totalBtc = (b.r_btc + b.m_btc).toFixed(8)
  return btcPrice != null
    ? `${totalBtc} BTC · €${btcPrice.toLocaleString()} /BTC`
    : `${totalBtc} BTC`
}

// withAddedAccounts builds the per-account breakdown: added accounts join
// their group, a built-in the user moved goes to its new group (groupOf),
// and renamed ones show their new name. Group totals already follow the same
// rules on the server (Balance.groups); this is only the per-account list.
export function withAddedAccounts(
  added: Account[],
  name?: (key: string, fallback?: string) => string,
  groupOf?: (key: string) => AccountGroup | undefined,
): { groups: GroupDef[]; other: AccountDef[] } {
  const def = (a: Account): AccountDef => ({ key: a.key, label: a.label, value: (b) => b.extra?.[a.key] ?? 0 })
  const rename = (a: AccountDef): AccountDef => (name ? { ...a, label: name(a.key, a.label) } : a)
  const builtins: { def: AccountDef; group: AccountGroup }[] = [
    ...GROUPS.flatMap((g) => g.accounts.map((a) => ({ def: a, group: g.group }))),
    ...OTHER_ACCOUNTS.map((a) => ({ def: a, group: 'other' as AccountGroup })),
  ].map((x) => ({ def: rename(x.def), group: groupOf?.(x.def.key) ?? x.group }))
  const inGroup = (g: AccountGroup) => [
    ...builtins.filter((x) => x.group === g).map((x) => x.def),
    ...added.filter((a) => a.group === g).map(def),
  ]
  return {
    groups: GROUPS.map((g) => ({ ...g, accounts: inGroup(g.group) })),
    other: inGroup('other'),
  }
}
