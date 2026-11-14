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
  /** The recurring item this pays (set from the editor's "Recurring payment" switch). */
  recurring_id?: number
  source: string
  pending?: boolean
}

export interface TxList { items: Tx[]; total: number; income: number; expense: number; transfer: number }

export interface Category { id: string; parent?: string; name: string; kind: Kind; essential: boolean; color?: string; sort: number; archived: boolean }

export interface Account {
  id: string; name: string; institution: string; kind: string; group: string; currency: string
  liquid: boolean; archived: boolean; sort: number; notes: string; details: Record<string, unknown> | null
  balance?: number | null; balance_date?: string; quantity?: number; source?: string
}

export interface Flow {
  period: string; income: number; spending: number; essential: number; discretionary: number; saved: number
  savings_rate: number; invested: number; principal: number; payroll_pension: number
  income_by: Record<string, number>; spending_by: Record<string, number>
  refunds?: number
  /** Salary still due for the month (its pay window is open): what the plan expects, and the rate counting it. */
  expected_salary?: number; expected_savings_rate?: number
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
/** One slice of the month's free budget (plan.FreePart): left = budgeted − spent, negative when over. */
export interface FreePart { budgeted: number; spent: number; left: number; lines: string[] }
/** The month's spending lines — obligations and saving left out — with needs and wants apart (plan.FreeBudget). */
/** The month from the income's side (plan.FreeMoney): free = income − fixed − saving;
 *  spent = essential + optional + unreviewed − refunds; free − spent = in_funds + left_to_spend
 *  (in_funds < 0: funds paid from earlier months' savings). */
export interface FreeMoney { income: number; fixed: number; saving: number; free: number; spent: number
  essential: number; optional: number; refunds?: number; unreviewed: number; in_funds: number; left_to_spend: number }
export interface FreeBudget extends FreePart { needs: FreePart; wants: FreePart
  /** Part of `left` still held by fund lines; left − in_funds is the day-to-day part. */
  in_funds?: number
  /** What funds paid this month (inside `spent`). */
  fund_spent?: number }

export interface Overview {
  date: string; net_worth: number; liquid: number; debt: number; by_group: Record<string, number>
  net_worth_30d: number; net_worth_ytd: number; net_worth_12m: number
  liquid_30d: number; liquid_ytd: number; liquid_12m: number
  spark: { date: string; value: number; liquid: number }[]
  month: Flow; last_month: Flow; avg12: Flow; year: Flow; month_progress: number; usual_saved?: number; usual_rate?: number
  plan: { safe_to_spend: number; income_base: number; spent: number; budgeted: number; free?: FreeBudget; free_money?: FreeMoney
    free_spent: number; days_left: number; per_day_left: number; avg_day: number; typical_day: number; typical_one_offs?: number; expected_day: number; expected_rest?: number; projected_left: number; run_out_day?: number
    free_planned?: number; track?: { month: string; left: number; spent: number }[] | null; funds_over?: LineBrief[] | null; unreviewed?: number; unreviewed_count?: number; over: LineBrief[] | null; lines?: LineBrief[] | null
    income_actual: number; fixed_planned: number; fixed_spent: number; saving_planned: number; saved_actual: number; fund_set_aside: number; fund_overdrawn?: number
    fixed?: { name: string; budgeted: number; spent: number; due?: string; paid?: string }[] | null
    days?: { day: number; spent: number; cum?: number; typical: number; proj?: number }[] | null
    events?: { day: number; kind: 'fixed' | 'income'; label: string; amount: number; done: boolean }[] | null }
  emergency: { cash: number; monthly_essential: number; months: number; target_months: number; target: number }
  /** The emergency fund after what leaves before payday and the wish list's set-asides. */
  emergency_plan?: EmergencyPlan
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
  lines: PlanLine[]; fixed_planned: number; saving_planned: number; spending_planned: number; fixed_spent: number; fixed_expense?: number; saved_actual: number
  discretionary_spent: number; fund_contributions: number; fund_spent: number; fund_overdrawn?: number; refunds?: number; unreviewed?: number; unreviewed_count?: number; safe_to_spend: number
  unbudgeted: { category: string; spent: number; year_spent: number; history: MonthCell[]; suggestion?: Suggestion }[]
  /** The month's spending lines as needs and wants (plan.FreeBudget). */
  free?: FreeBudget
  free_money?: FreeMoney
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
  id: number; title: string; amount: number; category: string; status: 'thinking' | 'saving' | 'bought' | 'skipped'; goal_id?: number
  cool_until: string; decided_on: string; review_on: string; worth: '' | 'yes' | 'meh' | 'no'; note: string; created_at: string
}
export type DecisionRecord = {
  saving: number; skipped: number; skipped_sum: number; bought: number; yes: number; meh: number; no: number; regretted_sum: number
  by_category: { category: string; rated: number; worth: number }[]
}
export type Impact = { amount: number; cooling_days: number; lines: { key: string; tone: 'good' | 'warn' | 'bad'; text: string }[] }

export type CreepWindow = { from: string; to: string; income: number; spending: number; fixed: number }
export type Creep = {
  years: number; then: CreepWindow; now: CreepWindow; income_growth: number; spending_growth: number; fixed_growth: number; gap: number
  tone: 'good' | 'warn' | 'bad'; text: string
  drivers: { category: string; then: number; now: number; change: number }[]
  fallers: { category: string; then: number; now: number; change: number }[]
  new: { merchant: string; category: string; monthly: number; since: string }[]; new_monthly: number
}
export type PriceWatch = {
  merchant: string; category: string; cadence: string; now: number; last_date: string; year_ago: number; year_ago_date?: string
  change: number; yearly: number; extra: number; next?: string; renews_soon: boolean; advice?: string; plan_change?: boolean
}
export type Prices = {
  items: PriceWatch[]; rising: number; extra: number; personal: number | null; basket: number; weight: number; excluded: number
  official: { rate: number; month: string; source: string } | null
}

export type OwnLine = { key: string; label: string; amount: number; estimate?: boolean; info?: boolean; link?: string }
export type OwnSettings = { tag: string; km_year: number; alt_per_km: number; rent: number }
export type OwnCost = {
  account_id: string; name: string; kind: 'property' | 'vehicle'; settings: OwnSettings; value: number; equity: number
  lines: OwnLine[]; running: OwnLine[]; year: number; month: number; per_km?: number; alt_year?: number
  verdict?: string; tone?: 'good' | 'warn'; missing: string[]
}

// ── independence, milestones, fees, subscriptions, cash calendar ──

export type FreedomLevel = { key: string; title: string; detail: string; monthly: number; capital: number; progress: number; date?: string; months: number; age?: number; done: boolean }
export type FIPlan = {
  annual_spend: number; spend_source: string; target: number; investable: number; progress: number; monthly_invested: number
  expected_return: number; withdrawal_rate: number; years_to_fi: number; fi_date?: string; fi_age?: number; target_age?: number; required_monthly?: number
  emergency: { cash: number; monthly_essential: number; months: number; target_months: number; target: number }
  levels: FreedomLevel[]; passive_now: number; obligations: number; essentials: number; spending: number
  coast: { age: number; needed: number; date?: string; months: number; done: boolean; detail: string; later?: { age: number; date?: string; done: boolean }[] }
  payouts: PayoutPoint[]
  /** Month by month for ten years (the 1–10 year views); only with ?monthly=1. */
  payouts_monthly?: PayoutPoint[]
  what_ifs: { extra: number; date?: string; sooner: number }[]
  savings_rate: number; locked: number; pillar3?: number; notes: string[]
}
export type Milestone = { key: string; title: string; detail: string; target?: number; current?: number; date?: string; months: number; done: boolean }
export type FeeLine = { id: string; name: string; kind: 'fund' | 'stock' | 'account'; value: number; ter: number; platform: number; source: 'yours' | 'typical' | 'none' | 'missing'; yearly: number; drag_20y: number; saving_20y: number }
export type Fees = { lines: FeeLine[]; value: number; yearly: number; weighted: number; drag_20y: number; saving_20y: number; missing: number; return: number; low_cost: number }
export type Subscription = {
  key: string; merchant: string; category: string; cadence: string; amount: number; yearly: number; since: string; last: string; next?: string
  new: boolean; trial_from?: string; verdict?: { verdict: 'keep' | 'cancel'; cancel_by?: string; decided_on?: string }; charged_after_cancel?: string
}
export type Subscriptions = { items: Subscription[]; monthly: number; yearly: number; to_cancel: number }
export type CalEvent = { label: string; kind: 'income' | 'bill' | 'transfer'; amount: number }
export type CashCalendar = { start: number; floor: number; daily: number; days: { date: string; events?: CalEvent[]; balance: number; low: boolean; short: boolean }[]; low: number; low_date: string; first_below?: string; first_short?: string }

// ── merchants ──

export type MerchantStat = {
  name: string; category: string; total: number; share: number; count: number; average: number; previous: number; change: number
  first: string; last: string; new: boolean; months: { month: string; amount: number }[]
}
export type MerchantReport = {
  from: string; to: string; prev_from: string; prev_to: string; total: number; unnamed: number; count: number; new_count: number; new_total: number
  merchants: MerchantStat[]; lapsed: { name: string; category: string; last: string; monthly: number }[]
}

/** A projected point: what is invested and what it could pay a month if you stopped then. `month` on the month-by-month points. */
export type PayoutPoint = { year: number; month?: string; age?: number; investable: number; monthly: number }

export type EmergencyPlan = {
  bank: number; cash: number; counts_cash: boolean; committed: number; set_aside: number; available: number
  monthly_essential: number; target_months: number; target: number; months: number; gap: number; spare: number; payday?: string
}

/** A transaction as a recurring payment: what it is marked as, what it already matches (and how), the items to pick from and the item it would make. */
export type TxRecurring = {
  recurring_id: number; match_id: number; match_by: '' | 'marked' | 'merchant' | 'habit'; marked: boolean
  items: { id: number; merchant: string; note: string; cadence: string; amount: number; kind: 'bill' | 'income' | 'transfer'; hidden: boolean }[]
  new: { merchant: string; amount: number; day: number; cadence: string; kind: string }
}

/** /insights/habits: spending by choice, complete months. */
export type HabitMonth = {
  month: string; spent: number; purchases: number; median: number; small: number; small_spent: number; days: number
  early: number; mid: number; late: number; weekend: number; eating_out: number; groceries: number
}
export type Habits = {
  months: HabitMonth[]
  weekdays: { day: number; average: number }[]
  season: { month: number; average: number; years: number }[]
  recent: [string, string]; year_ago: [string, string]
  changes: { key: 'spent' | 'purchases' | 'median' | 'small' | 'eating_out' | 'weekend' | 'days'; now: number; then: number }[]
}

// ── stress test ──

/** One crisis: percentages change the plan's own figures; months is how long it lasts (0 = the whole period). */
export type Shock = {
  months?: number; income_pct?: number; benefit?: number; everyday_pct?: number; fixed_pct?: number; one_off?: number; rate_pp?: number
  stocks_pct?: number; crypto_pct?: number; pension_pct?: number; property_pct?: number; pause_saving?: boolean; pause_goals?: boolean
}
export type StressLevel = 'ok' | 'tight' | 'pause' | 'sell' | 'short'
export type StressResult = {
  id: string; group?: string; name: string; blurb?: string; shock: Shock
  months?: { month: string; income: number; out: number; sold?: number; end: number; base: number }[]
  level: StressLevel; lowest: number; lowest_on: string; below_floor?: string; paused_from?: string; saving_cut: number; cash_out?: string; broke?: string
  sold?: { key: string; label: string; amount: number; net: number }[]; sold_total: number; short: number
  market_hit: number; loan_extra: number; monthly_gap: number; net_worth_end: number; base_net_worth_end: number
  /** Lost for good against the plan; paper_loss is the drop still held (back if prices recover); locked_in the part of the drop made real by selling. */
  cost: number; paper_loss: number; locked_in: number
  summary: string[]; levers?: { label: string; amount: number; monthly: boolean }[]
}
export type StressReport = {
  start: string; horizon: number; cash: number; floor: number; net_worth: number
  reserves: { key: string; label: string; value: number; net: number }[]; locked: number; real_assets: number
  /** Absent until there is a usual month to go by. */
  runway?: { burn: number; cash_months: number; reserve_months: number }
  /** The emergency target in months of a usual month's cost, beside the months of essentials it was set to. */
  floor_months?: number; target_months?: number
  base?: number[]; groups: { id: string; name: string }[]; scenarios: StressResult[]; custom?: StressResult; notes: string[]
}

// ── plan: trips and settings ──

/** A trip:<name> tag costed as a whole (/trips). */
export type Trip = {
  tag: string; name: string; from: string; to: string; days: number; count: number; total: number; booked_from?: string
  per_day: number; by_category: { name: string; amount: number }[]; budget?: number; done: boolean
}
/** Untagged travel that looks like one trip. */
export type TripSuggestion = { from: string; to: string; days: number; count: number; total: number; tx_ids: number[]; top: string[] }
/** A wish-list goal saving for a trip (tagged trip:…). */
export type PlannedTrip = { id: number; name: string; target: number; saved: number; spent?: number; tag: string; target_date: string }
export type TripDates = { tag: string; start: string; end: string }
export type Trips = { trips: Trip[]; suggestions: TripSuggestion[]; planned: PlannedTrip[]; dates: TripDates[] | null }

export type GrossChange = { from: string; gross: number }
export type SalaryRule = { from: string; paid_by_day: number }
/** plan.Settings: the income base, salary timing and FI inputs. */
export type PlanSettings = {
  income_mode: '' | 'median' | 'manual' | 'gross'; manual_income: number; gross_salary: number; gross_changes?: GrossChange[]; monthly_deductions: number
  target_age: number; birth_year: number; fi_monthly_spend: number; withdrawal_rate: number; expected_return: number; emergency_months: number
  emergency_exclude_cash?: boolean; salary_rules: SalaryRule[] | null; salary_account: string; buffers?: Record<string, number>
}

// ── bank sync ──

export type BankAccount = {
  id: number; connection_id: number; iban: string; display_name: string; account_id: string; last_synced_at: string; last_tx_date: string
  bank_balance: number | null; bank_balance_currency: string; bank_balance_date: string; bank_balance_fetched: string; open: number
}
/** A bank consent (/bank/connections); status is effective — a lapsed one reads "expired". */
export type BankConnection = {
  id: number; aspsp_name: string; aspsp_country: string; status: string; valid_until: string; last_error?: string; days_left: number; accounts: BankAccount[]
}
/** POST /sync: banks, IBKR and crypto in one go; skipped when an auto sync ran recently. */
export type SyncResult = {
  skipped?: boolean; bank?: { new: number }; ibkr?: { new: number; mismatches: number }
  balances?: { account: string; name: string; from: number; to: number; source: string }[]; problems?: string[]
}

// ── Interactive Brokers ──

export type IBKRStatus = { connected: boolean; lapsed?: boolean; sign_in_ends?: string; account: string; scope?: string; connected_at?: string; last_sync?: string; last_error?: string }
export type IBKRReport = {
  at: string; account: string; currency: string; net_liquidation: number; cash: number; app_balance?: number; balance_written: boolean
  positions: { ticker: string; description: string; currency: string; shares: number; app_shares: number; price: number; value: number; avg_price: number; unrealized: number }[]
  trades: { external_id: string; date: string; action: string; ticker: string; name: string; currency: string; shares: number; price: number; commission: number; recorded: boolean }[]
  new: number; mismatches: number
  orders?: IBKROrder[] | null; instructions?: IBKROrder[] | null; orders_error?: string
  trades_window?: string; trades_error?: string
}
export type IBKROrder = { id: string; symbol: string; description?: string; side: string; type: string; status?: string; quantity: number; price?: number; filled?: number; remaining?: number; tif?: string; created?: string; expires?: string }

// ── investments (wealth.Portfolio; amounts are euros in the share's own maths) ──

export type Trade = { id: number; date: string; account_id: string; action: 'buy' | 'sell'; ticker: string; shares: number; price: number; currency: string; notes: string; external_id?: string }
export type Holding = {
  accounts?: { account_id: string; shares: number }[]; ticker: string; name?: string; account_id: string; currency: string
  shares: number; avg_cost: number; cost: number; realized: number; first_buy: string; price?: number; price_as_of?: string
  value?: number; gain?: number; gain_pct?: number; value_eur?: number; cost_eur?: number; week52_high?: number; week52_low?: number
  cost_share: number; day_pct?: number; day_change_eur?: number; weight: number; quote_error?: string
}
export type Portfolio = { holdings: Holding[]; closed: Holding[]; cost_eur: number; value_eur: number; gain_eur: number; realized_eur: number; usd_per_eur: number; day_change_eur: number; live: boolean }
export type PerfPoint = { date: string; value_eur: number; cost_eur: number }
export type Scenarios = {
  positions: { ticker: string; value_eur: number; low_eur?: number; mean_eur?: number; high_eur?: number; analysts: number; basis: string }[]
  value_eur: number; low_eur: number; mean_eur: number; high_eur: number; covered: number
}
export type PricePoint = { date: string; close: number }
export type AnalystData = { ticker: string; target_low: number; target_mean: number; target_high: number; recommendation: string; num_analysts: number; currency: string }

// ── settings: security, banks, data ──

export type AuthStatus = { enabled: boolean; unlocked: boolean; passkeys: number; has_token: boolean; has_token_rw: boolean; has_token_sync?: boolean }
export type Passkey = { id: number; name: string; created_at: string }
export type BankSettings = {
  application_id: string; has_key: boolean; environment: string; redirect_url: string; owner_names: string[] | null; consent_days: number
  configured: boolean; keep_reserved_in_inbox: boolean
}
export type Backups = { snapshots: { name: string; size: number; at: string }[]; last_download: string }

// ── ledger and plan odds and ends ──

/** POST /bank/sync. */
export type BankSyncResult = {
  accounts: { bank_account_id: number; name: string; bank: string; fetched: number; new: number; from: string; error?: string; skipped?: string }[]
  fetched: number; new: number; updated: number; duplicates: number; pending: number; superseded: number; released: number
  auto_linked: number; reserved_added: number; settled: number; failed: number
}
/** POST /bank/inbox/commit. */
export type CommitResult = { imported: number[]; skipped: number; notes?: string[] }
/** Expenses that look recurring but aren't tracked yet (/insights/recurring). */
export type RecurringSuggestion = {
  name: string; merchant?: string; category: string; amount: number; every_days: number; spread_days: number; flexible: boolean
  count: number; last: string; next: string; notes: string[] | null
}
/** Money set aside for (or taken from) a wish-list goal. */
export type GoalMove = { id: number; goal_id: number; month: string; amount: number; kind: string; note: string; account_id: string; created_at: string }

// ── insights: spending breakdown ──

export type CategoryStat = {
  category: string; total: number; share: number; count: number; monthly_avg: number; previous: number; change: number
  top_merchant?: string; children?: CategoryStat[]
}
export type NamedCount = { name: string; category: string; amount: number; count: number }
/** /insights/breakdown: where the money went over a window. */
export type Breakdown = { from: string; to: string; categories: CategoryStat[]; merchants: NamedCount[]; largest: Tx[]; tags: NamedCount[] }

/** /usage/summary: which pages get used, and the ones you had to hunt for. */
export type UsageSummary = {
  since: string; sessions: number; events: number; views: number
  pages: { path: string; views: number; avg_sec: number; total_min: number; bounces: number; last_seen: string }[]
  actions: { path: string; label: string; count: number }[]
  hunts: { target: string; count: number; avg_hops: number; avg_sec: number; entry: string; typical_path: string[] | null }[]
  days: { day: string; views: number }[]; unused: string[]; suggestions: string[]; names: Record<string, string>
}

// ── wealth: balance history ──

export type BalanceReading = { account_id: string; at: string; value: number; source: string }
export type BalanceCell = { value: number; recorded?: boolean; source?: string; at?: string }
/** /balances/table: every date a balance was recorded, newest first, paged. */
export type BalanceTable = {
  accounts: string[]; rows: { date: string; cells: Record<string, BalanceCell>; readings?: BalanceReading[]; net_worth: number; liquid: number }[]
  page: number; pages: number; dates: number; size: number
}
/** /networth/movement: which accounts drove net worth between two dates. */
export type AccountMove = { account_id: string; name: string; group: string; start: number; end: number; change: number }

// ── insights: month review ──

/** /insights/month: one month against its own past (cfo.MonthReview). */
export type MonthReview = {
  month: string; complete: boolean; final: boolean; flow: Flow; previous: Flow; six_month_avg: Flow; verdict: string; tone: 'good' | 'neutral' | 'bad'
  highlights: { tone: string; text: string }[]
  net_worth: { start: number; end: number; change: number; points: { date: string; value: number; liquid: number }[] | null }
  categories: { category: string; spent: number; average: number; delta: number; count: number; top: Tx[] | null }[]
  top_expenses: Tx[]; top_income: Tx[]; daily: { date: string; spent: number; in: number }[]; trend: Flow[]
  budget?: { over: LineBrief[] | null; within_count: number; lines: LineBrief[] | null; safe_to_spend: number }
  owed_to_you: number; checks: { level: 'warn' | 'info'; text: string; link: string }[]
}

// ── wealth: loans ──

/** ledger.LoanDetails: a loan account's terms. */
export type LoanDetails = {
  lender?: string; asset_id?: string; base_rate: number; base_rate_name?: string; margin: number; rate_reset_date?: string
  monthly_payment: number; payment_day?: number; end_date?: string; start_date?: string; start_principal?: number
}
/** /loans: one loan with its schedule, equity and what's left to pay. */
export type LoanView = {
  account_id: string; name: string; details: LoanDetails; balance: number; balance_date: string; rate: number; months_left: number
  next_interest: number; next_principal: number; total_interest_left: number; payoff_date: string; asset_value?: number; equity?: number
  ltv?: number; days_to_reset?: number; schedule: { month: string; balance: number; interest: number; principal: number }[] | null
  paid_principal_12m: number; start_principal?: number; repaid?: number; repaid_pct?: number
  purchase_price?: number; down_payment?: number; appreciation?: number; paid_interest_12m: number
}
/** /insights/review: a calendar year against the one before. */
export type YearReview = {
  year: string; flow: Flow; previous: Flow; months: Flow[] | null; categories: CategoryStat[]; merchants: NamedCount[]; largest: Tx[]
  net_worth_start: number; net_worth_end: number; years: Flow[] | null
}
/** /balances?account=: one account's recorded values. */
export type BalancePoint = { account_id: string; date: string; value: number; quantity?: number; price?: number; source: string; at?: string }

/** /ai/scan and /ai/assist: a transaction drafted from a receipt photo or a sentence. */
export type AIScan = { date: string; amount: number; kind: Kind; category: string; merchant: string; note: string; account_id: string; tags: string[] | null; remark: string; currency: string }
