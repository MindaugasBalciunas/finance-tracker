import client from './client'

export type ReviewTotals = {
  income: number
  spending: number
  invested: number
  net_saved: number
  savings_rate?: number
}

export type MonthReview = ReviewTotals & {
  month: string
  complete: boolean
  previous: ReviewTotals
  six_month_avg: ReviewTotals
  net_worth?: {
    start_date: string; start: number; end_date: string; end: number; change: number
    points?: ReviewPoint[]
  }
  categories: { category: string; spent: number; average: number; delta: number }[]
  top_expenses: { id: number; date: string; amount: number; category: string; comment: string }[]
  budget?: {
    over: ReviewBudgetLine[]; within_count: number; safe_to_spend?: number
    lines?: ReviewBudgetLine[]
  }
  owed_to_you: number
  checks: { level: 'warn' | 'info'; text: string; link?: string }[]
  trend?: (ReviewTotals & { month: string })[]
  by_category?: ReviewCategory[]
  daily?: ReviewPoint[]
}

export type ReviewPoint = { date: string; value: number }
export type ReviewBudgetLine = { name: string; budgeted: number; spent: number }
export type ReviewCategory = { category: string; spent: number; average: number; delta: number }

export const reviewApi = {
  get: (month?: string) =>
    client.get<MonthReview>('/review', { params: month ? { month } : {} }).then((r) => r.data),
}
