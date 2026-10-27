// Package oauth lets an AI app — Claude's custom connectors, Claude Code,
// any MCP client — connect to the app's MCP endpoint with the owner's
// consent, the way MCP authorization expects: OAuth 2.1 with dynamic client
// registration, public clients and PKCE (S256).
//
// The owner approves each connection inside the app (behind its own lock)
// and picks read-only or read-write; the connected app then holds a short
// access token (ftoa_, one hour) and a rotating refresh token (ftor_, 30 days
// from its last use). Only hashes are stored. A connection is revoked in
// Settings → Security, and replaying a refresh token that was already used
// ends the connection (a stolen copy and the real app can't both keep it).
//
// Redirects may go only to Claude (claude.ai, claude.com) or to the
// person's own machine (localhost), so a look-alike app can't collect a
// code by sending the owner a link.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	AccessTTL  = time.Hour
	RefreshTTL = 30 * 24 * time.Hour
	codeTTL    = 2 * time.Minute
	maxClients = 50

	accessPrefix  = "ftoa_"
	refreshPrefix = "ftor_"
)

// Scopes: what a connection may do. The owner picks one on the consent
// page; the API enforces it exactly like the ftk_/ftkw_ tokens.
const (
	ReadOnly  = "ro"
	ReadWrite = "rw"
)

// Error is an OAuth error response (RFC 6749 §5.2).
type Error struct {
	Code string `json:"error"`
	Desc string `json:"error_description,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Desc }

func oerr(code, format string, a ...any) *Error {
	return &Error{Code: code, Desc: fmt.Sprintf(format, a...)}
}

type Client struct {
	ID           string   `json:"client_id"`
	Name         string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	CreatedAt    string   `json:"-"`
}

// Grant is one connected app as Settings lists it.
type Grant struct {
	ID         int64  `json:"id"`
	ClientName string `json:"client_name"`
	Scope      string `json:"scope"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

// Tokens is the token endpoint's answer.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// Request is an authorization request as the consent page carries it.
type Request struct {
	ClientID      string `json:"client_id"`
	RedirectURI   string `json:"redirect_uri"`
	State         string `json:"state"`
	CodeChallenge string `json:"code_challenge"`
	Method        string `json:"code_challenge_method"`
	ResponseType  string `json:"response_type"`
}

type code struct {
	req     Request
	scope   string
	expires time.Time
}

// Server holds the short-lived authorization codes (in memory: a code
// lives two minutes, so a restart only means pressing Connect again).
type Server struct {
	DB  *sql.DB
	Now func() time.Time

	mu    sync.Mutex
	codes map[string]code
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func random(prefix string) string {
	b := make([]byte, 32)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// AllowedRedirect: Claude's callback (claude.ai, claude.com over https) or
// a loopback address on the person's own machine.
func AllowedRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	host := u.Hostname()
	switch u.Scheme {
	case "https":
		return host == "claude.ai" || host == "claude.com"
	case "http":
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	return false
}

// Register adds a client (dynamic client registration, RFC 7591). Old
// clients that never connected make room once there are too many.
func Register(d *sql.DB, name string, redirects []string) (*Client, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "AI app"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	if len(redirects) == 0 || len(redirects) > 5 {
		return nil, oerr("invalid_redirect_uri", "one to five redirect URIs are needed")
	}
	for _, r := range redirects {
		if !AllowedRedirect(r) {
			return nil, oerr("invalid_redirect_uri", "redirects may go only to Claude (claude.ai, claude.com) or to localhost")
		}
	}
	// Registration is public: apps that never connected are dropped after an
	// hour, and a flood of fresh ones is refused rather than allowed to push
	// out the one the owner is about to approve.
	now := time.Now()
	if _, err := d.Exec(`DELETE FROM oauth_clients WHERE created_at < ? AND id NOT IN (SELECT client_id FROM oauth_grants)`, stamp(now.Add(-time.Hour))); err != nil {
		return nil, err
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM oauth_clients WHERE id NOT IN (SELECT client_id FROM oauth_grants)`).Scan(&n)
	if n >= maxClients {
		return nil, oerr("temporarily_unavailable", "too many apps registered in the last hour — try again later")
	}
	c := &Client{ID: random("ftc_")[:36], Name: name, RedirectURIs: redirects, CreatedAt: stamp(now)}
	raw, _ := json.Marshal(redirects)
	if _, err := d.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)`, c.ID, c.Name, string(raw), c.CreatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

func GetClient(d *sql.DB, id string) (*Client, error) {
	c := &Client{ID: id}
	var raw string
	if err := d.QueryRow(`SELECT name, redirect_uris, created_at FROM oauth_clients WHERE id=?`, id).Scan(&c.Name, &raw, &c.CreatedAt); err != nil {
		return nil, oerr("invalid_client", "unknown client — add the connector again")
	}
	json.Unmarshal([]byte(raw), &c.RedirectURIs)
	return c, nil
}

// Check validates an authorization request before the owner sees it.
// An error here must be shown to the owner, never redirected: the redirect
// itself is what failed to check out.
func Check(d *sql.DB, r Request) (*Client, error) {
	c, err := GetClient(d, r.ClientID)
	if err != nil {
		return nil, err
	}
	ok := false
	for _, u := range c.RedirectURIs {
		if u == r.RedirectURI {
			ok = true
		}
	}
	if !ok {
		return nil, oerr("invalid_request", "the redirect URI was not registered for this app")
	}
	if r.ResponseType != "code" {
		return nil, oerr("unsupported_response_type", "only the authorization code flow is supported")
	}
	if r.Method != "S256" || len(r.CodeChallenge) < 43 || len(r.CodeChallenge) > 128 {
		return nil, oerr("invalid_request", "PKCE with S256 is required")
	}
	return c, nil
}

// Redirect builds the URL the browser goes back to.
func Redirect(r Request, params url.Values) string {
	if r.State != "" {
		params.Set("state", r.State)
	}
	sep := "?"
	if strings.Contains(r.RedirectURI, "?") {
		sep = "&"
	}
	return r.RedirectURI + sep + params.Encode()
}

// Approve records the owner's yes and returns where to send the browser.
func (s *Server) Approve(r Request, scope string) (string, error) {
	if _, err := Check(s.DB, r); err != nil {
		return "", err
	}
	if scope != ReadOnly && scope != ReadWrite {
		return "", errors.New("choose read-only or read and write")
	}
	c := random("")
	s.mu.Lock()
	if s.codes == nil {
		s.codes = map[string]code{}
	}
	now := s.now()
	for k, v := range s.codes {
		if now.After(v.expires) {
			delete(s.codes, k)
		}
	}
	s.codes[hash(c)] = code{req: r, scope: scope, expires: now.Add(codeTTL)}
	s.mu.Unlock()
	return Redirect(r, url.Values{"code": {c}}), nil
}

// Deny sends the browser back with access_denied.
func Deny(d *sql.DB, r Request) (string, error) {
	if _, err := Check(d, r); err != nil {
		return "", err
	}
	return Redirect(r, url.Values{"error": {"access_denied"}}), nil
}

// Exchange trades a code for tokens (once; PKCE-checked).
func (s *Server) Exchange(clientID, c, verifier, redirectURI string) (*Tokens, error) {
	s.mu.Lock()
	k := hash(c)
	cd, ok := s.codes[k]
	delete(s.codes, k)
	s.mu.Unlock()
	if !ok || s.now().After(cd.expires) {
		return nil, oerr("invalid_grant", "the code is unknown, used or expired")
	}
	if cd.req.ClientID != clientID || cd.req.RedirectURI != redirectURI {
		return nil, oerr("invalid_grant", "the code was issued to another app or redirect")
	}
	sum := sha256.Sum256([]byte(verifier))
	if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(cd.req.CodeChallenge)) != 1 {
		return nil, oerr("invalid_grant", "PKCE verification failed")
	}
	now := s.now()
	t := s.issue(cd.scope, now)
	_, err := s.DB.Exec(`INSERT INTO oauth_grants(client_id,scope,access_hash,access_expires,refresh_hash,refresh_expires,created_at,last_used_at) VALUES(?,?,?,?,?,?,?,?)`,
		clientID, cd.scope, hash(t.AccessToken), stamp(now.Add(AccessTTL)), hash(t.RefreshToken), stamp(now.Add(RefreshTTL)), stamp(now), stamp(now))
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Server) issue(scope string, now time.Time) *Tokens {
	sc := "read"
	if scope == ReadWrite {
		sc = "read write"
	}
	return &Tokens{AccessToken: random(accessPrefix), RefreshToken: random(refreshPrefix), TokenType: "Bearer", ExpiresIn: int(AccessTTL.Seconds()), Scope: sc}
}

// Refresh rotates a connection's tokens. A refresh token used a second time
// means two holders: the connection ends.
func (s *Server) Refresh(clientID, refresh string) (*Tokens, error) {
	h := hash(refresh)
	var id int64
	var client, scope, exp string
	err := s.DB.QueryRow(`SELECT id, client_id, scope, refresh_expires FROM oauth_grants WHERE refresh_hash=?`, h).Scan(&id, &client, &scope, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		if res, _ := s.DB.Exec(`DELETE FROM oauth_grants WHERE prev_refresh_hash=?`, h); res != nil {
			if n, _ := res.RowsAffected(); n > 0 {
				return nil, oerr("invalid_grant", "this refresh token was already used — the connection was ended; connect again")
			}
		}
		return nil, oerr("invalid_grant", "unknown or revoked refresh token")
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if client != clientID {
		return nil, oerr("invalid_grant", "the refresh token belongs to another app")
	}
	if e, _ := time.Parse(time.RFC3339, exp); now.After(e) {
		s.DB.Exec(`DELETE FROM oauth_grants WHERE id=?`, id)
		return nil, oerr("invalid_grant", "the connection expired — connect again")
	}
	t := s.issue(scope, now)
	// Rotate only if the token is still the current one: of two requests
	// racing with the same refresh token, one wins and the other counts as
	// a replay — which ends the connection.
	res, err := s.DB.Exec(`UPDATE oauth_grants SET access_hash=?, access_expires=?, refresh_hash=?, refresh_expires=?, prev_refresh_hash=?, last_used_at=? WHERE id=? AND refresh_hash=?`,
		hash(t.AccessToken), stamp(now.Add(AccessTTL)), hash(t.RefreshToken), stamp(now.Add(RefreshTTL)), h, stamp(now), id, h)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		s.DB.Exec(`DELETE FROM oauth_grants WHERE id=?`, id)
		return nil, oerr("invalid_grant", "this refresh token was already used — the connection was ended; connect again")
	}
	return t, nil
}

// IsAccessToken tells a connector's token from other bearer tokens.
func IsAccessToken(tok string) bool { return strings.HasPrefix(tok, accessPrefix) }

// Scope returns ro, rw or "" (unknown, expired or revoked) for an access token.
func (s *Server) Scope(access string) string {
	if !IsAccessToken(access) {
		return ""
	}
	var id int64
	var scope, exp, last string
	if s.DB.QueryRow(`SELECT id, scope, access_expires, last_used_at FROM oauth_grants WHERE access_hash=?`, hash(access)).Scan(&id, &scope, &exp, &last) != nil {
		return ""
	}
	now := s.now()
	if e, err := time.Parse(time.RFC3339, exp); err != nil || now.After(e) {
		return ""
	}
	// "Last used" to the minute is plenty; spare a write on every call.
	if l, err := time.Parse(time.RFC3339, last); err != nil || now.Sub(l) > time.Minute {
		s.DB.Exec(`UPDATE oauth_grants SET last_used_at=? WHERE id=?`, stamp(now), id)
	}
	return scope
}

// Grants lists the connected apps, most recently used first.
func Grants(d *sql.DB) ([]Grant, error) {
	rows, err := d.Query(`SELECT g.id, c.name, g.scope, g.created_at, g.last_used_at FROM oauth_grants g JOIN oauth_clients c ON c.id=g.client_id ORDER BY g.last_used_at DESC, g.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.ClientName, &g.Scope, &g.CreatedAt, &g.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Revoke ends a connection at once: its tokens stop working on the next call.
func Revoke(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM oauth_grants WHERE id=?`, id)
	return err
}
