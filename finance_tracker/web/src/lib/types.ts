export type Kind = 'income' | 'expense' | 'transfer'

export interface Tx {
  id: number
  date: string
  kind: Kind
  amount: number
  account_id: string
  to_account_id: string
  category: string
  merchant: string
  note: string
  tags: string[]
  external_id?: string
  split_of?: number
  source: string
  pending?: boolean
}

export interface TxList { items: Tx[]; total: number; income: number; expense: number; transfer: number }

export interface Category { id: string; parent?: string; name: string; kind: Kind; essential: boolean; color?: string; sort: number; archived: boolean }

export interface Account {
  id: string; name: string; institution: string; kind: string; group: string; currency: string
  liquid: boolean; archived: boolean; sort: number; notes: string; details: any
  balance?: number | null; balance_date?: string; quantity?: number; source?: string
}

export interface Flow {
  period: string; income: number; spending: number; essential: number; discretionary: number; saved: number
  savings_rate: number; invested: number; principal: number; payroll_pension: number
  income_by: Record<string, number>; spending_by: Record<string, number>
}

export interface Snapshot {
  date: string; net_worth: number; liquid: number; assets: number; debt: number
  by_group: Record<string, number>; by_account?: Record<string, number>; stale?: Record<string, string>
}

export interface Recurring {
  id?: number; source: 'detected' | 'edited' | 'manual'; note?: string
  merchant: string; category: string; cadence: string; amount: number; monthly: number; last: string; next: string
  count: number; changed: boolean; last_amount: number
}

export interface Anomaly { category: string; spent: number; typical: number; ratio: number; projected: number }

export interface Overview {
  date: string; net_worth: number; liquid: number; debt: number; by_group: Record<string, number>
  net_worth_30d: number; net_worth_ytd: number; net_worth_12m: number
  liquid_30d: number; liquid_ytd: number; liquid_12m: number
  spark: { date: string; value: number; liquid: number }[]
  month: Flow; last_month: Flow; avg12: Flow; year: Flow; month_progress: number
  plan: { safe_to_spend: number; income_base: number; spent: number; budgeted: number;
    free_spent: number; days_left: number; per_day_left: number; avg_day: number; typical_day: number; expected_day: number; projected_left: number; over: { name: string; spent: number; budgeted: number; remaining: number }[] | null }
  emergency: { cash: number; monthly_essential: number; months: number; target_months: number; target: number }
  fi_progress: number; years_to_fi: number
  anomalies: Anomaly[] | null; upcoming: Recurring[] | null; stale: Record<string, string> | null
  inbox_open: number; recent: Tx[] | null
  checks: { level: string; text: string; link: string }[] | null; review_month?: string
}

export interface Step { from_month: string; amount: number }
export interface Budget {
  id: number; name: string; kind: 'fixed' | 'spending' | 'saving'; categories: string[]; tag: string
  period: 'monthly' | 'yearly'; fund: boolean; start_month: string; archived: boolean; sort: number; amount: number; steps: Step[]
}
export interface MonthCell { month: string; budget: number; spent: number; fund_end?: number }
export interface Suggestion { amount: number; median: number; p75: number; mean: number; max: number; months: number; lumpy: boolean; basis: string }
export interface PlanLine extends Budget {
  monthly_share: number; budgeted: number; spent: number; remaining: number; month_spent: number
  year?: { year: number; budget: number; spent: number; pace: number; projected: number; elapsed: number }
  fund_state?: { start_month: string; opening: number; contribution: number; spent: number; available: number; contributed: number; drawn: number }
  history: MonthCell[]; suggestion?: Suggestion
}
export interface PlanReport {
  month: string; months: string[]; income_base: number; income_base_source: string; income_actual: number
  lines: PlanLine[]; fixed_planned: number; saving_planned: number; spending_planned: number; fixed_spent: number; saved_actual: number
  discretionary_spent: number; fund_contributions: number; fund_spent: number; safe_to_spend: number
  unbudgeted: { category: string; spent: number; year_spent: number; history: MonthCell[]; suggestion?: Suggestion }[]
}

export interface Rule { id: number; priority: number; pattern: string; when_category: string; when_kind: string; set_category: string; set_merchant: string; add_tags: string[]; enabled: boolean }

export interface InboxRow {
  id: number; bank_account_id: number; external_id: string; raw_payee: string; raw_details: string; raw_currency: string; booking_date: string
  pending: boolean; date: string; kind: Kind; amount: number; account_id: string; to_account_id: string; category: string; merchant: string
  note: string; tags: string[]; edited: boolean; guessed: boolean; verdict: string; verdict_note: string; matched_tx_id?: number
  state: string; imported_tx_id?: number; match?: Tx
}
