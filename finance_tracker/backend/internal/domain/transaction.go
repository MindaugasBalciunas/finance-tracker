package domain

import (
	"strings"
	"time"
)

// Category represents a transaction category
type Category string

const (
	// Expense categories
	CategoryClothing          Category = "Clothing"
	CategoryDating            Category = "Dating"
	CategoryDivorce           Category = "Divorce"
	CategoryEntertainment     Category = "Entertainment"
	CategoryFinance           Category = "Finance"
	CategoryFood              Category = "Food"
	CategoryGaming            Category = "Gaming"
	CategoryGifts             Category = "Gifts"
	CategoryHealth            Category = "Health"
	CategoryHousing           Category = "Housing"
	CategoryKidsEducation     Category = "Kids - Education"
	CategoryKidsEntertainment Category = "Kids - Entertainment"
	CategoryKidsFood          Category = "Kids - Food"
	CategoryKidsGeneral       Category = "Kids - General"
	CategoryTransport         Category = "Transport"
	CategoryVacation          Category = "Vacation"

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
)

// ValidCategories is the authoritative list of allowed category values.
var ValidCategories = []Category{
	// Expense
	CategoryClothing,
	CategoryDating,
	CategoryDivorce,
	CategoryEntertainment,
	CategoryFinance,
	CategoryFood,
	CategoryGaming,
	CategoryGifts,
	CategoryHealth,
	CategoryHousing,
	CategoryKidsEducation,
	CategoryKidsEntertainment,
	CategoryKidsFood,
	CategoryKidsGeneral,
	CategoryTransport,
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
	ID     uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	Date   time.Time       `json:"date" gorm:"not null;index"`
	Type   TransactionType `json:"type" gorm:"not null"`
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

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed for API response (not persisted)
	AmountMoney Money `json:"amount" gorm:"-"`
}

// TransactionFilter holds filtering options for querying transactions
type TransactionFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
	Type     *TransactionType
	Category *Category
	Label    string
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

// LabelRule auto-applies a label to transactions on create when the category
// matches (empty = any) and the comment contains CommentMatch (empty = any).
type LabelRule struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Label        string    `json:"label" gorm:"not null"`
	Category     string    `json:"category" gorm:"not null;default:''"`
	CommentMatch string    `json:"comment_match" gorm:"not null;default:''"`
	CreatedAt    time.Time `json:"created_at"`
}

// Matches reports whether the rule applies to the transaction.
func (r *LabelRule) Matches(t *Transaction) bool {
	if r.Category != "" && !strings.EqualFold(r.Category, string(t.Category)) {
		return false
	}
	if r.CommentMatch != "" && !strings.Contains(strings.ToLower(t.Comment), strings.ToLower(r.CommentMatch)) {
		return false
	}
	return r.Category != "" || r.CommentMatch != ""
}

// Budget is a monthly financial plan line. Kind semantics:
//   - fixed: a known obligation (loan, alimony) — tracked as paid/pending
//   - investment: a monthly contribution target to reach
//   - spending: a monthly limit for discretionary spending
//
// Matching: transactions with Label (when set), otherwise by Category.
type Budget struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name      string    `json:"name" gorm:"not null"`
	Kind      string    `json:"kind" gorm:"not null"`
	Label     string    `json:"label" gorm:"not null;default:''"`
	Category  string    `json:"category" gorm:"not null;default:''"`
	Amount    float64   `json:"amount" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

var ValidBudgetKinds = []string{"fixed", "investment", "spending"}

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
