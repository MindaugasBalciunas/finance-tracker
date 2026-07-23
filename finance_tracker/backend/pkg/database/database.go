package database

import (
	"os"

	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewSQLiteDB(path string) (*gorm.DB, error) {
	// Per-statement SQL logging is wasted I/O (and SD-card wear) in production;
	// set DB_LOG=info to re-enable it for debugging.
	logLevel := logger.Warn
	if os.Getenv("DB_LOG") == "info" {
		logLevel = logger.Info
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, err
	}

	// WAL allows concurrent reads during writes; busy_timeout retries instead
	// of failing with SQLITE_BUSY when a write overlaps another statement.
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

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

	if err := db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.StockTrade{}, &domain.ExportLog{}, &domain.Asset{}, &domain.AuthSettings{}, &domain.WebauthnCredential{}, &domain.Budget{}, &domain.LabelRule{}, &domain.BudgetSettings{}); err != nil {
		return nil, err
	}

	// Back-fill source = 'Revolut' for any stock trades that pre-date the source column
	if err := db.Exec("UPDATE stock_trades SET source = 'Revolut' WHERE source = '' OR source IS NULL").Error; err != nil {
		return nil, err
	}

	applyDataCleanups(db)
	applyCategoryMigrations(db)

	return db, nil
}

// applyDataCleanups repairs comment text before category/label passes run:
// mojibake from an early CSV import (UTF-8 read as Latin-1), recurring typos,
// and vendor-name variants that should consolidate into one canonical form.
// Idempotent: REPLACE is a no-op once the bad substring is gone and the
// exact-match UPDATEs only touch rows still carrying an old spelling.
func applyDataCleanups(db *gorm.DB) {
	replacements := [][2]string{
		// Mojibake: ž/ū/š/Š ė Ė double-encoded by the old importer.
		{"Å½", "Ž"},
		{"Å«", "ū"},
		{"Å¡", "š"},
		{"Å ", "Š"},
		{"VakarienÄ", "Vakarienė"},
		{"rogutÄs", "rogutės"},
		{"Äjimas", "Ėjimas"},
		// Recurring typos.
		{"appratment", "apartment"},
		{"appartment", "apartment"},
		{"Appartment", "Apartment"},
		{"(hause)", "(house)"},
		{"pernsion", "pension"},
		{"Nest car fuel", "Neste car fuel"},
		{"Hearcut", "Haircut"},
		{"Kipykla", "Kirpykla"},
		{"Pateon ", "Patreon "},
		{"Wraperija", "Wraperia"},
		{"flovers", "flowers"},
		{"iceream", "ice cream"},
		// Naming consistency.
		{"partly payment", "partial payment"},
		{"Nexos 2026", "Nexos.ai 2026"},
	}
	for _, r := range replacements {
		db.Exec(`UPDATE transactions SET comment = REPLACE(comment, ?, ?) WHERE comment LIKE ?`,
			r[0], r[1], "%"+r[0]+"%")
	}

	// Same merchant, four spellings → one canonical comment each.
	db.Exec(`UPDATE transactions SET comment = 'Artea (INVL) 3rd pillar pension'
		WHERE type = 'investment' AND LOWER(comment) LIKE '%artea%'
		AND comment != 'Artea (INVL) 3rd pillar pension'`)
	db.Exec(`UPDATE transactions SET comment = 'Gym Plius'
		WHERE comment IN ('Gym plius', 'Gymplius', 'GymPlius (health)')`)
	db.Exec(`UPDATE transactions SET comment = 'iLunch' WHERE LOWER(TRIM(comment)) = 'ilunch'`)
}

// applyCategoryMigrations recategorizes known-misfiled transactions by vendor.
// Idempotent: each statement only touches rows still in the old category.
func applyCategoryMigrations(db *gorm.DB) {
	// Artea 3rd-pillar contributions belong under Pension, not Stocks & ETF.
	db.Exec(`UPDATE transactions SET category = 'Pension'
		WHERE type = 'investment' AND category = 'Stocks & ETF' AND LOWER(comment) LIKE '%artea%'`)

	// Utility vendors filed under Housing → Utilities.
	for _, vendor := range []string{"IGNITIS", "TELIA LIETUVA", "VILNIAUS VANDENYS", "ŠILUMOS TINKLAI", "SILUMOS TINKLAI", "SAUGOS TARNYBA ARGUS", "MANO BŪSTAS", "MANO BUSTAS", "MIESTO GIJOS", "ENERGIJOS SKIRSTYMO", "ARGUS"} {
		db.Exec(`UPDATE transactions SET category = 'Utilities'
			WHERE type = 'expense' AND category = 'Housing' AND UPPER(comment) LIKE ?`, "%"+vendor+"%")
	}
	// Manually-entered utility bills (electricity/gas/water/telecom/heating/
	// garbage) that pre-date the Utilities category.
	db.Exec(`UPDATE transactions SET category = 'Utilities'
		WHERE type = 'expense' AND category = 'Housing' AND (
			LOWER(comment) LIKE 'electricity%' OR LOWER(comment) = 'gas' OR LOWER(comment) LIKE 'gas %'
			OR LOWER(comment) LIKE 'water%' OR LOWER(comment) LIKE 'telia%' OR LOWER(comment) = 'bite 3go'
			OR LOWER(comment) LIKE '%gijos apartment%'
			OR LOWER(comment) LIKE '%šiukl%' OR LOWER(comment) LIKE '%siuskl%' OR LOWER(comment) LIKE '%šiūkl%')`)

	// Recurring subscription vendors filed under Entertainment → Subscriptions.
	for _, vendor := range []string{"YOUTUBEPREMIUM", "YOUTUBE PREMIUM", "PATREON", "CONTRIBEE", "NETFLIX", "SPOTIFY", "GOOGLE *GOOGLE ONE", "HBO", "DISNEY", "YOUTUBE", "CLAUDE.AI", "GOOGLE ONE", "DELFIPLIUS"} {
		db.Exec(`UPDATE transactions SET category = 'Subscriptions'
			WHERE type = 'expense' AND category = 'Entertainment' AND UPPER(comment) LIKE ?`, "%"+vendor+"%")
	}

	// addLabelSQL appends a label to matching rows without duplicating it.
	addLabel := func(label, where string, args ...interface{}) {
		db.Exec(`UPDATE transactions SET labels = CASE
			WHEN labels = '' THEN '`+label+`'
			WHEN (',' || labels || ',') LIKE '%,`+label+`,%' THEN labels
			ELSE labels || ',`+label+`' END
			WHERE `+where, args...)
	}

	// Pre-refinance house-loan payments were paid via the ex-wife's account
	// and imported under Housing: monthly ~1,284–1,650 annuity that hands over
	// to the direct SEB 'Loan return/interest' rows in 2026 → Finance + loan.
	db.Exec(`UPDATE transactions SET category = 'Finance'
		WHERE type = 'expense' AND category = 'Housing'
		AND (UPPER(comment) LIKE '%EVELINA%' OR UPPER(comment) LIKE '%PLYTNIKAIT%')`)
	addLabel("loan", `type = 'expense' AND category = 'Finance'
		AND (UPPER(comment) LIKE '%EVELINA%' OR UPPER(comment) LIKE '%PLYTNIKAIT%')
		AND amount >= 1200 AND amount <= 1700`)

	// RAV4 leasing rows get a leasing label for filtering.
	addLabel("leasing", `type = 'investment' AND category = 'Vehicle' AND LOWER(comment) LIKE '%rav4%'`)

	// Counterparty label for every Evelina-related row (review aid), plus a
	// rule so future ones are tagged automatically.
	addLabel("evelina", `UPPER(comment) LIKE '%EVELINA%' OR UPPER(comment) LIKE '%PLYTNIKAIT%'`)
	ensureRule := func(rule domain.LabelRule) {
		var count int64
		db.Model(&domain.LabelRule{}).Where("label = ? AND category = ? AND comment_match = ?", rule.Label, rule.Category, rule.CommentMatch).Count(&count)
		if count == 0 {
			db.Create(&rule)
		}
	}
	ensureRule(domain.LabelRule{Label: "evelina", CommentMatch: "evelina"})

	// Dating category exists only since end of April 2026 (Kristina) — tag all
	// of it and keep tagging future rows via a category rule.
	addLabel("kristina", `type = 'expense' AND category = 'Dating' AND date >= '2026-04-20'`)
	ensureRule(domain.LabelRule{Label: "kristina", Category: "Dating"})

	// Context labels mined from repetitive comment patterns. Each pattern
	// becomes a rule (future auto-tagging) and is applied to history here.
	contextLabels := map[string][]string{
		"flowers":    {"gele", "gėle", "gėlė", "flower", "žiedas"},
		"coffee":     {"kava", "kavin", "coffee", "vero cafe", "caffeine", "cafe"},
		"fuel":       {"circle k", "viada", "orlen", "neste", "degalin", "baltic petrol", "balticpetroleum"},
		"pharmacy":   {"vaistin", "benu vaist", "gintarin", "camelia", "anteja", "rossmann"},
		"groceries":  {"maxima", "lidl", "rimi", "norfa", "moki-vezi", "moki vezi", "moki vež", "iki ", "barbora", "supermaistas", "biedronka", "aldi", "prekybos taskas", "zabka"},
		"delivery":   {"wolt", "bolt food", "maisto mylet"},
		"taxi":       {"uber", "etransport", "bolt.eu", "citybee"},
		"parking":    {"parking", "unipark", "stova", "susisiekimo paslaugos"},
		"bars":       {"alaus", "baras", "vyno"},
		"aliexpress": {"aliexpress", "alipay"},
		"insurance":  {"insurance", "draudim", "gjensidige", "compensa"},
		"fees":       {"banko mokestis", "plan fee", "account fee", "bank fee"},
		"hotel":      {"hotel", "booking.com", "viesbut"},
		"flights":    {"ryanair", "wizz", "wizair", "airbaltic"},
		"restaurant": {"restoran", "restaurant", "pizza", "picer", "kebab", "mcdonald", "hesburger", "grill", "bistro", "drakonai", "sushi"},
		"lunch":      {"lunch", "darbo piet", "pietūs"},
		"gym":        {"gym"},
		"cinema":     {" kinas", "cinema", "apollo"},
		"beauty":     {"haircut", "barber", "kirpykl", "grozio", "grožio"},
		"therapy":    {"psichoterap", "emosesij", "emosession", "mindfulness"},
		// Per-store labels (alongside the generic groceries label) so store
		// totals and average basket size can be compared in Reports.
		"maxima":    {"maxima"},
		"lidl":      {"lidl"},
		"iki":       {"iki "},
		"rimi":      {"rimi"},
		"norfa":     {"norfa"},
		"moki-vezi": {"moki-vezi", "moki vezi", "moki vež"},
		"barbora":   {"barbora"},
	}
	for label, patterns := range contextLabels {
		for _, p := range patterns {
			ensureRule(domain.LabelRule{Label: label, CommentMatch: p})
			addLabel(label, `LOWER(comment) LIKE ?`, "%"+p+"%")
		}
	}

	// The bare "kinas" cinema pattern false-positived on restaurant names
	// containing it as a suffix (Pekinas); replaced by " kinas" above.
	// Retire the old rule and strip the mislabel from affected rows.
	db.Exec(`DELETE FROM label_rules WHERE label = 'cinema' AND comment_match = 'kinas'`)
	db.Exec(`UPDATE transactions
		SET labels = TRIM(REPLACE(',' || labels || ',', ',cinema,', ','), ',')
		WHERE LOWER(comment) LIKE '%pekinas%' AND (',' || labels || ',') LIKE '%,cinema,%'`)
	// Bolt rides (but not Bolt Food) are taxi — history only; too ambiguous
	// as a standing rule, new ones are caught by the category suggestion.
	addLabel("taxi", `LOWER(comment) LIKE '%bolt%' AND LOWER(comment) NOT LIKE '%bolt food%'`)
}
