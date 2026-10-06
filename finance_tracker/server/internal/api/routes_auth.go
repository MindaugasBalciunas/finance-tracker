package api

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"ft/internal/auth"
)

// ── auth ────────────────────────────────────────────────────────────

func (s *Server) unlocked(r *http.Request) bool {
	if !s.Auth.Enabled() {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.Auth.ValidSession(c.Value)
}

var errLocked = &HTTPError{401, "locked"}

// gatePublic are the paths someone with a passkey needs before unlocking:
// the app itself, the lock screen's status check and the passkey login.
// PIN login is deliberately not here — it stays behind the web password, so a
// PIN can't be guessed from the internet.
func gatePublic(p string) bool {
	if !strings.HasPrefix(p, "/api/") {
		return true // the app shell and its assets: code, no data
	}
	switch p {
	case "/api/health", "/api/auth/status", "/api/auth/passkey/login/begin", "/api/auth/passkey/login/finish", "/api/auth/logout":
		return true
	}
	return false
}

// gateAllows answers nginx's auth_request: may this request skip the web
// username and password? Only once a passkey is registered: then the app
// shell and passkey login are open, and everything else needs the session
// that a passkey (or PIN) login created. Without a passkey: never.
func (s *Server) gateAllows(r *http.Request) bool {
	if !s.Auth.Enabled() {
		return false
	}
	if c, err := r.Cookie(sessionCookie); err == nil && s.Auth.ValidSession(c.Value) {
		return true
	}
	uri := r.Header.Get("X-Original-URI")
	if i := strings.IndexAny(uri, "?#"); i >= 0 {
		uri = uri[:i]
	}
	if uri == "" || strings.Contains(uri, "..") {
		return false
	}
	return gatePublic(path.Clean(uri)) && s.Auth.Status("").Passkeys > 0
}

func (s *Server) authRoutes() {
	// nginx auth_request: 204 lets the request through without the web
	// password, 401 falls back to it. Says nothing else.
	s.mux.HandleFunc("GET /api/auth/gate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.gateAllows(r) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	s.handle("GET /api/auth/status", func(w http.ResponseWriter, r *http.Request) (any, error) {
		tok := ""
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
		return s.Auth.Status(tok), nil
	})
	s.handle("POST /api/auth/pin/login", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Pin string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		tok, err := s.Auth.VerifyPin(in.Pin)
		if err != nil {
			if errors.Is(err, auth.ErrTooManyAttempts) {
				return nil, &HTTPError{429, err.Error()}
			}
			return nil, &HTTPError{401, err.Error()}
		}
		setSession(w, r, tok)
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/pin/setup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct {
			CurrentPin string `json:"current_pin"`
			Pin        string `json:"pin"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := s.Auth.SetupPin(in.CurrentPin, in.Pin); err != nil {
			return nil, err
		}
		tok, err := s.Auth.VerifyPin(in.Pin)
		if err == nil {
			setSession(w, r, tok)
		}
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/pin/disable", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct{ Pin string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Auth.DisableLock(in.Pin)
	})
	s.handle("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			s.Auth.DestroySession(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/passkey/register/begin", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		h, o := hostOrigin(r)
		return s.Auth.BeginRegistration(h, o)
	})
	s.handle("POST /api/auth/passkey/register/finish", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		h, o := hostOrigin(r)
		return map[string]bool{"ok": true}, s.Auth.FinishRegistration(h, o, r.URL.Query().Get("name"), r)
	})
	s.handle("POST /api/auth/passkey/login/begin", func(w http.ResponseWriter, r *http.Request) (any, error) {
		h, o := hostOrigin(r)
		return s.Auth.BeginLogin(h, o)
	})
	s.handle("POST /api/auth/passkey/login/finish", func(w http.ResponseWriter, r *http.Request) (any, error) {
		h, o := hostOrigin(r)
		tok, err := s.Auth.FinishLogin(h, o, r)
		if err != nil {
			return nil, &HTTPError{401, err.Error()}
		}
		setSession(w, r, tok)
		return map[string]bool{"ok": true}, nil
	})
	s.handle("GET /api/auth/passkeys", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		return s.Auth.Passkeys()
	})
	s.handle("DELETE /api/auth/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Auth.DeletePasskey(id)
	})
	s.handle("POST /api/auth/token", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct{ RW bool }
		decode(r, &in)
		tok, err := s.Auth.MintToken(in.RW)
		return map[string]string{"token": tok}, err
	})
	s.handle("DELETE /api/auth/token", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		return map[string]bool{"ok": true}, s.Auth.RevokeToken(r.URL.Query().Get("rw") == "1")
	})
}
