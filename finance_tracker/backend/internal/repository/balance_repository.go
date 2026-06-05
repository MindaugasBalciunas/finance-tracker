package repository

import (
	"errors"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

//go:generate mockery --name=BalanceRepository --output=./mock --outpkg=mock
type BalanceRepository interface {
	Create(b *domain.Balance) error
	GetByID(id uint) (*domain.Balance, error)
	Update(b *domain.Balance) error
	Delete(id uint) error
	DeleteAll() error
	List(filter domain.BalanceFilter) ([]domain.Balance, error)
	GetLatest() (*domain.Balance, error)
	GetLatestManual() (*domain.Balance, error)
	UpsertAuto(b *domain.Balance) error
	GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error)
}

type balanceRepository struct {
	db *gorm.DB
}

func NewBalanceRepository(db *gorm.DB) BalanceRepository {
	return &balanceRepository{db: db}
}

func (r *balanceRepository) Create(b *domain.Balance) error {
	var existing domain.Balance
	err := r.db.Where("date = ? AND is_auto = ?", b.Date, false).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(b).Error
	}
	if err != nil {
		return err
	}
	b.ID = existing.ID
	return r.db.Save(b).Error
}

func (r *balanceRepository) GetByID(id uint) (*domain.Balance, error) {
	var b domain.Balance
	if err := r.db.First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *balanceRepository) Update(b *domain.Balance) error {
	return r.db.Save(b).Error
}

func (r *balanceRepository) Delete(id uint) error {
	return r.db.Delete(&domain.Balance{}, id).Error
}

func (r *balanceRepository) DeleteAll() error {
	return r.db.Where("1 = 1").Delete(&domain.Balance{}).Error
}

func (r *balanceRepository) List(filter domain.BalanceFilter) ([]domain.Balance, error) {
	query := r.db.Model(&domain.Balance{})
	query = applyBalanceFilters(query, filter)

	var balances []domain.Balance
	if err := query.Order("date DESC").Find(&balances).Error; err != nil {
		return nil, err
	}
	return balances, nil
}

func (r *balanceRepository) GetLatest() (*domain.Balance, error) {
	var b domain.Balance
	if err := r.db.Order("date DESC").First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *balanceRepository) GetLatestManual() (*domain.Balance, error) {
	var b domain.Balance
	if err := r.db.Where("is_auto = ?", false).Order("date DESC").First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *balanceRepository) UpsertAuto(b *domain.Balance) error {
	var existing domain.Balance
	err := r.db.Where("date = ? AND is_auto = ?", b.Date, true).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		b.IsAuto = true
		return r.db.Create(b).Error
	}
	if err != nil {
		return err
	}
	b.ID = existing.ID
	b.IsAuto = true
	return r.db.Save(b).Error
}

func (r *balanceRepository) GetTrend(filter domain.BalanceFilter) (*domain.BalanceTrend, error) {
	balances, err := r.List(filter)
	if err != nil {
		return nil, err
	}

	// Reverse to chronological order
	for i, j := 0, len(balances)-1; i < j; i, j = i+1, j-1 {
		balances[i], balances[j] = balances[j], balances[i]
	}

	trend := &domain.BalanceTrend{
		Dates:  []string{},
		Totals: []float64{},
		Accounts: map[string][]float64{
			"seb":        {},
			"swed":       {},
			"swed_etf":   {},
			"seb_pen":    {},
			"luminor":    {},
			"art":        {},
			"cash":       {},
			"rev_m":      {},
			"rev_r":      {},
			"r_btc":      {},
			"m_btc":      {},
			"rev_stocks": {},
			"ibkr_stocks": {},
		},
	}

	for _, b := range balances {
		trend.Dates = append(trend.Dates, b.Date.Format("2006-01-02"))
		trend.Totals = append(trend.Totals, b.Total)
		trend.Accounts["seb"] = append(trend.Accounts["seb"], b.Seb)
		trend.Accounts["swed"] = append(trend.Accounts["swed"], b.Swed)
		trend.Accounts["swed_etf"] = append(trend.Accounts["swed_etf"], b.SwedETF)
		trend.Accounts["seb_pen"] = append(trend.Accounts["seb_pen"], b.SebPen)
		trend.Accounts["luminor"] = append(trend.Accounts["luminor"], b.Luminor)
		trend.Accounts["art"] = append(trend.Accounts["art"], b.Art)
		trend.Accounts["cash"] = append(trend.Accounts["cash"], b.Cash)
		trend.Accounts["rev_m"] = append(trend.Accounts["rev_m"], b.RevM)
		trend.Accounts["rev_r"] = append(trend.Accounts["rev_r"], b.RevR)
		trend.Accounts["r_btc"] = append(trend.Accounts["r_btc"], b.RBTC)
		trend.Accounts["m_btc"] = append(trend.Accounts["m_btc"], b.MBTC)
		trend.Accounts["rev_stocks"] = append(trend.Accounts["rev_stocks"], b.RevStocks)
		trend.Accounts["ibkr_stocks"] = append(trend.Accounts["ibkr_stocks"], b.IBKRStocks)
	}

	return trend, nil
}

func applyBalanceFilters(query *gorm.DB, filter domain.BalanceFilter) *gorm.DB {
	if filter.DateFrom != nil {
		query = query.Where("date >= ?", filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("date <= ?", filter.DateTo)
	}
	return query
}
