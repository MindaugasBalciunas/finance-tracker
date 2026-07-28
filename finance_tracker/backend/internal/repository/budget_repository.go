package repository

import (
	"strings"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

type BudgetRepository interface {
	ListBudgets() ([]domain.Budget, error)
	GetBudget(id uint) (*domain.Budget, error)
	SaveBudget(b *domain.Budget) error
	DeleteBudget(id uint) error

	ListRules() ([]domain.LabelRule, error)
	SaveRule(r *domain.LabelRule) error
	DeleteRule(id uint) error

	// ApplyLabel tags every transaction matching the rule with the label.
	// Returns the number of transactions updated.
	ApplyLabel(rule domain.LabelRule) (int, error)
	// PreviewLabel counts what ApplyLabel would touch without writing:
	// total matching transactions and how many of them lack the label.
	PreviewLabel(rule domain.LabelRule) (matches, unlabeled int, err error)
	// DistinctLabels lists labels in use; category != "" scopes to that category.
	DistinctLabels(category string) ([]string, error)

	GetSettings() (*domain.BudgetSettings, error)
	SaveSettings(s *domain.BudgetSettings) error

	// SuggestCategory proposes a category for a new transaction from similar
	// historical rows (comment substring first, exact amount as fallback).
	SuggestCategory(txType, comment string, amount float64) (category string, matches int, basis string, err error)
}

type budgetRepository struct {
	db *gorm.DB
}

func NewBudgetRepository(db *gorm.DB) BudgetRepository {
	return &budgetRepository{db: db}
}

func (r *budgetRepository) ListBudgets() ([]domain.Budget, error) {
	var budgets []domain.Budget
	if err := r.db.Order("kind ASC, amount DESC").Find(&budgets).Error; err != nil {
		return nil, err
	}
	return budgets, nil
}

func (r *budgetRepository) GetBudget(id uint) (*domain.Budget, error) {
	var b domain.Budget
	if err := r.db.First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *budgetRepository) SaveBudget(b *domain.Budget) error {
	return r.db.Save(b).Error
}

func (r *budgetRepository) DeleteBudget(id uint) error {
	return r.db.Delete(&domain.Budget{}, id).Error
}

func (r *budgetRepository) ListRules() ([]domain.LabelRule, error) {
	var rules []domain.LabelRule
	if err := r.db.Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *budgetRepository) SaveRule(rule *domain.LabelRule) error {
	return r.db.Save(rule).Error
}

func (r *budgetRepository) DeleteRule(id uint) error {
	return r.db.Delete(&domain.LabelRule{}, id).Error
}

// commentMatchLike converts a rule comment pattern to its LIKE form:
// '^'-anchored patterns match the start of the comment, others anywhere.
func commentMatchLike(pattern string) string {
	p := strings.ToLower(pattern)
	if anchored, ok := strings.CutPrefix(p, "^"); ok {
		return anchored + "%"
	}
	return "%" + p + "%"
}

func (r *budgetRepository) ApplyLabel(rule domain.LabelRule) (int, error) {
	var txs []domain.Transaction
	query := r.db.Model(&domain.Transaction{})
	if rule.Category != "" {
		query = query.Where("category = ?", rule.Category)
	}
	if rule.CommentMatch != "" {
		query = query.Where("LOWER(comment) LIKE ?", commentMatchLike(rule.CommentMatch))
	}
	if err := query.Find(&txs).Error; err != nil {
		return 0, err
	}
	count := 0
	for i := range txs {
		if txs[i].HasLabel(rule.Label) {
			continue
		}
		txs[i].AddLabel(rule.Label)
		if err := r.db.Model(&domain.Transaction{}).Where("id = ?", txs[i].ID).
			Update("labels", txs[i].Labels).Error; err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (r *budgetRepository) PreviewLabel(rule domain.LabelRule) (int, int, error) {
	base := func() *gorm.DB {
		q := r.db.Model(&domain.Transaction{})
		if rule.Category != "" {
			q = q.Where("category = ?", rule.Category)
		}
		if rule.CommentMatch != "" {
			q = q.Where("LOWER(comment) LIKE ?", commentMatchLike(rule.CommentMatch))
		}
		return q
	}
	var matches, labeled int64
	if err := base().Count(&matches).Error; err != nil {
		return 0, 0, err
	}
	if err := base().Where("(',' || labels || ',') LIKE ?", "%,"+strings.ToLower(rule.Label)+",%").
		Count(&labeled).Error; err != nil {
		return 0, 0, err
	}
	return int(matches), int(matches - labeled), nil
}

func (r *budgetRepository) GetSettings() (*domain.BudgetSettings, error) {
	var s domain.BudgetSettings
	if err := r.db.FirstOrCreate(&s, domain.BudgetSettings{ID: 1}).Error; err != nil {
		return nil, err
	}
	if s.IncomeMode == "" {
		s.IncomeMode = "median"
	}
	return &s, nil
}

func (r *budgetRepository) SaveSettings(s *domain.BudgetSettings) error {
	s.ID = 1
	return r.db.Save(s).Error
}

func (r *budgetRepository) SuggestCategory(txType, comment string, amount float64) (string, int, string, error) {
	type row struct {
		Category string
		N        int
	}
	comment = strings.TrimSpace(comment)

	// Primary signal: same words in the comment (merchant names recur).
	if len(comment) >= 3 {
		var res row
		q := r.db.Model(&domain.Transaction{}).
			Select("category, COUNT(*) as n").
			Where("comment != '' AND LOWER(comment) LIKE ?", "%"+strings.ToLower(comment)+"%")
		if txType != "" {
			q = q.Where("type = ?", txType)
		}
		if err := q.Group("category").Order("n DESC").Limit(1).Scan(&res).Error; err != nil {
			return "", 0, "", err
		}
		if res.N >= 2 {
			return res.Category, res.N, "comment", nil
		}
	}

	// Fallback: recurring identical amounts (subscriptions, standing orders).
	if amount > 0 {
		var res row
		q := r.db.Model(&domain.Transaction{}).
			Select("category, COUNT(*) as n").
			Where("ABS(amount - ?) < 0.005", amount)
		if txType != "" {
			q = q.Where("type = ?", txType)
		}
		if err := q.Group("category").Order("n DESC").Limit(1).Scan(&res).Error; err != nil {
			return "", 0, "", err
		}
		if res.N >= 3 {
			return res.Category, res.N, "amount", nil
		}
	}
	return "", 0, "", nil
}

func (r *budgetRepository) DistinctLabels(category string) ([]string, error) {
	var rows []string
	q := r.db.Model(&domain.Transaction{}).Where("labels != ''")
	if category != "" {
		// Scoped to one category — powers category-aware label suggestions.
		q = q.Where("category = ?", category)
	}
	if err := q.Distinct().Pluck("labels", &rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		for _, l := range strings.Split(row, ",") {
			if l != "" && !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out, nil
}
