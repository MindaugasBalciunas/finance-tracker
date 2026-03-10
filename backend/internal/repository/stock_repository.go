package repository

import (
	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

//go:generate mockery --name=StockRepository --output=./mock --outpkg=mock
type StockRepository interface {
	Create(t *domain.StockTrade) error
	GetByID(id uint) (*domain.StockTrade, error)
	Update(t *domain.StockTrade) error
	Delete(id uint) error
	ListAll() ([]domain.StockTrade, error)
}

type stockRepository struct {
	db *gorm.DB
}

func NewStockRepository(db *gorm.DB) StockRepository {
	return &stockRepository{db: db}
}

func (r *stockRepository) Create(t *domain.StockTrade) error {
	return r.db.Create(t).Error
}

func (r *stockRepository) GetByID(id uint) (*domain.StockTrade, error) {
	var t domain.StockTrade
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *stockRepository) Update(t *domain.StockTrade) error {
	return r.db.Save(t).Error
}

func (r *stockRepository) Delete(id uint) error {
	return r.db.Delete(&domain.StockTrade{}, id).Error
}

func (r *stockRepository) ListAll() ([]domain.StockTrade, error) {
	var trades []domain.StockTrade
	if err := r.db.Order("date ASC, id ASC").Find(&trades).Error; err != nil {
		return nil, err
	}
	return trades, nil
}
