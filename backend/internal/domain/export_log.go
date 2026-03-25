package domain

import "time"

// ExportLog tracks when data exports were performed
type ExportLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	Type       string    `gorm:"not null;index"` // "full" or "partial"
	ExportedAt time.Time `gorm:"not null"`
}
