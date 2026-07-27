// Adaptive time bucketing for the stacked period charts: monthly bars are
// unreadable past ~2 years (an all-history view is 190+ bars), so long
// periods aggregate to quarters, and very long ones to years.

export type Granularity = 'month' | 'quarter' | 'year'

export function pickGranularity(monthCount: number): Granularity {
  if (monthCount > 72) return 'year'
  if (monthCount > 24) return 'quarter'
  return 'month'
}

// Sortable bucket key for a YYYY-MM-DD (or longer ISO) date string.
export function bucketKey(dateStr: string, g: Granularity): string {
  const ym = dateStr.slice(0, 7)
  if (g === 'month') return ym
  const year = ym.slice(0, 4)
  if (g === 'year') return year
  const quarter = Math.floor((parseInt(ym.slice(5, 7), 10) - 1) / 3) + 1
  return `${year}-Q${quarter}`
}

export function bucketLabel(key: string, g: Granularity): string {
  if (g === 'year') return key
  if (g === 'quarter') {
    const [year, q] = key.split('-')
    return `${q} '${year.slice(2)}`
  }
  const [year, month] = key.split('-')
  return new Date(parseInt(year, 10), parseInt(month, 10) - 1)
    .toLocaleDateString('en', { month: 'short', year: '2-digit' })
}

export const GRANULARITY_NOTE: Record<Granularity, string | null> = {
  month: null,
  quarter: 'Grouped by quarter — the period is too long for monthly bars',
  year: 'Grouped by year — the period is too long for monthly bars',
}
