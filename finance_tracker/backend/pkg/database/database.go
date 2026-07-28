package database

import (
	"os"
	"strings"

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

	// Retail-chain bank strings (merchant + terminal + city noise) → one
	// canonical merchant name. User-written comments with extra context
	// (e.g. "Moki vezi, Chemical spray…") are left untouched.
	db.Exec(`UPDATE transactions SET comment = 'Moki Veži'
		WHERE comment LIKE 'MOKI VEZI %' OR comment LIKE 'MOKI-VEZI %' OR LOWER(TRIM(comment)) IN ('moki veži.', 'moki veži', 'moki vezi')`)
	db.Exec(`UPDATE transactions SET comment = 'Kesko Senukai'
		WHERE UPPER(comment) LIKE '%KESKO SENUKAI%'`)
	db.Exec(`UPDATE transactions SET comment = 'Depo'
		WHERE comment IN ('DEPO VILNIUS', 'DEPO PANEVEZYS')`)
	db.Exec(`UPDATE transactions SET comment = 'Pigu.lt' WHERE comment = 'UAB PIGU' OR comment = 'UAB "PIGU"'`)
	db.Exec(`UPDATE transactions SET comment = 'Varlė.lt' WHERE comment = 'Varle UAB'`)
	// Fee refunds with no payee used to render as a dangling "Card refund:".
	db.Exec(`UPDATE transactions SET comment = 'Card refund (Swedbank)'
		WHERE TRIM(comment) = 'Card refund:'`)
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
		// "^iki" anchors to the comment start: as a substring, "iki" is the
		// Lithuanian "until" ("nuoma iki 24d") and hid inside other words.
		"groceries":  {"maxima", "lidl", "rimi", "norfa", "^iki", "barbora", "supermaistas", "biedronka", "aldi", "prekybos taskas", "zabka"},
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
		// Kindergarten/school vendors and phrases — differentiates the unified
		// Kids category (see the category unification block below).
		"education": {"skaitlis", "vaikystės stebuklas", "daržel", "darzel"},
		// Divorce is a label now, not a category; Nekartu is the mediation
		// service used through the process.
		"divorce": {"nekartu"},
		// Own-money movements inside investment/Finance.
		"cash":    {"cash withdrawal", "cash deposit"},
		"revolut": {"revolut top up"},
		"beauty":     {"haircut", "barber", "kirpykl", "grozio", "grožio"},
		"therapy":    {"psichoterap", "emosesij", "emosession", "mindfulness"},
		// Per-store labels (alongside the generic groceries label) so store
		// totals and average basket size can be compared in Reports.
		"maxima":  {"maxima"},
		"lidl":    {"lidl"},
		"iki":     {"^iki"},
		"rimi":    {"rimi"},
		"norfa":   {"norfa"},
		"barbora": {"barbora"},
		// Moki Veži/Senukai/Depo are DIY & building-materials chains, and
		// Pigu/Varlė are electronics e-shops — not groceries. "depo " keeps a
		// trailing space so "deposit" comments stay unlabeled.
		"diy":         {"moki vež", "moki vezi", "moki-vezi", "senukai", "kesko", "depo ", "ermita", "ikea"},
		"electronics": {"pigu", "varle", "varlė", "electronic trade", "mk trade", "samsung", "technikos centr"},
		// Per-shop labels on DIY/home chains, mirroring the grocery store
		// labels, so per-store spend can be compared.
		"senukai":   {"senukai", "kesko"},
		"depo":      {"depo "},
		"ikea":      {"ikea"},
		"jysk":      {"jysk"},
		"moki-vezi": {"moki vež", "moki vezi", "moki-vezi"},
		// Rules behind labels the user created by hand, so future rows
		// self-apply and history backfills.
		"security":   {"argus", "saugos tarnyba"},
		"fast food":  {"kebab", "mcdonald", "hesburger", "burgermeister", "burger king", "kfc"},
		"work lunch": {"work lunch", "team lunch", "su kolega", "colleag", "ilunch", "darbo piet"},
		"nexos":      {"nexos"},
		"ibkr":       {"ibkr"},
		// Employer labels on salary rows — backfills statement imports that
		// predate employer labeling and keeps future rows consistent.
		"vipps mobilepay": {"vipps"},
		"mobilepay":       {"mobilepay a/s"},
		"danske":          {"danske bank"},
		"barclays":        {"barclays"},
		"doclogix":        {"doclogix"},
		"app camp":        {"app camp"},
		"pvcase":          {"pvcase"},
	}
	for label, patterns := range contextLabels {
		for _, p := range patterns {
			ensureRule(domain.LabelRule{Label: label, CommentMatch: p})
			// '^'-anchored patterns match the comment start (LabelRule syntax).
			like := "%" + p + "%"
			if anchored, ok := strings.CutPrefix(p, "^"); ok {
				like = anchored + "%"
			}
			addLabel(label, `LOWER(comment) LIKE ?`, like)
		}
	}

	// IBKR deposits phrased as "top up from Swedbank" don't mention ibkr;
	// history only — "top up" alone is too ambiguous as a standing rule
	// (Revolut top-ups share the phrase).
	addLabel("ibkr", `type = 'investment' AND category = 'Stocks & ETF'
		AND LOWER(comment) LIKE '%top up%' AND LOWER(comment) NOT LIKE '%revolut%'`)

	// The bare "depo" pattern used to hit "deposit" comments; the rule is now
	// "depo " and the canonical "Depo" rows get their labels via exact match.
	db.Exec(`DELETE FROM label_rules WHERE label IN ('diy', 'depo') AND comment_match = 'depo'`)
	for _, l := range []string{"diy", "depo"} {
		db.Exec(`UPDATE transactions
			SET labels = TRIM(REPLACE(',' || labels || ',', ',`+l+`,', ','), ',')
			WHERE LOWER(comment) LIKE '%deposit%' AND (',' || labels || ',') LIKE '%,`+l+`,%'`)
		addLabel(l, `LOWER(comment) = 'depo'`)
	}

	// The bare "kinas" cinema pattern false-positived on restaurant names
	// containing it as a suffix (Pekinas); replaced by " kinas" above.
	// Retire the old rule and strip the mislabel from affected rows.
	db.Exec(`DELETE FROM label_rules WHERE label = 'cinema' AND comment_match = 'kinas'`)
	db.Exec(`UPDATE transactions
		SET labels = TRIM(REPLACE(',' || labels || ',', ',cinema,', ','), ',')
		WHERE LOWER(comment) LIKE '%pekinas%' AND (',' || labels || ',') LIKE '%,cinema,%'`)

	// Likewise, a bare "burger" fast-food pattern hit grocery runs mentioning
	// burgers; only named venues remain above.
	db.Exec(`DELETE FROM label_rules WHERE label = 'fast food' AND comment_match = 'burger'`)
	db.Exec(`UPDATE transactions
		SET labels = TRIM(REPLACE(',' || labels || ',', ',fast food,', ','), ',')
		WHERE LOWER(comment) LIKE '%bbq burger%' AND (',' || labels || ',') LIKE '%,fast food,%'`)

	// Moki Veži is a DIY chain, not a grocery store: retire its groceries
	// patterns and per-store label, strip both labels from its rows, and
	// refile the rows misfiled under Food. Tokvila is RAV4 tyre service →
	// Transport.
	db.Exec(`DELETE FROM label_rules WHERE label = 'groceries' AND comment_match IN ('moki-vezi', 'moki vezi', 'moki vež')`)
	db.Exec(`DELETE FROM label_rules WHERE label = 'moki-vezi'`)
	for _, l := range []string{"groceries", "moki-vezi"} {
		db.Exec(`UPDATE transactions
			SET labels = TRIM(REPLACE(',' || labels || ',', ',`+l+`,', ','), ',')
			WHERE (LOWER(comment) LIKE '%moki vež%' OR LOWER(comment) LIKE '%moki vezi%' OR LOWER(comment) LIKE '%moki-vezi%')
			AND (',' || labels || ',') LIKE '%,`+l+`,%'`)
	}
	db.Exec(`UPDATE transactions SET category = 'Housing'
		WHERE type = 'expense' AND category = 'Food'
		AND (LOWER(comment) LIKE '%moki vež%' OR LOWER(comment) LIKE '%moki vezi%' OR LOWER(comment) LIKE '%moki-vezi%')`)
	db.Exec(`UPDATE transactions SET category = 'Transport'
		WHERE type = 'expense' AND category = 'Housing' AND UPPER(comment) LIKE '%TOKVILA%'`)

	// Rows entered without an account almost certainly went through the
	// Swedbank card (the everyday account) — default the money side.
	db.Exec(`UPDATE transactions SET debit_account = 'swed'
		WHERE type IN ('expense', 'investment') AND (debit_account IS NULL OR debit_account = '')`)
	db.Exec(`UPDATE transactions SET credit_account = 'swed'
		WHERE type = 'income' AND (credit_account IS NULL OR credit_account = '')`)
	// Bolt rides (but not Bolt Food) are taxi — history only; too ambiguous
	// as a standing rule, new ones are caught by the category suggestion.
	addLabel("taxi", `LOWER(comment) LIKE '%bolt%' AND LOWER(comment) NOT LIKE '%bolt food%'`)

	// --- Category unification (v1.4.0): categories say what domain the money
	// went to, labels differentiate within it. The four Kids sub-categories
	// collapse into one Kids category whose old distinction lives on as
	// education/entertainment/food labels; every Kids row also carries the
	// kids label so it groups with kid-related rows in other categories
	// (Clothing, Transport…). Labels must be added while the old category
	// still identifies the rows, so the category flip comes last.
	for old, sub := range map[string]string{
		"Kids - Education":     "education",
		"Kids - Entertainment": "entertainment",
		"Kids - Food":          "food",
		"Kids - General":       "",
	} {
		if sub != "" {
			addLabel(sub, `category = ?`, old)
		}
		addLabel("kids", `category = ?`, old)
		db.Exec(`UPDATE transactions SET category = 'Kids' WHERE category = ?`, old)
	}
	ensureRule(domain.LabelRule{Label: "kids", Category: "Kids"})

	// Divorce was a life event, not a spending domain: costs live on under
	// Finance and recoveries under Reimbursement, both differentiated by the
	// divorce label (plus the nekartu comment rule above for future rows).
	addLabel("divorce", `category = 'Divorce'`)
	db.Exec(`UPDATE transactions SET category = 'Reimbursement'
		WHERE type = 'income' AND category = 'Divorce'`)
	db.Exec(`UPDATE transactions SET category = 'Finance' WHERE category = 'Divorce'`)

	// Bare "Cash" comments are ATM movements too; too short for a standing
	// comment rule (would substring-match e.g. "cashback"), so history only.
	addLabel("cash", `LOWER(comment) = 'cash'`)

	// Budgets and label rules pointing at retired categories follow their
	// rows. The old Kids - Entertainment spending limit becomes the whole-Kids
	// limit: €250 matches the recent median of non-alimony kids spending
	// (alimony no longer counts against spending limits — it is covered by the
	// fixed Alimony budget).
	db.Exec(`UPDATE budgets SET name = 'Kids', category = 'Kids',
		amount = CASE WHEN kind = 'spending' AND amount < 250 THEN 250 ELSE amount END
		WHERE category LIKE 'Kids - %'`)
	db.Exec(`DELETE FROM budgets WHERE category = 'Kids' AND id NOT IN
		(SELECT MIN(id) FROM budgets WHERE category = 'Kids' GROUP BY kind, label)`)
	db.Exec(`UPDATE label_rules SET category = 'Kids' WHERE category LIKE 'Kids - %'`)
	// A category-scoped rule on Divorce would over-apply if pointed at the
	// much broader Finance — retire instead of remapping.
	db.Exec(`DELETE FROM label_rules WHERE category = 'Divorce'`)
	db.Exec(`UPDATE budgets SET category = 'Finance' WHERE category = 'Divorce'`)
	// The category rename can leave byte-identical rules behind — dedupe.
	db.Exec(`DELETE FROM label_rules WHERE id NOT IN
		(SELECT MIN(id) FROM label_rules GROUP BY label, category, comment_match)`)

	// --- iki label cleanup (v1.4.1): the old "iki " substring pattern (and an
	// even older bare "iki" one) hit the Lithuanian word "until" ("nuoma iki
	// 24d") and left stale labels across nine categories. The store always
	// opens the comment ("IKI PILAITE…", "Iki. Quick shopping…", "IKIUKAS…"),
	// so the rules above now use the '^iki' anchored pattern. Retire the old
	// rules and strip the mislabels — groceries first, while the iki label
	// still marks the false-positive rows (only rows matching no other
	// grocery pattern lose it, so hand-labeled rows survive).
	db.Exec(`DELETE FROM label_rules WHERE comment_match IN ('iki', 'iki ') AND label IN ('groceries', 'iki')`)
	db.Exec(`UPDATE transactions
		SET labels = TRIM(REPLACE(',' || labels || ',', ',groceries,', ','), ',')
		WHERE (',' || labels || ',') LIKE '%,iki,%'
		AND (',' || labels || ',') LIKE '%,groceries,%'
		AND LOWER(comment) NOT LIKE 'iki%'
		AND LOWER(comment) NOT LIKE '%maxima%' AND LOWER(comment) NOT LIKE '%lidl%'
		AND LOWER(comment) NOT LIKE '%rimi%' AND LOWER(comment) NOT LIKE '%norfa%'
		AND LOWER(comment) NOT LIKE '%barbora%' AND LOWER(comment) NOT LIKE '%supermaistas%'
		AND LOWER(comment) NOT LIKE '%biedronka%' AND LOWER(comment) NOT LIKE '%aldi%'
		AND LOWER(comment) NOT LIKE '%prekybos taskas%' AND LOWER(comment) NOT LIKE '%zabka%'`)
	db.Exec(`UPDATE transactions
		SET labels = TRIM(REPLACE(',' || labels || ',', ',iki,', ','), ',')
		WHERE (',' || labels || ',') LIKE '%,iki,%' AND LOWER(comment) NOT LIKE 'iki%'`)
}
