import type { MonthReview, ReviewExpense } from '../api/review'
import { formatEuro } from './format'

export type Tone = 'good' | 'neutral' | 'bad'

export type Highlight = { icon: string; text: string; tone: Tone }

export type Highlights = {
  headline: string
  tone: Tone
  bullets: Highlight[]
}

// A month's story in plain words, built from the review data: the verdict,
// then why — which categories and which payments moved spending, whether
// income was unusual, the big one-offs, how much was fixed obligations, the
// budget, and the good news. Every number is the review's own; nothing is
// estimated here.
export function buildHighlights(r: MonthReview, maxBullets = 6): Highlights {
  const usual = r.six_month_avg
  const hasUsual = usual.income > 0 || usual.spending > 0
  const bullets: Highlight[] = []
  const name = (e: ReviewExpense) => shorten(e.comment || e.category)
  const mentioned = new Set<number>()

  // Verdict.
  let headline: string
  let tone: Tone
  if (r.income <= 0 && r.spending <= 0) {
    headline = 'No income or spending recorded this month'
    tone = 'neutral'
  } else if (r.net_saved < 0) {
    headline = `You spent ${formatEuro(-r.net_saved)} more than you earned`
    tone = 'bad'
  } else {
    const rate = r.savings_rate != null ? ` — ${r.savings_rate.toFixed(0)}% of income` : ''
    headline = `You saved ${formatEuro(r.net_saved)}${rate}`
    tone = usual.savings_rate != null && r.savings_rate != null && r.savings_rate < usual.savings_rate - 2 ? 'neutral' : 'good'
  }

  // Spending vs usual, and what drove it.
  if (hasUsual && r.spending > 0) {
    const diff = r.spending - usual.spending
    const threshold = Math.max(100, usual.spending * 0.05)
    if (Math.abs(diff) < threshold) {
      bullets.push({ icon: '🛒', tone: 'good', text: `Spending was about usual at ${formatEuro(r.spending)}.` })
    } else {
      const up = diff > 0
      const drivers = (r.by_category ?? [])
        .filter((c) => (up ? c.delta >= 100 : c.delta <= -100))
        .sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta))
        .slice(0, 2)
      // Name the payment that explains the jump — the largest one that is
      // not a regular monthly payment. If they all recur, say so.
      const why = drivers.map((c) => {
        const head = `${c.category} ${up ? '+' : '−'}${formatEuro(Math.abs(c.delta))}`
        if (!up) return head
        const moved = c.top?.find((t) => t.moved_from)
        if (moved && moved.amount >= Math.abs(c.delta) * 0.5) {
          mentioned.add(moved.id)
          return `${head} (${name(moved)} ${formatEuro(moved.amount)} is now filed here instead of ${moved.moved_from})`
        }
        const unusual = c.top?.find((t) => !t.recurring)
        if (unusual) {
          mentioned.add(unusual.id)
          return `${head} (${name(unusual)} ${formatEuro(unusual.amount)})`
        }
        return c.top?.length ? `${head} (regular payments were higher)` : head
      })
      bullets.push({
        icon: '🛒',
        tone: up ? 'bad' : 'good',
        text: `Spending was ${formatEuro(r.spending)}, ${formatEuro(Math.abs(diff))} ${up ? 'above' : 'below'} your usual ${formatEuro(usual.spending)}.`
          + (why.length ? ` ${up ? 'Mostly' : 'Mainly'} ${joinAnd(why)}.` : ''),
      })
    }
  }

  // Income, only when it was unusual.
  if (hasUsual && usual.income > 0) {
    const diff = r.income - usual.income
    if (Math.abs(diff) >= Math.max(200, usual.income * 0.1)) {
      const up = diff > 0
      const extra = up ? (r.top_income ?? []).find((t) => t.category !== 'Salary') : undefined
      bullets.push({
        icon: '💶',
        tone: up ? 'good' : 'bad',
        text: `Income was ${formatEuro(r.income)}, ${formatEuro(Math.abs(diff))} ${up ? 'more' : 'less'} than usual`
          + (extra ? ` — including ${name(extra)} ${formatEuro(extra.amount)}.` : '.'),
      })
    }
  }

  // Big one-offs not already named above. Regular payments (alimony, loan)
  // are not one-offs — the fixed-obligations line covers them.
  if (r.spending > 0) {
    const big = r.top_expenses.filter((e) =>
      !mentioned.has(e.id) && !e.recurring && (e.amount >= 500 || e.amount >= r.spending * 0.15))
    if (big.length) {
      const sum = big.reduce((s, e) => s + e.amount, 0)
      bullets.push({
        icon: '✨',
        tone: 'neutral',
        text: `${big.length === 1 ? 'One big payment' : 'Big one-offs'}: ${joinAnd(big.slice(0, 3).map((e) => `${name(e)} ${formatEuro(e.amount)}`))}`
          + ` — ${pct(sum, r.spending)} of the month's spending.`,
      })
    }
  }

  // Fixed obligations: what was never a choice.
  if ((r.fixed ?? 0) > 0 && r.spending > 0) {
    bullets.push({
      icon: '🔒',
      tone: 'neutral',
      text: `${formatEuro(r.fixed!)} (${pct(r.fixed!, r.spending)}) went to fixed obligations — loans, alimony, leasing. `
        + `Day-to-day spending was ${formatEuro(r.spending - r.fixed!)}.`,
    })
  }

  // Budget.
  const lines = r.budget?.lines ?? []
  if (lines.length) {
    const over = lines.filter((l) => l.spent > l.budgeted + 0.5).sort((a, b) => (b.spent - b.budgeted) - (a.spent - a.budgeted))
    bullets.push(over.length
      ? {
          icon: '🎯',
          tone: 'bad',
          text: `${over.length} of ${lines.length} budget lines went over — most of all ${over[0].name} (${formatEuro(over[0].spent)} of ${formatEuro(over[0].budgeted)}).`,
        }
      : { icon: '🎯', tone: 'good', text: `All ${lines.length} monthly budget lines stayed within budget.` })
  }

  // The best news among categories, if any category fell well below usual.
  // A category that only looks lower because a regular payment was re-filed
  // elsewhere is not good news.
  const refiledFrom = new Set((r.by_category ?? []).flatMap((c) => (c.top ?? []).map((t) => t.moved_from).filter(Boolean)))
  const better = (r.by_category ?? [])
    .filter((c) => c.delta <= -100 && !refiledFrom.has(c.category))
    .sort((a, b) => a.delta - b.delta)[0]
  if (better && !(bullets[0]?.tone === 'good' && bullets[0].text.includes(better.category))) {
    bullets.push({ icon: '👍', tone: 'good', text: `${better.category} was ${formatEuro(-better.delta)} below usual (${formatEuro(better.spent)}).` })
  }

  if (r.owed_to_you >= 0.01) {
    bullets.push({ icon: '🤝', tone: 'neutral', text: `${formatEuro(r.owed_to_you)} is still owed to you.` })
  }

  return { headline, tone, bullets: bullets.slice(0, maxBullets) }
}

function pct(part: number, whole: number): string {
  return `${Math.round((part / whole) * 100)}%`
}

function joinAnd(items: string[]): string {
  if (items.length <= 1) return items.join('')
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

// Bank comments run long ("Restoranas Smelyne, Panevėžys. Family reunion"):
// keep the first clause, at most ~32 characters. A clause ends at
// punctuation followed by a space, so "Aliments 2026.09 Evelina" stays whole.
export function shorten(s: string): string {
  const first = s.split(/[.,;:]\s|\s\(/)[0].trim() || s.trim()
  return first.length > 32 ? `${first.slice(0, 30).trimEnd()}…` : first
}
