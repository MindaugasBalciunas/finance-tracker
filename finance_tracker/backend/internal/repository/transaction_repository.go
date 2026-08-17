package repository

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
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

// orderClause maps the whitelisted sort columns to SQL; anything else falls
// back to newest-first. Ties break on date so pagination stays stable.
func orderClause(filter domain.TransactionFilter) string {
	col, def := "date", "desc"
	switch filter.Sort {
	case "amount":
		col = "amount"
	case "comment":
		col, def = "comment COLLATE NOCASE", "asc"
	}
	dir := filter.Dir
	if dir != "asc" && dir != "desc" {
		dir = def
	}
	if col == "date" {
		return "date " + dir + ", id " + dir
	}
	return col + " " + dir + ", date DESC"
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
	if err := query.Order(orderClause(filter)).Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
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
	// Transfers move money between own accounts — neither spending nor
	// investing, so they contribute nothing to the totals. The rows stay in
	// the ByCategory breakdown below for drill-downs.
	const amountExpr = "SUM(CASE WHEN category = 'Transfers' THEN 0 ELSE amount END) as total"
	var results []aggregateResult
	if err := query.Select("type, " + amountExpr).Group("type").Scan(&results).Error; err != nil {
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
		"strftime('%Y', date) as year, strftime('%m', date) as month, type, " + amountExpr,
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
	// The SQL ORDER BY is lost when rebuilding through the map (Go map
	// iteration is randomized) — restore chronological order.
	sort.Slice(summary.ByMonth, func(i, j int) bool {
		if summary.ByMonth[i].Year != summary.ByMonth[j].Year {
			return summary.ByMonth[i].Year < summary.ByMonth[j].Year
		}
		return summary.ByMonth[i].Month < summary.ByMonth[j].Month
	})

	// By label — labels are a comma multiset, so aggregate in Go: a row with
	// several labels counts toward each. Transfers rows are internal moves
	// and excluded, matching the totals above.
	type labelRow struct {
		Labels   string
		Amount   float64
		Category domain.Category
	}
	var labelRows []labelRow
	lQuery := applyTransactionFilters(r.db.Model(&domain.Transaction{}), filter)
	if err := lQuery.Select("labels, amount, category").Where("labels != ''").Scan(&labelRows).Error; err != nil {
		return nil, err
	}
	agg := map[string]*domain.LabelSummary{}
	for _, row := range labelRows {
		if row.Category == "Transfers" {
			continue
		}
		for _, l := range strings.Split(row.Labels, ",") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if agg[l] == nil {
				agg[l] = &domain.LabelSummary{Label: l}
			}
			agg[l].Total += row.Amount
			agg[l].Count++
		}
	}
	summary.ByLabel = []domain.LabelSummary{}
	for _, ls := range agg {
		summary.ByLabel = append(summary.ByLabel, *ls)
	}
	sort.Slice(summary.ByLabel, func(i, j int) bool { return summary.ByLabel[i].Total > summary.ByLabel[j].Total })
	if len(summary.ByLabel) > 40 {
		summary.ByLabel = summary.ByLabel[:40]
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
	if filter.Label != "" {
		// Labels are stored as "a,b,c" — wrap both sides with commas for
		// exact-token match. The filter itself may be a comma list: the
		// default matches any of them (multi-label budgets drill down with
		// "restaurant,fast food"), LabelMode "all" requires every one
		// (intersecting cross-cutting labels, e.g. kids ∩ entertainment).
		var conds []string
		var args []interface{}
		for _, l := range strings.Split(filter.Label, ",") {
			if l = strings.TrimSpace(l); l != "" {
				conds = append(conds, "(',' || labels || ',') LIKE ?")
				args = append(args, "%,"+l+",%")
			}
		}
		if len(conds) > 0 {
			joiner := " OR "
			if filter.LabelMode == "all" {
				joiner = " AND "
			}
			query = query.Where("("+strings.Join(conds, joiner)+")", args...)
		}
	}
	if filter.Search != "" {
		query = query.Where("LOWER(comment) LIKE ?", "%"+strings.ToLower(filter.Search)+"%")
	}
	if filter.AmountMin != nil {
		query = query.Where("amount >= ?", *filter.AmountMin)
	}
	if filter.AmountMax != nil {
		query = query.Where("amount <= ?", *filter.AmountMax)
	}
	return query
}
