package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ft/internal/auth"
	"ft/internal/db"
	"ft/internal/demo"
)

// Demo mode: a second, complete API over a separate database of fictional
// finances. A switch sends data requests there; login, the lock and the
// switch itself always stay on the real database, and nothing in demo mode can
// read or write real files, bank credentials or AI keys.

// demoAlwaysReal are routes the real server answers even in demo mode.
func demoAlwaysReal(p string) bool {
	return p == "/api/health" || strings.HasPrefix(p, "/api/auth/") || p == "/api/demo" || strings.HasPrefix(p, "/api/demo/")
}

// demoBlocked are routes that touch real files, credentials or money flows.
func demoBlocked(r *http.Request) bool {
	p := r.URL.Path
	for _, pre := range []string{"/api/backups", "/api/import/", "/api/bank/", "/api/ibkr", "/api/ai/settings", "/api/ai/test", "/api/export/backup.json"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// demoLockedAuth are auth changes refused while the demo is on: whoever holds
// the device then must not be able to add a passkey or token, or change the
// PIN, and so keep a way into the real data after the demo ends.
func demoLockedAuth(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return false
	}
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/auth/pin/setup") || strings.HasPrefix(p, "/api/auth/pin/disable") ||
		strings.HasPrefix(p, "/api/auth/passkey/register") || strings.HasPrefix(p, "/api/auth/passkeys") || p == "/api/auth/token"
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	if s.demoOn.Load() && demoLockedAuth(r) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "security settings are locked while demo mode is on"})
		return
	}
	if s.demoOn.Load() && !demoAlwaysReal(r.URL.Path) {
		if d := s.demoServer(); d != nil {
			if demoBlocked(r) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "not available in demo mode — switch demo off in Settings → Data"})
				return
			}
			d.mux.ServeHTTP(w, r)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) demoPath() string {
	if s.DBPath == "" {
		return ""
	}
	// Its own folder, so its snapshots never mix with the real backups.
	return filepath.Join(filepath.Dir(s.DBPath), "demo", "finance-demo.db")
}

// demoServer opens (and on first use seeds) the demo database.
func (s *Server) demoServer() *Server {
	s.demoMu.Lock()
	defer s.demoMu.Unlock()
	if s.demo != nil {
		return s.demo
	}
	d, err := s.openDemo()
	if err != nil {
		return nil
	}
	s.demo = newServer(d, s.demoPath(), s.Version, true)
	s.demo.Auth = s.Auth // never consulted (the real middleware ran), but never a second lock
	return s.demo
}

func (s *Server) openDemo() (*sql.DB, error) {
	path := s.demoPath()
	var d *sql.DB
	var err error
	if path == "" {
		d, err = db.OpenMemory()
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		d, err = db.Open(path)
	}
	if err != nil {
		return nil, err
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n)
	if n == 0 {
		if err := demo.Seed(d, time.Now()); err != nil {
			d.Close()
			return nil, err
		}
	}
	return d, nil
}

func (s *Server) loadDemoFlag() {
	var raw string
	var v struct {
		On bool `json:"on"`
	}
	if s.DB.QueryRow(`SELECT value FROM settings WHERE key='demo'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &v)
	}
	s.demoOn.Store(v.On)
}

func (s *Server) demoRoutes() {
	// protected: a PIN is set, so leaving the demo needs it.
	s.handle("GET /api/demo", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"on": s.demoOn.Load(), "protected": s.Auth != nil && s.Auth.Enabled()}, nil
	})
	s.handle("PUT /api/demo", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			On  bool   `json:"on"`
			Pin string `json:"pin"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		// Back to the real data only with the owner's PIN: handing over an
		// unlocked phone in demo mode must not hand over the finances.
		if !in.On && s.demoOn.Load() && s.Auth != nil && s.Auth.Enabled() {
			if err := s.Auth.ConfirmPin(in.Pin); err != nil {
				if errors.Is(err, auth.ErrTooManyAttempts) {
					return nil, &HTTPError{http.StatusTooManyRequests, "too many wrong PINs — try again later"}
				}
				return nil, &HTTPError{http.StatusForbidden, "wrong PIN"}
			}
		}
		if in.On && s.demoServer() == nil {
			return nil, errors.New("could not create the demo data")
		}
		raw, _ := json.Marshal(map[string]bool{"on": in.On})
		if _, err := s.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('demo',?)`, string(raw)); err != nil {
			return nil, err
		}
		s.demoOn.Store(in.On)
		return map[string]bool{"on": in.On}, nil
	})
	// Throw the demo away and generate a fresh one (e.g. after playing with it).
	s.handle("POST /api/demo/reset", func(w http.ResponseWriter, r *http.Request) (any, error) {
		s.demoMu.Lock()
		if s.demo != nil {
			s.demo.DB.Close()
			s.demo = nil
		}
		if p := s.demoPath(); p != "" {
			for _, f := range []string{p, p + "-wal", p + "-shm"} {
				os.Remove(f)
			}
		}
		s.demoMu.Unlock()
		if s.demoServer() == nil {
			return nil, errors.New("could not create the demo data")
		}
		return map[string]bool{"ok": true}, nil
	})
}
