package database

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
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

	if err := db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.StockTrade{}, &domain.ExportLog{}, &domain.Asset{}, &domain.AuthSettings{}, &domain.WebauthnCredential{}, &domain.Budget{}, &domain.LabelRule{}, &domain.BudgetSettings{}, &domain.AISettings{}, &domain.AIChatMessage{}, &domain.AIActivity{}); err != nil {
		return nil, err
	}

	// Back-fill source = 'Revolut' for any stock trades that pre-date the source column
	if err := db.Exec("UPDATE stock_trades SET source = 'Revolut' WHERE source = '' OR source IS NULL").Error; err != nil {
		return nil, err
	}

	applyDataCleanups(db)
	applyCategoryMigrations(db)
	applyLabelCleanups(db)
	applyBalanceBackfills(db)

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
		// Canonical singular — "bars" was merged into "bar" in v1.10.0, and
		// re-seeding the old name would resurrect it on every boot.
		"bar":        {"alaus", "baras", "vyno"},
		"aliexpress": {"aliexpress", "alipay"},
		"insurance":  {"insurance", "draudim", "gjensidige", "compensa"},
		// Canonical name — the user merged "fees" into "bank fee" (v1.10.0).
		"bank fee":   {"banko mokestis", "plan fee", "account fee", "bank fee"},
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
	// Payroll-labeled rows are the exception: pension contributions deducted
	// from gross salary never touched any bank account.
	db.Exec(`UPDATE transactions SET debit_account = 'swed'
		WHERE type IN ('expense', 'investment') AND (debit_account IS NULL OR debit_account = '')
		AND (',' || labels || ',') NOT LIKE '%,payroll,%'`)
	db.Exec(`UPDATE transactions SET credit_account = 'swed'
		WHERE type = 'income' AND (credit_account IS NULL OR credit_account = '')
		AND (',' || labels || ',') NOT LIKE '%,payroll,%'`)
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

	// --- Transfers category (v1.5.0): own-money movements between accounts
	// (ATM cash withdrawals/deposits, Revolut top-ups) were parked under
	// investment/Finance; they are transfers, not investments, and now have
	// their own category. Expense-side Finance (loans, fees, taxes) is
	// untouched, and the rows keep their cash/revolut labels.
	db.Exec(`UPDATE transactions SET category = 'Transfers'
		WHERE type = 'investment' AND category = 'Finance'`)

	// --- INVL payout arrival (v1.7.0): the 2022 partial-withdrawal landing
	// on Swedbank ("INVL EXTREMO III … Dalinė išmoka…", net of tax) was
	// imported as income — it is the bank side of an own-money movement out
	// of the pension fund. Statement-imported comments carry the raw bank
	// string, so match on it; idempotent because the type flips.
	addLabel("artea", `UPPER(comment) LIKE '%INVL%'`)
	db.Exec(`UPDATE transactions SET type = 'investment', category = 'Transfers', debit_account = 'art'
		WHERE type = 'income' AND UPPER(comment) LIKE '%INVL%'
		AND (comment LIKE '%išmoka%' OR comment LIKE '%ismoka%')`)
}

// applyLabelCleanups (v1.10.0) repairs the label vocabulary: normalizes
// drifted lists, fixes typos observed in real data, applies the merges the
// user made through the Labels UI (so every instance converges to the same
// vocabulary), splits "work lunch"-style compounds, and untangles the
// over-broad bank-fee labeling. Renames go through the same repository code
// the /labels/rename endpoint uses, so transactions, label rules and budget
// label lists all stay consistent. Idempotent: every pass only matches rows
// still in the old state.
func applyLabelCleanups(db *gorm.DB) {
	repo := repository.NewBudgetRepository(db)

	// Normalize first — the renames below match exact tokens, so a drifted
	// list like "kids, aparment" (stray space) would otherwise escape them.
	var txs []domain.Transaction
	db.Where("labels != ''").Find(&txs)
	for i := range txs {
		if n := domain.NormalizeLabels(txs[i].Labels); n != txs[i].Labels {
			db.Model(&domain.Transaction{}).Where("id = ?", txs[i].ID).Update("labels", n)
		}
	}

	fixes := [][2]string{
		// typo → canonical, as found in the wild
		{"aparment", "apartment"},
		{"dyi", "diy"},
		{"lotery", "lottery"},
		{"shoose", "shoes"},
		{"ente", "entertainment"},
		{"ent", "entertainment"},
		{"hea", "health"},
		{"bars", "bar"},
		{"air condicionier", "air conditioner"},
		// merges the user confirmed in the Labels UI
		{"fees", "bank fee"},
		{"mobilepay", "vipps mobilepay"},
	}
	for _, f := range fixes {
		repo.RenameLabel(f[0], f[1]) //nolint:errcheck // best-effort, like the other passes
	}

	// Compound labels split into their parts: "lunch" then covers every
	// lunch and "work" marks the work context on its own.
	splitLabel(db, "work lunch", []string{"work", "lunch"})
	splitLabel(db, "work dinner", []string{"work", "dinner"})

	scrubBankFee(db)
}

// splitLabel replaces one label token with several: transactions and budget
// label lists get all the parts, and each rule assigning the label becomes
// one rule per part (collapsing into already-existing identical rules).
func splitLabel(db *gorm.DB, from string, parts []string) {
	to := strings.Join(parts, ",")
	var txs []domain.Transaction
	db.Where("(',' || labels || ',') LIKE ?", "%,"+from+",%").Find(&txs)
	for i := range txs {
		if next, changed := domain.RenameLabelToken(txs[i].Labels, from, to); changed {
			db.Model(&domain.Transaction{}).Where("id = ?", txs[i].ID).Update("labels", next)
		}
	}
	var budgets []domain.Budget
	db.Where("label != ''").Find(&budgets)
	for i := range budgets {
		if next, changed := domain.RenameLabelToken(budgets[i].Label, from, to); changed {
			db.Model(&domain.Budget{}).Where("id = ?", budgets[i].ID).Update("label", next)
		}
	}
	var rules []domain.LabelRule
	db.Where("LOWER(label) = ?", from).Find(&rules)
	for _, rule := range rules {
		for _, p := range parts {
			var dup int64
			db.Model(&domain.LabelRule{}).
				Where("LOWER(label) = ? AND category = ? AND comment_match = ?", p, rule.Category, rule.CommentMatch).
				Count(&dup)
			if dup == 0 {
				db.Create(&domain.LabelRule{Label: p, Category: rule.Category, CommentMatch: rule.CommentMatch})
			}
		}
		db.Delete(&domain.LabelRule{}, rule.ID)
	}
}

// scrubBankFee (v1.10.0) untangles the over-broad "seb"/"swedbank" comment
// rules: any comment mentioning the bank got labeled 'bank fee' — card
// refunds, e-invoice utility payments, even Robur fund purchases. Kill the
// two rules, move Robur/mini-investment rows where they belong (investment,
// Stocks & ETF, 'etf' label) and keep 'bank fee' only on Finance expenses.
func scrubBankFee(db *gorm.DB) {
	db.Exec(`DELETE FROM label_rules WHERE label = 'bank fee' AND comment_match IN ('seb', 'swedbank')`)

	robur := "LOWER(comment) LIKE '%robur%' OR LOWER(comment) LIKE '%mini invest%'"
	db.Exec(`UPDATE transactions SET type = 'investment', category = 'Stocks & ETF'
		WHERE (` + robur + `) AND type = 'expense'`)
	var roburTxs []domain.Transaction
	db.Where(robur).Where("(',' || labels || ',') NOT LIKE '%,etf,%'").Find(&roburTxs)
	for i := range roburTxs {
		roburTxs[i].AddLabel("etf")
		db.Model(&domain.Transaction{}).Where("id = ?", roburTxs[i].ID).Update("labels", roburTxs[i].Labels)
	}

	var mislabeled []domain.Transaction
	db.Where("(',' || labels || ',') LIKE '%,bank fee,%'").
		Where("NOT (type = 'expense' AND category = 'Finance')").Find(&mislabeled)
	for i := range mislabeled {
		if next, changed := domain.RemoveLabelToken(mislabeled[i].Labels, "bank fee"); changed {
			db.Model(&domain.Transaction{}).Where("id = ?", mislabeled[i].ID).Update("labels", next)
		}
	}
}

// applyBalanceBackfills reconstructs balance history statements can't see.
// Each pass anchors on its own column's first tracked value and only fills
// zeroes, so tracked values are never touched and reruns are no-ops.
func applyBalanceBackfills(db *gorm.DB) {
	backfillCashBuffer(db)
	backfillSebPension(db)
}

// backfillCashBuffer: the physical cash pocket was ~€750 around Sept 2020
// and grew steadily to the first tracked cash amount (€6,000 on the
// 2024-01-31 snapshot) — fill linearly in time.
func backfillCashBuffer(db *gorm.DB) {
	var first domain.Balance
	if err := db.Where("cash > 0").Order("date").First(&first).Error; err != nil {
		return // nothing to anchor the buffer to
	}
	start := time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
	if !first.Date.After(start) {
		return
	}
	const startCash = 750.0
	var rows []domain.Balance
	if err := db.Where("cash = 0 AND date >= ? AND date < ?", start, first.Date).Find(&rows).Error; err != nil {
		return
	}
	span := first.Date.Sub(start).Hours()
	for i := range rows {
		frac := rows[i].Date.Sub(start).Hours() / span
		cash := math.Round((startCash+(first.Cash-startCash)*frac)*100) / 100
		db.Model(&domain.Balance{}).Where("id = ?", rows[i].ID).Updates(map[string]interface{}{
			"cash":  cash,
			"total": math.Round((rows[i].Total+cash)*100) / 100,
		})
	}
}

// sebPenContributions is every Sodra transfer into the SEB 2nd-pillar fund
// (contract HB00558811, PENSIJA 5 → SWEDBANK PENSIJA 1989-1995), per the
// Sodra report generated 2026-08-01: main + participant + state parts
// summed per transfer date. Participation ran 2015-05 … 2019-06
// (contribution suspension approved from 2019-06-01; two corrections
// landed 2019-12-05). Sums to the report's stated totals to the cent:
// 1,957.55 + 2,492.92 + 676.46 = €5,126.93.
var sebPenContributions = []struct {
	date  string
	total float64
}{
	{"2015-05-12", 46.15}, {"2015-06-10", 46.66}, {"2015-07-08", 57.44}, {"2015-08-12", 65.72},
	{"2015-09-08", 53.85}, {"2015-10-13", 53.72}, {"2015-11-10", 46.15}, {"2015-12-08", 41.26},
	{"2016-01-12", 46.15}, {"2016-02-09", 46.15}, {"2016-03-08", 67.28}, {"2016-04-11", 66.54},
	{"2016-05-10", 138.12}, {"2016-06-10", 71.24}, {"2016-07-11", 70.86}, {"2016-08-09", 72.72},
	{"2016-09-09", 71.24}, {"2016-10-11", 70.48}, {"2016-11-09", 144.43}, {"2016-12-12", 105.90},
	{"2017-01-10", 121.24}, {"2017-02-10", 105.90}, {"2017-03-13", 107.55}, {"2017-04-11", 106.61},
	{"2017-05-09", 106.61}, {"2017-06-09", 106.61}, {"2017-07-11", 107.13}, {"2017-08-09", 107.83},
	{"2017-09-11", 105.43}, {"2017-10-10", 106.61}, {"2017-11-13", 106.61}, {"2017-12-11", 107.91},
	{"2018-01-10", 106.61}, {"2018-02-09", 106.61}, {"2018-03-09", 108.82}, {"2018-04-09", 107.76},
	{"2018-05-08", 196.98}, {"2018-06-07", 115.12}, {"2018-07-05", 115.12}, {"2018-08-07", 115.12},
	{"2018-09-06", 115.12}, {"2018-10-04", 116.80}, {"2018-11-08", 114.50}, {"2018-12-06", 115.12},
	{"2019-01-11", 126.00}, {"2019-02-08", 126.04}, {"2019-03-08", 126.36}, {"2019-04-05", 127.85},
	{"2019-05-09", 181.02}, {"2019-06-06", 161.69}, {"2019-07-05", 149.79}, {"2019-12-05", 16.40},
}

// sebPenTrackedFrom is the first hand-tracked seb_pen snapshot. Everything
// earlier is reconstruction and gets overwritten by this pass, so a better
// data source (like the Sodra report above, which replaced an
// estimate-based model) corrects previous runs on the next boot.
var sebPenTrackedFrom = time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

// backfillSebPension reconstructs the SEB 2nd-pillar history before its
// first hand-tracked value from the fund's actual money flows: during the
// contribution era the pot equals what Sodra had transferred so far (the
// PENSIJA 5 fund was conservative — returns were noise next to inflows);
// after the last transfer it only compounds, at the rate implied by the
// hand-tracked anchor.
func backfillSebPension(db *gorm.DB) {
	var anchor domain.Balance
	if err := db.Where("seb_pen > 0 AND date >= ?", sebPenTrackedFrom).
		Order("date").First(&anchor).Error; err != nil {
		return // nothing to anchor the coast era to
	}

	type step struct {
		date time.Time
		cum  float64
	}
	series := make([]step, 0, len(sebPenContributions))
	cum := 0.0
	for _, c := range sebPenContributions {
		d, err := time.Parse("2006-01-02", c.date)
		if err != nil {
			continue
		}
		cum = math.Round((cum+c.total)*100) / 100
		series = append(series, step{d, cum})
	}
	last := series[len(series)-1]

	yearsBetween := func(a, b time.Time) float64 { return b.Sub(a).Hours() / (24 * 365.25) }
	rate := 1.0
	if y := yearsBetween(last.date, anchor.Date); y > 0.25 && anchor.SebPen > last.cum {
		rate = math.Pow(anchor.SebPen/last.cum, 1/y)
	}
	value := func(t time.Time) float64 {
		if t.Before(last.date) {
			v := 0.0
			for _, s := range series {
				if s.date.After(t) {
					break
				}
				v = s.cum
			}
			return v
		}
		return last.cum * math.Pow(rate, yearsBetween(last.date, t))
	}

	var rows []domain.Balance
	if err := db.Where("date < ?", anchor.Date).Find(&rows).Error; err != nil {
		return
	}
	for i := range rows {
		v := math.Round(value(rows[i].Date)*100) / 100
		if v < 1 {
			v = 0
		}
		if math.Abs(v-rows[i].SebPen) < 0.005 {
			continue
		}
		db.Model(&domain.Balance{}).Where("id = ?", rows[i].ID).Updates(map[string]interface{}{
			"seb_pen": v,
			"total":   math.Round((rows[i].Total-rows[i].SebPen+v)*100) / 100,
		})
	}
}
