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
