package repository

import (
	"errors"
	"time"

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
	// Chat history, oldest first, capped to the most recent `limit` turns.
	ListChat(limit int) ([]domain.AIChatMessage, error)
	AppendChat(msgs ...*domain.AIChatMessage) error
	ClearChat() error
	// LogActivity records one AI-interaction digest and prunes entries older
	// than 30 days. RecentActivity returns digests newest-first.
	LogActivity(a *domain.AIActivity) error
	RecentActivity(since time.Time, limit int) ([]domain.AIActivity, error)
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

func (r *insightRepository) ListChat(limit int) ([]domain.AIChatMessage, error) {
	// Fetch the newest `limit` rows, then reverse to chronological order.
	var msgs []domain.AIChatMessage
	if err := r.db.Order("id DESC").Limit(limit).Find(&msgs).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (r *insightRepository) AppendChat(msgs ...*domain.AIChatMessage) error {
	for _, m := range msgs {
		if err := r.db.Create(m).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *insightRepository) ClearChat() error {
	return r.db.Exec("DELETE FROM ai_chat_messages").Error
}

func (r *insightRepository) LogActivity(a *domain.AIActivity) error {
	if err := r.db.Create(a).Error; err != nil {
		return err
	}
	// Rolling window — the digests are short-term memory, not an archive.
	return r.db.Exec("DELETE FROM ai_activities WHERE created_at < ?", time.Now().AddDate(0, 0, -30)).Error
}

func (r *insightRepository) RecentActivity(since time.Time, limit int) ([]domain.AIActivity, error) {
	var out []domain.AIActivity
	err := r.db.Where("created_at >= ?", since).Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *insightRepository) SaveAISettings(s *domain.AISettings) error {
	s.ID = 1
	// Silence SQL logging for this one write: the row holds the plaintext
	// gateway API key, and GORM's Warn/Info logger interpolates bound values
	// into any slow-or-errored statement it prints — the key must never
	// reach the log sink.
	return r.db.Session(&gorm.Session{Logger: r.db.Logger.LogMode(logger.Silent)}).Save(s).Error
}
