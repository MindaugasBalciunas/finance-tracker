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

func TestApplyDataCleanups(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	mojibake := seedTx(t, db, "expense", "Food", "Artistai PietÅ«s su kolega")
	mojibake2 := seedTx(t, db, "expense", "Health", "Å½ygis")
	mojibake3 := seedTx(t, db, "expense", "Kids - Food", "Å erbetas  360arena (kids)")
	artea1 := seedTx(t, db, "investment", "Pension", "Artea 3rd pernsion")
	artea2 := seedTx(t, db, "investment", "Pension", "Artea pensija")
	arteaOK := seedTx(t, db, "investment", "Pension", "Artea (INVL) 3rd pillar pension")
	gym := seedTx(t, db, "expense", "Health", "GymPlius (health)")
	nexos := seedTx(t, db, "income", "Salary", "Nexos 2026.05 (partly payment)")
	typo := seedTx(t, db, "expense", "Housing", "Electricity (appartment)")
	nest := seedTx(t, db, "expense", "Transport", "Nest car fuel (easter trip)")
	teliaManual := seedTx(t, db, "expense", "Housing", "Telia phone")
	pateon := seedTx(t, db, "expense", "Entertainment", "Pateon Algis Ramanauskas")
	claude := seedTx(t, db, "expense", "Entertainment", "claude.ai pro plan")

	applyDataCleanups(db)
	applyCategoryMigrations(db)

	assert.Equal(t, "Artistai Pietūs su kolega", commentOf(t, db, mojibake))
	assert.Equal(t, "Žygis", commentOf(t, db, mojibake2))
	assert.Equal(t, "Šerbetas  360arena (kids)", commentOf(t, db, mojibake3))
	assert.Equal(t, "Artea (INVL) 3rd pillar pension", commentOf(t, db, artea1), "typo variant consolidates")
	assert.Equal(t, "Artea (INVL) 3rd pillar pension", commentOf(t, db, artea2), "LT variant consolidates")
	assert.Equal(t, "Artea (INVL) 3rd pillar pension", commentOf(t, db, arteaOK), "canonical row untouched")
	assert.Equal(t, "Gym Plius", commentOf(t, db, gym))
	assert.Equal(t, "Nexos.ai 2026.05 (partial payment)", commentOf(t, db, nexos))
	assert.Equal(t, "Electricity (apartment)", commentOf(t, db, typo))
	assert.Equal(t, "Neste car fuel (easter trip)", commentOf(t, db, nest))
	assert.Equal(t, "Patreon Algis Ramanauskas", commentOf(t, db, pateon))

	// Cleaned comments feed the category/label passes that follow.
	assert.Equal(t, "Utilities", categoryOf(t, db, typo), "manual electricity bill refiles to Utilities")
	assert.Equal(t, "Utilities", categoryOf(t, db, teliaManual), "manual Telia bill refiles to Utilities")
	assert.Equal(t, "Subscriptions", categoryOf(t, db, pateon), "fixed Patreon typo refiles to Subscriptions")
	assert.Equal(t, "Subscriptions", categoryOf(t, db, claude))
	assert.Contains(t, labelsOf(t, db, nest), "fuel", "fixed Neste typo picks up fuel label")
	assert.Contains(t, labelsOf(t, db, mojibake), "lunch", "repaired Pietūs picks up lunch label")
	assert.Contains(t, labelsOf(t, db, gym), "gym")

	// Idempotent: a second full pass changes nothing.
	applyDataCleanups(db)
	applyCategoryMigrations(db)
	assert.Equal(t, "Artistai Pietūs su kolega", commentOf(t, db, mojibake))
	assert.Equal(t, "Nexos.ai 2026.05 (partial payment)", commentOf(t, db, nexos))
	assert.Equal(t, "Gym Plius", commentOf(t, db, gym))
}

func TestNewContextLabelRules(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	bolt := seedTx(t, db, "expense", "Transport", "BOLT.EU/O/2407191413 10134 Tallinn")
	boltFood := seedTx(t, db, "expense", "Food", "Bolt food - atviros metalo dirbtuves(food)")
	iki := seedTx(t, db, "expense", "Food", "IKI PILAITE 06222 VILNIUS")
	laisvalaikis := seedTx(t, db, "expense", "Gifts", "LAISVALAIKIO DOVANOS, UAB")
	mokiVezi := seedTx(t, db, "expense", "Food", "MOKI VEZI 06229 VILNIUS")
	insurance := seedTx(t, db, "expense", "Finance", "SWEDBANK P&C INSURANCE AS LIETUVOS FILIALAS")
	fee := seedTx(t, db, "expense", "Finance", "Swedbank plan fee")
	hotel := seedTx(t, db, "expense", "Vacation", "Hotel at Booking.com 1017 CE Amsterdam")
	flight := seedTx(t, db, "expense", "Vacation", "RYANAIR Milan")
	pizza := seedTx(t, db, "expense", "Food", "KAVINE BON PIZZA")
	barbora := seedTx(t, db, "expense", "Food", "BARBORA 03159 VILNIUS")

	applyCategoryMigrations(db)

	assert.Contains(t, labelsOf(t, db, bolt), "taxi", "Bolt ride gets taxi")
	assert.NotContains(t, labelsOf(t, db, boltFood), "taxi", "Bolt Food stays delivery")
	assert.Contains(t, labelsOf(t, db, boltFood), "delivery")
	assert.Contains(t, labelsOf(t, db, iki), "groceries")
	assert.NotContains(t, labelsOf(t, db, laisvalaikis), "groceries", "'iki ' must not match LAISVALAIKIO")
	assert.Contains(t, labelsOf(t, db, mokiVezi), "diy", "Moki Veži is a DIY chain")
	assert.NotContains(t, labelsOf(t, db, mokiVezi), "groceries", "Moki Veži is not groceries")
	assert.Contains(t, labelsOf(t, db, insurance), "insurance")
	assert.Contains(t, labelsOf(t, db, fee), "fees")
	assert.Contains(t, labelsOf(t, db, hotel), "hotel")
	assert.Contains(t, labelsOf(t, db, flight), "flights")
	assert.Contains(t, labelsOf(t, db, pizza), "restaurant")
	assert.Contains(t, labelsOf(t, db, pizza), "coffee", "kavinė still tags coffee alongside")
	assert.Contains(t, labelsOf(t, db, barbora), "groceries")

	// Store labels stack on top of the generic groceries label.
	assert.Contains(t, labelsOf(t, db, iki), "iki")
	assert.Contains(t, labelsOf(t, db, barbora), "barbora")
	assert.NotContains(t, labelsOf(t, db, laisvalaikis), "iki", "'iki ' store label must not match LAISVALAIKIO")

	maxima := seedTx(t, db, "expense", "Food", "MAXIMA LT, X-559 VILNIUS")
	norfaPharm := seedTx(t, db, "expense", "Health", "NORFOS VAISTINE 06200 VILNIUS")
	applyCategoryMigrations(db)
	assert.Contains(t, labelsOf(t, db, maxima), "maxima")
	assert.Contains(t, labelsOf(t, db, maxima), "groceries")
	assert.NotContains(t, labelsOf(t, db, norfaPharm), "norfa", "NORFOS VAISTINE is pharmacy, not the store")
}

func TestRetailChainCorrections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	moki := seedTx(t, db, "expense", "Food", "MOKI-VEZI 06232 VILNIUS")
	// Simulate the retired grocery labeling from a previous version.
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", moki).
		Update("labels", "groceries,moki-vezi").Error)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "groceries", CommentMatch: "moki vezi"}).Error)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "moki-vezi", CommentMatch: "moki vezi"}).Error)
	mokiCtx := seedTx(t, db, "expense", "Housing", "Moki vezi, Chemical spray for bugs and water filter salt")
	tokvila := seedTx(t, db, "expense", "Housing", "TOKVILA ZAL122")
	senukai := seedTx(t, db, "expense", "Housing", "UAB KESKO SENUKAI DIGITAL")
	pigu := seedTx(t, db, "expense", "Entertainment", "UAB PIGU")
	lidl := seedTx(t, db, "expense", "Food", "LIDL/60182 LIDL PILAIT 06293 VILNIUS")

	applyDataCleanups(db)
	applyCategoryMigrations(db)

	// Comments unified for pure bank strings, context comments untouched.
	assert.Equal(t, "Moki Veži", commentOf(t, db, moki))
	assert.Equal(t, "Moki vezi, Chemical spray for bugs and water filter salt", commentOf(t, db, mokiCtx))
	assert.Equal(t, "Kesko Senukai", commentOf(t, db, senukai))
	assert.Equal(t, "Pigu.lt", commentOf(t, db, pigu))

	// Categories: Moki Veži Food→Housing, Tokvila Housing→Transport.
	assert.Equal(t, "Housing", categoryOf(t, db, moki))
	assert.Equal(t, "Transport", categoryOf(t, db, tokvila))
	assert.Equal(t, "Food", categoryOf(t, db, lidl), "real groceries stay")

	// Labels: diy replaces groceries/moki-vezi on DIY chains.
	assert.Equal(t, "diy", labelsOf(t, db, moki))
	assert.Contains(t, labelsOf(t, db, mokiCtx), "diy")
	assert.Contains(t, labelsOf(t, db, senukai), "diy")
	assert.Contains(t, labelsOf(t, db, pigu), "electronics")
	assert.Contains(t, labelsOf(t, db, lidl), "groceries")

	// Retired rules gone even if imported back from an old export.
	var stale int64
	db.Model(&domain.LabelRule{}).Where("label = 'moki-vezi' OR (label = 'groceries' AND comment_match LIKE 'moki%')").Count(&stale)
	assert.EqualValues(t, 0, stale)

	// Idempotent.
	applyDataCleanups(db)
	applyCategoryMigrations(db)
	assert.Equal(t, "diy", labelsOf(t, db, moki))
	assert.Equal(t, "Moki Veži", commentOf(t, db, moki))
}

func TestUserLabelRules(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	argus := seedTx(t, db, "expense", "Utilities", "UAB SAUGOS TARNYBA ARGUS")
	argusManual := seedTx(t, db, "expense", "Utilities", "Argus (security)")
	kebab := seedTx(t, db, "expense", "Food", "Pammukale. Kebab")
	mcd := seedTx(t, db, "expense", "Food", "McDonalds 44000017 LT-04352 Vilnius")
	ilunch := seedTx(t, db, "expense", "Food", "iLunch food with colleagues")
	nexos := seedTx(t, db, "income", "Salary", "Nexos.ai 2026.07 (partial payment)")
	ibkrTop := seedTx(t, db, "investment", "Stocks & ETF", "ibkr top up")
	swedTop := seedTx(t, db, "investment", "Stocks & ETF", "top up from Swedbank")
	revTop := seedTx(t, db, "investment", "Stocks & ETF", "Revolut top up")

	applyCategoryMigrations(db)

	assert.Contains(t, labelsOf(t, db, argus), "security")
	assert.Contains(t, labelsOf(t, db, argusManual), "security")
	assert.Contains(t, labelsOf(t, db, kebab), "fast food")
	assert.Contains(t, labelsOf(t, db, kebab), "restaurant")
	assert.Contains(t, labelsOf(t, db, mcd), "fast food")
	bbq := seedTx(t, db, "expense", "Food", "Lidl. Groceries mostly for bbq burgers date at home")
	applyCategoryMigrations(db)
	assert.NotContains(t, labelsOf(t, db, bbq), "fast food", "grocery bbq run is not fast food")
	assert.Contains(t, labelsOf(t, db, ilunch), "work lunch")
	assert.Contains(t, labelsOf(t, db, ilunch), "lunch")
	assert.Contains(t, labelsOf(t, db, nexos), "nexos")
	assert.Contains(t, labelsOf(t, db, ibkrTop), "ibkr")
	assert.Contains(t, labelsOf(t, db, swedTop), "ibkr", "Swedbank->IBKR top-up backfilled")
	assert.NotContains(t, labelsOf(t, db, revTop), "ibkr", "Revolut top-up is not IBKR")
}

func TestCinemaRuleDoesNotMatchPekinas(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	// Row mislabeled by the old bare-"kinas" rule (kept its other labels).
	pekinas := seedTx(t, db, "expense", "Food", "Pekinas. guanbao")
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", pekinas).
		Update("labels", "work lunch,cinema").Error)
	apollo := seedTx(t, db, "expense", "Entertainment", "APOLLO KINAS, UAB")
	// Simulate the retired rule existing from a previous version.
	require.NoError(t, db.Create(&domain.LabelRule{Label: "cinema", CommentMatch: "kinas"}).Error)

	applyCategoryMigrations(db)

	assert.Equal(t, "work lunch", labelsOf(t, db, pekinas), "mislabel stripped, other labels kept")
	assert.Contains(t, labelsOf(t, db, apollo), "cinema", "real cinema still matches via ' kinas'")
	var oldRule int64
	db.Model(&domain.LabelRule{}).Where("label = 'cinema' AND comment_match = 'kinas'").Count(&oldRule)
	assert.EqualValues(t, 0, oldRule, "retired rule removed")

	// Idempotent on rerun.
	applyCategoryMigrations(db)
	assert.Equal(t, "work lunch", labelsOf(t, db, pekinas))
}

func commentOf(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var tx domain.Transaction
	require.NoError(t, db.First(&tx, id).Error)
	return tx.Comment
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
