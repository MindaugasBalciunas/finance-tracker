package domain

import (
	"strings"
	"time"
)

// Category represents a transaction category
type Category string

const (
	// Expense categories
	CategoryClothing      Category = "Clothing"
	CategoryDating        Category = "Dating"
	CategoryEntertainment Category = "Entertainment"
	CategoryFinance       Category = "Finance"
	CategoryFood          Category = "Food"
	CategoryGifts         Category = "Gifts"
	CategoryHealth        Category = "Health"
	CategoryHousing       Category = "Housing"
	CategoryKids          Category = "Kids"
	CategorySubscriptions Category = "Subscriptions"
	CategoryTransport     Category = "Transport"
	CategoryUtilities     Category = "Utilities"
	CategoryVacation      Category = "Vacation"

	// Income categories
	CategorySalary        Category = "Salary"
	CategoryFreelance     Category = "Freelance"
	CategoryReimbursement Category = "Reimbursement"

	// Investment categories
	CategoryStocksETF  Category = "Stocks & ETF"
	CategoryPension    Category = "Pension"
	CategoryCrypto     Category = "Crypto"
	CategoryRealEstate Category = "Real Estate"
	CategoryVehicle    Category = "Vehicle"
	// Transfers is money moved between own accounts (ATM cash, Revolut
	// top-ups) — investment-typed because it is not income or spending, but
	// excluded from invested totals.
	CategoryTransfers Category = "Transfers"
)

// ValidCategories is the authoritative list of allowed category values.
var ValidCategories = []Category{
	// Expense
	CategoryClothing,
	CategoryDating,
	CategoryEntertainment,
	CategoryFinance,
	CategoryFood,
	CategoryGifts,
	CategoryHealth,
	CategoryHousing,
	CategoryKids,
	CategorySubscriptions,
	CategoryTransport,
	CategoryUtilities,
	CategoryVacation,
	// Income
	CategorySalary,
	CategoryFreelance,
	CategoryReimbursement,
	// Investment
	CategoryStocksETF,
	CategoryPension,
	CategoryCrypto,
	CategoryRealEstate,
	CategoryVehicle,
	CategoryTransfers,
}

// legacyCategory describes a retired category value: the canonical category
// that replaced it plus the labels that preserve the old distinction.
type legacyCategory struct {
	canonical Category
	// income overrides canonical for income rows (Divorce recoveries are
	// reimbursements, not finance costs). Empty = same as canonical.
	income Category
	labels []string
}

// legacyCategories keeps old exports and clients importable: retired values
// are mapped, never rejected. The startup migration applies the same mapping
// to rows already in the database.
var legacyCategories = map[Category]legacyCategory{
	"Kids - Education":     {canonical: CategoryKids, labels: []string{"kids", "education"}},
	"Kids - Entertainment": {canonical: CategoryKids, labels: []string{"kids", "entertainment"}},
	"Kids - Food":          {canonical: CategoryKids, labels: []string{"kids", "food"}},
	"Kids - General":       {canonical: CategoryKids, labels: []string{"kids"}},
	"Divorce":              {canonical: CategoryFinance, income: CategoryReimbursement, labels: []string{"divorce"}},
	"Gaming":               {canonical: CategoryEntertainment, labels: []string{"gaming"}},
}

// CanonicalCategory resolves a possibly-retired category value to its current
// form and returns the labels that carry the retired distinction. Non-legacy
// values pass through unchanged (IsValidCategory decides acceptance).
func CanonicalCategory(t TransactionType, c Category) (Category, []string) {
	// Finance stays a valid expense category; only its investment-side use
	// (own-money movements) was renamed to Transfers in v1.5.0. The
	// cash/revolut labels these rows carry come from standing rules.
	if t == TransactionTypeInvestment && c == CategoryFinance {
		return CategoryTransfers, nil
	}
	l, ok := legacyCategories[c]
	if !ok {
		return c, nil
	}
	if t == TransactionTypeIncome && l.income != "" {
		return l.income, l.labels
	}
	return l.canonical, l.labels
}

// IsValidCategory returns true if the given category is in the allowed list.
func IsValidCategory(c Category) bool {
	for _, v := range ValidCategories {
		if v == c {
			return true
		}
	}
	return false
}

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeExpense    TransactionType = "expense"
	TransactionTypeIncome     TransactionType = "income"
	TransactionTypeInvestment TransactionType = "investment"
)

// Transaction represents a financial transaction record
// All amounts are in EUR.
type Transaction struct {
	ID   uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Date time.Time `json:"date" gorm:"not null;index;index:idx_tx_date_type,priority:1"`
	// Type is indexed both alone and composite with Date — every summary
	// filters/groups on type, and the dominant query is date-range + type.
	Type   TransactionType `json:"type" gorm:"not null;index;index:idx_tx_date_type,priority:2"`
	Amount float64         `json:"-" gorm:"not null"` // DB column; use AmountMoney in responses

	Comment  string   `json:"comment"`
	Category Category `json:"category" gorm:"index"`

	// Labels are free-form lowercase tags stored as a comma-separated list
	// (e.g. "loan,fixed"). Applied manually or by LabelRule on create.
	Labels string `json:"labels" gorm:"not null;default:''"`

	// Legacy field — kept for backward compat with existing rows. New rows use DebitAccount/CreditAccount.
	SourceAccount string `json:"source_account" gorm:"default:''"`

	// DebitAccount is the account decreased by this transaction (expense: paying account;
	// transfer/investment: funding source). Empty = no account debited.
	DebitAccount string `json:"debit_account" gorm:"not null;default:''"`

	// CreditAccount is the account increased by this transaction (income: receiving account;
	// transfer/investment: destination). Empty = no account credited.
	CreditAccount string `json:"credit_account" gorm:"not null;default:''"`

	// ExternalID is the provider row id for PSD2 imports
	// ("eb:<linkID>:<entry_reference>"). Empty for manual and CSV rows —
	// which is exactly why content-based dedup still has to run alongside it.
	ExternalID string `json:"external_id,omitempty" gorm:"index;default:''"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed for API response (not persisted)
	AmountMoney Money `json:"amount" gorm:"-"`

	// BalanceNote explains why creating this transaction did NOT move an
	// account balance (no account selected, dated before the latest
	// snapshot, no snapshot to base on…). Empty when the balance was
	// adjusted — or when the response isn't from a create/update. Set on the
	// create path only; a silent skip is how "the balance didn't adjust"
	// goes unnoticed, so the UI shows this verbatim.
	BalanceNote string `json:"balance_note,omitempty" gorm:"-"`
}

// TransactionFilter holds filtering options for querying transactions
type TransactionFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
	Type     *TransactionType
	Category *Category
	// Label may be a comma list; LabelMode "all" requires every label on the
	// row (intersection), anything else matches any of them (union).
	Label     string
	LabelMode string
	// Search is a case-insensitive substring match on the comment.
	Search string
	// AmountMin/AmountMax bound the transaction amount (EUR), inclusive.
	AmountMin *float64
	AmountMax *float64
	// Sort: "date" (default), "amount" or "comment"; Desc defaults to true
	// for date/amount (newest/biggest first) and is overridable via Dir.
	Sort     string
	Dir      string // "asc" | "desc" | "" (per-column default)
	Page     int
	PageSize int
}

// NormalizeLabels canonicalizes a comma-separated label list: lowercase,
// trimmed, deduplicated, no empties.
func NormalizeLabels(raw string) string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		l := strings.ToLower(strings.TrimSpace(part))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return strings.Join(out, ",")
}

// HasLabel reports whether the transaction carries the given label.
func (t *Transaction) HasLabel(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	for _, l := range strings.Split(t.Labels, ",") {
		if l == label {
			return true
		}
	}
	return false
}

// AddLabel appends a label if not already present.
func (t *Transaction) AddLabel(label string) {
	if t.HasLabel(label) {
		return
	}
	t.Labels = NormalizeLabels(t.Labels + "," + label)
}

// FixedObligationLabels mark money that isn't a spending decision (loan
// instalments, alimony, …). Insights and the frontend (FIXED_LABELS in
// utils/labels.ts) treat them specially, so label management refuses to
// rename or delete them.
var FixedObligationLabels = []string{"loan", "alimony", "leasing", "evelina"}

// IsFixedObligationLabel reports whether the label is one of the protected
// fixed-obligation labels.
func IsFixedObligationLabel(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	for _, f := range FixedObligationLabels {
		if label == f {
			return true
		}
	}
	return false
}

// RenameLabelToken rewrites one token of a comma-separated label list,
// deduplicating when the target token is already present. The boolean
// reports whether the list changed.
func RenameLabelToken(labels, from, to string) (string, bool) {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	changed := false
	parts := strings.Split(labels, ",")
	for i, p := range parts {
		if strings.ToLower(strings.TrimSpace(p)) == from {
			parts[i] = to
			changed = true
		}
	}
	if !changed {
		return labels, false
	}
	return NormalizeLabels(strings.Join(parts, ",")), true
}

// RemoveLabelToken drops one token from a comma-separated label list. The
// boolean reports whether the list changed.
func RemoveLabelToken(labels, label string) (string, bool) {
	label = strings.ToLower(strings.TrimSpace(label))
	changed := false
	var out []string
	for _, p := range strings.Split(labels, ",") {
		if strings.ToLower(strings.TrimSpace(p)) == label {
			changed = true
			continue
		}
		out = append(out, p)
	}
	if !changed {
		return labels, false
	}
	return NormalizeLabels(strings.Join(out, ",")), true
}

// LabelStat summarizes one label's footprint across the database.
type LabelStat struct {
	Label        string  `json:"label"`
	Transactions int     `json:"transactions"`
	Amount       float64 `json:"amount"`
	Rules        int     `json:"rules"`
	Budgets      int     `json:"budgets"`
	Fixed        bool    `json:"fixed"`
	FirstUsed    string  `json:"first_used"`
	LastUsed     string  `json:"last_used"`
}

// RelabelResult counts what a label rename/delete touched.
type RelabelResult struct {
	Transactions int `json:"transactions"`
	Rules        int `json:"rules"`
	Budgets      int `json:"budgets"`
}

// LabelRule auto-applies a label to transactions on create when the category
// matches (empty = any) and the comment contains CommentMatch (empty = any).
// A CommentMatch starting with '^' anchors to the start of the comment —
// for short store names ("iki") that substring-match everyday words.
type LabelRule struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Label        string    `json:"label" gorm:"not null;index"`
	Category     string    `json:"category" gorm:"not null;default:''"`
	CommentMatch string    `json:"comment_match" gorm:"not null;default:''"`
	CreatedAt    time.Time `json:"created_at"`
}

// CommentPatternMatches reports whether a rule comment pattern (possibly
// '^'-anchored) matches the comment. Case-insensitive.
func CommentPatternMatches(pattern, comment string) bool {
	p := strings.ToLower(pattern)
	c := strings.ToLower(comment)
	if anchored, ok := strings.CutPrefix(p, "^"); ok {
		return strings.HasPrefix(c, anchored)
	}
	return strings.Contains(c, p)
}

// Matches reports whether the rule applies to the transaction.
func (r *LabelRule) Matches(t *Transaction) bool {
	if r.Category != "" && !strings.EqualFold(r.Category, string(t.Category)) {
		return false
	}
	if r.CommentMatch != "" && !CommentPatternMatches(r.CommentMatch, t.Comment) {
		return false
	}
	return r.Category != "" || r.CommentMatch != ""
}

// Budget is a financial plan line. Kind semantics:
//   - fixed: a known obligation (loan, alimony) — tracked as paid/pending
//   - investment: a monthly contribution target to reach
//   - spending: a limit for discretionary spending
//   - trip: a one-off budget for one trip, matched by its trip:… label;
//     outside the monthly plan (the Vacation line already carries it)
//
// Period says what Amount covers: a month, or a calendar year. A Fund line
// (spending only) accrues its monthly share from StartMonth on, spending
// draws it down and whatever is left carries over — the shape lumpy costs
// like holidays need, where a monthly cap is wrong eleven months a year.
//
// Matching: transactions with Label (when set), otherwise by Category.
// Label may be a comma-separated list ("restaurant,fast food,delivery") —
// a transaction carrying any of them matches, so label groups can share
// one budget.
type Budget struct {
	ID       uint    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name     string  `json:"name" gorm:"not null"`
	Kind     string  `json:"kind" gorm:"not null"`
	Label    string  `json:"label" gorm:"not null;default:''"`
	Category string  `json:"category" gorm:"not null;default:''"`
	Amount   float64 `json:"amount" gorm:"not null"`
	Period   string  `json:"period" gorm:"not null;default:'monthly'"`
	Fund     bool    `json:"fund" gorm:"not null;default:false"`
	// StartMonth ("YYYY-MM") is when a fund starts accruing. Empty = the
	// month the line was created.
	StartMonth string    `json:"start_month" gorm:"not null;default:''"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Budget periods.
const (
	PeriodMonthly = "monthly"
	PeriodYearly  = "yearly"
)

// MonthlyShare is what the line claims of one month's income.
func (b *Budget) MonthlyShare(amount float64) float64 {
	if b.Period == PeriodYearly {
		return amount / 12
	}
	return amount
}

// BudgetAmount is one step of a line's amount history: Amount applies from
// FromMonth ("YYYY-MM") until the next step. Changing a budget adds a step
// instead of rewriting the line, so past months stay judged against the
// limit they actually had. A line with no steps uses Budget.Amount
// everywhere; FromMonth "" is the open-ended first step.
type BudgetAmount struct {
	ID        uint    `json:"id" gorm:"primaryKey;autoIncrement"`
	BudgetID  uint    `json:"budget_id" gorm:"not null;index"`
	FromMonth string  `json:"from_month" gorm:"not null;default:''"`
	Amount    float64 `json:"amount" gorm:"not null"`
}

// BudgetSettings is a single-row table configuring the Budget page's income
// base. IncomeMode: "median" (auto from history), "manual" (ManualIncome), or
// "gross" (net computed client-side from GrossSalary − MonthlyDeductions
// under LT employee tax rules).
type BudgetSettings struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	IncomeMode        string    `json:"income_mode" gorm:"not null;default:'median'"`
	ManualIncome      float64   `json:"manual_income" gorm:"not null;default:0"`
	GrossSalary       float64   `json:"gross_salary" gorm:"not null;default:0"`
	MonthlyDeductions float64   `json:"monthly_deductions" gorm:"not null;default:0"`
	UpdatedAt         time.Time `json:"updated_at"`
}

var ValidIncomeModes = []string{"median", "manual", "gross"}

func IsValidIncomeMode(m string) bool {
	for _, v := range ValidIncomeModes {
		if v == m {
			return true
		}
	}
	return false
}

var ValidBudgetKinds = []string{"fixed", "investment", "spending", "trip"}

// TripLabelPrefix marks the labels that group one trip's transactions.
const TripLabelPrefix = "trip:"

func IsValidBudgetKind(k string) bool {
	for _, v := range ValidBudgetKinds {
		if v == k {
			return true
		}
	}
	return false
}

// TransactionSummary holds aggregated transaction data
type TransactionSummary struct {
	TotalExpenses    float64           `json:"total_expenses"`
	TotalIncome      float64           `json:"total_income"`
	TotalInvestments float64           `json:"total_investments"`
	NetBalance       float64           `json:"net_balance"`
	ByCategory       []CategorySummary `json:"by_category"`
	ByMonth          []MonthlySummary  `json:"by_month"`
	ByLabel          []LabelSummary    `json:"by_label"`
}

// LabelSummary holds totals per label within a filtered summary. A row
// carrying several labels counts toward each of them; Transfers rows are
// excluded (internal moves, not money in or out).
type LabelSummary struct {
	Label string  `json:"label"`
	Total float64 `json:"total"`
	Count int     `json:"count"`
}

// CategorySummary holds totals per category
type CategorySummary struct {
	Category Category        `json:"category"`
	Type     TransactionType `json:"type"`
	Total    float64         `json:"total"`
	Count    int             `json:"count"`
}

// MonthlySummary holds monthly totals
type MonthlySummary struct {
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	MonthName   string  `json:"month_name"`
	Expenses    float64 `json:"expenses"`
	Income      float64 `json:"income"`
	Investments float64 `json:"investments"`
}

// PaginatedTransactions wraps a list of transactions with pagination metadata
type PaginatedTransactions struct {
	Data       []Transaction `json:"data"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}
