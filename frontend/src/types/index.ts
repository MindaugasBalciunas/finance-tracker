// --- Shared monetary type ---

export type Currency = 'EUR' | 'USD'

export interface Money {
  value: number
  currency: Currency
}

// --- Transactions ---

export type TransactionType = 'expense' | 'income' | 'investment'

export type Category = string

export interface Transaction {
  id: number
  date: string
  type: TransactionType
  amount: Money // always EUR
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

// --- Balances ---

export interface Balance {
  id: number
  date: string
  total: number    // EUR - recomputed with live BTC price when available
  seb: number      // EUR
  swed: number     // EUR
  swed_etf: number // EUR
  swed_pen: number // EUR
  luminor: number  // EUR
  art: number      // EUR
  cash: number     // EUR
  rev_m: number    // EUR
  rev_r: number    // EUR
  r_btc: number    // BTC units (stored)
  m_btc: number    // BTC units (stored)
  btc_price: number // EUR/BTC at snapshot time (0 = legacy row)
  rev_stocks: number // EUR
  created_at: string
  updated_at: string
  // Computed EUR values (only present when the account has BTC holdings)
  r_btc_eur?: number
  m_btc_eur?: number
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
  btc_price?: number
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

// --- Stocks ---

export type StockAction = 'buy' | 'sell'

export interface StockTrade {
  id: number
  date: string
  action: StockAction
  ticker: string
  shares: number
  price_per_share: Money // price with explicit currency
  currency: string
  notes: string
  created_at: string
  updated_at: string
}

export interface CreateStockTradeInput {
  date: string
  action: StockAction
  ticker: string
  shares: number
  price_per_share: number
  currency?: string
  notes?: string
}

export interface StockHolding {
  ticker: string
  currency: string
  shares: number
  avg_cost: Money
  total_cost: Money
  realized_gain: Money
}

export interface StockPortfolio {
  holdings: StockHolding[]
  total_cost: Money
  total_realized_gain: Money
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
