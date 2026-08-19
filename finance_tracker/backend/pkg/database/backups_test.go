package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backupNames lists backup files with the given prefix, sorted by ReadDir.
func backupNames(t *testing.T, dir, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestPreMigrationBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "finance.db")
	backups := filepath.Join(dir, "backups")

	// Fresh install: no pre-existing file to protect, so no backup.
	db, err := NewSQLiteDB(path)
	require.NoError(t, err)
	assert.Empty(t, backupNames(t, backups, "pre-migrate-"))

	tx := domain.Transaction{Date: time.Now(), Type: domain.TransactionTypeExpense, Amount: 42, Category: "Food", Comment: "backup smoke row"}
	require.NoError(t, db.Create(&tx).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// Reopen: the database now has data, so the boot sequence must snapshot
	// it before running migrations.
	db2, err := NewSQLiteDB(path)
	require.NoError(t, err)
	pre := backupNames(t, backups, "pre-migrate-")
	require.Len(t, pre, 1, "reopening a non-empty DB must write a pre-migration backup")

	// The backup is a valid database holding the row (VACUUM INTO output).
	bak, err := NewSQLiteDB(filepath.Join(backups, pre[0]))
	require.NoError(t, err)
	var count int64
	require.NoError(t, bak.Model(&domain.Transaction{}).Where("comment = ?", "backup smoke row").Count(&count).Error)
	assert.EqualValues(t, 1, count)

	// Nightly backup: writes once per day, second call is a no-op.
	require.NoError(t, nightlyBackup(db2, path))
	require.NoError(t, nightlyBackup(db2, path))
	assert.Len(t, backupNames(t, backups, "nightly-"), 1)
}

// A burst of restarts must not each write a pre-migration backup: the prune
// budget would otherwise evict good pre-incident snapshots in favor of
// copies of a damaged state (exactly how a crash-loop could destroy the only
// recent good backups).
func TestPreMigrationBackupDedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "finance.db")
	db, err := NewSQLiteDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Create(&domain.Transaction{Date: time.Now(), Type: domain.TransactionTypeExpense, Amount: 1, Category: "Food", Comment: "x"}).Error)

	backups := filepath.Join(dir, "backups")
	// Three back-to-back pre-migration backups (a restart burst) collapse to
	// one, because the second and third land inside preMigrateMinInterval.
	for i := 0; i < 3; i++ {
		require.NoError(t, PreMigrationBackup(db, path))
	}
	assert.Len(t, backupNames(t, backups, "pre-migrate-"), 1,
		"a restart burst must collapse to a single pre-migration backup")
}

func TestBackupPrune(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"nightly-20260101.db", "nightly-20260102.db", "nightly-20260103.db"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}
	prune(dir, "nightly-", 2)
	names := backupNames(t, dir, "nightly-")
	assert.Equal(t, []string{"nightly-20260102.db", "nightly-20260103.db"}, names,
		"prune must drop the oldest files beyond the retention count")
}
