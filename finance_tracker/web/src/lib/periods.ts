import { daysAgo, todayISO } from './format'
// Chart periods shared by the net worth charts (Home, Wealth).
export const RANGES = [{ value: '3m', label: '3M' }, { value: '6m', label: '6M' }, { value: 'ytd', label: 'YTD' }, { value: '1y', label: '1Y' }, { value: '3y', label: '3Y' }, { value: '5y', label: '5Y' }, { value: 'all', label: 'All' }]
export const SHORT = ['3m', '6m', 'ytd']

/** First date of a period ('' = since records began). */
export function rangeFrom(r: string) {
  if (r === 'all') return ''
  const d = new Date()
  if (r === 'ytd') return `${d.getFullYear() - 1}-12-31`
  if (r.endsWith('m')) d.setMonth(d.getMonth() - Number(r.slice(0, -1)))
  else d.setFullYear(d.getFullYear() - Number(r.slice(0, -1)))
  return d.toISOString().slice(0, 10)
}
/** Daily points up to a year (exact highs and lows), weekly for 3Y, monthly
 *  beyond. Every series ends on today, so a fresh balance shows at once. */
export const rangeStep = (r: string) => (SHORT.includes(r) || r === '1y' ? 'day' : r === '3y' ? 'week' : 'month')
export const rangeLabel = (r: string) => (r === 'all' ? 'since records began' : r === 'ytd' ? 'this year' : `over ${r.toUpperCase()}`)
export const rangeTick = (r: string) => (d: string) => (r === '5y' || r === 'all' ? d.slice(0, 4) : new Date(d).toLocaleDateString('en-GB', SHORT.includes(r) ? { day: 'numeric', month: 'short' } : { month: 'short', year: '2-digit' }))

/** Ledger-style periods, shared by the Ledger and Fun facts. */
export const PERIODS = [
  { value: 'month', label: 'This month' }, { value: '30', label: '30 days' }, { value: '90', label: '3 months' },
  { value: 'year', label: 'This year' }, { value: '365', label: '12 months' }, { value: 'all', label: 'All time' },
]

export function periodRange(p: string): { from?: string; to?: string } {
  const t = todayISO()
  switch (p) {
    case 'month': return { from: t.slice(0, 8) + '01', to: t }
    case 'year': return { from: t.slice(0, 5) + '01-01', to: t }
    case 'all': return {}
    default: return { from: daysAgo(Number(p)), to: t }
  }
}
