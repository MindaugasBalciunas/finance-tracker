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

export function cryptoSubtitle(b: Balance, btcPrice: number | null): string {
  const totalBtc = (b.r_btc + b.m_btc).toFixed(8)
  return btcPrice != null
    ? `${totalBtc} BTC · €${btcPrice.toLocaleString()} /BTC`
    : `${totalBtc} BTC`
}
