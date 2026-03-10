// --- Base Types ---

export type Currency = 'EUR' | 'USD' | 'BTC'

/** Money represents an amount with explicit currency */
export interface Money {
  value: number
  currency: Currency
}

/** CryptoAmount represents a cryptocurrency amount with price context and conversion */
export interface CryptoAmount {
  amount: number
  unit: Currency // 'BTC', 'ETH', etc.
  price_per_unit: number
  price_valid_at?: string
  converted_value?: number
  converted_currency?: Currency
}

// --- Transactions ---

export type TransactionType = 'expense' | 'income' | 'investment'

export type Category = string

export interface Transaction {
  id: number
  date: string
  type: TransactionType
  amount: number // EUR
  comment: string
  category: Category
  created_at: string
  updated_at: string
  // Enhanced field
  amount_money?: Money // amount with explicit EUR currency
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
  total: number // EUR - sum of all accounts
  seb: number // EUR
  swed: number // EUR
  swed_etf: number // EUR
  swed_pen: number // EUR
  luminor: number // EUR
  art: number // EUR
  cash: number // EUR
  rev_m: number // EUR
  rev_r: number // EUR
  r_btc: number      // stored in BTC units
  m_btc: number      // stored in BTC units
  btc_price: number  // EUR/BTC at snapshot time (0 = legacy row)
  rev_stocks: number // EUR
  created_at: string
  updated_at: string
  
  // Enhanced computed fields
  r_btc_eur?: number  // BTC → EUR (deprecated, use r_btc_computed instead)
  m_btc_eur?: number  // BTC → EUR (deprecated, use m_btc_computed instead)
  r_btc_computed?: CryptoAmount // Revolut BTC with EUR conversion
  m_btc_computed?: CryptoAmount // Mobile BTC with EUR conversion
  total_eur?: Money // Total in EUR with explicit currency
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
  r_btc?: number      // BTC units
  m_btc?: number      // BTC units
  btc_price?: number  // EUR/BTC at snapshot time
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
  price_per_share: number
  currency: string // 'USD', 'EUR', etc.
  notes: string
  created_at: string
  updated_at: string
  
  // Enhanced computed fields
  price_per_share_money?: Money // Price per share with explicit currency
  total_cost_money?: Money // Shares * Price per share with currency
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
  currency: string // 'USD', 'EUR', etc.
  shares: number
  avg_cost_usd: number // (DEPRECATED: use avg_cost_money instead)
  total_cost_usd: number // (DEPRECATED: use total_cost_money instead)
  realized_gain: number // (DEPRECATED: use realized_gain_money instead)
  
  // Enhanced computed fields
  avg_cost_money?: Money // Average cost per share with currency
  total_cost_money?: Money // Total invested with currency
  realized_gain_money?: Money // Realized gain with currency
}

export interface StockPortfolio {
  holdings: StockHolding[]
  total_cost_usd: number // (DEPRECATED: use total_cost_money instead)
  total_realized_gain: number // (DEPRECATED: use total_realized_gain_money instead)
  
  // Enhanced computed fields
  total_cost_money?: Money // Total cost with currency
  total_realized_gain_money?: Money // Total realized gain with currency
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
