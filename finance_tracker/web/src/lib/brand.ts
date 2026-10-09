import type { Account } from './types'

// Institution colours, so a Swedbank account is orange and a SEB one green
// in every chart. Several accounts at one bank get shades of its colour.
// Checked with the dataviz validator (all pairs, normal vision ΔE ≥ 15)
// except Swedbank↔SEB under red/green CVD, which the brands fix; the legend
// and stack gaps carry identity there.
const BRANDS: [RegExp, string][] = [
  [/swed/i, '#ee7023'],
  [/\bseb\b/i, '#41a62a'],
  [/revolut/i, '#2f6fde'],
  [/ibkr|interactive/i, '#d81b2a'],
  [/artea|šiauli|siauli/i, '#ec6fb0'],
  [/luminor/i, '#5a3fa0'],
  [/btc|bitcoin/i, '#e0b400'], // gold, not Bitcoin orange: it sat too close to Swedbank
  [/\bcash\b/i, '#0f9fb0'],
]
// Accounts without a bank (house, car, solar…) by kind.
const KIND: Record<string, string> = { property: 'var(--s5)', vehicle: 'var(--s8)', cash: '#0f9fb0', loan: 'var(--s7)' }

export function brandColor(a: Pick<Account, 'institution' | 'name' | 'kind'>): string | undefined {
  const hay = `${a.institution ?? ''} ${a.name}`
  return BRANDS.find(([re]) => re.test(hay))?.[1] ?? KIND[a.kind]
}

// Shade n of a base colour: the biggest account keeps the brand colour, the
// next ones step lighter/darker so they stay distinct when stacked.
const SHADES = ['', 'white 32%', 'black 28%', 'white 55%', 'black 45%']
const shade = (c: string, n: number) => (n === 0 ? c : `color-mix(in oklab, ${c}, ${SHADES[n % SHADES.length]})`)

/** A stable colour per account: brand shade, else a palette slot. */
export function accountColors(accounts: Account[]): Record<string, string> {
  const out: Record<string, string> = {}
  const used: Record<string, number> = {}
  let slot = 1
  for (const a of [...accounts].sort((x, y) => Math.abs(y.balance ?? 0) - Math.abs(x.balance ?? 0))) {
    const base = brandColor(a)
    if (base) {
      const n = used[base] ?? 0
      used[base] = n + 1
      out[a.id] = shade(base, n)
    } else {
      out[a.id] = `var(--s${((slot++ - 1) % 8) + 1})`
    }
  }
  return out
}

/** Icon for an account kind. */
export const KIND_ICON: Record<string, string> = {
  checking: 'bank', savings: 'piggy', cash: 'wallet', brokerage: 'trend', pension: 'umbrella', crypto: 'bitcoin',
  property: 'home', vehicle: 'car', loan: 'card', other: 'dots',
}
export const accountIcon = (a: Pick<Account, 'kind' | 'name'>) => (/solar/i.test(a.name) ? 'sun2' : KIND_ICON[a.kind] ?? 'dots')

/** The bank (or kind) an account belongs to, for grouping. */
export function bankOf(a: Pick<Account, 'institution' | 'name' | 'kind'>): string {
  // A house or car is not money held at a bank, even when the bank lent for it.
  if (a.kind === 'property') return 'Property'
  if (a.kind === 'vehicle') return 'Vehicles'
  if (a.institution?.trim()) return a.institution.trim()
  if (/btc|bitcoin/i.test(a.name)) return 'Bitcoin'
  return ({ cash: 'Cash', property: 'Property', vehicle: 'Vehicles', loan: 'Loans' } as Record<string, string>)[a.kind] ?? 'Other'
}

/** Coefficient of variation: 0 = never moves. Used to put steady money at the base. */
export function volatility(values: number[]): number {
  const v = values.filter((x) => Number.isFinite(x))
  if (v.length < 2) return 0
  const mean = v.reduce((a, b) => a + b, 0) / v.length
  if (mean <= 0) return 9
  const sd = Math.sqrt(v.reduce((a, b) => a + (b - mean) ** 2, 0) / v.length)
  return sd / mean
}

/** How jumpy a balance is in euros: the average move between points —
 *  salary in and out, money parked or pulled, an account opened or closed.
 *  Stacked charts put the lowest at the bottom, so the layers above ride on
 *  steady ground instead of spiking with it. */
export function jitter(values: number[]): number {
  const v = values.filter((x) => Number.isFinite(x))
  if (v.length < 2) return 0
  let moved = 0
  for (let i = 1; i < v.length; i++) moved += Math.abs(v[i] - v[i - 1])
  return moved / (v.length - 1)
}

/** The owner's banks in order of use: Swedbank first, then SEB, Revolut, the rest. */
const BANK_ORDER = [/swed/i, /\bseb\b/i, /revolut/i]
export function bankRank(a: Pick<Account, 'institution' | 'name'>): number {
  const hay = `${a.institution ?? ''} ${a.name}`
  const i = BANK_ORDER.findIndex((re) => re.test(hay))
  return i < 0 ? BANK_ORDER.length : i
}
/** Sort accounts: bank order first, then the account's own sort order. */
export const byBank = <T extends Pick<Account, 'institution' | 'name' | 'sort'>>(list: T[]): T[] =>
  [...list].sort((a, b) => bankRank(a) - bankRank(b) || (a.sort ?? 0) - (b.sort ?? 0))
