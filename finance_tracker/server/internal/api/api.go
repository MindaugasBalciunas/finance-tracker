// Package api is the HTTP surface: JSON over net/http, one handler file per
// domain, all under /api.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ft/internal/ai"
	"ft/internal/auth"
	"ft/internal/bank"
	"ft/internal/bank/openbanking"
)

type Server struct {
	DB      *sql.DB
	DBPath  string
	Auth    *auth.Service
	Bank    *bank.Service
	AI      *ai.Assistant
	Version string
	mux     *http.ServeMux
}

func New(d *sql.DB, dbPath, version string) *Server {
	s := &Server{DB: d, DBPath: dbPath, Version: version, Auth: &auth.Service{DB: d}, Bank: &bank.Service{DB: d}, mux: http.NewServeMux()}
	s.AI = &ai.Assistant{DB: d, Client: &ai.Client{DB: d}, Bank: s.Bank}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.recoverer(s.authenticate(s.mux)) }

func (s *Server) handle(pattern string, h func(w http.ResponseWriter, r *http.Request) (any, error)) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		v, err := h(w, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if v == nil {
			return // handler wrote the response itself
		}
		writeJSON(w, http.StatusOK, v)
	})
}

// HTTPError carries a status.
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string { return e.Msg }

func bad(msg string) error      { return &HTTPError{http.StatusBadRequest, msg} }
func notFound(msg string) error { return &HTTPError{http.StatusNotFound, msg} }

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var he *HTTPError
	var api *openbanking.APIError
	switch {
	case errors.As(err, &he):
		status = he.Status
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	case errors.As(err, &api):
		status = http.StatusBadGateway
		if api.Expired() {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "the bank connection has expired — reconnect it", "expired": true})
			return
		}
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err := dec.Decode(v); err != nil {
		return bad("invalid request body: " + err.Error())
	}
	return nil
}

func idParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, bad("invalid id")
	}
	return id, nil
}

func qint(r *http.Request, k string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil {
		return v
	}
	return def
}

func qlist(r *http.Request, k string) []string {
	var out []string
	for _, v := range r.URL.Query()[k] {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic %s %s: %v", r.Method, r.URL.Path, v)
				writeJSON(w, 500, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const sessionCookie = "ft_session"

// Read routes a read-only token may use: data, never settings, secrets,
// exports or the chat.
var tokenReadPrefixes = []string{"/api/health", "/api/overview", "/api/transactions", "/api/categories", "/api/accounts", "/api/tags",
	"/api/merchants", "/api/rules", "/api/networth", "/api/balances", "/api/loans", "/api/portfolio", "/api/trades", "/api/market",
	"/api/plan", "/api/budgets", "/api/trips", "/api/insights", "/api/bank/inbox", "/api/bank/connections", "/api/ai/context"}

// Writes a read-write token may make: improve the data, never move money
// into the ledger from the bank or touch settings.
var tokenWriteRoutes = []struct{ method, prefix string }{
	{"PUT", "/api/transactions/"}, {"POST", "/api/rules"}, {"PUT", "/api/rules/"}, {"DELETE", "/api/rules/"},
	{"POST", "/api/tags/rename"}, {"PUT", "/api/bank/inbox/"},
}

func pathUnder(p, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(p, prefix)
	}
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/api/auth/") || p == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.Auth.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(sessionCookie); err == nil && s.Auth.ValidSession(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			scope := s.Auth.TokenScope(strings.TrimSpace(bearer))
			if scope == "" {
				writeJSON(w, 401, map[string]string{"error": "invalid API token"})
				return
			}
			if r.Method == http.MethodGet {
				for _, pre := range tokenReadPrefixes {
					if pathUnder(p, pre) {
						next.ServeHTTP(w, r)
						return
					}
				}
			} else if scope == "rw" {
				for _, wr := range tokenWriteRoutes {
					if r.Method == wr.method && pathUnder(p, wr.prefix) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			writeJSON(w, 403, map[string]string{"error": "this route is outside the API token's scope"})
			return
		}
		writeJSON(w, 401, map[string]string{"error": "locked"})
	})
}

func isHTTPS(r *http.Request) bool { return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" }

func setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(auth.SessionTTL.Seconds()),
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

// hostOrigin prefers the browser's Origin header: proxies may rewrite Host,
// but the WebAuthn RP ID must match what the user sees.
func hostOrigin(r *http.Request) (string, string) {
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			return u.Host, o
		}
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return r.Host, scheme + "://" + r.Host
}

func psu(r *http.Request) openbanking.PSU {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return openbanking.PSU{IPAddress: ip, UserAgent: r.UserAgent()}
}

func today() string { return time.Now().Format("2006-01-02") }
