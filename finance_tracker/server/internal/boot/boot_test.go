package boot_test

import (
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"ft/internal/boot"
	"ft/internal/testutil"
)

func v1File(t *testing.T, dir string, rows ...string) string {
	path := filepath.Join(dir, "finance.db")
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(testutil.V1Schema); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := d.Exec(r); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()
	return path
}

func sum(t *testing.T, path string) [32]byte {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestFirstStartConvertsV1WithoutTouchingIt(t *testing.T) {
	dir := t.TempDir()
	legacy := v1File(t, dir,
		`INSERT INTO transactions(id,date,type,amount,comment,category,labels,debit_account) VALUES(1,'2026-09-01 00:00:00+00:00','expense',23.4,'Lidl','Food','groceries','swed')`,
		`INSERT INTO balances(date,swed) VALUES('2026-09-30 00:00:00+00:00',1000)`)
	before := sum(t, legacy)
	path := filepath.Join(dir, "finance-v2.db")
	d, ver, err := boot.Open(path, legacy)
	if err != nil || ver == nil || !ver.OK {
		t.Fatal(err, ver)
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&n)
	d.Close()
	if n != 1 {
		t.Fatal(n)
	}
	if sum(t, legacy) != before {
		t.Fatal("the v1 database was modified")
	}
	// Second start: no reconversion, data intact.
	d, ver, err = boot.Open(path, legacy)
	if err != nil || ver != nil {
		t.Fatal("second start must not convert again", err)
	}
	d.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&n)
	d.Close()
	if n != 1 {
		t.Fatal(n)
	}
}

func TestFirstStartWithoutV1SeedsCategories(t *testing.T) {
	dir := t.TempDir()
	d, ver, err := boot.Open(filepath.Join(dir, "finance-v2.db"), filepath.Join(dir, "missing.db"))
	if err != nil || ver != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&n)
	if n < 50 {
		t.Fatal("categories not seeded", n)
	}
}

func TestBadV1RefusesToStartAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "finance.db")
	os.WriteFile(legacy, []byte("not a database"), 0o644)
	path := filepath.Join(dir, "finance-v2.db")
	if _, _, err := boot.Open(path, legacy); err == nil {
		t.Fatal("garbage v1 accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("half-made v2 database left behind — the next start would skip conversion")
	}
}
