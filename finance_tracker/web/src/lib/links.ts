// Routes to the transactions behind a figure — the same shape the server
// puts in fun facts and findings (`link`). Use with <Link to> or navigate().

export type LedgerQuery = {
  category?: string | string[]
  tag?: string
  merchant?: string
  account?: string
  kind?: 'income' | 'expense' | 'transfer'
  from?: string // YYYY-MM-DD
  to?: string
  month?: string // YYYY-MM, instead of from/to
  year?: string // YYYY, instead of from/to
  choices?: boolean // leave obligations out (mortgage, alimony…), as "spending by choice" does
  salaryMonth?: boolean // a salary paid early next month counts for the month it pays for
}

// The last day of a month (YYYY-MM) as YYYY-MM-DD.
export const monthEnd = (m: string) => `${m}-${String(new Date(Number(m.slice(0, 4)), Number(m.slice(5, 7)), 0).getDate()).padStart(2, '0')}`

/** Money in as cash flow counts it: every income category but refunds
 *  (money back lowers spending instead). Pass the category tree. */
export const incomeCategories = (tree: { id: string; kind: string }[]) => tree.filter((c) => c.kind === 'income' && c.id !== 'refunds').map((c) => c.id)

export function ledgerLink(q: LedgerQuery): string {
  const p = new URLSearchParams()
  for (const c of [q.category ?? []].flat()) if (c) p.append('category', c)
  if (q.tag) p.set('tag', q.tag)
  if (q.merchant) p.set('merchant', q.merchant)
  if (q.account) p.set('account', q.account)
  if (q.kind) p.set('kind', q.kind)
  const from = q.month ? `${q.month}-01` : q.year ? `${q.year}-01-01` : q.from
  const to = q.month ? monthEnd(q.month) : q.year ? `${q.year}-12-31` : q.to
  if (q.choices) p.set('choices', '1')
  if (q.salaryMonth) p.set('salary', 'month')
  if (from) p.set('from', from)
  if (to) p.set('to', to)
  // Without dates the Ledger shows this month; a link means all of it.
  if (!from && !to) p.set('period', 'all')
  return `/ledger?${p.toString()}`
}
