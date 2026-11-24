// Package db opens the SQLite database, applies the schema and keeps
// on-disk backups. Schema changes are numbered migrations tracked in
// PRAGMA user_version — there are no "repair on every boot" passes.
package db

import (
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite"

	"ft/internal/merchant"
)

// merchant_key(m) matches a place across spellings in SQL, the way the
// insights group it (the Ledger's merchant filter uses it).
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("merchant_key", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		s, _ := args[0].(string)
		return merchant.Key(s), nil
	})
}

//go:embed schema.sql
var schemaV1 string

// MortgagePrincipalFix (migration 22) puts the mortgage principal back to
// the loan's start. From Sep 2022 to Apr 2026 each mortgage payment (mostly
// to Evelina before the divorce) was one "housing.mortgage" expense, so its
// principal counted as spending, not as wealth built: invested and principal
// ran about €330–590 a month low. Each payment now keeps the interest and
// gets a linked split part for the principal: a transfer.debt from Swedbank
// to the mortgage, that month's drop in the loan balance on the 17th as
// production had it on 2026-11-24. A redraw of the computed history follows
// these parts; May 2024, with no payment to split, takes the rest. Two
// payments in Oct 2023 share that month's drop by amount. 13 Jan 2023's
// €1,170 "Lizingas" is that month's mortgage payment (owner, 2026-11-24).
// Imported split parts move no balance, so net worth is unchanged. Unsplit
// folds a principal back into its payment, which stays filed as interest.
// Exact ids, dates, amounts and accounts only: a row edited since, or a
// database without them, is left alone. Restoring a JSON backup from before
// schema 22 runs it again (backup.Restore).
const MortgagePrincipalFix = `CREATE TEMP TABLE mortgage_fix(id INTEGER PRIMARY KEY, date TEXT, amount INTEGER, principal INTEGER);
INSERT INTO mortgage_fix VALUES
  (52336,'2022-09-19',113200,15350),
  (52393,'2022-10-14',117000,18410),
  (52455,'2022-11-17',113000,15303),
  (52514,'2022-12-14',113000,15354),
  (52576,'2023-01-13',117000,33746),
  (52635,'2023-02-15',113200,15674),
  (52692,'2023-03-11',151200,45812),
  (52757,'2023-04-14',150000,45013),
  (52807,'2023-05-12',150000,45161),
  (52885,'2023-06-14',150000,45310),
  (52947,'2023-07-10',150000,45459),
  (53043,'2023-08-11',150000,45609),
  (53113,'2023-09-14',165000,57635),
  (53175,'2023-10-13',165000,78854),
  (53184,'2023-10-16',67000,32020),
  (53257,'2023-11-14',165000,58190),
  (53331,'2023-12-15',165000,58381),
  (50032,'2024-01-15',165000,58574),
  (50087,'2024-02-14',165000,58766),
  (50143,'2024-03-11',165000,58960),
  (50203,'2024-04-10',165000,59154),
  (50324,'2024-06-09',165000,59469),
  (50378,'2024-07-08',163000,58081),
  (50450,'2024-08-13',163000,58272),
  (50483,'2024-09-08',156000,52922),
  (50543,'2024-10-08',156000,53096),
  (50630,'2024-11-08',156000,53271),
  (50697,'2024-12-06',156000,53446),
  (50794,'2025-01-08',156000,53622),
  (50871,'2025-02-07',156000,53799),
  (50935,'2025-03-07',134400,36873),
  (50988,'2025-04-08',134462,37044),
  (51063,'2025-05-08',134462,37166),
  (51114,'2025-06-06',134462,37288),
  (51183,'2025-07-07',134462,37411),
  (51287,'2025-08-08',134400,37485),
  (51351,'2025-09-05',128400,32857),
  (51404,'2025-10-06',128400,32966),
  (51475,'2025-11-07',128400,33074),
  (51581,'2025-12-08',128400,33184),
  (42,'2026-01-15',128400,33292),
  (108,'2026-02-15',128400,33402),
  (175,'2026-03-16',128788,33819),
  (265,'2026-04-17',128788,33931);
INSERT INTO transactions(date,kind,amount,account_id,to_account_id,category,merchant,note,tags,split_of,source,created_at,updated_at)
SELECT t.date,'transfer',f.principal,t.account_id,'mortgage','transfer.debt',t.merchant,'Mortgage principal',t.tags,t.id,'import',t.created_at,strftime('%Y-%m-%dT%H:%M:%SZ','now')
FROM transactions t JOIN mortgage_fix f ON f.id=t.id AND f.date=t.date AND f.amount=t.amount
WHERE t.account_id='swed' AND t.split_of IS NULL AND t.category IN ('housing.mortgage','transfer.debt')
  AND EXISTS (SELECT 1 FROM accounts WHERE id='mortgage')
  AND EXISTS (SELECT 1 FROM categories WHERE id='housing.mortgage_interest')
  AND EXISTS (SELECT 1 FROM categories WHERE id='transfer.debt')
  AND NOT EXISTS (SELECT 1 FROM transactions p WHERE p.split_of=t.id);
UPDATE transactions SET kind='expense', to_account_id=NULL, category='housing.mortgage_interest',
  amount=amount-(SELECT f.principal FROM mortgage_fix f WHERE f.id=transactions.id),
  updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
WHERE id IN (SELECT f.id FROM mortgage_fix f JOIN transactions t ON t.id=f.id AND t.amount=f.amount)
  AND EXISTS (SELECT 1 FROM transactions p WHERE p.split_of=transactions.id AND p.category='transfer.debt' AND p.note='Mortgage principal');
DROP TABLE mortgage_fix;`

// migrations[i] takes the schema from version i to i+1. Append only.
var migrations = []string{
	schemaV1,
	// 2: II and III pillar pensions can be cashed out, so they count as liquid.
	`UPDATE accounts SET liquid = 1 WHERE kind = 'pension';`,
	// 3: owner edits to recurring costs — added by hand, adjusted, or hidden.
	`CREATE TABLE IF NOT EXISTS recurring_items (
    id         INTEGER PRIMARY KEY,
    merchant   TEXT NOT NULL,
    category   TEXT NOT NULL DEFAULT '',
    cadence    TEXT NOT NULL DEFAULT 'monthly' CHECK (cadence IN ('monthly','quarterly','yearly')),
    amount     INTEGER NOT NULL DEFAULT 0,      -- cents per charge; 0 = keep the detected amount
    next_date  TEXT NOT NULL DEFAULT '',        -- '' = keep the detected next date
    note       TEXT NOT NULL DEFAULT '',
    hidden     INTEGER NOT NULL DEFAULT 0,      -- 1 = not recurring, stop showing it
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS recurring_items_merchant ON recurring_items(lower(merchant));`,
	// 4: card reservations can enter the ledger before they book.
	`ALTER TABLE transactions ADD COLUMN pending INTEGER NOT NULL DEFAULT 0;`,
	// 5: private, on-device usage analytics (which pages, how you get there).
	`CREATE TABLE IF NOT EXISTS usage_events (
    id       INTEGER PRIMARY KEY,
    at       TEXT NOT NULL,             -- when the page was entered / the click happened (UTC)
    kind     TEXT NOT NULL CHECK (kind IN ('view','action')),
    path     TEXT NOT NULL,             -- route, no query string
    from_path TEXT NOT NULL DEFAULT '',
    label    TEXT NOT NULL DEFAULT '',  -- clicked control (digits masked)
    dwell_ms INTEGER NOT NULL DEFAULT 0, -- time on the page (views)
    x        REAL,                      -- click position as a share of page width (heat maps)
    y        REAL,                      -- … and of page height
    vw       INTEGER NOT NULL DEFAULT 0 -- viewport width in px (phone vs desktop)
);
CREATE INDEX IF NOT EXISTS usage_events_at ON usage_events(at);`,
	// 6: recurring costs on a flexible rhythm ("about every 5 weeks").
	`ALTER TABLE recurring_items ADD COLUMN every_days INTEGER NOT NULL DEFAULT 0;`,
	// 7: recurring money movements, not just bills — standing orders between
	// accounts and expected income — with the accounts involved and the usual
	// day, so cash can be planned per account. Existing items stay bills.
	`ALTER TABLE recurring_items ADD COLUMN kind TEXT NOT NULL DEFAULT 'bill' CHECK (kind IN ('bill','transfer','income'));
ALTER TABLE recurring_items ADD COLUMN from_account TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_items ADD COLUMN to_account TEXT NOT NULL DEFAULT '';
ALTER TABLE recurring_items ADD COLUMN day INTEGER NOT NULL DEFAULT 0;`,
	// 8: trades remember the broker's own trade id (IBKR sync), so a trade is
	// never imported twice. Existing trades keep '' (entered by hand).
	`ALTER TABLE trades ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS trades_external_id ON trades(external_id) WHERE external_id <> '';`,
	// 9: every reading of today's balance with its time — a bank sync at
	// 08:10 and another at 17:40 both stay visible. balances keeps one value
	// per day (what net worth uses); this is the intraday trail beside it.
	`CREATE TABLE IF NOT EXISTS balance_log (
    id         INTEGER PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date       TEXT NOT NULL,              -- local day the value is for
    at         TEXT NOT NULL,              -- when it was read (UTC RFC3339)
    value      INTEGER NOT NULL,
    quantity   REAL,
    price      INTEGER,
    source     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS balance_log_date ON balance_log(date, account_id);`,
	// 10: an answer keeps the euro amounts it gave that matched none of the
	// data it read, so the warning survives answers finished in the background.
	`ALTER TABLE ai_messages ADD COLUMN unchecked TEXT NOT NULL DEFAULT '';`,
	// 11: two recurring items can share a merchant (Telia phone and Telia
	// internet) — the note tells them apart.
	`DROP INDEX IF EXISTS recurring_items_merchant;
CREATE UNIQUE INDEX IF NOT EXISTS recurring_items_merchant_note ON recurring_items(lower(merchant), lower(note));`,
	// 12: the wish list — things saved up for (a 3D printer, a sauna, a
	// trip) in priority order, funded from what is left at month end after
	// investing and obligations. goal_moves is the money put in or taken out.
	`CREATE TABLE IF NOT EXISTS goals (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    target      INTEGER NOT NULL CHECK (target >= 0),   -- cents
    priority    INTEGER NOT NULL DEFAULT 0,             -- 1 = funded first
    target_date TEXT NOT NULL DEFAULT '',               -- optional YYYY-MM-DD
    note        TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    tag         TEXT NOT NULL DEFAULT '',               -- trip:<name> makes it a planned trip; its tagged spending counts against it
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','bought','dropped')),
    done_on     TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS goal_moves (
    id         INTEGER PRIMARY KEY,
    goal_id    INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    month      TEXT NOT NULL,                  -- YYYY-MM the money is for (funding) or was moved
    amount     INTEGER NOT NULL,               -- cents; negative takes money out
    kind       TEXT NOT NULL DEFAULT 'manual' CHECK (kind IN ('funding','manual')),
    note       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS goal_moves_goal ON goal_moves(goal_id);`,
	// 13: the owner's 3rd-pillar (Artea) contract was signed before 2025, so
	// its contributions still earn the 20% income-tax refund. Recorded only
	// where no answer was given yet (a fresh database asks in the app).
	`INSERT OR IGNORE INTO settings(key,value)
SELECT 'tax', '{"pillar3_before_2025":true}' WHERE EXISTS (SELECT 1 FROM accounts WHERE id='artea');`,
	// 14: three Navaturas trip payments had their description typed into the
	// merchant ("Final payment for Navaturas Egypt trip"); the note already
	// carries it, so the merchant becomes the tour operator. Exact strings
	// only — a row edited since stays as it is.
	`UPDATE transactions SET merchant='Navaturas' WHERE merchant IN
('20% Prepaid for Navaturas Egipt trip','20% Prepaid for Navaturas BGY/Madonna da','Final payment for Navaturas Egypt trip');`,
	// 15: a trip's real dates, so expenses inside them get its trip:… tag
	// automatically (until now a trip's period was guessed from its rows).
	`CREATE TABLE IF NOT EXISTS trip_dates (
    tag        TEXT PRIMARY KEY,      -- trip:italy-spring-2026
    start_date TEXT NOT NULL,         -- first day, YYYY-MM-DD
    end_date   TEXT NOT NULL,         -- last day
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);`,
	// 16: AI apps connected over OAuth (Claude's custom connectors and other
	// MCP clients): the registered apps and each approved connection, with
	// only hashes of its tokens.
	`CREATE TABLE IF NOT EXISTS oauth_clients (
    id            TEXT PRIMARY KEY,      -- ftc_…
    name          TEXT NOT NULL,         -- what the app calls itself, e.g. Claude
    redirect_uris TEXT NOT NULL,         -- JSON array
    created_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_grants (
    id                INTEGER PRIMARY KEY,
    client_id         TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    scope             TEXT NOT NULL,                 -- ro | rw
    access_hash       TEXT NOT NULL,
    access_expires    TEXT NOT NULL,
    refresh_hash      TEXT NOT NULL,
    refresh_expires   TEXT NOT NULL,
    prev_refresh_hash TEXT NOT NULL DEFAULT '',      -- replaying it ends the connection
    created_at        TEXT NOT NULL,
    last_used_at      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_oauth_access ON oauth_grants(access_hash);
CREATE INDEX IF NOT EXISTS idx_oauth_refresh ON oauth_grants(refresh_hash);`,
	// 17: wish-list money set aside in a savings account (a goal held there,
	// so the account's balance shows what is spoken for), and trips moved to
	// history once they are over (no more mapping prompts for them).
	// Existing moves aren't tied to an account; every trip stays as it was.
	`ALTER TABLE goal_moves ADD COLUMN account_id TEXT NOT NULL DEFAULT '';  -- '' = not tied to an account
CREATE TABLE IF NOT EXISTS trip_history (
    tag        TEXT PRIMARY KEY,      -- trip:italy-spring-2026
    state      TEXT NOT NULL CHECK (state IN ('done','active')),  -- done = in history; active = kept on the list
    updated_at TEXT NOT NULL
);`,
	// Decision journal: a purchase considered before it is made, with a
	// cooling-off date, what was decided, and whether it was worth it when
	// looked back on. New table; nothing existing changes.
	`CREATE TABLE IF NOT EXISTS decisions (
    id         INTEGER PRIMARY KEY,
    title      TEXT NOT NULL,
    amount     INTEGER NOT NULL,                  -- cents
    category   TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'thinking' CHECK (status IN ('thinking','saving','bought','skipped')),  -- saving: on the wish list
    cool_until TEXT NOT NULL DEFAULT '',          -- YYYY-MM-DD: decide on or after this day
    decided_on TEXT NOT NULL DEFAULT '',
    review_on  TEXT NOT NULL DEFAULT '',          -- YYYY-MM-DD: ask whether it was worth it
    worth      TEXT NOT NULL DEFAULT '' CHECK (worth IN ('','yes','meh','no')),
    note       TEXT NOT NULL DEFAULT '',
    goal_id    INTEGER NOT NULL DEFAULT 0,        -- the wish-list goal saving for it (0 = none)
    created_at TEXT NOT NULL
);`,
	// 19: a transaction marked as a recurring payment, so Plan → Recurring
	// knows it was paid that month even when the bank names the merchant
	// differently. 0 = not linked (every existing row); no foreign key, so
	// older backups restore as they are — deleting an item clears its links.
	`ALTER TABLE transactions ADD COLUMN recurring_id INTEGER NOT NULL DEFAULT 0;  -- recurring_items.id, 0 = none`,
	// 20: Insights → Optional: the owner's verdict on a regular optional
	// place or category — worth it, cut, or not sure — and what it cost a
	// month when judged, so a "cut" can be followed up. New table; nothing
	// existing changes.
	`CREATE TABLE IF NOT EXISTS optional_verdicts (
    kind       TEXT NOT NULL CHECK (kind IN ('merchant','category')),
    key        TEXT NOT NULL,                     -- merchant key (merchant.Key) or category id
    name       TEXT NOT NULL DEFAULT '',          -- how it was shown when judged
    verdict    TEXT NOT NULL CHECK (verdict IN ('worth','cut','unsure')),
    usual      INTEGER NOT NULL DEFAULT 0,        -- cents: a month on average when judged
    judged_on  TEXT NOT NULL,                     -- YYYY-MM-DD
    updated_at TEXT NOT NULL,
    PRIMARY KEY (kind, key)
);`,
	// 21: the owner's own call on one payment, set from the Ledger row: a
	// must or optional. '' = not marked (every existing row), and the
	// payment then follows its category's essential flag as before.
	`ALTER TABLE transactions ADD COLUMN need TEXT NOT NULL DEFAULT '' CHECK (need IN ('','must','optional'));`,
	// 22: mortgage principal back to the loan's start — see MortgagePrincipalFix.
	MortgagePrincipalFix,
}

// Open opens (creating if needed) the database at path and brings the schema
// up to date. A database that already has data is snapshotted before any
// migration runs.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: a single-user app never notices the serialisation, and
	// SQLITE_BUSY can never happen.
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	if err := migrate(d, path); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// OpenMemory is for tests.
func OpenMemory() (*sql.DB, error) {
	d, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := migrate(d, ""); err != nil {
		return nil, err
	}
	return d, nil
}

func migrate(d *sql.DB, path string) error {
	var version int
	if err := d.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= len(migrations) {
		return nil
	}
	if version > 0 && path != "" {
		if err := Backup(d, path, "pre-migrate"); err != nil {
			return fmt.Errorf("pre-migration backup failed, refusing to migrate: %w", err)
		}
	}
	for v := version; v < len(migrations); v++ {
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup retention per prefix.
var keep = map[string]int{"nightly": 14, "pre-migrate": 10, "pre-restore": 5, "manual": 10}

// BackupDir is the folder next to the database that holds snapshots.
func BackupDir(dbPath string) string { return filepath.Join(filepath.Dir(dbPath), "backups") }

// Backup writes a consistent point-in-time copy with VACUUM INTO and prunes
// older copies of the same kind.
func Backup(d *sql.DB, dbPath, prefix string) error {
	dir := BackupDir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, fmt.Sprintf("%s-%s.db", prefix, time.Now().UTC().Format("20060102-150405")))
	_ = os.Remove(target)
	if _, err := d.Exec("VACUUM INTO ?", target); err != nil {
		return err
	}
	if err := scrub(target); err != nil {
		os.Remove(target) // never leave a copy with the secret in it
		return err
	}
	prune(dir, prefix, keep[prefix])
	return nil
}

// scrub takes credentials that are easy to grant again out of a snapshot,
// so dozens of copies on the device don't each carry them: the IBKR sign-in
// and connected AI apps (reconnect in two clicks after restoring a snapshot).
func scrub(path string) error {
	s, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer s.Close()
	if _, err = s.Exec(`UPDATE settings SET value=json_remove(value,'$.access_token','$.refresh_token','$.expires_at') WHERE key='ibkr' AND json_valid(value)`); err != nil {
		return err
	}
	// Connected AI apps reconnect in two clicks too. Snapshots taken before
	// migration 16 have no such table, so its absence is not an error.
	var n int
	if s.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='oauth_grants'`).Scan(&n); n > 0 {
		_, err = s.Exec(`DELETE FROM oauth_grants`)
	}
	return err
}

func prune(dir, prefix string, n int) {
	if n <= 0 {
		n = 10
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix+"-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > n {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// LatestBackup reports when the newest snapshot of a kind was written.
func LatestBackup(dbPath, prefix string) time.Time {
	entries, _ := os.ReadDir(BackupDir(dbPath))
	var newest time.Time
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix+"-") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// Now is the timestamp format used everywhere.
func Now() string { return time.Now().UTC().Format(time.RFC3339) }
