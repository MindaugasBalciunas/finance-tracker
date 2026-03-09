export type TransactionType = 'expense' | 'income' | 'investment'

export type Category = string

export interface Transaction {
  id: number
  date: string
  type: TransactionType
  amount: number
  comment: string
  category: Category
  created_at: string
  updated_at: string
}

export interface CreateTransactionInput {
  date: string
  type: TransactionType
  amount: number
  comment?: string
  category: Category
}

export interface UpdateTransactionInput {
  date?: string
  type?: TransactionType
  amount?: number
  comment?: string
  category?: Category
}

export interface PaginatedTransactions {
  data: Transaction[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface CategorySummary {
  category: Category
  type: TransactionType
  total: number
  count: number
}

export interface MonthlySummary {
  year: number
  month: number
  month_name: string
  expenses: number
  income: number
  investments: number
}

export interface TransactionSummary {
  total_expenses: number
  total_income: number
  total_investments: number
  net_balance: number
  by_category: CategorySummary[]
  by_month: MonthlySummary[]
}

export interface Balance {
  id: number
  date: string
  total: number
  seb: number
  swed: number
  swed_etf: number
  swed_pen: number
  luminor: number
  art: number
  cash: number
  rev_m: number
  rev_r: number
  r_btc: number
  m_btc: number
  rev_stocks: number
  created_at: string
  updated_at: string
}

export interface CreateBalanceInput {
  date: string
  total?: number
  seb?: number
  swed?: number
  swed_etf?: number
  swed_pen?: number
  luminor?: number
  art?: number
  cash?: number
  rev_m?: number
  rev_r?: number
  r_btc?: number
  m_btc?: number
  rev_stocks?: number
}

export interface BalanceTrend {
  dates: string[]
  totals: number[]
  accounts: Record<string, number[]>
}

export interface AccountAllocation {
  account: string
  amount: number
  percentage: number
}

export interface TransactionFilter {
  date_from?: string
  date_to?: string
  type?: TransactionType
  category?: Category
  page?: number
  page_size?: number
}

export interface BalanceFilter {
  date_from?: string
  date_to?: string
}
