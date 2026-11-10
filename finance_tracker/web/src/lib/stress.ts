import type { Shock, StressLevel } from './types'

/** How each outcome reads, worst last. */
export const LEVELS: Record<StressLevel, { label: string; tone: 'good' | 'warn' | 'bad'; icon: string }> = {
  ok: { label: 'You get through', tone: 'good', icon: 'check' },
  tight: { label: 'Tight', tone: 'warn', icon: 'alert' },
  pause: { label: 'Invest less', tone: 'warn', icon: 'alert' },
  sell: { label: 'Must sell', tone: 'warn', icon: 'alert' },
  short: { label: 'Runs short', tone: 'bad', icon: 'alert' },
}

export const STRESS_ICONS: Record<string, string> = {
  job_loss: 'briefcase', long_search: 'calendar', pay_cut: 'wallet', illness: 'heart', parental: 'baby',
  market_crash: 'down', crypto_winter: 'bitcoin', housing_slump: 'home',
  rates: 'percent', inflation: 'bag', big_bill: 'alert',
  recession: 'trend', stagflation: 'coins', perfect_storm: 'bolt', custom: 'spark',
}

/** A shock with nothing in it changes nothing — no point asking the server. */
export function shockEmpty(s: Shock): boolean {
  return !Object.entries(s).some(([k, v]) => k !== 'months' && !!v)
}

/** Drops zero / false fields so the request and the query key stay small. */
export function cleanShock(s: Shock): Shock {
  return Object.fromEntries(Object.entries(s).filter(([, v]) => !!v)) as Shock
}

/** "6 months" / "the whole period" for a crisis's length. */
export function lasts(s: Shock, horizon: number): string {
  return s.months ? `${s.months} month${s.months === 1 ? '' : 's'}` : `all ${horizon} months`
}
