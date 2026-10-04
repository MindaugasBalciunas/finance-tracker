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
