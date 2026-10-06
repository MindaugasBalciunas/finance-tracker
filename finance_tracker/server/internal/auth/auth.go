// Package auth is the app lock: PIN + passkeys (WebAuthn) for people, and
// two bearer tokens (read-only, read-write) for machines such as the MCP
// server. nginx basic auth sits in front of all of it.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/crypto/bcrypt"

	"ft/internal/db"
)

const (
	SessionTTL = 30 * 24 * time.Hour // the cookie's lifetime; the server ends a session after IdleTimeout without use
	// IdleTimeout: a session ends after this long without a request — the
	// fingerprint (or PIN) is asked again. Each use moves it forward.
	IdleTimeout     = 15 * time.Minute
	pinAttemptLimit = 5
	pinLockoutBase  = 30 * time.Second
	pinLockoutMax   = 15 * time.Minute
)

var pinPattern = regexp.MustCompile(`^\d{4,8}$`)

var (
	ErrInvalidPin      = errors.New("the PIN must be 4–8 digits")
	ErrWrongPin        = errors.New("wrong PIN")
	ErrLockDisabled    = errors.New("app lock is not enabled")
	ErrNoCeremony      = errors.New("no pending passkey ceremony — start again")
	ErrTooManyAttempts = errors.New("too many failed attempts — try again later")
)

type settings struct {
	PinHash     string `json:"pin_hash"`
	Enabled     bool   `json:"enabled"`
	TokenHash   string `json:"token_hash"`
	TokenRWHash string `json:"token_rw_hash"`
}

type owner struct{ creds []webauthn.Credential }

func (u *owner) WebAuthnID() []byte                         { return []byte("finance-tracker-owner") }
func (u *owner) WebAuthnName() string                       { return "owner" }
func (u *owner) WebAuthnDisplayName() string                { return "Finance Tracker" }
func (u *owner) WebAuthnCredentials() []webauthn.Credential { return u.creds }

type Status struct {
	Enabled    bool `json:"enabled"`
	Unlocked   bool `json:"unlocked"`
	Passkeys   int  `json:"passkeys"`
	HasToken   bool `json:"has_token"`
	HasTokenRW bool `json:"has_token_rw"`
}

type Service struct {
	DB *sql.DB
	// Clock, for tests; nil = time.Now.
	Clock func() time.Time

	mu         sync.Mutex
	pendingReg *webauthn.SessionData
	// Login challenges in flight, by challenge: anyone may start a passkey
	// login (it needs no password), so one stranger's attempt must not
	// replace the owner's. Few, and short-lived.
	pendingLogin map[string]pendingLogin
	failures     int
	lockedUntil  time.Time
}

func (s *Service) load() (settings, error) {
	var st settings
	var raw string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key='auth'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal([]byte(raw), &st)
}

func (s *Service) save(st settings) error {
	raw, _ := json.Marshal(st)
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('auth',?)`, string(raw))
	return err
}

// Enabled fails closed: an unreadable lock state counts as locked.
func (s *Service) Enabled() bool {
	st, err := s.load()
	if err != nil {
		return true
	}
	return st.Enabled
}

func (s *Service) Status(token string) Status {
	st, _ := s.load()
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials`).Scan(&n)
	return Status{Enabled: st.Enabled, Unlocked: !st.Enabled || s.ValidSession(token), Passkeys: n,
		HasToken: st.TokenHash != "", HasTokenRW: st.TokenRWHash != ""}
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Service) newSession() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	now := s.now().UTC()
	s.DB.Exec(`DELETE FROM auth_sessions WHERE expires_at < ?`, now.Format(time.RFC3339))
	_, err := s.DB.Exec(`INSERT INTO auth_sessions(token_hash,expires_at) VALUES(?,?)`, hash(token), now.Add(IdleTimeout).Format(time.RFC3339))
	return token, err
}

func (s *Service) ValidSession(token string) bool {
	if token == "" {
		return false
	}
	var exp string
	h := hash(token)
	if s.DB.QueryRow(`SELECT expires_at FROM auth_sessions WHERE token_hash=?`, h).Scan(&exp) != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, exp)
	now := s.now().UTC()
	if err != nil || !now.Before(t) {
		s.DB.Exec(`DELETE FROM auth_sessions WHERE token_hash=?`, h)
		return false
	}
	// In use: push the idle expiry forward (written at most once a minute).
	if t.Sub(now) < IdleTimeout-time.Minute {
		s.DB.Exec(`UPDATE auth_sessions SET expires_at=? WHERE token_hash=?`, now.Add(IdleTimeout).Format(time.RFC3339), h)
	}
	return true
}

func (s *Service) DestroySession(token string) {
	s.DB.Exec(`DELETE FROM auth_sessions WHERE token_hash=?`, hash(token))
}

func (s *Service) destroyAll() { s.DB.Exec(`DELETE FROM auth_sessions`) }

func (s *Service) throttled() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.lockedUntil) {
		return ErrTooManyAttempts
	}
	return nil
}

func (s *Service) fail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	if s.failures >= pinAttemptLimit {
		lock := pinLockoutBase << (s.failures - pinAttemptLimit)
		if lock <= 0 || lock > pinLockoutMax {
			lock = pinLockoutMax
		}
		s.lockedUntil = time.Now().Add(lock)
	}
}

func (s *Service) reset() {
	s.mu.Lock()
	s.failures, s.lockedUntil = 0, time.Time{}
	s.mu.Unlock()
}

func (s *Service) checkPin(st settings, pin string) error {
	if err := s.throttled(); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(st.PinHash), []byte(pin)) != nil {
		s.fail()
		return ErrWrongPin
	}
	s.reset()
	return nil
}

// SetupPin enables the lock or changes the PIN (the current one is required
// when a PIN is set). Every session ends.
func (s *Service) SetupPin(current, next string) error {
	if !pinPattern.MatchString(next) {
		return ErrInvalidPin
	}
	st, err := s.load()
	if err != nil {
		return err
	}
	if st.Enabled {
		if err := s.checkPin(st, current); err != nil {
			return err
		}
	}
	h, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	st.PinHash, st.Enabled = string(h), true
	if err := s.save(st); err != nil {
		return err
	}
	s.destroyAll()
	return nil
}

func (s *Service) VerifyPin(pin string) (string, error) {
	st, err := s.load()
	if err != nil {
		return "", err
	}
	if !st.Enabled {
		return "", ErrLockDisabled
	}
	if err := s.checkPin(st, pin); err != nil {
		return "", err
	}
	return s.newSession()
}

// ConfirmPin checks the PIN for a sensitive action without starting a
// session; wrong guesses count toward the same lockout as logging in.
func (s *Service) ConfirmPin(pin string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	if !st.Enabled {
		return ErrLockDisabled
	}
	return s.checkPin(st, pin)
}

func (s *Service) DisableLock(pin string) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	if !st.Enabled {
		return ErrLockDisabled
	}
	if err := s.checkPin(st, pin); err != nil {
		return err
	}
	st.Enabled, st.PinHash = false, ""
	if err := s.save(st); err != nil {
		return err
	}
	s.destroyAll()
	return nil
}

// ── passkeys ────────────────────────────────────────────────────────

// rp is bound to the host the request came in on: credentials are
// origin-bound, so each access hostname enrolls separately.
func rp(host, origin string) (*webauthn.WebAuthn, error) {
	id := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		id = h
	}
	return webauthn.New(&webauthn.Config{RPDisplayName: "Finance Tracker", RPID: id, RPOrigins: []string{origin}})
}

type credRow struct {
	id   int64
	blob []byte
}

func (s *Service) loadOwner() (*owner, []credRow, error) {
	rows, err := s.DB.Query(`SELECT id, credential FROM webauthn_credentials ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	u := &owner{}
	var out []credRow
	for rows.Next() {
		var r credRow
		rows.Scan(&r.id, &r.blob)
		var c webauthn.Credential
		if json.Unmarshal(r.blob, &c) == nil {
			u.creds = append(u.creds, c)
		}
		out = append(out, r)
	}
	return u, out, nil
}

func (s *Service) BeginRegistration(host, origin string) (*protocol.CredentialCreation, error) {
	w, err := rp(host, origin)
	if err != nil {
		return nil, err
	}
	u, _, err := s.loadOwner()
	if err != nil {
		return nil, err
	}
	opts, sess, err := w.BeginRegistration(u, webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
		AuthenticatorAttachment: protocol.Platform, UserVerification: protocol.VerificationRequired,
		ResidentKey: protocol.ResidentKeyRequirementPreferred}))
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pendingReg = sess
	s.mu.Unlock()
	return opts, nil
}

func (s *Service) FinishRegistration(host, origin, name string, r *http.Request) error {
	s.mu.Lock()
	sess := s.pendingReg
	s.pendingReg = nil
	s.mu.Unlock()
	if sess == nil {
		return ErrNoCeremony
	}
	w, err := rp(host, origin)
	if err != nil {
		return err
	}
	u, _, err := s.loadOwner()
	if err != nil {
		return err
	}
	cred, err := w.FinishRegistration(u, *sess, r)
	if err != nil {
		return err
	}
	blob, _ := json.Marshal(cred)
	if strings.TrimSpace(name) == "" {
		name = "Device"
	}
	_, err = s.DB.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES(?,?,?)`, name, blob, db.Now())
	return err
}

func (s *Service) BeginLogin(host, origin string) (*protocol.CredentialAssertion, error) {
	w, err := rp(host, origin)
	if err != nil {
		return nil, err
	}
	u, _, err := s.loadOwner()
	if err != nil {
		return nil, err
	}
	if len(u.creds) == 0 {
		return nil, errors.New("no passkey enrolled")
	}
	// The fingerprint (or device PIN) is required, not just a tap: with a
	// passkey standing in for the web password, presence alone isn't enough.
	opts, sess, err := w.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}
	s.keepLogin(sess)
	return opts, nil
}

type pendingLogin struct {
	sess *webauthn.SessionData
	at   time.Time
}

const (
	maxPendingLogins = 16
	pendingLoginTTL  = 5 * time.Minute
)

// keepLogin stores a challenge, dropping expired ones and, when full, the oldest.
func (s *Service) keepLogin(sess *webauthn.SessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingLogin == nil {
		s.pendingLogin = map[string]pendingLogin{}
	}
	now := time.Now()
	oldest := ""
	for k, p := range s.pendingLogin {
		if now.Sub(p.at) > pendingLoginTTL {
			delete(s.pendingLogin, k)
		} else if oldest == "" || p.at.Before(s.pendingLogin[oldest].at) {
			oldest = k
		}
	}
	if len(s.pendingLogin) >= maxPendingLogins {
		delete(s.pendingLogin, oldest)
	}
	s.pendingLogin[sess.Challenge] = pendingLogin{sess, now}
}

func (s *Service) FinishLogin(host, origin string, r *http.Request) (string, error) {
	s.mu.Lock()
	none := len(s.pendingLogin) == 0
	s.mu.Unlock()
	if none {
		return "", ErrNoCeremony
	}
	parsed, err := protocol.ParseCredentialRequestResponse(r)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	p, ok := s.pendingLogin[parsed.Response.CollectedClientData.Challenge]
	delete(s.pendingLogin, parsed.Response.CollectedClientData.Challenge)
	s.mu.Unlock()
	if !ok || time.Since(p.at) > pendingLoginTTL {
		return "", ErrNoCeremony
	}
	sess := p.sess
	w, err := rp(host, origin)
	if err != nil {
		return "", err
	}
	u, rows, err := s.loadOwner()
	if err != nil {
		return "", err
	}
	cred, err := w.ValidateLogin(u, *sess, parsed)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		var stored webauthn.Credential
		if json.Unmarshal(row.blob, &stored) == nil && string(stored.ID) == string(cred.ID) {
			if blob, err := json.Marshal(cred); err == nil {
				s.DB.Exec(`UPDATE webauthn_credentials SET credential=? WHERE id=?`, blob, row.id)
			}
		}
	}
	return s.newSession()
}

type Passkey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (s *Service) Passkeys() ([]Passkey, error) {
	rows, err := s.DB.Query(`SELECT id,name,created_at FROM webauthn_credentials ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Passkey{}
	for rows.Next() {
		var p Passkey
		rows.Scan(&p.ID, &p.Name, &p.CreatedAt)
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) DeletePasskey(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM webauthn_credentials WHERE id=?`, id)
	return err
}

// ── API tokens ──────────────────────────────────────────────────────

// MintToken creates a read-only (ftk_) or read-write (ftkw_) token, stores
// only its hash and returns the plaintext once.
func (s *Service) MintToken(rw bool) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	prefix := "ftk_"
	if rw {
		prefix = "ftkw_"
	}
	token := prefix + hex.EncodeToString(raw)
	st, err := s.load()
	if err != nil {
		return "", err
	}
	if rw {
		st.TokenRWHash = hash(token)
	} else {
		st.TokenHash = hash(token)
	}
	return token, s.save(st)
}

func (s *Service) RevokeToken(rw bool) error {
	st, err := s.load()
	if err != nil {
		return err
	}
	if rw {
		st.TokenRWHash = ""
	} else {
		st.TokenHash = ""
	}
	return s.save(st)
}

// TokenScope returns "ro", "rw" or "" for a bearer token.
func (s *Service) TokenScope(token string) string {
	st, err := s.load()
	if err != nil || token == "" {
		return ""
	}
	eq := func(stored string) bool {
		return stored != "" && subtle.ConstantTimeCompare([]byte(hash(token)), []byte(stored)) == 1
	}
	switch {
	case strings.HasPrefix(token, "ftkw_") && eq(st.TokenRWHash):
		return "rw"
	case strings.HasPrefix(token, "ftk_") && eq(st.TokenHash):
		return "ro"
	}
	return ""
}
