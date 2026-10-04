package backup_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ft/internal/backup"
	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

func seed(t *testing.T, d *sql.DB) {
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(23.45), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed", Tags: []string{"kids"}})
	Tx(t, d, ledger.Tx{Date: "2026-09-17", Amount: E(466.21), Category: "transfer.debt", AccountID: "seb", ToAccountID: "mortgage"})
	Bal(t, d, "swed", "2026-09-30", 1234.56)
	r := ledger.Rule{Pattern: "lidl", SetCategory: "food.groceries", AddTags: []string{"x"}, Enabled: true}
	ledger.SaveRule(d, &r)
	b := plan.Budget{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(400)}
	plan.Save(d, &b, "")
	d.Exec(`INSERT INTO settings(key,value) VALUES('ai','{"api_key":"sk-secret","model":"claude-opus-5-5","enabled":true}')`)
	d.Exec(`INSERT INTO settings(key,value) VALUES('auth','{"pin_hash":"h","enabled":true}')`)
	d.Exec(`INSERT INTO settings(key,value) VALUES('bank','{"private_key_pem":"PEM"}')`)
	d.Exec(`INSERT INTO settings(key,value) VALUES('ai_context','"brief"')`)
	d.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('iPhone',X'00FF10','2026-01-01')`)
	d.Exec(`INSERT INTO trades(date,action,ticker,shares,price,currency,created_at) VALUES('2026-01-01','buy','VWCE',1.5,120.25,'EUR','x')`)
}

// dump renders every table for comparison.
func dump(t *testing.T, d *sql.DB, skip ...string) map[string][]string {
	out := map[string][]string{}
	rows, _ := d.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	var tables []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()
	for _, tb := range tables {
		if strings.Contains(strings.Join(skip, ","), tb) {
			continue
		}
		r, _ := d.Query(`SELECT * FROM ` + tb + ` ORDER BY 1`)
		cols, _ := r.Columns()
		for r.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			r.Scan(p...)
			out[tb] = append(out[tb], fmt.Sprintf("%v", v))
		}
		r.Close()
	}
	return out
}

func TestRoundTripWithSecretsIsExact(t *testing.T) {
	src := DB(t)
	seed(t, src)
	f, err := backup.Export(src, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(f)
	dst, _ := db.OpenMemory()
	defer dst.Close()
	if _, err := backup.Restore(dst, raw); err != nil {
		t.Fatal(err)
	}
	a, b := dump(t, src, "auth_sessions"), dump(t, dst, "auth_sessions")
	if !reflect.DeepEqual(a, b) {
		for k := range a {
			if !reflect.DeepEqual(a[k], b[k]) {
				t.Errorf("table %s differs:\n%v\n%v", k, a[k], b[k])
			}
		}
	}
	// Export → restore → export is a fixed point.
	f2, _ := backup.Export(dst, true)
	f.ExportedAt, f2.ExportedAt = "", ""
	j1, _ := json.Marshal(f)
	j2, _ := json.Marshal(f2)
	if string(j1) != string(j2) {
		t.Error("second export differs from the first")
	}
}

func TestBackupWithoutSecrets(t *testing.T) {
	src := DB(t)
	seed(t, src)
	f, _ := backup.Export(src, false)
	raw, _ := json.Marshal(f)
	for _, secret := range []string{"sk-secret", "PEM", `"pin_hash"`} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("secret %q leaked into a plain backup", secret)
		}
	}
	if _, ok := f.Tables["webauthn_credentials"]; ok {
		t.Error("passkeys in a plain backup")
	}
	// Restoring it onto an instance keeps that instance's own secrets.
	dst := DB(t)
	dst.Exec(`INSERT INTO settings(key,value) VALUES('ai','{"api_key":"sk-mine","model":"old"}')`)
	dst.Exec(`INSERT INTO settings(key,value) VALUES('auth','{"pin_hash":"mine","enabled":true}')`)
	dst.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('Mac',X'01','x')`)
	counts, err := backup.Restore(dst, raw)
	if err != nil {
		t.Fatal(err)
	}
	if counts["transactions"] != 2 {
		t.Fatal(counts)
	}
	var ai, au string
	dst.QueryRow(`SELECT value FROM settings WHERE key='ai'`).Scan(&ai)
	dst.QueryRow(`SELECT value FROM settings WHERE key='auth'`).Scan(&au)
	if !strings.Contains(ai, "sk-mine") || !strings.Contains(ai, "claude-opus-5-5") {
		t.Error("restored AI settings must keep this instance's key and take the backup's model:", ai)
	}
	if !strings.Contains(au, "mine") {
		t.Error("PIN replaced by a plain restore", au)
	}
	var n int
	dst.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials`).Scan(&n)
	if n != 1 {
		t.Error("passkeys wiped", n)
	}
}

func TestRestoreRejects(t *testing.T) {
	d := DB(t)
	if _, err := backup.Restore(d, []byte(`{"transactions":[]}`)); err == nil || !strings.Contains(err.Error(), "v1") {
		t.Error("a v1 file must be pointed at the v1 importer", err)
	}
	if _, err := backup.Restore(d, []byte(`{"format":"finance-tracker-backup","schema":999}`)); err == nil {
		t.Error("newer schema accepted")
	}
	if _, err := backup.Restore(d, []byte(`not json`)); err == nil {
		t.Error("garbage accepted")
	}
}

// A backup missing a column a newer schema has still restores (defaults fill it).
func TestOlderBackupRestoresIntoNewerSchema(t *testing.T) {
	src := DB(t)
	seed(t, src)
	f, _ := backup.Export(src, false)
	tab := f.Tables["budgets"]
	idx := -1
	for i, c := range tab.Columns {
		if c == "period" {
			idx = i
		}
	}
	tab.Columns = append(tab.Columns[:idx], tab.Columns[idx+1:]...)
	for i := range tab.Rows {
		tab.Rows[i] = append(tab.Rows[i][:idx], tab.Rows[i][idx+1:]...)
	}
	f.Tables["budgets"] = tab
	raw, _ := json.Marshal(f)
	dst := DB(t)
	if _, err := backup.Restore(dst, raw); err != nil {
		t.Fatal(err)
	}
	var period string
	dst.QueryRow(`SELECT period FROM budgets`).Scan(&period)
	if period != "monthly" {
		t.Error(period)
	}
}
