package repository

import (
	"errors"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type InsightRepository interface {
	Create(insight *domain.AIInsight) error
	GetLatest() (*domain.AIInsight, error)
	List(limit int) ([]domain.AIInsight, error)
	GetAISettings() (*domain.AISettings, error)
	SaveAISettings(s *domain.AISettings) error
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

// GetAISettings returns the singleton gateway configuration, creating the
// row (with the nexos.ai default URL) on first read.
func (r *insightRepository) GetAISettings() (*domain.AISettings, error) {
	var s domain.AISettings
	if err := r.db.FirstOrCreate(&s, domain.AISettings{ID: 1}).Error; err != nil {
		return nil, err
	}
	if s.GatewayURL == "" {
		s.GatewayURL = domain.DefaultGatewayURL
	}
	return &s, nil
}

func (r *insightRepository) SaveAISettings(s *domain.AISettings) error {
	s.ID = 1
	// Silence SQL logging for this one write: the row holds the plaintext
	// gateway API key, and GORM's Warn/Info logger interpolates bound values
	// into any slow-or-errored statement it prints — the key must never
	// reach the log sink.
	return r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Silent)}).Save(s).Error
}
