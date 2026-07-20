package repository

import (
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

//go:generate mockery --name=AssetRepository --output=./mock --outpkg=mock
type AssetRepository interface {
	Create(a *domain.Asset) error
	GetByID(id uint) (*domain.Asset, error)
	Update(a *domain.Asset) error
	Delete(id uint) error
	DeleteAll() error
	ListAll() ([]domain.Asset, error)
	ListSince(since time.Time) ([]domain.Asset, error)
}

type assetRepository struct {
	db *gorm.DB
}

func NewAssetRepository(db *gorm.DB) AssetRepository {
	return &assetRepository{db: db}
}

func (r *assetRepository) Create(a *domain.Asset) error {
	return r.db.Create(a).Error
}

func (r *assetRepository) GetByID(id uint) (*domain.Asset, error) {
	var a domain.Asset
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *assetRepository) Update(a *domain.Asset) error {
	return r.db.Save(a).Error
}

func (r *assetRepository) Delete(id uint) error {
	return r.db.Delete(&domain.Asset{}, id).Error
}

func (r *assetRepository) DeleteAll() error {
	return r.db.Where("1 = 1").Delete(&domain.Asset{}).Error
}

func (r *assetRepository) ListAll() ([]domain.Asset, error) {
	var assets []domain.Asset
	if err := r.db.Order("purchase_date ASC, id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *assetRepository) ListSince(since time.Time) ([]domain.Asset, error) {
	var assets []domain.Asset
	if err := r.db.Where("created_at >= ?", since).Order("purchase_date ASC, id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}
