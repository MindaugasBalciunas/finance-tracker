package database

import (
	"math"
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

func TestCategoryUnification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}, &domain.Budget{}))

	school := seedTx(t, db, "expense", "Kids - Education", "SKAITLIS UAB")
	arena := seedTx(t, db, "expense", "Kids - Entertainment", "360 arena")
	dinner := seedTx(t, db, "expense", "Kids - Food", "Vakarienė 360arena (kids)")
	alimony := seedTx(t, db, "expense", "Kids - General", "Aliments 2026.06")
	lawyer := seedTx(t, db, "expense", "Divorce", "POVILAS LATVYS (Už teisines paslaugas)")
	nekartu := seedTx(t, db, "expense", "Divorce", "Nekartu")
	refund := seedTx(t, db, "income", "Divorce", "Nekartu - Evelina")
	atm := seedTx(t, db, "investment", "Finance", "Cash withdrawal (ATM)")
	revolut := seedTx(t, db, "investment", "Finance", "Revolut top up")
	adult := seedTx(t, db, "expense", "Entertainment", "Cinema night")

	// Pre-unification budget and category-scoped rule follow their rows.
	require.NoError(t, db.Create(&domain.Budget{Name: "Kids - Entertainment", Kind: "spending", Category: "Kids - Entertainment", Amount: 50}).Error)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "alimony", Category: "Kids - General", CommentMatch: "alim"}).Error)

	applyCategoryMigrations(db)

	// One Kids category; the old sub-category lives on as a label.
	for id, sub := range map[uint]string{school: "education", arena: "entertainment", dinner: "food"} {
		assert.Equal(t, "Kids", categoryOf(t, db, id))
		assert.Contains(t, labelsOf(t, db, id), sub)
		assert.Contains(t, labelsOf(t, db, id), "kids")
	}
	assert.Equal(t, "Kids", categoryOf(t, db, alimony))
	assert.Contains(t, labelsOf(t, db, alimony), "kids")
	assert.NotContains(t, labelsOf(t, db, adult), "kids", "adult entertainment untouched")

	// Divorce dissolves into Finance/Reimbursement + divorce label.
	assert.Equal(t, "Finance", categoryOf(t, db, lawyer))
	assert.Equal(t, "Finance", categoryOf(t, db, nekartu))
	assert.Equal(t, "Reimbursement", categoryOf(t, db, refund))
	for _, id := range []uint{lawyer, nekartu, refund} {
		assert.Contains(t, labelsOf(t, db, id), "divorce")
	}

	// Own-money movements move to Transfers and get differentiated by labels.
	assert.Equal(t, "Transfers", categoryOf(t, db, atm))
	assert.Equal(t, "Transfers", categoryOf(t, db, revolut))
	assert.Contains(t, labelsOf(t, db, atm), "cash")
	assert.Contains(t, labelsOf(t, db, revolut), "revolut")

	// Budget follows: whole-Kids limit, floor raised to the €250 median.
	var b domain.Budget
	require.NoError(t, db.First(&b, "category = ?", "Kids").Error)
	assert.Equal(t, "Kids", b.Name)
	assert.EqualValues(t, 250, b.Amount)
	var oldBudgets int64
	db.Model(&domain.Budget{}).Where("category LIKE 'Kids - %'").Count(&oldBudgets)
	assert.EqualValues(t, 0, oldBudgets)

	// Rule follows the rename; category-Kids rule exists for future rows.
	var rule domain.LabelRule
	require.NoError(t, db.First(&rule, "label = 'alimony'").Error)
	assert.Equal(t, "Kids", rule.Category)
	var kidsRule int64
	db.Model(&domain.LabelRule{}).Where("label = 'kids' AND category = 'Kids'").Count(&kidsRule)
	assert.EqualValues(t, 1, kidsRule)

	// New rows created under Kids match the standing category rule.
	newKid := domain.Transaction{Date: time.Now(), Type: "expense", Amount: 12, Category: "Kids", Comment: "Zoo tickets"}
	kidsCatRule := domain.LabelRule{Label: "kids", Category: "Kids"}
	assert.True(t, kidsCatRule.Matches(&newKid))

	// Idempotent: rerun changes nothing and creates no duplicates.
	applyCategoryMigrations(db)
	assert.Equal(t, "Kids", categoryOf(t, db, alimony))
	db.Model(&domain.LabelRule{}).Where("label = 'kids' AND category = 'Kids'").Count(&kidsRule)
	assert.EqualValues(t, 1, kidsRule)
	var kidsBudgets int64
	db.Model(&domain.Budget{}).Where("category = 'Kids'").Count(&kidsBudgets)
	assert.EqualValues(t, 1, kidsBudgets)
	assert.Equal(t, "education,kids", labelsOf(t, db, school), "labels not duplicated on rerun")
}

func TestIkiLabelCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	store := seedTx(t, db, "expense", "Food", "IKI PILAITE 06222 VILNIUS")
	storeSmall := seedTx(t, db, "expense", "Food", "IKIUKAS NR. 831")
	flowers := seedTx(t, db, "expense", "Dating", "Iki flowers")
	// False positives of the retired "iki " substring pattern: "iki" is the
	// Lithuanian "until". Simulate the labels the old rule left behind.
	rent := seedTx(t, db, "expense", "Vacation", "ShortStopPasilaiciai (Nuoma iki 12.01)")
	stale := seedTx(t, db, "income", "Reimbursement", "Zita (Vacation to Tenerife)")
	for _, id := range []uint{rent, stale} {
		require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", id).
			Update("labels", "groceries,iki").Error)
	}
	// A hand-labeled groceries row with no store pattern must survive.
	manual := seedTx(t, db, "expense", "Food", "Farmers market veggies")
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", manual).
		Update("labels", "groceries").Error)
	// Retired substring rules present from a previous version.
	require.NoError(t, db.Create(&domain.LabelRule{Label: "groceries", CommentMatch: "iki "}).Error)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "iki", CommentMatch: "iki "}).Error)

	applyCategoryMigrations(db)

	// Real store rows (comment starts with iki) are labeled by the anchored rule.
	for _, id := range []uint{store, storeSmall, flowers} {
		assert.Contains(t, labelsOf(t, db, id), "iki")
		assert.Contains(t, labelsOf(t, db, id), "groceries")
	}
	// False positives lose both labels; the hand-labeled row keeps groceries.
	assert.Equal(t, "", labelsOf(t, db, rent))
	assert.Equal(t, "", labelsOf(t, db, stale))
	assert.Equal(t, "groceries", labelsOf(t, db, manual))

	// Old substring rules retired, anchored replacements in place.
	var old, anchored int64
	db.Model(&domain.LabelRule{}).Where("comment_match = 'iki '").Count(&old)
	db.Model(&domain.LabelRule{}).Where("comment_match = '^iki'").Count(&anchored)
	assert.EqualValues(t, 0, old)
	assert.EqualValues(t, 2, anchored, "groceries + iki anchored rules")

	// Idempotent.
	applyCategoryMigrations(db)
	assert.Equal(t, "", labelsOf(t, db, rent))
	assert.Contains(t, labelsOf(t, db, store), "iki")
}

func TestAnchoredCommentPatterns(t *testing.T) {
	assert.True(t, domain.CommentPatternMatches("^iki", "IKI PILAITE VILNIUS"))
	assert.True(t, domain.CommentPatternMatches("^iki", "Iki. Quick shopping"))
	assert.False(t, domain.CommentPatternMatches("^iki", "Nuoma iki 24d"))
	assert.True(t, domain.CommentPatternMatches("iki ", "Nuoma iki 24d"), "unanchored stays substring")

	rule := domain.LabelRule{Label: "iki", CommentMatch: "^iki"}
	assert.True(t, rule.Matches(&domain.Transaction{Comment: "IKI EXPRESS"}))
	assert.False(t, rule.Matches(&domain.Transaction{Comment: "Nuoma iki 24d"}))
}

func TestInvlPayoutReclassAndPayrollAccounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.LabelRule{}))

	payout := seedTx(t, db, "income", "Reimbursement", "INVL EXTREMO III 16+ (/Dalinė išmoka/900604/MINDAUGAS BALČIŪNAS/3/INVL33007679/)")
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", payout).Update("credit_account", "swed").Error)
	salary := seedTx(t, db, "income", "Salary", "Nexos.ai 2026.07")
	payroll := seedTx(t, db, "income", "Salary", "Artea (INVL) pension contribution via payroll — employer (Vipps)")
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", payroll).Update("labels", "artea,payroll,vipps mobilepay").Error)
	payrollInv := seedTx(t, db, "investment", "Pension", "Artea (INVL) 3rd pillar pension (payroll — employer)")
	require.NoError(t, db.Model(&domain.Transaction{}).Where("id = ?", payrollInv).Update("labels", "artea,payroll").Error)

	applyCategoryMigrations(db)

	// The payout arrival becomes the bank side of a pension→bank transfer.
	var p domain.Transaction
	require.NoError(t, db.First(&p, payout).Error)
	assert.Equal(t, domain.TransactionTypeInvestment, p.Type)
	assert.Equal(t, "Transfers", string(p.Category))
	assert.Equal(t, "art", p.DebitAccount)
	assert.Equal(t, "swed", p.CreditAccount)
	assert.Contains(t, p.Labels, "artea")

	// Payroll rows keep their empty accounts; normal rows get the default.
	var s, pr, pi domain.Transaction
	require.NoError(t, db.First(&s, salary).Error)
	require.NoError(t, db.First(&pr, payroll).Error)
	require.NoError(t, db.First(&pi, payrollInv).Error)
	assert.Equal(t, "swed", s.CreditAccount)
	assert.Equal(t, "", pr.CreditAccount)
	assert.Equal(t, "", pi.DebitAccount)

	// Idempotent.
	applyCategoryMigrations(db)
	require.NoError(t, db.First(&p, payout).Error)
	assert.Equal(t, "Transfers", string(p.Category))
}

func TestCashBufferBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Balance{}))

	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return d
	}
	before := domain.Balance{Date: day("2020-06-30"), Swed: 900, Total: 900}
	start := domain.Balance{Date: day("2020-09-30"), Swed: 1000, Total: 1000}
	mid := domain.Balance{Date: day("2022-05-15"), Swed: 2000, Total: 2000}
	anchor := domain.Balance{Date: day("2024-01-31"), Swed: 3000, Cash: 6000, Total: 9000}
	for _, b := range []*domain.Balance{&before, &start, &mid, &anchor} {
		require.NoError(t, db.Create(b).Error)
	}

	applyBalanceBackfills(db)

	cashOf := func(id uint) (float64, float64) {
		var b domain.Balance
		require.NoError(t, db.First(&b, id).Error)
		return b.Cash, b.Total
	}
	c, total := cashOf(before.ID)
	assert.Zero(t, c, "pre-window snapshot untouched")
	assert.EqualValues(t, 900, total)

	c, total = cashOf(start.ID)
	assert.InDelta(t, 872.09, c, 0.01, "≈€750 buffer grown one month into the window")
	assert.InDelta(t, 1000+c, total, 0.01)

	midCash, _ := cashOf(mid.ID)
	// Linear halfway-ish between 750 and 6000 (2022-05-15 is ~50% of the span).
	assert.Greater(t, midCash, 3000.0)
	assert.Less(t, midCash, 4000.0)

	c, total = cashOf(anchor.ID)
	assert.EqualValues(t, 6000, c, "anchor untouched")
	assert.EqualValues(t, 9000, total)

	// Idempotent: cash != 0 everywhere in the window now.
	applyBalanceBackfills(db)
	c2, total2 := cashOf(mid.ID)
	assert.Equal(t, midCash, c2)
	assert.InDelta(t, 2000+midCash, total2, 0.01)
}

func TestSebPensionBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}))

	// The embedded Sodra series must sum to the report's stated total.
	cum := 0.0
	for _, c := range sebPenContributions {
		cum += c.total
	}
	assert.InDelta(t, 5126.93, cum, 0.001, "series matches the Sodra report total")

	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return d
	}
	prejoin := domain.Balance{Date: day("2014-06-30"), Swed: 100, Total: 100}
	// A wrong synthetic value left by the previous estimate-based model —
	// the pass is self-correcting and must overwrite it (and fix the total).
	stale := domain.Balance{Date: day("2015-06-30"), Swed: 500, SebPen: 4000, Total: 4500}
	mid := domain.Balance{Date: day("2017-01-15"), Swed: 700, Total: 700}
	coast := domain.Balance{Date: day("2021-12-05"), Swed: 900, Total: 900} // 2y after last transfer
	// Hand-tracked anchor inside the tracked era: exactly double the
	// contributed total → implied rate = 2^(1/years since last transfer).
	anchor := domain.Balance{Date: day("2024-01-31"), SebPen: 10253.86, Total: 10253.86}
	for _, b := range []*domain.Balance{&prejoin, &stale, &mid, &coast, &anchor} {
		require.NoError(t, db.Create(b).Error)
	}

	applyBalanceBackfills(db)

	penOf := func(id uint) (float64, float64) {
		var b domain.Balance
		require.NoError(t, db.First(&b, id).Error)
		return b.SebPen, b.Total
	}
	pen, total := penOf(prejoin.ID)
	assert.Zero(t, pen, "before joining the 2nd pillar")
	assert.EqualValues(t, 100, total)

	// Cumulative transfers by 2015-06-30: 46.15 + 46.66 = 92.81.
	pen, total = penOf(stale.ID)
	assert.InDelta(t, 92.81, pen, 0.01, "stale synthetic value overwritten with the real cumulative")
	assert.InDelta(t, 500+92.81, total, 0.01, "total corrected by the delta")

	// Cumulative by 2017-01-15: all 2015 + 2016 transfers + 2017-01-10.
	pen, _ = penOf(mid.ID)
	assert.InDelta(t, 1503.30, pen, 0.01)

	// Coast era: 5126.93 × rate² with rate = 2^(1/years(2019-12-05 → anchor)).
	years := day("2024-01-31").Sub(day("2019-12-05")).Hours() / 24 / 365.25
	rate := math.Pow(2, 1/years)
	pen, total = penOf(coast.ID)
	assert.InDelta(t, 5126.93*rate*rate, pen, 5)
	assert.InDelta(t, 900+pen, total, 0.01)

	penA, totalA := penOf(anchor.ID)
	assert.EqualValues(t, 10253.86, penA, "anchor untouched")
	assert.EqualValues(t, 10253.86, totalA)

	// Idempotent: identical values on rerun.
	coastPen, coastTotal := penOf(coast.ID)
	applyBalanceBackfills(db)
	p2, t2 := penOf(coast.ID)
	assert.Equal(t, coastPen, p2)
	assert.Equal(t, coastTotal, t2)
}

func TestCanonicalCategoryLegacyValues(t *testing.T) {
	cat, labels := domain.CanonicalCategory("expense", "Kids - Food")
	assert.EqualValues(t, "Kids", cat)
	assert.Equal(t, []string{"kids", "food"}, labels)

	cat, labels = domain.CanonicalCategory("expense", "Divorce")
	assert.EqualValues(t, "Finance", cat)
	assert.Equal(t, []string{"divorce"}, labels)

	cat, _ = domain.CanonicalCategory("income", "Divorce")
	assert.EqualValues(t, "Reimbursement", cat)

	cat, labels = domain.CanonicalCategory("expense", "Food")
	assert.EqualValues(t, "Food", cat)
	assert.Nil(t, labels)
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
