import type { DipFund, DipLevel, DipPlanName } from './types'

/** Each plan's words: what its holdings are called and where it sits. The
 *  plans claim the cash above the safety net in this order. */
export const PLAN_WORDS: Record<DipPlanName, { items: string; edit: string; stands: string; after?: string }> = {
  funds: { items: 'funds', edit: 'Funds & reserve', stands: 'Where each fund stands' },
  bets: { items: 'stocks', edit: 'Stocks & reserve', stands: 'Where each stock stands', after: 'the funds’ reserve' },
  crypto: { items: 'coins', edit: 'Coins & reserve', stands: 'Where bitcoin stands', after: 'the funds’ and bets’ reserves' },
}

// The crash plan's wording and small arithmetic, kept out of the page so it
// can be tested: what each zone means, the next rung down, and prices in
// the listing's own currency at its tick (VALL trades around €4, so it
// needs three decimals).

export const ZONE: Record<DipFund['zone'], { label: string; tone: 'good' | 'warn' | 'bad' | 'muted'; say: string }> = {
  calm: { label: 'Calm', tone: 'muted', say: 'Above every rung. Keep the monthly amount going, nothing extra.' },
  dip: { label: 'Dip', tone: 'warn', say: 'Under the −10% line of the 200-day average. Nothing extra: such dips usually mend on their own.' },
  t1: { label: 'Rung 1', tone: 'good', say: 'At or under the first rung. The first third of the reserve goes in.' },
  t2: { label: 'Rung 2', tone: 'good', say: 'At or under the second rung. The second third of the reserve goes in.' },
  t3: { label: 'Rung 3', tone: 'bad', say: 'In the deepest wave zone. The last third of the reserve goes in.' },
}

/** The highest buying rung (1–3) not yet reached — whether or not a reserve
 *  is set yet, so a €0 reserve never reads as "every rung reached". */
export function nextRung(f: DipFund): DipLevel | undefined {
  return f.levels.filter((l) => !l.reached && l.key !== 'dip').sort((a, b) => b.price - a.price)[0]
}

/** One x-axis tick per year: the first day of each year on the chart. */
export function yearTicks(dates: string[]): string[] {
  return dates.filter((d, i) => i > 0 && d.slice(0, 4) !== dates[i - 1].slice(0, 4))
}

/** A price in the listing's currency, at its tick: €602.49, €4.123. */
export function px(v: number | undefined, currency = 'EUR'): string {
  if (v == null || !Number.isFinite(v)) return '—'
  const digits = v < 10 ? 3 : 2
  try {
    return new Intl.NumberFormat('en-GB', { style: 'currency', currency: currency || 'EUR', minimumFractionDigits: digits, maximumFractionDigits: digits }).format(v)
  } catch {
    return v.toFixed(digits)
  }
}

/** "12% lower" / "reached" — how far the price must fall to a rung. */
export function awayText(l: DipLevel): string {
  if (l.reached) return 'reached'
  return `${Math.round(-l.away * 1000) / 10}% lower`
}

/** What a rung's order costs: whole shares × limit, never above the tranche. */
export const orderCost = (l: DipLevel) => Math.round(l.shares * l.price * 100) / 100

/** Every limit buy to keep at the broker, across funds, highest first per
 *  fund: the rungs not reached and not bought that release money. */
export function standingOrders(funds: DipFund[]) {
  return funds.flatMap((f) => f.levels.filter((l) => l.status === 'order' && l.tranche > 0).map((l) => ({ fund: f, level: l })))
}

/** Rungs whose third is due now (reached, not bought). */
export const dueRungs = (f: DipFund) => f.levels.filter((l) => l.status === 'buy')

/** The order as one line to paste into a note or the broker's search. */
export const orderLine = (f: DipFund, l: DipLevel) =>
  `${f.symbol.split('.')[0]} limit buy ${l.shares} @ ${px(l.price, f.currency)} (${f.symbol})`

/** Orders grouped one line per fund, keeping their order (highest first). */
export function ordersByFund(orders: { fund: DipFund; level: DipLevel }[]) {
  const out: { fund: DipFund; levels: DipLevel[] }[] = []
  for (const o of orders) {
    const g = out.find((x) => x.fund.symbol === o.fund.symbol)
    if (g) g.levels.push(o.level)
    else out.push({ fund: o.fund, levels: [o.level] })
  }
  return out
}

/** A total in the listing's currency, to the cent: €2,838.82, US$461.76. */
export function amount(v: number, currency = 'EUR'): string {
  try {
    return new Intl.NumberFormat('en-GB', { style: 'currency', currency: currency || 'EUR', minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(v)
  } catch {
    return v.toFixed(2)
  }
}

/** How close a rung is, for its colour: buy (reached, due now), near (within
 *  5%: the next order to fill), far. Green means act, amber heading there. */
export function closeness(l: DipLevel): 'buy' | 'near' | 'far' {
  if (l.status === 'buy') return 'buy'
  if (l.status !== 'order') return 'far' // the −10% line buys nothing; bought and held rungs are done
  return l.away > -0.05 ? 'near' : 'far'
}

/** The same three colours everywhere a rung shows: chart line, ladder price, order pill. */
export const TONE = {
  buy: { line: 'var(--hue-good)', text: 'text-good' },
  near: { line: 'var(--hue-warn)', text: 'text-warn' },
  far: { line: 'var(--chart-axis)', text: 'text-ink' },
} as const

/** Each average's colour, the same on the chart's lines and wherever its rung
 *  shows (ladder, order pills, the plan's rules), so the eye ties a price to
 *  its line. The −44% rung has its own. */
export const SERIES = { close: 'var(--s1)', sma200: 'var(--s2)', sma2y: 'var(--s3)', sma4y: 'var(--s4)', deep: 'var(--s5)' } as const

export function rungColor(l: Pick<DipLevel, 'label'>): string {
  if (l.label.includes('200-day')) return SERIES.sma200
  if (l.label.includes('2-year')) return SERIES.sma2y
  if (l.label.includes('4-year')) return SERIES.sma4y
  return SERIES.deep
}
