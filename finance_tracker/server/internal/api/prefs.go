package api

import (
	"database/sql"
	"encoding/json"
)

// Prefs are view choices that follow the owner across devices (stored on
// the server, not in one browser's localStorage).
type Prefs struct {
	LiquidOnly bool `json:"liquid_only"` // net worth views show liquid assets only
}

func loadPrefs(q interface{ QueryRow(string, ...any) *sql.Row }) Prefs {
	var p Prefs
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key='prefs'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &p)
	}
	return p
}

func savePrefs(e interface {
	Exec(string, ...any) (sql.Result, error)
}, p Prefs) error {
	raw, _ := json.Marshal(p)
	_, err := e.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('prefs',?)`, string(raw))
	return err
}
