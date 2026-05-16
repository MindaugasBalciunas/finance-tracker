package repository

import (
	"errors"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

type ExportLogRepository interface {
	Save(exportType string) error
	GetLastTime(exportType string) (*time.Time, error)
}

type exportLogRepository struct {
	db *gorm.DB
}

func NewExportLogRepository(db *gorm.DB) ExportLogRepository {
	return &exportLogRepository{db: db}
}

func (r *exportLogRepository) Save(exportType string) error {
	return r.db.Create(&domain.ExportLog{
		Type:       exportType,
		ExportedAt: time.Now(),
	}).Error
}

func (r *exportLogRepository) GetLastTime(exportType string) (*time.Time, error) {
	var log domain.ExportLog
	err := r.db.Where("type = ?", exportType).Order("exported_at DESC").First(&log).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &log.ExportedAt, nil
}
