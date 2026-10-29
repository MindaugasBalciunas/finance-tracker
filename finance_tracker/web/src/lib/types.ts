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
  refunds?: number
}

export interface Snapshot {
  date: string; net_worth: number; liquid: number; assets: number; debt: number
  by_group: Record<string, number>; by_account?: Record<string, number>; stale?: Record<string, string>
}

export interface Recurring {
  id?: number; source: 'detected' | 'edited' | 'manual'; note?: string; every_days?: number
  kind: 'bill' | 'transfer' | 'income'; from_account?: string; to_account?: string; day?: number; done: boolean; overdue: boolean
  merchant: string; category: string; cadence: string; amount: number; monthly: number; last: string; next: string
  count: number; changed: boolean; last_amount: number
}

export interface Anomaly { category: string; spent: number; typical: number; ratio: number; projected: number }

export interface LineBrief { name: string; spent: number; budgeted: number; remaining: number }

export interface Overview {
  date: string; net_worth: number; liquid: number; debt: number; by_group: Record<string, number>
  net_worth_30d: number; net_worth_ytd: number; net_worth_12m: number
  liquid_30d: number; liquid_ytd: number; liquid_12m: number
  spark: { date: string; value: number; liquid: number }[]
  month: Flow; last_month: Flow; avg12: Flow; year: Flow; month_progress: number; usual_saved?: number; usual_rate?: number
  plan: { safe_to_spend: number; income_base: number; spent: number; budgeted: number;
    free_spent: number; days_left: number; per_day_left: number; avg_day: number; typical_day: number; typical_one_offs?: number; expected_day: number; expected_rest?: number; projected_left: number; run_out_day?: number
    free_planned?: number; track?: { month: string; left: number; spent: number }[] | null; funds_over?: LineBrief[] | null; unreviewed?: number; unreviewed_count?: number; over: LineBrief[] | null; lines?: LineBrief[] | null
    income_actual: number; fixed_planned: number; fixed_spent: number; saving_planned: number; saved_actual: number; fund_set_aside: number; fund_overdrawn?: number
    fixed?: { name: string; budgeted: number; spent: number; due?: string; paid?: string }[] | null
    days?: { day: number; spent: number; cum?: number; typical: number; proj?: number }[] | null
    events?: { day: number; kind: 'fixed' | 'income'; label: string; amount: number; done: boolean }[] | null }
  emergency: { cash: number; monthly_essential: number; months: number; target_months: number; target: number }
  fi_progress: number; years_to_fi: number
  anomalies: Anomaly[] | null; upcoming: Recurring[] | null; stale: Record<string, string> | null
  inbox_open: number; recent: Tx[] | null
  checks: { level: string; text: string; link: string }[] | null; review_month?: string
  cash?: CashPlan
  actions?: Action[] | null
}

export interface CashItem { date: string; label: string; kind: 'obligation' | 'bill' | 'transfer' | 'income' | 'spending'; amount: number; within?: boolean }
export interface CashAccount {
  id: string; name: string; balance: number; balance_date: string; items: CashItem[] | null
  end: number; low: number; low_date: string; needed: number; needed_by: string
  stages: { amount: number; by: string; for: string }[] | null
  buffer: number; buffer_auto: boolean; buffer_why: string; idle: number
}
export interface CashPlan {
  payday: string; salary_account: string; accounts: CashAccount[]; moves: { from: string; from_name: string; to: string; to_name: string; amount: number; by: string; for: string }[]
  short: number; daily_account: string; daily: number; cash: number
  to_savings: { from: string; from_name: string; to: string; to_name: string; amount: number; by: string; for: string }[] | null
}
export interface Action { level: 'bad' | 'warn' | 'info'; kind: string; text: string; detail?: string; link?: string; due?: string }

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
  discretionary_spent: number; fund_contributions: number; fund_spent: number; fund_overdrawn?: number; refunds?: number; unreviewed?: number; unreviewed_count?: number; safe_to_spend: number
  unbudgeted: { category: string; spent: number; year_spent: number; history: MonthCell[]; suggestion?: Suggestion }[]
}

export interface Rule { id: number; priority: number; pattern: string; when_category: string; when_kind: string; set_category: string; set_merchant: string; add_tags: string[]; enabled: boolean }

export interface InboxRow {
  id: number; bank_account_id: number; external_id: string; raw_payee: string; raw_details: string; raw_currency: string; booking_date: string
  pending: boolean; date: string; kind: Kind; amount: number; account_id: string; to_account_id: string; category: string; merchant: string
  note: string; tags: string[]; edited: boolean; guessed: boolean; verdict: string; verdict_note: string; matched_tx_id?: number
  state: string; imported_tx_id?: number; match?: Tx
}

export interface NameAmount { name: string; amount: number; count: number }
export interface TagStat {
  tag: string; trip: boolean; count: number; spent: number; income: number; net: number; first: string; last: string
  months: number[]; year: number; per_month: number; categories: NameAmount[]; merchants: NameAmount[]; rules: number
}
export interface TagReport { tags: TagStat[]; tagged_share: number; year_spent: number; months: string[] }
export interface RuleStat { id: number; matches: number; recent: number; last: string; decides: number; overridden: number; shadowed: boolean; duplicate: number }
export interface RuleReport { rules: RuleStat[]; coverage: number; unused: number; shadowed: number; duplicates: number; overrode: number; top: number[] | null }
export interface CategoryUse { category: string; count: number; year: number; last: string; rules: number; budgeted: boolean }

// ── planning tools: payday, next euro, decisions, drift, ownership ──

export type RunStep = {
  key: string; kind: 'saving' | 'wishlist' | 'move'; title: string; detail?: string
  from?: string; from_name?: string; to?: string; to_name?: string
  amount: number; done: number; status: 'done' | 'partial' | 'todo'; by?: string; link?: string
}
export type Runbook = { month: string; payday: string; expected: string; salary: number; steps: RunStep[]; total: number; done_total: number; open: number }

export type EuroOption = { key: string; title: string; why: string; return: number; sure: boolean; room?: number; amount: number; to?: string; to_name?: string; link?: string }
export type NextEuro = { sum: number; options: EuroOption[]; notes: string[] }

export type Decision = {
  id: number; title: string; amount: number; category: string; status: 'thinking' | 'bought' | 'skipped'
  cool_until: string; decided_on: string; review_on: string; worth: '' | 'yes' | 'meh' | 'no'; note: string; created_at: string
}
export type DecisionRecord = {
  skipped: number; skipped_sum: number; bought: number; yes: number; meh: number; no: number; regretted_sum: number
  by_category: { category: string; rated: number; worth: number }[]
}
export type Impact = { amount: number; cooling_days: number; lines: { key: string; tone: 'good' | 'warn' | 'bad'; text: string }[] }

export type CreepWindow = { from: string; to: string; income: number; spending: number; fixed: number }
export type Creep = {
  years: number; then: CreepWindow; now: CreepWindow; income_growth: number; spending_growth: number; fixed_growth: number; gap: number
  tone: 'good' | 'warn' | 'bad'; text: string
  drivers: { category: string; then: number; now: number; change: number }[]
  new: { merchant: string; category: string; monthly: number; since: string }[]; new_monthly: number
}
export type PriceWatch = {
  merchant: string; category: string; cadence: string; now: number; last_date: string; year_ago: number; year_ago_date?: string
  change: number; yearly: number; extra: number; next?: string; renews_soon: boolean; advice?: string
}
export type Prices = {
  items: PriceWatch[]; rising: number; extra: number; personal: number | null; basket: number; weight: number
  official: { rate: number; month: string; source: string } | null
}

export type OwnLine = { key: string; label: string; amount: number; estimate?: boolean; link?: string }
export type OwnSettings = { tag: string; km_year: number; alt_per_km: number; rent: number }
export type OwnCost = {
  account_id: string; name: string; kind: 'property' | 'vehicle'; settings: OwnSettings; value: number; equity: number
  lines: OwnLine[]; running: OwnLine[]; year: number; month: number; per_km?: number; alt_year?: number
  verdict?: string; tone?: 'good' | 'warn'; missing: string[]
}
