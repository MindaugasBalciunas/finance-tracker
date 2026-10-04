package repository

import (
	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

type AccountRepository interface {
	List() ([]domain.Account, error)
	GetByID(id uint) (*domain.Account, error)
	GetByKey(key string) (*domain.Account, error)
	Create(a *domain.Account) error
	Save(a *domain.Account) error
}

type accountRepository struct{ db *gorm.DB }

func NewAccountRepository(db *gorm.DB) AccountRepository { return &accountRepository{db: db} }

func (r *accountRepository) List() ([]domain.Account, error) {
	var out []domain.Account
	err := r.db.Order("sort_order, id").Find(&out).Error
	return out, err
}

func (r *accountRepository) GetByID(id uint) (*domain.Account, error) {
	var a domain.Account
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *accountRepository) GetByKey(key string) (*domain.Account, error) {
	var a domain.Account
	if err := r.db.Where("key = ?", key).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *accountRepository) Create(a *domain.Account) error { return r.db.Create(a).Error }
func (r *accountRepository) Save(a *domain.Account) error   { return r.db.Save(a).Error }

// accountGroups maps added-account keys to their group. A missing table
// (fake databases in tests) reads as "no added accounts".
func accountGroups(db *gorm.DB) map[string]string {
	var accs []domain.Account
	if !db.Migrator().HasTable(&domain.Account{}) {
		return nil
	}
	if err := db.Where("builtin = ?", false).Find(&accs).Error; err != nil {
		return nil
	}
	out := make(map[string]string, len(accs))
	for _, a := range accs {
		out[a.Key] = string(a.Group)
	}
	return out
}

// annotateExtraGroups fills ExtraGroups from Extra. An added account whose
// row is gone counts as "other" — its money is still money.
func annotateExtraGroups(groups map[string]string, bs ...*domain.Balance) {
	for _, b := range bs {
		if len(b.Extra) == 0 {
			continue
		}
		b.ExtraGroups = map[string]float64{}
		for k, v := range b.Extra {
			g := groups[k]
			if g == "" {
				g = string(domain.AccountGroupOther)
			}
			b.ExtraGroups[g] += v
		}
	}
}
