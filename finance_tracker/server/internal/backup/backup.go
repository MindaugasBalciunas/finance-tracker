// Package backup writes and restores the whole database as one JSON file.
//
// The format is a table dump (columns + rows), so a restore reproduces the
// database exactly — there is no per-entity mapping to drift out of sync
// when the schema grows. Secrets (AI key, bank private key, PIN hash,
// passkeys, bank sessions) are left out unless explicitly requested; a
// restore without them keeps the secrets already on this instance.
package backup

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Format = "finance-tracker-backup"

// Tables in dependency order (parents first).
var tables = []string{"settings", "accounts", "categories", "balances", "transactions", "rules", "budgets", "budget_amounts", "trades", "recurring_items",
	"bank_connections", "bank_accounts", "bank_inbox", "webauthn_credentials", "ai_messages", "ai_spend", "ai_topups"}

// Secret-only tables and settings keys.
var secretTables = map[string]bool{"webauthn_credentials": true, "bank_connections": true, "bank_accounts": true, "bank_inbox": true}
var secretSettings = map[string]bool{"auth": true, "bank": true, "ibkr": true} // ibkr: OAuth tokens

type Table struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

type File struct {
	Format         string           `json:"format"`
	Schema         int              `json:"schema"`
	ExportedAt     string           `json:"exported_at"`
	IncludeSecrets bool             `json:"include_secrets"`
	Counts         map[string]int   `json:"counts"`
	Tables         map[string]Table `json:"tables"`
}

func encodeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return map[string]string{"$b64": base64.StdEncoding.EncodeToString(b)}
	}
	return v
}

func decodeValue(v any) any {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m["$b64"].(string); ok {
			b, _ := base64.StdEncoding.DecodeString(s)
			return b
		}
	}
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	return v
}

// Export dumps every table.
func Export(d *sql.DB, secrets bool) (*File, error) {
	f := &File{Format: Format, ExportedAt: time.Now().UTC().Format(time.RFC3339), IncludeSecrets: secrets, Counts: map[string]int{}, Tables: map[string]Table{}}
	d.QueryRow(`PRAGMA user_version`).Scan(&f.Schema)
	for _, t := range tables {
		if secretTables[t] && !secrets {
			continue
		}
		rows, err := d.Query(`SELECT * FROM ` + t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		cols, _ := rows.Columns()
		tab := Table{Columns: cols, Rows: [][]any{}}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			if t == "settings" {
				key, _ := vals[0].(string)
				if secretSettings[key] && !secrets {
					continue
				}
				if key == "ai" && !secrets {
					vals[1] = stripKey(vals[1])
				}
			}
			for i := range vals {
				vals[i] = encodeValue(vals[i])
			}
			tab.Rows = append(tab.Rows, vals)
		}
		rows.Close()
		f.Tables[t] = tab
		f.Counts[t] = len(tab.Rows)
	}
	return f, nil
}

func stripKey(v any) any {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		return v
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		return v
	}
	delete(m, "api_key")
	b, _ := json.Marshal(m)
	return string(b)
}

// Restore replaces the database contents with the file's. Tables absent
// from the file (secrets in a secret-less backup) are kept as they are.
func Restore(d *sql.DB, raw []byte) (map[string]int, error) {
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("not a backup file: %w", err)
	}
	if f.Format != Format {
		return nil, errors.New("not a Finance Tracker v2 backup — use “Import v1 backup” for files from the old app")
	}
	var schema int
	d.QueryRow(`PRAGMA user_version`).Scan(&schema)
	if f.Schema > schema {
		return nil, fmt.Errorf("this backup is from a newer version (schema %d, this app has %d) — update the app first", f.Schema, schema)
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return nil, err
	}
	// Preserve this instance's secrets when the file has none.
	keepSettings := map[string]string{}
	if !f.IncludeSecrets {
		rows, _ := tx.Query(`SELECT key, value FROM settings`)
		for rows.Next() {
			var k, v string
			rows.Scan(&k, &v)
			if secretSettings[k] || k == "ai" {
				keepSettings[k] = v
			}
		}
		rows.Close()
	}
	counts := map[string]int{}
	for i := len(tables) - 1; i >= 0; i-- {
		t := tables[i]
		if _, ok := f.Tables[t]; !ok {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			return nil, fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	for _, t := range tables {
		tab, ok := f.Tables[t]
		if !ok {
			continue
		}
		cols := existingColumns(tx, t, tab.Columns)
		if len(cols) == 0 {
			continue
		}
		idx := map[string]int{}
		for i, c := range tab.Columns {
			idx[c] = i
		}
		q := fmt.Sprintf(`INSERT INTO %s(%s) VALUES(%s)`, t, `"`+strings.Join(cols, `","`)+`"`, strings.TrimSuffix(strings.Repeat("?,", len(cols)), ","))
		stmt, err := tx.Prepare(q)
		if err != nil {
			return nil, err
		}
		for _, row := range tab.Rows {
			args := make([]any, len(cols))
			for i, c := range cols {
				args[i] = decodeValue(row[idx[c]])
			}
			if _, err := stmt.Exec(args...); err != nil {
				stmt.Close()
				return nil, fmt.Errorf("restoring %s: %w", t, err)
			}
		}
		stmt.Close()
		counts[t] = len(tab.Rows)
	}
	for k, v := range keepSettings {
		if k == "ai" {
			// Restored AI settings carry no key; put this instance's back.
			var cur, restored map[string]any
			json.Unmarshal([]byte(v), &cur)
			var rv string
			if tx.QueryRow(`SELECT value FROM settings WHERE key='ai'`).Scan(&rv) == nil && json.Unmarshal([]byte(rv), &restored) == nil {
				if restored["api_key"] == nil || restored["api_key"] == "" {
					restored["api_key"] = cur["api_key"]
				}
				b, _ := json.Marshal(restored)
				v = string(b)
			}
		}
		tx.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, k, v)
	}
	// Data changes from migrations newer than the backup (schema 2: pensions
	// are liquid).
	if f.Schema < 2 {
		if _, err := tx.Exec(`UPDATE accounts SET liquid = 1 WHERE kind = 'pension'`); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return counts, nil
}

// existingColumns keeps the file's columns that this schema still has, so
// an older backup restores into a newer schema (new columns take defaults).
func existingColumns(tx *sql.Tx, table string, fileCols []string) []string {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		have[n] = true
	}
	rows.Close()
	var out []string
	for _, c := range fileCols {
		if have[c] {
			out = append(out, c)
		}
	}
	return out
}
