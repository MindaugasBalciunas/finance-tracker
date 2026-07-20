package database

import (
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
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

	// Migrate swed_pen → seb_pen
	var swedPenExists, sebPenExists int
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('balances') WHERE name = 'swed_pen'").Scan(&swedPenExists)
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('balances') WHERE name = 'seb_pen'").Scan(&sebPenExists)
	if swedPenExists > 0 && sebPenExists == 0 {
		if err := db.Exec("ALTER TABLE balances RENAME COLUMN swed_pen TO seb_pen").Error; err != nil {
			return nil, err
		}
	} else if swedPenExists > 0 && sebPenExists > 0 {
		// AutoMigrate already added seb_pen; copy any non-zero data then drop old column
		if err := db.Exec("UPDATE balances SET seb_pen = swed_pen WHERE seb_pen = 0 AND swed_pen != 0").Error; err != nil {
			return nil, err
		}
		if err := db.Exec("ALTER TABLE balances DROP COLUMN swed_pen").Error; err != nil {
			return nil, err
		}
	}

	// Drop old unique indexes on balances.date (allow multiple snapshots per day).
	db.Exec("DROP INDEX IF EXISTS idx_balances_date")
	db.Exec("DROP INDEX IF EXISTS uni_balances_date")
	db.Exec("DROP INDEX IF EXISTS idx_balances_date_auto")

	// Drop is_auto column if it exists (auto-snapshots feature removed).
	var isAutoExists int
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('balances') WHERE name = 'is_auto'").Scan(&isAutoExists)
	if isAutoExists > 0 {
		db.Exec("DELETE FROM balances WHERE is_auto = 1")
		db.Exec("ALTER TABLE balances DROP COLUMN is_auto")
	}

	if err := db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.StockTrade{}, &domain.ExportLog{}, &domain.Asset{}); err != nil {
		return nil, err
	}

	// Back-fill source = 'Revolut' for any stock trades that pre-date the source column
	if err := db.Exec("UPDATE stock_trades SET source = 'Revolut' WHERE source = '' OR source IS NULL").Error; err != nil {
		return nil, err
	}

	return db, nil
}
