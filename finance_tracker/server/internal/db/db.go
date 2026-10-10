// Package db opens the SQLite database, applies the schema and keeps
// on-disk backups. Schema changes are numbered migrations tracked in
// PRAGMA user_version — there are no "repair on every boot" passes.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaV1 string

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
// (reconnect in two clicks after restoring a snapshot).
func scrub(path string) error {
	s, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Exec(`UPDATE settings SET value=json_remove(value,'$.access_token','$.refresh_token','$.expires_at') WHERE key='ibkr' AND json_valid(value)`)
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
