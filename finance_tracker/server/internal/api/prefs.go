package api

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Prefs are view choices that follow the owner across devices (stored on
// the server, not in one browser's localStorage).
type Prefs struct {
	LiquidOnly bool              `json:"liquid_only"` // net worth views show liquid assets only
	Periods    map[string]string `json:"periods"`     // chart → chosen period (e.g. "wealth": "3y")
	// Accounts switched off in "Where my money is". A full list on every
	// write (unlike periods, which merge per key).
	HiddenAccounts []string `json:"hidden_accounts"`
	// Columns switched off in Wealth → History. nil = never configured (the
	// client then hides valuations like house, car and solar by default).
	HistoryHidden *[]string `json:"history_hidden,omitempty"`
}

// validPrefs keeps the stored blob small and boring.
func validPrefs(p Prefs) error {
	if p.HistoryHidden != nil && len(*p.HistoryHidden) > 200 {
		return errors.New("too many hidden columns")
	}
	if len(p.HiddenAccounts) > 200 {
		return errors.New("too many hidden accounts")
	}
	if len(p.Periods) > 40 {
		return errors.New("too many periods")
	}
	for k, v := range p.Periods {
		if len(k) > 32 || len(v) > 16 || k == "" {
			return errors.New("invalid period")
		}
	}
	return nil
}

func loadPrefs(q interface{ QueryRow(string, ...any) *sql.Row }) Prefs {
	var p Prefs
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key='prefs'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &p)
	}
	if p.Periods == nil {
		p.Periods = map[string]string{}
	}
	if p.HiddenAccounts == nil {
		p.HiddenAccounts = []string{}
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
