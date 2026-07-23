package database

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func seedTx(t *testing.T, db *gorm.DB, typ, category, comment string) uint {
	t.Helper()
	tx := domain.Transaction{Date: time.Now(), Type: domain.TransactionType(typ), Amount: 10, Category: domain.Category(category), Comment: comment}
	require.NoError(t, db.Create(&tx).Error)
	return tx.ID
}

func categoryOf(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var tx domain.Transaction
	require.NoError(t, db.First(&tx, id).Error)
	return string(tx.Category)
}

func TestApplyCategoryMigrations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}))

	artea := seedTx(t, db, "investment", "Stocks & ETF", "Artea pensija")
	arteaOK := seedTx(t, db, "investment", "Pension", "Artea 3rd pillar")
	vwce := seedTx(t, db, "investment", "Stocks & ETF", "ibkr top up")
	ignitis := seedTx(t, db, "expense", "Housing", "UAB IGNITIS")
	telia := seedTx(t, db, "expense", "Housing", "TELIA LIETUVA AB")
	heating := seedTx(t, db, "expense", "Housing", "AB VILNIAUS ŠILUMOS TINKLAI")
	rent := seedTx(t, db, "expense", "Housing", "Rent (cash) — apartment")
	yt := seedTx(t, db, "expense", "Entertainment", "GOOGLE *YouTubePremium SW1W 9TQ")
	patreon := seedTx(t, db, "expense", "Entertainment", "Patreon* Membership Dublin")
	bar := seedTx(t, db, "expense", "Entertainment", `BARAS "ARTISTAI"`)

	applyCategoryMigrations(db)

	assert.Equal(t, "Pension", categoryOf(t, db, artea), "Artea moves to Pension")
	assert.Equal(t, "Pension", categoryOf(t, db, arteaOK), "already-correct row untouched")
	assert.Equal(t, "Stocks & ETF", categoryOf(t, db, vwce), "non-Artea ETF stays")
	assert.Equal(t, "Utilities", categoryOf(t, db, ignitis))
	assert.Equal(t, "Utilities", categoryOf(t, db, telia))
	assert.Equal(t, "Utilities", categoryOf(t, db, heating))
	assert.Equal(t, "Housing", categoryOf(t, db, rent), "non-Evelina rent stays in Housing")
	assert.Equal(t, "Subscriptions", categoryOf(t, db, yt))
	assert.Equal(t, "Subscriptions", categoryOf(t, db, patreon))
	assert.Equal(t, "Entertainment", categoryOf(t, db, bar), "bars stay in Entertainment")

	// Idempotent: second run changes nothing further.
	applyCategoryMigrations(db)
	assert.Equal(t, "Pension", categoryOf(t, db, artea))
	assert.Equal(t, "Utilities", categoryOf(t, db, ignitis))
	assert.Equal(t, "Subscriptions", categoryOf(t, db, yt))
}

func TestEvelinaLoanMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	loan1 := seedTxAmount(t, db, "expense", "Housing", "EVELINA BALČIŪNIENĖ", 1650)
	loan2 := seedTxAmount(t, db, "expense", "Housing", "EVELINA PLYTNIKAITĖ", 1284)
	transfer := seedTxAmount(t, db, "expense", "Finance", "EVELINA BALČIŪNIENĖ", 1000)
	vacation := seedTxAmount(t, db, "expense", "Vacation", "EVELINA BALCIUNIENE", 1600)
	leasing := seedTxAmount(t, db, "investment", "Vehicle", "RAV4 leasing payment (contract via Evelina)", 330)
	sebLoan := seedTxAmount(t, db, "expense", "Finance", "Loan interest", 755)

	applyCategoryMigrations(db)

	// Loan payments via Evelina move to Finance and carry the loan label.
	assert.Equal(t, "Finance", categoryOf(t, db, loan1))
	assert.Equal(t, "Finance", categoryOf(t, db, loan2))
	assert.Contains(t, labelsOf(t, db, loan1), "loan")
	assert.Contains(t, labelsOf(t, db, loan2), "loan")
	// Round transfers stay Finance without the loan label.
	assert.Equal(t, "Finance", categoryOf(t, db, transfer))
	assert.NotContains(t, labelsOf(t, db, transfer), "loan")
	// Vacation stays; leasing labeled; everyone Evelina-related gets the counterparty tag.
	assert.Equal(t, "Vacation", categoryOf(t, db, vacation))
	assert.Contains(t, labelsOf(t, db, leasing), "leasing")
	assert.Contains(t, labelsOf(t, db, leasing), "evelina")
	assert.Contains(t, labelsOf(t, db, vacation), "evelina")
	assert.NotContains(t, labelsOf(t, db, sebLoan), "evelina")

	// The counterparty rule was created exactly once, even after reruns.
	applyCategoryMigrations(db)
	var rules int64
	db.Model(&domain.LabelRule{}).Where("label = 'evelina'").Count(&rules)
	assert.EqualValues(t, 1, rules)
	assert.Equal(t, "loan,evelina", labelsOf(t, db, loan1))
}

func seedTxAmount(t *testing.T, db *gorm.DB, typ, category, comment string, amount float64) uint {
	t.Helper()
	tx := domain.Transaction{Date: time.Now(), Type: domain.TransactionType(typ), Amount: amount, Category: domain.Category(category), Comment: comment}
	require.NoError(t, db.Create(&tx).Error)
	return tx.ID
}

func labelsOf(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var tx domain.Transaction
	require.NoError(t, db.First(&tx, id).Error)
	return tx.Labels
}
