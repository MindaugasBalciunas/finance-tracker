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

	// Rename swed_pen → seb_pen if old column still exists
	var colExists int
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('balances') WHERE name = 'swed_pen'").Scan(&colExists)
	if colExists > 0 {
		if err := db.Exec("ALTER TABLE balances RENAME COLUMN swed_pen TO seb_pen").Error; err != nil {
			return nil, err
		}
	}

	if err := db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.StockTrade{}, &domain.ExportLog{}); err != nil {
		return nil, err
	}

	return db, nil
}
