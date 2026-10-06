// Package ibkr connects to Interactive Brokers through IBKR's own MCP server
// (the "connector" that Claude and other AI apps use), read-only.
//
// Sign-in is IBKR's standard OAuth for MCP clients: the app registers itself
// (dynamic client registration, public client, PKCE), asks only for the
// "mcp.read" scope, and keeps the refresh token as a secret setting (left
// out of backups without secrets). Access can be revoked here or in IBKR
// Client Portal → Settings → Manage Third-Party Consents.
//
// Only the read tools in ReadTools are ever called — the client refuses any
// other tool name, so nothing here can place orders or change the account.
package ibkr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ft/internal/db"
)

const (
	DefaultAuthBase = "https://api.ibkr.com"
	DefaultMCPURL   = "https://api.ibkr.com/v1/api/mcp-public"
	Scope           = "mcp.read"
	settingsKey     = "ibkr"
	statePrefix     = "ibkr."
)

// ReadTools are the only IBKR tools this app calls.
var ReadTools = map[string]bool{
	"get_account_summary":   true,
	"get_account_positions": true,
	"get_account_balances":  true,
	"get_account_trades":    true,
}

var (
	ErrNotConnected = errors.New("Interactive Brokers is not connected — connect it in Settings → Banks")
	ErrToolRefused  = errors.New("only read-only IBKR tools are allowed")
)

// State is everything kept about the connection (settings key "ibkr").
type State struct {
	ClientID     string `json:"client_id,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Account      string `json:"account,omitempty"` // app account the IBKR value goes to (default "ibkr")
	ConnectedAt  string `json:"connected_at,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	// A sign-in in progress: the state echoed back and the PKCE verifier.
	PendingState    string `json:"pending_state,omitempty"`
	PendingVerifier string `json:"pending_verifier,omitempty"`
	PendingAt       string `json:"pending_at,omitempty"`
}

type Service struct {
	DB       *sql.DB
	AuthBase string // tests point these at a fake IBKR
	MCPURL   string
	HTTP     *http.Client
	Now      func() time.Time
	mu       sync.Mutex
}

func New(d *sql.DB) *Service {
	return &Service{DB: d, AuthBase: DefaultAuthBase, MCPURL: DefaultMCPURL, HTTP: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (s *Service) load() State {
	var raw string
	var st State
	if s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, settingsKey).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &st)
	}
	if st.Account == "" {
		st.Account = "ibkr"
	}
	return st
}

func (s *Service) save(st State) error {
	raw, _ := json.Marshal(st)
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingsKey, string(raw))
	return err
}

// Status is what the UI may see — never the tokens.
type Status struct {
	Connected   bool   `json:"connected"`
	Account     string `json:"account"`
	Scope       string `json:"scope,omitempty"`
	ConnectedAt string `json:"connected_at,omitempty"`
	LastSync    string `json:"last_sync,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

func (s *Service) Status() Status {
	st := s.load()
	return Status{Connected: st.RefreshToken != "" || st.AccessToken != "", Account: st.Account, Scope: st.Scope,
		ConnectedAt: st.ConnectedAt, LastSync: st.LastSync, LastError: st.LastError}
}

func (s *Service) Connected() bool { return s.Status().Connected }

// SetAccount chooses the app account the IBKR value is recorded on.
func (s *Service) SetAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	st.Account = id
	return s.save(st)
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Service) postJSON(ctx context.Context, u string, in, out any) error {
	body, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("IBKR %s: %d %s", u, res.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}

func (s *Service) postForm(ctx context.Context, u string, form url.Values, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("IBKR token endpoint: %d %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Connect registers the app with IBKR when needed and returns the address of
// IBKR's consent page. redirectURI is this app's own address.
func (s *Service) Connect(ctx context.Context, redirectURI string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return "", errors.New("IBKR sign-in needs the app's https address (or localhost)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	if st.ClientID == "" || st.RedirectURI != redirectURI {
		var reg struct {
			ClientID string `json:"client_id"`
		}
		err := s.postJSON(ctx, s.AuthBase+"/oauth2/register", map[string]any{
			"client_name":                "Finance Tracker (self-hosted, read-only)",
			"redirect_uris":              []string{redirectURI},
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
			"token_endpoint_auth_method": "none",
			"scope":                      Scope,
		}, &reg)
		if err != nil {
			return "", fmt.Errorf("registering with IBKR: %w", err)
		}
		if reg.ClientID == "" {
			return "", errors.New("IBKR did not return a client id")
		}
		st.ClientID, st.RedirectURI = reg.ClientID, redirectURI
	}
	verifier := random(48)
	sum := sha256.Sum256([]byte(verifier))
	st.PendingState, st.PendingVerifier, st.PendingAt = statePrefix+random(24), verifier, s.Now().UTC().Format(time.RFC3339)
	if err := s.save(st); err != nil {
		return "", err
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {st.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scope},
		"state":                 {st.PendingState},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"resource":              {s.MCPURL},
	}
	return s.AuthBase + "/oauth2/authorize?" + q.Encode(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// readOnly reports whether a granted scope stays within read-only access.
// An empty scope means the server didn't say: the request was mcp.read.
func readOnly(scope string) bool {
	for _, sc := range strings.Fields(scope) {
		if sc != Scope {
			return false
		}
	}
	return true
}

var errTooMuch = errors.New("IBKR granted more than read-only access — the app refused it and revoked the sign-in")

func (s *Service) keep(st *State, t tokenResponse) {
	st.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		st.RefreshToken = t.RefreshToken
	}
	if t.ExpiresIn <= 0 {
		t.ExpiresIn = 600
	}
	st.ExpiresAt = s.Now().UTC().Add(time.Duration(t.ExpiresIn) * time.Second).Format(time.RFC3339)
	if t.Scope != "" {
		st.Scope = t.Scope
	}
}

// IsCallbackState tells an IBKR sign-in return from a bank one.
func IsCallbackState(state string) bool { return strings.HasPrefix(state, statePrefix) }

// Callback finishes the sign-in with the code IBKR sent back.
func (s *Service) Callback(ctx context.Context, code, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	if st.PendingState == "" || state != st.PendingState {
		return errors.New("this IBKR sign-in is not the one started here — start again")
	}
	if at, err := time.Parse(time.RFC3339, st.PendingAt); err != nil || s.Now().Sub(at) > 15*time.Minute {
		return errors.New("the IBKR sign-in took too long — start again")
	}
	var t tokenResponse
	err := s.postForm(ctx, s.AuthBase+"/oauth2/api/v1/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {st.RedirectURI},
		"client_id":     {st.ClientID},
		"code_verifier": {st.PendingVerifier},
		"resource":      {s.MCPURL},
	}, &t)
	st.PendingState, st.PendingVerifier, st.PendingAt = "", "", ""
	if err != nil {
		s.save(st)
		return err
	}
	if t.AccessToken == "" {
		s.save(st)
		return errors.New("IBKR returned no access token")
	}
	if !readOnly(t.Scope) { // never keep a token that could trade
		for _, tok := range []string{t.RefreshToken, t.AccessToken} {
			if tok != "" {
				s.postForm(ctx, s.AuthBase+"/oauth2/api/v1/token/revoke", url.Values{"token": {tok}, "client_id": {st.ClientID}}, nil)
			}
		}
		s.save(st)
		return errTooMuch
	}
	s.keep(&st, t)
	st.ConnectedAt, st.LastError = s.Now().UTC().Format(time.RFC3339), ""
	return s.save(st)
}

// Disconnect revokes the token at IBKR (best effort) and forgets it.
func (s *Service) Disconnect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	for _, tok := range []string{st.RefreshToken, st.AccessToken} {
		if tok != "" {
			s.postForm(ctx, s.AuthBase+"/oauth2/api/v1/token/revoke", url.Values{"token": {tok}, "client_id": {st.ClientID}}, nil)
		}
	}
	return s.save(State{ClientID: st.ClientID, RedirectURI: st.RedirectURI, Account: st.Account})
}

// accessToken returns a valid access token, refreshing it when it is about
// to expire. A refused refresh means the consent is gone.
func (s *Service) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	if st.AccessToken == "" && st.RefreshToken == "" {
		return "", ErrNotConnected
	}
	exp, _ := time.Parse(time.RFC3339, st.ExpiresAt)
	if st.AccessToken != "" && s.Now().Add(2*time.Minute).Before(exp) {
		return st.AccessToken, nil
	}
	if st.RefreshToken == "" {
		return "", errors.New("the IBKR sign-in has expired — connect again")
	}
	var t tokenResponse
	if err := s.postForm(ctx, s.AuthBase+"/oauth2/api/v1/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {st.RefreshToken}, "client_id": {st.ClientID}, "resource": {s.MCPURL},
	}, &t); err != nil {
		st.LastError = "sign-in refresh failed — connect again"
		s.save(st)
		return "", fmt.Errorf("the IBKR sign-in could not be renewed (%v) — connect again", err)
	}
	if !readOnly(t.Scope) {
		s.postForm(ctx, s.AuthBase+"/oauth2/api/v1/token/revoke", url.Values{"token": {t.AccessToken}, "client_id": {st.ClientID}}, nil)
		st.AccessToken, st.RefreshToken, st.LastError = "", "", errTooMuch.Error()
		s.save(st)
		return "", errTooMuch
	}
	s.keep(&st, t)
	if err := s.save(st); err != nil {
		return "", err
	}
	return st.AccessToken, nil
}

// conn is one MCP session with IBKR's server (streamable HTTP).
type conn struct {
	s       *Service
	token   string
	session string
	id      int
}

func (s *Service) open(ctx context.Context) (*conn, error) {
	tok, err := s.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	c := &conn{s: s, token: tok}
	if _, err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "finance-tracker", "version": "2"},
	}); err != nil {
		return nil, err
	}
	c.notify(ctx, "notifications/initialized")
	return c, nil
}

func (c *conn) do(ctx context.Context, msg map[string]any) (*http.Response, error) {
	body, _ := json.Marshal(msg)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.s.MCPURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	}
	return c.s.HTTP.Do(req)
}

func (c *conn) notify(ctx context.Context, method string) {
	if res, err := c.do(ctx, map[string]any{"jsonrpc": "2.0", "method": method}); err == nil {
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
}

func (c *conn) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.id++
	res, err := c.do(ctx, map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if sid := res.Header.Get("Mcp-Session-Id"); sid != "" {
		c.session = sid
	}
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("IBKR refused the sign-in — connect again")
	}
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("IBKR %s: %d %s", method, res.StatusCode, strings.TrimSpace(string(raw)))
	}
	raw, err := readRPC(res, c.id)
	if err != nil {
		return nil, err
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, fmt.Errorf("IBKR %s: %s", method, env.Error.Message)
	}
	return env.Result, nil
}

// readRPC returns the JSON-RPC response with the given id, from a plain JSON
// body or a server-sent event stream.
func readRPC(res *http.Response, id int) (json.RawMessage, error) {
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		return raw, err
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	var data strings.Builder
	flush := func() json.RawMessage {
		defer data.Reset()
		if data.Len() == 0 {
			return nil
		}
		var probe struct {
			ID int `json:"id"`
		}
		raw := []byte(data.String())
		if json.Unmarshal(raw, &probe) == nil && probe.ID == id {
			return raw
		}
		return nil
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if raw := flush(); raw != nil {
				return raw, nil
			}
			continue
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimPrefix(v, " "))
		}
	}
	if raw := flush(); raw != nil {
		return raw, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("IBKR sent no answer")
}

// call runs one read tool and returns its JSON text content.
func (c *conn) call(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	if !ReadTools[tool] {
		return nil, ErrToolRefused
	}
	if args == nil {
		args = map[string]any{}
	}
	res, err := c.rpc(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return nil, err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	if out.IsError {
		return nil, fmt.Errorf("IBKR %s: %s", tool, text.String())
	}
	return json.RawMessage(text.String()), nil
}

// Live runs one read tool in a fresh session — for the assistant.
func (s *Service) Live(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	if !ReadTools[tool] {
		return nil, ErrToolRefused
	}
	c, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	return c.call(ctx, tool, args)
}

func (s *Service) note(lastErr string, synced bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.load()
	st.LastError = lastErr
	if synced {
		st.LastSync = db.Now()
	}
	s.save(st)
}
