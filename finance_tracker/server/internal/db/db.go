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
	prune(dir, prefix, keep[prefix])
	return nil
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
