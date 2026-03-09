package database

import (
	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewSQLiteDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}); err != nil {
		return nil, err
	}

	return db, nil
}
