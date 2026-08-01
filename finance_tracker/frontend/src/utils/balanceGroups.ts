import type { Balance } from '../types'

export function freeCash(b: Balance): number {
  return b.seb + b.swed + b.cash + b.rev_m + b.rev_r
}

export function investments(b: Balance): number {
  return b.swed_etf + b.rev_stocks + b.ibkr_stocks
}

export function pensions(b: Balance): number {
  return b.seb_pen + b.art
}

export function cryptoEur(b: Balance): number {
  return (b.r_btc_eur ?? 0) + (b.m_btc_eur ?? 0)
}

export interface AccountDef {
  key: string
  label: string
  value: (b: Balance) => number
}

export interface GroupDef {
  key: string
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
    key: 'Free cash', color: '#22c55e', total: freeCash,
    accounts: [
      { key: 'seb', label: 'SEB', value: (b) => b.seb },
      { key: 'swed', label: 'Swedbank', value: (b) => b.swed },
      { key: 'cash', label: 'Cash', value: (b) => b.cash },
      { key: 'rev_m', label: 'Revolut M', value: (b) => b.rev_m },
      { key: 'rev_r', label: 'Revolut R', value: (b) => b.rev_r },
    ],
  },
  {
    key: 'Investments', color: '#3b82f6', total: investments,
    accounts: [
      { key: 'ibkr_stocks', label: 'IBKR stocks', value: (b) => b.ibkr_stocks },
      { key: 'swed_etf', label: 'Swedbank ETF', value: (b) => b.swed_etf },
      { key: 'rev_stocks', label: 'Rev M stocks', value: (b) => b.rev_stocks },
    ],
  },
  {
    key: 'Pensions', color: '#6d28d9', total: pensions,
    accounts: [
      { key: 'seb_pen', label: 'SEB pension', value: (b) => b.seb_pen },
      { key: 'art', label: 'Artea pension', value: (b) => b.art },
    ],
  },
  {
    key: 'Crypto', color: '#f59e0b', total: cryptoEur,
    accounts: [
      { key: 'm_btc', label: 'M BTC (€)', value: (b) => b.m_btc_eur ?? 0 },
      { key: 'r_btc', label: 'R BTC (€)', value: (b) => b.r_btc_eur ?? 0 },
    ],
  },
]

// Accounts outside every group (currently only Luminor) fold into a gray
// "Other" bucket, mirroring the hero bar's leftover segment.
export const OTHER_COLOR = '#9ca3af'
export const OTHER_ACCOUNTS: AccountDef[] = [
  { key: 'luminor', label: 'Luminor', value: (b) => b.luminor },
]

export function cryptoSubtitle(b: Balance, btcPrice: number | null): string {
  const totalBtc = (b.r_btc + b.m_btc).toFixed(8)
  return btcPrice != null
    ? `${totalBtc} BTC · €${btcPrice.toLocaleString()} /BTC`
    : `${totalBtc} BTC`
}
