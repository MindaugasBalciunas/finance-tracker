package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

// Snapshots leave the IBKR sign-in out (reconnecting is two clicks); the
// live database keeps it, and everything else in the snapshot is untouched.
func TestSnapshotsLeaveIBKRTokenOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finance-v2.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Exec(`INSERT INTO settings(key,value) VALUES('ibkr','{"client_id":"cid","access_token":"AT","refresh_token":"RT","expires_at":"x","account":"ibkr"}'),('plan','{"x":1}')`)
	if err := Backup(d, path, "manual"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(BackupDir(path))
	if len(entries) != 1 {
		t.Fatalf("snapshots %v", entries)
	}
	snap, err := Open(filepath.Join(BackupDir(path), entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var ib, plan, live string
	snap.QueryRow(`SELECT value FROM settings WHERE key='ibkr'`).Scan(&ib)
	snap.QueryRow(`SELECT value FROM settings WHERE key='plan'`).Scan(&plan)
	d.QueryRow(`SELECT value FROM settings WHERE key='ibkr'`).Scan(&live)
	if strings.Contains(ib, "AT") || strings.Contains(ib, "RT") || !strings.Contains(ib, `"client_id":"cid"`) || plan != `{"x":1}` {
		t.Fatalf("snapshot ibkr %s plan %s", ib, plan)
	}
	if !strings.Contains(live, "RT") {
		t.Fatal("the live database lost the sign-in")
	}
}

// Migration 13 records that the owner's Artea contract predates 2025 — only
// on a database that has the Artea account, and never over an answer given.
func TestArteaPredates2025(t *testing.T) {
	setup := func(t *testing.T, extra string) *sql.DB {
		path := filepath.Join(t.TempDir(), "a.db")
		raw, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		for v := 0; v < 12; v++ {
			if _, err := raw.Exec(migrations[v]); err != nil {
				t.Fatal(err)
			}
		}
		raw.Exec(`INSERT INTO accounts(id,name,kind,liquid,created_at,updated_at) VALUES('artea','Artea','pension',1,'x','x')`)
		if extra != "" {
			raw.Exec(extra)
		}
		raw.Exec(`PRAGMA user_version = 12`)
		raw.Close()
		d, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		return d
	}
	var v string
	setup(t, "").QueryRow(`SELECT value FROM settings WHERE key='tax'`).Scan(&v)
	if v != `{"pillar3_before_2025":true}` {
		t.Errorf("recorded %q", v)
	}
	v = ""
	setup(t, `INSERT INTO settings(key,value) VALUES('tax','{"pillar3_before_2025":false}')`).QueryRow(`SELECT value FROM settings WHERE key='tax'`).Scan(&v)
	if v != `{"pillar3_before_2025":false}` {
		t.Errorf("overwrote the owner's answer: %q", v)
	}
	d, _ := OpenMemory()
	defer d.Close()
	if d.QueryRow(`SELECT value FROM settings WHERE key='tax'`).Scan(&v) == nil {
		t.Error("a fresh database without Artea got an answer")
	}
}

// Migration 14 names the tour operator on the three Navaturas payments that
// carried their description as the merchant, and touches nothing else.
func TestNavaturasMerchant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for v := 0; v < 13; v++ {
		if _, err := raw.Exec(migrations[v]); err != nil {
			t.Fatal(err)
		}
	}
	raw.Exec(`INSERT INTO categories(id,name,kind) VALUES('travel','Travel','expense')`)
	for i, m := range []string{"Final payment for Navaturas Egypt trip", "20% Prepaid for Navaturas Egipt trip", "Navaturas tours"} {
		if _, err := raw.Exec(`INSERT INTO transactions(id,date,kind,amount,category,merchant,note,tags,source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			i+1, "2026-09-21", "expense", 92000, "travel", m, "trip note", "", "manual", "x", "x"); err != nil {
			t.Fatal(err)
		}
	}
	raw.Exec(`PRAGMA user_version = 13`)
	raw.Close()
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var got []string
	rows, _ := d.Query(`SELECT merchant FROM transactions ORDER BY id`)
	for rows.Next() {
		var m string
		rows.Scan(&m)
		got = append(got, m)
	}
	rows.Close()
	if strings.Join(got, "|") != "Navaturas|Navaturas|Navaturas tours" {
		t.Errorf("merchants %q", got)
	}
}

// Migration 22 splits the old one-row mortgage payments into interest and a
// linked principal part (the month's drop in the loan balance): the
// payment's total and every balance stay as they were. A payment edited
// since (another amount) and rows on other accounts are left alone.
func TestMortgagePaymentsGetTheirPrincipalBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-07-20T18:55:58Z"
	for _, q := range []string{
		`INSERT OR IGNORE INTO accounts(id,name,kind,created_at,updated_at) VALUES('swed','Swedbank','checking','x','x'),('mortgage','Mortgage','loan','x','x'),('revolut','Revolut','checking','x','x')`,
		`INSERT OR IGNORE INTO categories(id,parent,name,kind) VALUES('housing',NULL,'Housing','expense'),('housing.mortgage','housing','Mortgage','expense'),('housing.mortgage_interest','housing','Mortgage interest','expense'),('transfer',NULL,'Transfer','transfer'),('transfer.debt','transfer','Debt','transfer')`,
		// Two payments in October 2023 share the month's drop by amount.
		`INSERT INTO transactions(id,date,kind,amount,account_id,category,merchant,note,tags,source,created_at,updated_at) VALUES
		 (53175,'2023-10-13','expense',165000,'swed','housing.mortgage','Evelina','Būsto paskola',',evelina,house,','import','` + now + `','` + now + `'),
		 (53184,'2023-10-16','expense',67000,'swed','housing.mortgage','Evelina','Būsto paskola',',evelina,house,','import','` + now + `','` + now + `'),
		 (52576,'2023-01-13','transfer',117000,'swed','transfer.debt','Evelina','Lizingas',',evelina,','import','` + now + `','` + now + `'),
		 (265,'2026-04-17','expense',130000,'swed','housing.mortgage','','House payment','','import','` + now + `','` + now + `'),
		 (175,'2026-03-16','expense',128788,'revolut','housing.mortgage','','House payment','','import','` + now + `','` + now + `')`,
		`PRAGMA user_version = 21`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	d.Close()
	if d, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	type row struct {
		kind, category, to string
		amount             int64
	}
	get := func(q string, args ...any) row {
		var r row
		var to sql.NullString
		if err := d.QueryRow(q, args...).Scan(&r.kind, &r.category, &to, &r.amount); err != nil {
			t.Fatal(q, args, err)
		}
		r.to = to.String
		return r
	}
	parent := `SELECT kind,category,to_account_id,amount FROM transactions WHERE id=?`
	part := `SELECT kind,category,to_account_id,amount FROM transactions WHERE split_of=?`
	for _, c := range []struct {
		id                  int64
		interest, principal int64
	}{{53175, 165000 - 78854, 78854}, {53184, 67000 - 32020, 32020}, {52576, 117000 - 33746, 33746}} {
		if p := get(parent, c.id); p != (row{"expense", "housing.mortgage_interest", "", c.interest}) {
			t.Errorf("%d interest %+v", c.id, p)
		}
		if p := get(part, c.id); p != (row{"transfer", "transfer.debt", "mortgage", c.principal}) {
			t.Errorf("%d principal %+v", c.id, p)
		}
	}
	// A part is imported (it moves no balance) and keeps its payment's
	// created_at, so Home doesn't list 44 old payments as new.
	var src, created, acct string
	d.QueryRow(`SELECT source, created_at, account_id FROM transactions WHERE split_of=53175`).Scan(&src, &created, &acct)
	if src != "import" || created != now || acct != "swed" {
		t.Errorf("part source %q created %q account %q", src, created, acct)
	}
	// 16 Mar 2026's payment was from Swedbank; this one names another account.
	if p := get(parent, 175); p != (row{"expense", "housing.mortgage", "", 128788}) {
		t.Errorf("other account changed: %+v", p)
	}
	// 17 Apr 2026's payment is €1,287.88 in the data; this one was edited.
	if p := get(parent, 265); p != (row{"expense", "housing.mortgage", "", 130000}) {
		t.Errorf("edited payment changed: %+v", p)
	}
	var parts int
	d.QueryRow(`SELECT COUNT(*) FROM transactions WHERE split_of=265`).Scan(&parts)
	if parts != 0 {
		t.Errorf("edited payment got %d parts", parts)
	}
}

// Without a mortgage account there is nowhere to put the principal: the
// payments are left exactly as they were.
func TestMortgageFixLeavesABookWithoutTheLoanAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT OR IGNORE INTO accounts(id,name,kind,created_at,updated_at) VALUES('swed','Swedbank','checking','x','x')`,
		`INSERT OR IGNORE INTO categories(id,parent,name,kind) VALUES('housing',NULL,'Housing','expense'),('housing.mortgage','housing','Mortgage','expense'),('housing.mortgage_interest','housing','Mortgage interest','expense'),('transfer',NULL,'Transfer','transfer'),('transfer.debt','transfer','Debt','transfer')`,
		`INSERT INTO transactions(id,date,kind,amount,account_id,category,source,created_at,updated_at) VALUES(265,'2026-04-17','expense',128788,'swed','housing.mortgage','import','x','x')`,
		`PRAGMA user_version = 21`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	d.Close()
	if d, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n, amount int
	var cat string
	d.QueryRow(`SELECT COUNT(*) FROM transactions WHERE split_of=265`).Scan(&n)
	d.QueryRow(`SELECT amount, category FROM transactions WHERE id=265`).Scan(&amount, &cat)
	if n != 0 || amount != 128788 || cat != "housing.mortgage" {
		t.Fatalf("parts %d, amount %d, category %s", n, amount, cat)
	}
}
