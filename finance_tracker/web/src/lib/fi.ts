import type { FIPlan, PayoutPoint } from './types'

/** The FI projection's time spans; up to 10 years reads month by month. */
export const FI_SPANS = [{ value: '1', label: '1Y' }, { value: '3', label: '3Y' }, { value: '5', label: '5Y' }, { value: '10', label: '10Y' }, { value: '30', label: '30Y' }]

export type FIPoint = PayoutPoint & { label: string }

/** The points for the next `years`: a point a month up to ten years (so the
 *  next year or three isn't two dots on a 30-year line), else a point a year. */
export function fiSeries(f: Pick<FIPlan, 'payouts' | 'payouts_monthly'>, years: number): FIPoint[] {
  const monthly = f.payouts_monthly ?? []
  if (years <= 10 && monthly.length > 1) {
    return monthly.slice(0, years * 12 + 1).map((p) => ({ ...p, label: p.month ?? String(p.year) }))
  }
  return f.payouts.slice(0, years + 1).map((p) => ({ ...p, label: String(p.year) }))
}
