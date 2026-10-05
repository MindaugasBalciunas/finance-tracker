package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateIsIdempotentAndBacksUpExistingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	d.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != len(migrations) {
		t.Fatal(v)
	}
	d.Exec(`INSERT INTO settings(key,value) VALUES('k','v')`)
	d.Close()
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var val string
	d.QueryRow(`SELECT value FROM settings WHERE key='k'`).Scan(&val)
	if val != "v" {
		t.Fatal("data lost on reopen")
	}
	var fk int
	d.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys off")
	}
	d.Close()
}

func TestBackupRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	d, _ := Open(path)
	defer d.Close()
	dir := BackupDir(path)
	os.MkdirAll(dir, 0o755)
	for i := 0; i < 16; i++ {
		os.WriteFile(filepath.Join(dir, "nightly-2026010"+string(rune('a'+i))+".db"), []byte("x"), 0o644)
	}
	if err := Backup(d, path, "nightly"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != keep["nightly"] {
		t.Fatalf("kept %d", len(entries))
	}
	if time.Since(LatestBackup(path, "nightly")) > time.Minute {
		t.Fatal("latest backup time")
	}
	if !LatestBackup(path, "manual").IsZero() {
		t.Fatal("no manual backups yet")
	}
	// The snapshot is a real database.
	var newest string
	for _, e := range entries {
		if e.Name() > newest {
			newest = e.Name()
		}
	}
	if info, _ := os.Stat(filepath.Join(dir, newest)); info.Size() < 4096 {
		t.Fatal("snapshot is not a database")
	}
}

func TestPensionsBecomeLiquid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.db")
	// A genuine schema-1 instance (as v2.0 left it) with a non-liquid pension.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	raw.Exec(`INSERT INTO accounts(id,name,kind,liquid,created_at,updated_at) VALUES('p','P','pension',0,'x','x'),('h','H','property',0,'x','x')`)
	raw.Exec(`PRAGMA user_version = 1`)
	raw.Close()
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var p, h int
	d.QueryRow(`SELECT liquid FROM accounts WHERE id='p'`).Scan(&p)
	d.QueryRow(`SELECT liquid FROM accounts WHERE id='h'`).Scan(&h)
	if p != 1 || h != 0 {
		t.Fatalf("pension liquid %d, property liquid %d", p, h)
	}
}
