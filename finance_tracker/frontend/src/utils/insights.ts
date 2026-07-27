import type { Transaction, MonthlySummary } from '../types'
import { formatEuro } from './format'
import { FIXED_LABELS, txLabels, hasAnyLabel as txHasAnyOf, discretionary } from './labels'

export type InsightTone = 'good' | 'warn' | 'bad' | 'info'

export interface Insight {
  icon: string
  text: string
  tone: InsightTone
}

function monthKey(dateStr: string): string {
  return dateStr.slice(0, 7)
}

// Projects the current month's spend from its daily pace and compares it to
// the average of the complete months in the period.
export function spendPaceInsight(byMonth: MonthlySummary[], now: Date): Insight | null {
  const curYear = now.getFullYear()
  const curMonth = now.getMonth() + 1
  const current = byMonth.find((m) => m.year === curYear && m.month === curMonth)
  const complete = byMonth.filter((m) => !(m.year === curYear && m.month === curMonth))
  if (!current || complete.length === 0) return null

  const avg = complete.reduce((s, m) => s + m.expenses, 0) / complete.length
  if (avg <= 0) return null

  const day = now.getDate()
  if (day < 5) return null // too little signal to project

  const daysInMonth = new Date(curYear, curMonth, 0).getDate()
  const projected = (current.expenses / day) * daysInMonth
  const pct = ((projected - avg) / avg) * 100
  const monthName = now.toLocaleDateString('en', { month: 'long' })

  if (Math.abs(pct) < 10) {
    return {
      icon: '✓',
      tone: 'good',
      text: `${monthName} spending is on track — projected ${formatEuro(projected)}, in line with your ${formatEuro(avg)} monthly average.`,
    }
  }
  if (pct >= 0) {
    return {
      icon: '▲',
      tone: pct >= 25 ? 'bad' : 'warn',
      text: `${monthName} is tracking ${pct.toFixed(0)}% above your average — projected ${formatEuro(projected)} vs ${formatEuro(avg)} typical.`,
    }
  }
  return {
    icon: '▼',
    tone: 'good',
    text: `${monthName} is tracking ${Math.abs(pct).toFixed(0)}% below your average — projected ${formatEuro(projected)} vs ${formatEuro(avg)} typical.`,
  }
}

// Compares the latest month IN THE SELECTED PERIOD against the average of the
// earlier months in that period and reports the biggest movers. Anchoring on
// the data (not the calendar) keeps "Last month" style filters honest —
// otherwise every category reads as "down 100%" against an empty current month.
export function categoryMoverInsights(
  expenses: Transaction[],
  now: Date,
  limit = 2
): Insight[] {
  const byCatMonth: Record<string, Record<string, number>> = {}
  const monthSet = new Set<string>()

  // Fixed obligations (alimony inside Kids - General, loan inside Finance…)
  // are not spending decisions — with them in, a recurring transfer that
  // started mid-period reads as a fake "category up X%" trend.
  for (const tx of discretionary(expenses)) {
    const mk = monthKey(tx.date)
    monthSet.add(mk)
    const cat = tx.category as string
    if (!byCatMonth[cat]) byCatMonth[cat] = {}
    byCatMonth[cat][mk] = (byCatMonth[cat][mk] ?? 0) + tx.amount.value
  }

  const months = [...monthSet].sort()
  if (months.length < 2) return []
  const curKey = months[months.length - 1]
  const priorMonths = months.slice(0, -1)

  const calKey = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const [ay, am] = curKey.split('-').map(Number)
  const monthPhrase = curKey === calKey
    ? 'this month'
    : `in ${new Date(ay, am - 1).toLocaleDateString('en', { month: 'long' })}`

  const movers: { cat: string; cur: number; avg: number; delta: number; pct: number }[] = []
  for (const [cat, months] of Object.entries(byCatMonth)) {
    const cur = months[curKey] ?? 0
    const avg = priorMonths.reduce((s, m) => s + (months[m] ?? 0), 0) / priorMonths.length
    if (avg < 20 && cur < 20) continue
    const delta = cur - avg
    const pct = avg > 0 ? (delta / avg) * 100 : 100
    if (Math.abs(delta) >= 50 && Math.abs(pct) >= 25) {
      movers.push({ cat, cur, avg, delta, pct })
    }
  }

  return movers
    .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta))
    .slice(0, limit)
    .map(({ cat, cur, avg, pct }) => {
      const up = pct >= 0
      return {
        icon: up ? '▲' : '▼',
        tone: up ? ('warn' as const) : ('good' as const),
        text: `${cat} is ${up ? 'up' : 'down'} ${Math.abs(pct) >= 995 ? '>10x' : `${Math.abs(pct).toFixed(0)}%`} ${monthPhrase} — ${formatEuro(cur)} vs ${formatEuro(avg)} average.`,
      }
    })
}

// Shared label plumbing lives in utils/labels.ts; local aliases keep the
// insight functions readable.
const hasAnyLabel = txHasAnyOf

// The biggest label-level movers, latest data month vs the average of the
// earlier months in the period — the label twin of categoryMoverInsights.
// Labels cut across categories (a "restaurant" euro can live in Food or
// Entertainment), so this catches drifts the category view smears out.
export function labelMoverInsights(
  expenses: Transaction[],
  now: Date,
  limit = 2
): Insight[] {
  const byLabelMonth: Record<string, Record<string, number>> = {}
  const monthSet = new Set<string>()

  for (const tx of discretionary(expenses)) {
    const mk = monthKey(tx.date)
    monthSet.add(mk)
    for (const l of txLabels(tx)) {
      if (!byLabelMonth[l]) byLabelMonth[l] = {}
      byLabelMonth[l][mk] = (byLabelMonth[l][mk] ?? 0) + tx.amount.value
    }
  }

  const months = [...monthSet].sort()
  if (months.length < 2) return []
  const curKey = months[months.length - 1]
  const priorMonths = months.slice(0, -1)

  const calKey = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const [ay, am] = curKey.split('-').map(Number)
  const monthPhrase = curKey === calKey
    ? 'this month'
    : `in ${new Date(ay, am - 1).toLocaleDateString('en', { month: 'long' })}`

  const movers: { label: string; cur: number; avg: number; delta: number; pct: number }[] = []
  for (const [label, perMonth] of Object.entries(byLabelMonth)) {
    const cur = perMonth[curKey] ?? 0
    const avg = priorMonths.reduce((s, m) => s + (perMonth[m] ?? 0), 0) / priorMonths.length
    if (avg < 20 && cur < 20) continue
    const delta = cur - avg
    const pct = avg > 0 ? (delta / avg) * 100 : 100
    if (Math.abs(delta) >= 50 && Math.abs(pct) >= 25) {
      movers.push({ label, cur, avg, delta, pct })
    }
  }

  return movers
    .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta))
    .slice(0, limit)
    .map(({ label, cur, avg, pct }) => {
      // A label with no history isn't "up 100%" — it's new spending.
      if (avg <= 0) {
        return {
          icon: '✦',
          tone: 'warn' as const,
          text: `New spending on “${label}” ${monthPhrase} — ${formatEuro(cur)}, nothing in the earlier months of the period.`,
        }
      }
      const up = pct >= 0
      return {
        icon: up ? '▲' : '▼',
        tone: up ? ('warn' as const) : ('good' as const),
        text: `“${label}” spending is ${up ? 'up' : 'down'} ${Math.abs(pct) >= 995 ? '>10x' : `${Math.abs(pct).toFixed(0)}%`} ${monthPhrase} — ${formatEuro(cur)} vs ${formatEuro(avg)} average.`,
      }
    })
}

// How much of the period's spending carries at least one label. Unlabeled
// money is invisible to every label insight, so surfacing the gap makes the
// label system self-improving.
export function labelCoverageInsight(expenses: Transaction[]): Insight | null {
  const total = expenses.reduce((s, tx) => s + tx.amount.value, 0)
  if (total <= 0) return null
  const unlabeled = expenses.filter((tx) => txLabels(tx).length === 0)
  const unlabeledSum = unlabeled.reduce((s, tx) => s + tx.amount.value, 0)
  if (unlabeledSum < 50) return null
  const pct = (unlabeledSum / total) * 100
  if (pct < 5) return null
  return {
    icon: '🏷',
    tone: pct >= 25 ? 'warn' : 'info',
    text: `${(100 - pct).toFixed(0)}% of spending is labeled — ${formatEuro(unlabeledSum)} (${pct.toFixed(0)}%) across ${unlabeled.length} transactions has no label yet.`,
  }
}

// Fixed obligations as one line: how much of the period's spending is
// pre-committed (loan, alimony, leasing) before any choices are made.
export function fixedShareInsight(expenses: Transaction[]): Insight | null {
  const total = expenses.reduce((s, tx) => s + tx.amount.value, 0)
  const fixed = expenses.filter((tx) => hasAnyLabel(tx, FIXED_LABELS))
  const fixedSum = fixed.reduce((s, tx) => s + tx.amount.value, 0)
  if (total <= 0 || fixedSum <= 0 || fixed.length < 2) return null
  const pct = (fixedSum / total) * 100
  return {
    icon: '🔒',
    tone: 'info',
    text: `Fixed obligations (loan, alimony, leasing): ${formatEuro(fixedSum)} — ${pct.toFixed(0)}% of all spending in the period.`,
  }
}

// Eating out (restaurants, fast food, delivery, coffee, bars, lunches)
// against grocery runs — the classic discretionary lever.
export function eatingOutInsight(expenses: Transaction[]): Insight | null {
  const OUT = ['restaurant', 'fast food', 'delivery', 'coffee', 'bars', 'lunch']
  let out = 0
  let groceries = 0
  for (const tx of expenses) {
    if (hasAnyLabel(tx, OUT)) out += tx.amount.value
    else if (hasAnyLabel(tx, ['groceries'])) groceries += tx.amount.value
  }
  if (out < 50 || groceries <= 0) return null
  const ratio = (out / (out + groceries)) * 100
  return {
    icon: '🍽',
    tone: ratio >= 50 ? 'warn' : 'info',
    text: `Eating out cost ${formatEuro(out)} vs ${formatEuro(groceries)} groceries — ${ratio.toFixed(0)}% of food money spent out.`,
  }
}

// Groups discretionary expenses by comment (the de-facto merchant field) and
// reports where the most freely-spent money went.
export function topPayeeInsight(expenses: Transaction[]): Insight | null {
  const totals: Record<string, { total: number; count: number; label: string }> = {}
  for (const tx of discretionary(expenses)) {
    const raw = (tx.comment ?? '').trim()
    if (!raw) continue
    const key = raw.toLowerCase()
    if (!totals[key]) totals[key] = { total: 0, count: 0, label: raw }
    totals[key].total += tx.amount.value
    totals[key].count += 1
  }
  const top = Object.values(totals).sort((a, b) => b.total - a.total)[0]
  if (!top || top.count < 2) return null
  const label = top.label.length > 42 ? `${top.label.slice(0, 42)}…` : top.label
  return {
    icon: '🏷',
    tone: 'info',
    text: `Top merchant (excluding fixed obligations): “${label}” — ${formatEuro(top.total)} across ${top.count} payments.`,
  }
}

export function largestExpenseInsight(expenses: Transaction[]): Insight | null {
  let max: Transaction | null = null
  for (const tx of discretionary(expenses)) {
    if (max == null || tx.amount.value > max.amount.value) max = tx
  }
  if (!max || max.amount.value < 100) return null
  const label = (max.comment ?? '').trim() || (max.category as string)
  const short = label.length > 42 ? `${label.slice(0, 42)}…` : label
  return {
    icon: '💶',
    tone: 'info',
    text: `Largest single expense: ${formatEuro(max.amount.value)} — “${short}” on ${max.date.slice(0, 10)}.`,
  }
}

export function buildInsights(
  expenses: Transaction[],
  byMonth: MonthlySummary[],
  now: Date
): Insight[] {
  const items: Insight[] = []
  const pace = spendPaceInsight(byMonth, now)
  if (pace) items.push(pace)
  items.push(...categoryMoverInsights(expenses, now))
  items.push(...labelMoverInsights(expenses, now))
  const fixed = fixedShareInsight(expenses)
  if (fixed) items.push(fixed)
  const eating = eatingOutInsight(expenses)
  if (eating) items.push(eating)
  const payee = topPayeeInsight(expenses)
  if (payee) items.push(payee)
  const largest = largestExpenseInsight(expenses)
  if (largest) items.push(largest)
  const coverage = labelCoverageInsight(expenses)
  if (coverage) items.push(coverage)
  return items
}
