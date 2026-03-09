package domain

import (
	"time"
)

// Category represents a transaction category
type Category string

const (
	CategoryFood             Category = "Food"
	CategoryKids             Category = "Kids"
	CategoryKidsFood         Category = "Kids(food)"
	CategoryHealth           Category = "Health"
	CategoryFinance          Category = "Finance"
	CategoryInvestment       Category = "Investment"
	CategoryEntertainment    Category = "Entertainment"
	CategoryHouseExpense     Category = "House expense"
	CategoryCredit           Category = "Credit"
	CategoryCar              Category = "Car"
	CategoryIncome           Category = "Income"
	CategoryClothes          Category = "Clothes"
	CategoryKidsSchool       Category = "Kids school"
	CategoryKidsEntertainment Category = "Kids (Entertainment)"
	CategoryDivorce          Category = "Divorce"
)

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeExpense    TransactionType = "expense"
	TransactionTypeIncome     TransactionType = "income"
	TransactionTypeInvestment TransactionType = "investment"
)

// Transaction represents a financial transaction record
type Transaction struct {
	ID         uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	Date       time.Time       `json:"date" gorm:"not null;index"`
	Type       TransactionType `json:"type" gorm:"not null"`
	Amount     float64         `json:"amount" gorm:"not null"`
	Comment    string          `json:"comment"`
	Category   Category        `json:"category" gorm:"index"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
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
	TotalExpenses    float64            `json:"total_expenses"`
	TotalIncome      float64            `json:"total_income"`
	TotalInvestments float64            `json:"total_investments"`
	NetBalance       float64            `json:"net_balance"`
	ByCategory       []CategorySummary  `json:"by_category"`
	ByMonth          []MonthlySummary   `json:"by_month"`
}

// CategorySummary holds totals per category
type CategorySummary struct {
	Category Category `json:"category"`
	Type     TransactionType `json:"type"`
	Total    float64  `json:"total"`
	Count    int      `json:"count"`
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
