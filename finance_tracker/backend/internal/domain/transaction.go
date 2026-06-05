package domain

import (
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
	ID            uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	Date          time.Time       `json:"date" gorm:"not null;index"`
	Type          TransactionType `json:"type" gorm:"not null"`
	Amount        float64         `json:"-" gorm:"not null"`     // DB column; use AmountMoney in responses
	Comment       string          `json:"comment"`
	Category      Category        `json:"category" gorm:"index"`
	SourceAccount string          `json:"source_account" gorm:"default:''"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`

	// Computed for API response (not persisted)
	AmountMoney Money `json:"amount" gorm:"-"`
}

// TransactionFilter holds filtering options for querying transactions
type TransactionFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
	Type     *TransactionType
	Category *Category
	Page     int
	PageSize int
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
