// --- Shared monetary type ---

export type Currency = 'EUR' | 'USD'

export interface Money {
  value: number
  currency: Currency
}

// --- Transactions ---

export type TransactionType = 'expense' | 'income' | 'investment'

export type Category = string

export type AccountKey =
  | 'seb' | 'swed' | 'swed_etf' | 'seb_pen' | 'luminor'
  | 'art' | 'rev_m' | 'rev_r' | 'rev_stocks' | 'ibkr_stocks' | 'cash'

export const ACCOUNT_LABELS: Record<AccountKey, string> = {
  seb: 'SEB',
  swed: 'Swedbank',
  swed_etf: 'Swed ETF',
  seb_pen: 'SEB Pension',
  luminor: 'Luminor',
  art: 'Artea',
  rev_m: 'Revolut M',
  rev_r: 'Revolut R',
  rev_stocks: 'Rev Stocks',
  ibkr_stocks: 'IBKR',
  cash: 'Cash',
}

export interface Transaction {
  id: number
  date: string
  type: TransactionType
  amount: Money // always EUR
  comment: string
  category: Category
  // Comma-separated lowercase tags, e.g. "loan,fixed"
  labels: string
  // Legacy field — present on old rows, empty on new rows
  source_account: string
  // New fields — one or both set depending on transaction type
  debit_account: string
  credit_account: string
  created_at: string
  updated_at: string
}

export interface CreateTransactionInput {
  date: string
  type: TransactionType
  amount: number
  comment?: string
  category: Category
  labels?: string
  // Rule labels the user removed in the form — the server skips these rules
  // for this transaction only.
  suppressed_labels?: string
  debit_account?: string
  credit_account?: string
}

export interface UpdateTransactionInput {
  date?: string
  type?: TransactionType
  amount?: number
  comment?: string
  category?: Category
  labels?: string
  debit_account?: string
  credit_account?: string
}

export type BudgetKind = 'fixed' | 'investment' | 'spending'

export interface Budget {
  id: number
  name: string
  kind: BudgetKind
  label: string
  category: string
  amount: number
  created_at: string
  updated_at: string
}

export type IncomeMode = 'median' | 'manual' | 'gross'

export interface BudgetSettings {
  id: number
  income_mode: IncomeMode
  manual_income: number
  gross_salary: number
  monthly_deductions: number
  updated_at: string
}

export interface BudgetSettingsInput {
  income_mode: IncomeMode
  manual_income?: number
  gross_salary?: number
  monthly_deductions?: number
}

export interface BudgetInput {
  name: string
  kind: BudgetKind
  label?: string
  category?: string
  amount: number
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
  is_auto: boolean // true = auto-generated after a transaction mutation; not user-created
  total: number    // EUR - recomputed with live BTC price when available
  seb: number      // EUR
  swed: number     // EUR
  swed_etf: number // EUR
  seb_pen: number // EUR
  luminor: number  // EUR
  art: number      // EUR
  cash: number     // EUR
  rev_m: number    // EUR
  rev_r: number    // EUR
  r_btc: number    // BTC units (stored)
  m_btc: number    // BTC units (stored)
  btc_price: number // EUR/BTC at snapshot time (0 = legacy row)
  rev_stocks: number  // EUR - Revolut stocks portfolio
  ibkr_stocks: number // EUR - IBKR portfolio
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
  seb_pen?: number
  luminor?: number
  art?: number
  cash?: number
  rev_m?: number
  rev_r?: number
  r_btc?: number
  m_btc?: number
  btc_price?: number
  rev_stocks?: number
  ibkr_stocks?: number
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
export type StockSource = 'Revolut' | 'IBKR'

export interface StockTrade {
  id: number
  date: string
  action: StockAction
  ticker: string
  shares: number
  price_per_share: Money // price with explicit currency
  currency: string
  source: StockSource
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
  source: StockSource
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

// --- Assets ---

export type AssetType = 'vehicle' | 'real_estate' | 'solar' | 'other'

export const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  vehicle: 'Vehicle',
  real_estate: 'Real Estate',
  solar: 'Solar',
  other: 'Other',
}

export interface Asset {
  id: number
  name: string
  type: AssetType
  purchase_date?: string
  purchase_price: number // EUR
  current_value: number // EUR — latest valuation
  valuation_date?: string
  notes: string
  loan_remaining: number // EUR outstanding; 0 = owned outright
  loan_remaining_date?: string
  loan_rate?: string
  loan_account?: string
  loan_paid_off_date?: string
  created_at: string
  updated_at: string
  equity: number // current_value − loan_remaining (computed by backend)
}

export interface CreateAssetInput {
  name: string
  type: AssetType
  purchase_date?: string
  purchase_price: number
  current_value?: number
  valuation_date?: string
  notes?: string
  loan_remaining?: number
  loan_remaining_date?: string
  loan_rate?: string
  loan_account?: string
  loan_paid_off_date?: string
}

export interface AssetSummary {
  count: number
  total_purchase_price: number
  total_value: number
  total_loans: number
  net_equity: number
}

export interface TransactionFilter {
  date_from?: string
  date_to?: string
  type?: TransactionType
  category?: Category
  // Comma list; label_mode 'all' = row must carry every label, default any.
  label?: string
  label_mode?: 'all'
  search?: string
  page?: number
  page_size?: number
}

export interface BalanceFilter {
  date_from?: string
  date_to?: string
}
