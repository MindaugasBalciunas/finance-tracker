const eur0 = new Intl.NumberFormat('en-GB', { style: 'currency', currency: 'EUR', maximumFractionDigits: 0 })
const eur2 = new Intl.NumberFormat('en-GB', { style: 'currency', currency: 'EUR', minimumFractionDigits: 2, maximumFractionDigits: 2 })

// Cents matter on small amounts: €11.46 must never read "€11". Below €1,000 a
// value with cents keeps them; larger overview figures round to whole euros.
const hasCents = (v: number) => Math.abs(Math.round(v * 100) - Math.round(v) * 100) > 0
const smallExact = (v: number) => Math.abs(v) < 1000 && hasCents(v)

/** €1,234 for overviews; €11.46 when a small amount has cents. */
export const eur = (v: number | null | undefined) => (v == null || isNaN(v) ? '—' : smallExact(v) ? eur2.format(v) : eur0.format(Math.round(v) === 0 ? 0 : v))
/** €1,234.56 — exact amounts in lists. */
export const eurc = (v: number | null | undefined) => (v == null || isNaN(v) ? '—' : eur2.format(v))
/** €12.3k / €1.2M — axis ticks and tight tiles. */
export function eurk(v: number): string {
  const a = Math.abs(v)
  const s = v < 0 ? '−' : ''
  if (a >= 1e6) return `${s}€${(a / 1e6).toFixed(a >= 1e7 ? 0 : 1)}M`
  if (a >= 1e3) return `${s}€${(a / 1e3).toFixed(a >= 1e5 ? 0 : 1)}k`
  return `${s}€${hasCents(a) ? a.toFixed(2) : Math.round(a)}`
}
export const signed = (v: number) => (v > 0 ? '+' : v < 0 ? '−' : '') + (smallExact(v) ? eur2 : eur0).format(Math.abs(v))
export const pct = (v: number | null | undefined, digits = 0) => (v == null || isNaN(v) ? '—' : `${(v * 100).toFixed(digits)}%`)

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
export function monthLabel(ym: string, long = false): string {
  const [y, m] = ym.split('-')
  const name = long ? new Date(+y, +m - 1, 1).toLocaleString('en-GB', { month: 'long' }) : MONTHS[+m - 1]
  return `${name} ${long ? y : `’${y.slice(2)}`}`
}
export function dayLabel(d: string): string {
  const t = new Date(d + 'T00:00:00')
  const today = new Date()
  const y = new Date()
  y.setDate(today.getDate() - 1)
  const same = (a: Date, b: Date) => a.toDateString() === b.toDateString()
  if (same(t, today)) return 'Today'
  if (same(t, y)) return 'Yesterday'
  return t.toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', year: t.getFullYear() === today.getFullYear() ? undefined : 'numeric' })
}
export const shortDate = (d: string) => new Date(d + 'T00:00:00').toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: '2-digit' })

export const todayISO = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
export const thisMonth = () => todayISO().slice(0, 7)
export function addMonths(ym: string, n: number): string {
  const [y, m] = ym.split('-').map(Number)
  const d = new Date(y, m - 1 + n, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}
export function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Parse a typed amount: "12,5", "12.5", "1 234,56", "1,234.56", "€ 9.99".
 *  The last "," or "." is the decimal point; the other is a thousands
 *  separator. Returns undefined (never NaN) when it isn't a number. */
export function parseNum(input: string | number | null | undefined): number | undefined {
  if (input == null) return undefined
  if (typeof input === 'number') return Number.isFinite(input) ? input : undefined
  let s = input.replace(/[\s €$£]/g, '')
  if (!s) return undefined
  const lastComma = s.lastIndexOf(','), lastDot = s.lastIndexOf('.')
  if (lastComma >= 0 && lastDot >= 0) {
    // Both present: whichever comes last is the decimal separator.
    s = lastComma > lastDot ? s.replace(/\./g, '').replace(',', '.') : s.replace(/,/g, '')
  } else if (lastComma >= 0) {
    // "1,234,567" (several commas) is thousands; a single comma is a decimal.
    s = (s.match(/,/g) ?? []).length > 1 ? s.replace(/,/g, '') : s.replace(',', '.')
  }
  if (!/^[-+]?(\d+\.?\d*|\.\d+)$/.test(s)) return undefined
  const n = Number(s)
  return Number.isFinite(n) ? n : undefined
}
