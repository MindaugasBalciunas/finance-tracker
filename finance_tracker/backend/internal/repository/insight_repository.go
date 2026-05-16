package repository

import (
	"errors"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

type InsightRepository interface {
	Create(insight *domain.AIInsight) error
	GetLatest() (*domain.AIInsight, error)
	List(limit int) ([]domain.AIInsight, error)
}

type insightRepository struct {
	db *gorm.DB
}

func NewInsightRepository(db *gorm.DB) InsightRepository {
	return &insightRepository{db: db}
}

func (r *insightRepository) Create(insight *domain.AIInsight) error {
	return r.db.Create(insight).Error
}

func (r *insightRepository) GetLatest() (*domain.AIInsight, error) {
	var insight domain.AIInsight
	err := r.db.Order("created_at DESC").First(&insight).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no insights found")
		}
		return nil, err
	}
	return &insight, nil
}

func (r *insightRepository) List(limit int) ([]domain.AIInsight, error) {
	var insights []domain.AIInsight
	err := r.db.Order("created_at DESC").Limit(limit).Find(&insights).Error
	return insights, err
}
