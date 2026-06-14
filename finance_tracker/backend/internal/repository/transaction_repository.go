package repository

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

//go:generate mockery --name=TransactionRepository --output=./mock --outpkg=mock
type TransactionRepository interface {
	Create(tx *domain.Transaction) error
	GetByID(id uint) (*domain.Transaction, error)
	Update(tx *domain.Transaction) error
	Delete(id uint) error
	DeleteBatch(ids []uint) error
	DeleteAll() error
	List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error)
	ListAll() ([]domain.Transaction, error)
	ListSince(since time.Time) ([]domain.Transaction, error)
	ListStrictlyAfter(since time.Time) ([]domain.Transaction, error)
	GetDistinctComments() ([]string, error)
	GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error)
}

type transactionRepository struct {
	db *gorm.DB
}

func NewTransactionRepository(db *gorm.DB) TransactionRepository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) Create(tx *domain.Transaction) error {
	return r.db.Create(tx).Error
}

func (r *transactionRepository) GetByID(id uint) (*domain.Transaction, error) {
	var tx domain.Transaction
	if err := r.db.First(&tx, id).Error; err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *transactionRepository) Update(tx *domain.Transaction) error {
	return r.db.Save(tx).Error
}

func (r *transactionRepository) Delete(id uint) error {
	return r.db.Delete(&domain.Transaction{}, id).Error
}

func (r *transactionRepository) DeleteBatch(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Delete(&domain.Transaction{}, ids).Error
}

func (r *transactionRepository) DeleteAll() error {
	return r.db.Where("1 = 1").Delete(&domain.Transaction{}).Error
}

func (r *transactionRepository) List(filter domain.TransactionFilter) (*domain.PaginatedTransactions, error) {
	query := r.db.Model(&domain.Transaction{})
	query = applyTransactionFilters(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var transactions []domain.Transaction
	if err := query.Order("date DESC").Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
		return nil, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &domain.PaginatedTransactions{
		Data:       transactions,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (r *transactionRepository) ListAll() ([]domain.Transaction, error) {
	var transactions []domain.Transaction
	if err := r.db.Order("date DESC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

func (r *transactionRepository) ListSince(since time.Time) ([]domain.Transaction, error) {
	var transactions []domain.Transaction
	if err := r.db.Where("date >= ?", since).Order("date ASC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

func (r *transactionRepository) ListStrictlyAfter(since time.Time) ([]domain.Transaction, error) {
	var transactions []domain.Transaction
	if err := r.db.Where("date > ?", since).Order("date ASC, id ASC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	return transactions, nil
}

func (r *transactionRepository) GetDistinctComments() ([]string, error) {
	var comments []string
	if err := r.db.Model(&domain.Transaction{}).
		Where("comment != ''").
		Distinct("comment").
		Order("comment ASC").
		Pluck("comment", &comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *transactionRepository) GetSummary(filter domain.TransactionFilter) (*domain.TransactionSummary, error) {
	query := r.db.Model(&domain.Transaction{})
	query = applyTransactionFilters(query, filter)

	type aggregateResult struct {
		Type   domain.TransactionType
		Total  float64
	}
	var results []aggregateResult
	if err := query.Select("type, SUM(amount) as total").Group("type").Scan(&results).Error; err != nil {
		return nil, err
	}

	summary := &domain.TransactionSummary{
		ByCategory: []domain.CategorySummary{},
		ByMonth:    []domain.MonthlySummary{},
	}
	for _, r := range results {
		switch r.Type {
		case domain.TransactionTypeExpense:
			summary.TotalExpenses = r.Total
		case domain.TransactionTypeIncome:
			summary.TotalIncome = r.Total
		case domain.TransactionTypeInvestment:
			summary.TotalInvestments = r.Total
		}
	}
	summary.NetBalance = summary.TotalIncome - summary.TotalExpenses - summary.TotalInvestments

	// By category
	type catResult struct {
		Category domain.Category
		Type     domain.TransactionType
		Total    float64
		Count    int
	}
	var catResults []catResult
	catQuery := r.db.Model(&domain.Transaction{})
	catQuery = applyTransactionFilters(catQuery, filter)
	if err := catQuery.Select("category, type, SUM(amount) as total, COUNT(*) as count").Group("category, type").Scan(&catResults).Error; err != nil {
		return nil, err
	}
	for _, c := range catResults {
		summary.ByCategory = append(summary.ByCategory, domain.CategorySummary{
			Category: c.Category,
			Type:     c.Type,
			Total:    c.Total,
			Count:    c.Count,
		})
	}

	// By month — SQLite strftime returns strings, so scan into string fields then convert
	type monthResult struct {
		Year  string
		Month string
		Type  domain.TransactionType
		Total float64
	}
	var monthResults []monthResult
	mQuery := r.db.Model(&domain.Transaction{})
	mQuery = applyTransactionFilters(mQuery, filter)
	if err := mQuery.Select(
		"strftime('%Y', date) as year, strftime('%m', date) as month, type, SUM(amount) as total",
	).Group("year, month, type").Order("year, month").Scan(&monthResults).Error; err != nil {
		return nil, err
	}

	monthMap := map[string]*domain.MonthlySummary{}
	monthNames := []string{"", "Sau", "Vas", "Kov", "Bal", "Geg", "Bir", "Lie", "Rgp", "Rgs", "Spa", "Lap", "Grd"}
	for _, m := range monthResults {
		yr, _ := strconv.Atoi(m.Year)
		mo, _ := strconv.Atoi(m.Month)
		key := fmt.Sprintf("%s-%s", m.Year, m.Month)
		if _, ok := monthMap[key]; !ok {
			monthName := ""
			if mo >= 1 && mo <= 12 {
				monthName = monthNames[mo]
			}
			monthMap[key] = &domain.MonthlySummary{
				Year:      yr,
				Month:     mo,
				MonthName: monthName,
			}
		}
		switch m.Type {
		case domain.TransactionTypeExpense:
			monthMap[key].Expenses = m.Total
		case domain.TransactionTypeIncome:
			monthMap[key].Income = m.Total
		case domain.TransactionTypeInvestment:
			monthMap[key].Investments = m.Total
		}
	}
	for _, ms := range monthMap {
		summary.ByMonth = append(summary.ByMonth, *ms)
	}

	return summary, nil
}

func applyTransactionFilters(query *gorm.DB, filter domain.TransactionFilter) *gorm.DB {
	if filter.DateFrom != nil {
		query = query.Where("date >= ?", filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("date <= ?", filter.DateTo)
	}
	if filter.Type != nil {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Category != nil {
		query = query.Where("category = ?", filter.Category)
	}
	return query
}
