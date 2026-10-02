package repository

import (
	"sort"
	"strings"
	"time"

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
	// LabelStats aggregates every label's footprint: transaction count and
	// volume, plus how many rules and budgets reference it.
	LabelStats() ([]domain.LabelStat, error)
	// RenameLabel rewrites a label everywhere it appears — transactions,
	// label rules and budget label lists. Renaming onto an existing label
	// merges the two (lists are deduplicated, rules that become identical
	// collapse into one).
	RenameLabel(from, to string) (domain.RelabelResult, error)
	// DeleteLabel removes a label from all transactions and budget label
	// lists and deletes rules that assign it.
	DeleteLabel(label string) (domain.RelabelResult, error)

	GetSettings() (*domain.BudgetSettings, error)
	SaveSettings(s *domain.BudgetSettings) error

	// SuggestCategory proposes a category for a new transaction from similar
	// historical rows (comment substring first, exact amount as fallback).
	SuggestCategory(txType, comment string, amount float64) (category string, matches int, basis string, err error)

	// SuggestLabels proposes the labels that similar historical rows agree
	// on, so a recurring merchant keeps the labels it already had.
	SuggestLabels(txType, comment string) (labels []string, matches int, err error)
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
		// Two patterns, OR'd, because SQLite's LOWER() is ASCII-only: it
		// leaves "Ė" alone while Go's ToLower turns the pattern's into "ė",
		// so a lowered pattern can never match a Lithuanian merchant name.
		// LIKE is already case-insensitive for ASCII, so the raw pattern
		// covers those rows; the lowered one keeps matching history that was
		// stored lowercase. The union can only find more, never fewer.
		q := r.db.Model(&domain.Transaction{}).
			Select("category, COUNT(*) as n").
			Where("comment != '' AND (comment LIKE ? OR LOWER(comment) LIKE ?)",
				"%"+comment+"%", "%"+strings.ToLower(comment)+"%")
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

// SuggestLabels returns the labels that similar past transactions agree on.
//
// Label rules only cover what the user has bothered to write a rule for;
// everything else is in the history and nowhere else. A merchant labelled
// "groceries, barbora" seven times should not come back from the bank bare
// just because no rule spells it out.
//
// Agreement, not presence: a label has to be on at least half the matching
// rows (and at least two of them). One row tagged "bar" by accident is not a
// pattern, and copying it forward would spread the mistake.
func (r *budgetRepository) SuggestLabels(txType, comment string) ([]string, int, error) {
	comment = strings.TrimSpace(comment)
	if len(comment) < 3 {
		return nil, 0, nil
	}
	var rows []string
	// Same two-pattern match as SuggestCategory — see the note there about
	// SQLite's ASCII-only LOWER().
	q := r.db.Model(&domain.Transaction{}).
		Where("comment != '' AND (comment LIKE ? OR LOWER(comment) LIKE ?)",
			"%"+comment+"%", "%"+strings.ToLower(comment)+"%")
	if txType != "" {
		q = q.Where("type = ?", txType)
	}
	if err := q.Pluck("labels", &rows).Error; err != nil {
		return nil, 0, err
	}
	if len(rows) < 2 {
		return nil, len(rows), nil
	}

	counts := map[string]int{}
	// Order of first appearance, so the result is stable rather than map-random.
	var order []string
	for _, raw := range rows {
		seen := map[string]bool{}
		for _, l := range strings.Split(raw, ",") {
			l = strings.ToLower(strings.TrimSpace(l))
			if l == "" || seen[l] {
				continue
			}
			seen[l] = true
			if counts[l] == 0 {
				order = append(order, l)
			}
			counts[l]++
		}
	}
	threshold := (len(rows) + 1) / 2
	if threshold < 2 {
		threshold = 2
	}
	var out []string
	for _, l := range order {
		if counts[l] >= threshold {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return counts[out[i]] > counts[out[j]] })
	return out, len(rows), nil
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

func (r *budgetRepository) LabelStats() ([]domain.LabelStat, error) {
	type txRow struct {
		Labels string
		Amount float64
		Date   time.Time
	}
	var txs []txRow
	if err := r.db.Model(&domain.Transaction{}).Where("labels != ''").
		Select("labels", "amount", "date").Find(&txs).Error; err != nil {
		return nil, err
	}
	stats := map[string]*domain.LabelStat{}
	get := func(label string) *domain.LabelStat {
		if s, ok := stats[label]; ok {
			return s
		}
		s := &domain.LabelStat{Label: label, Fixed: domain.IsFixedObligationLabel(label)}
		stats[label] = s
		return s
	}
	for _, tx := range txs {
		day := tx.Date.Format("2006-01-02")
		for _, l := range strings.Split(tx.Labels, ",") {
			if l == "" {
				continue
			}
			s := get(l)
			s.Transactions++
			s.Amount += tx.Amount
			if s.FirstUsed == "" || day < s.FirstUsed {
				s.FirstUsed = day
			}
			if day > s.LastUsed {
				s.LastUsed = day
			}
		}
	}
	rules, err := r.ListRules()
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		get(strings.ToLower(rule.Label)).Rules++
	}
	budgets, err := r.ListBudgets()
	if err != nil {
		return nil, err
	}
	for _, b := range budgets {
		for _, l := range strings.Split(b.Label, ",") {
			if l != "" {
				get(l).Budgets++
			}
		}
	}
	out := make([]domain.LabelStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Transactions != out[j].Transactions {
			return out[i].Transactions > out[j].Transactions
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

// relabelEverywhere applies rewrite to every transaction label list carrying
// the token and every budget label list, and hands rules with the label to
// fixRule. All inside one DB transaction: a rename/delete is all-or-nothing.
func (r *budgetRepository) relabelEverywhere(label string,
	rewrite func(list string) (string, bool),
	fixRule func(tx *gorm.DB, rule domain.LabelRule) (bool, error)) (domain.RelabelResult, error) {
	var res domain.RelabelResult
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		// ",," contains the empty token, so an empty label would match every
		// UNLABELED transaction. Defense in depth behind the handler check —
		// this method is also called from the startup migration.
		return res, nil
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var txs []domain.Transaction
		if err := tx.Where("(',' || labels || ',') LIKE ?", "%,"+label+",%").Find(&txs).Error; err != nil {
			return err
		}
		for i := range txs {
			next, changed := rewrite(txs[i].Labels)
			if !changed {
				continue
			}
			if err := tx.Model(&domain.Transaction{}).Where("id = ?", txs[i].ID).
				Update("labels", next).Error; err != nil {
				return err
			}
			res.Transactions++
		}
		var rules []domain.LabelRule
		if err := tx.Where("LOWER(label) = ?", label).Find(&rules).Error; err != nil {
			return err
		}
		for _, rule := range rules {
			changed, err := fixRule(tx, rule)
			if err != nil {
				return err
			}
			if changed {
				res.Rules++
			}
		}
		var budgets []domain.Budget
		if err := tx.Where("label != ''").Find(&budgets).Error; err != nil {
			return err
		}
		for i := range budgets {
			next, changed := rewrite(budgets[i].Label)
			if !changed {
				continue
			}
			if next == "" && budgets[i].Category == "" {
				// The label was the budget's only matcher — without it the
				// budget can never match anything but would keep occupying
				// the monthly plan. Remove it outright.
				if err := tx.Delete(&domain.Budget{}, budgets[i].ID).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&domain.Budget{}).Where("id = ?", budgets[i].ID).
				Update("label", next).Error; err != nil {
				return err
			}
			res.Budgets++
		}
		return nil
	})
	return res, err
}

func (r *budgetRepository) RenameLabel(from, to string) (domain.RelabelResult, error) {
	from = strings.ToLower(strings.TrimSpace(from))
	to = domain.NormalizeLabels(to) // single token: lowercase + trim
	return r.relabelEverywhere(from,
		func(list string) (string, bool) { return domain.RenameLabelToken(list, from, to) },
		func(tx *gorm.DB, rule domain.LabelRule) (bool, error) {
			// A rule that becomes identical to an existing one collapses.
			var dup int64
			if err := tx.Model(&domain.LabelRule{}).
				Where("LOWER(label) = ? AND category = ? AND comment_match = ? AND id != ?",
					to, rule.Category, rule.CommentMatch, rule.ID).Count(&dup).Error; err != nil {
				return false, err
			}
			if dup > 0 {
				return true, tx.Delete(&domain.LabelRule{}, rule.ID).Error
			}
			return true, tx.Model(&domain.LabelRule{}).Where("id = ?", rule.ID).
				Update("label", to).Error
		})
}

func (r *budgetRepository) DeleteLabel(label string) (domain.RelabelResult, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	return r.relabelEverywhere(label,
		func(list string) (string, bool) { return domain.RemoveLabelToken(list, label) },
		func(tx *gorm.DB, rule domain.LabelRule) (bool, error) {
			return true, tx.Delete(&domain.LabelRule{}, rule.ID).Error
		})
}
