package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	nightlyKeep    = 14
	preMigrateKeep = 10
	backupInterval = 24 * time.Hour
	// preMigrateMinInterval collapses a burst of restarts into a single
	// pre-migration snapshot. Without it, a crash-loop (or a rapid rebuild
	// cycle) writes one backup per restart, and the keep-N prune then evicts
	// the good pre-incident snapshots in favor of copies of the damaged
	// state. Nightly backups remain the durable safety net.
	preMigrateMinInterval = 30 * time.Minute
)

// backupDir returns the directory backups live in (a "backups" folder next
// to the database file), creating it if needed.
func backupDir(dbPath string) (string, error) {
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// vacuumInto writes a consistent point-in-time copy of the live database.
// VACUUM INTO fails if the target exists, so a stale same-name file is
// removed first. The path is server-generated; quotes escaped regardless.
func vacuumInto(db *gorm.DB, target string) error {
	_ = os.Remove(target)
	quoted := strings.ReplaceAll(target, "'", "''")
	return db.Exec(fmt.Sprintf("VACUUM INTO '%s'", quoted)).Error
}

// PreMigrationBackup snapshots the database before the boot migration
// sequence mutates it — startup runs ALTER/UPDATE/DELETE passes on the only
// copy of the data. The caller decides whether the database pre-existed
// (the check must happen before gorm.Open touches the file).
func PreMigrationBackup(db *gorm.DB, dbPath string) error {
	dir, err := backupDir(dbPath)
	if err != nil {
		return err
	}
	// Skip if a pre-migration snapshot was taken very recently: a restart
	// burst must not spend the whole retention budget on copies of the same
	// (possibly already-damaged) state and evict older, good backups.
	if newest := newestBackupTime(dir, "pre-migrate-"); !newest.IsZero() &&
		time.Since(newest) < preMigrateMinInterval {
		return nil
	}
	target := filepath.Join(dir, "pre-migrate-"+time.Now().Format("20060102-150405")+".db")
	if err := vacuumInto(db, target); err != nil {
		return err
	}
	prune(dir, "pre-migrate-", preMigrateKeep)
	return nil
}

// newestBackupTime returns the mtime of the most recent backup with prefix,
// or the zero time when none exist.
func newestBackupTime(dir, prefix string) time.Time {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}

// StartBackupLoop writes a nightly VACUUM INTO snapshot with retention.
// Today's snapshot is written immediately if it doesn't exist yet.
func StartBackupLoop(db *gorm.DB, dbPath string) {
	go func() {
		for {
			if err := nightlyBackup(db, dbPath); err != nil {
				log.Printf("nightly backup failed: %v", err)
			}
			time.Sleep(backupInterval)
		}
	}()
}

func nightlyBackup(db *gorm.DB, dbPath string) error {
	dir, err := backupDir(dbPath)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "nightly-"+time.Now().Format("20060102")+".db")
	if _, err := os.Stat(target); err == nil {
		return nil // today's snapshot already exists
	}
	if err := vacuumInto(db, target); err != nil {
		return err
	}
	prune(dir, "nightly-", nightlyKeep)
	log.Printf("nightly backup written: %s", target)
	return nil
}

// prune keeps the newest `keep` files matching prefix and deletes the rest.
// Names embed timestamps, so lexicographic order is chronological.
func prune(dir, prefix string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}
