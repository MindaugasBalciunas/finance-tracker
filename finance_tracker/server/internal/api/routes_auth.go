package api

import (
	"errors"
	"net/http"

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

// The nginx gates (auth_request). nginx — not the app — decides which paths
// may skip the web password, by its own normalised location matching; the app
// only answers two questions and never looks at the URL:
//
//	/api/auth/gate       — is there a live session?           (every path)
//	/api/auth/gate/open  — is a passkey registered, or a live  (app shell,
//	                       session?                             passkey login)
func (s *Server) hasSession(r *http.Request) bool {
	if !s.Auth.Enabled() {
		return false
	}
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.Auth.ValidSession(c.Value)
}

func gateAnswer(w http.ResponseWriter, ok bool) {
	w.Header().Set("Cache-Control", "no-store")
	if ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}

func (s *Server) authRoutes() {
	s.mux.HandleFunc("GET /api/auth/gate", func(w http.ResponseWriter, r *http.Request) {
		gateAnswer(w, s.hasSession(r))
	})
	s.mux.HandleFunc("GET /api/auth/gate/open", func(w http.ResponseWriter, r *http.Request) {
		gateAnswer(w, s.hasSession(r) || (s.Auth.Enabled() && s.Auth.Status("").Passkeys > 0))
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
