package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 7 * 24 * time.Hour

// PIN brute-force throttle: after pinAttemptLimit consecutive failures every
// further failure extends a lockout window, doubling from pinLockoutBase up
// to pinLockoutMax. A 4-digit PIN is only 10,000 guesses without this.
const (
	pinAttemptLimit = 5
	pinLockoutBase  = 30 * time.Second
	pinLockoutMax   = 15 * time.Minute
)

var pinPattern = regexp.MustCompile(`^\d{4,8}$`)

var (
	ErrInvalidPin      = errors.New("invalid PIN")
	ErrWrongPin        = errors.New("wrong PIN")
	ErrLockDisabled    = errors.New("app lock is not enabled")
	ErrNoCeremony      = errors.New("no pending WebAuthn ceremony — call begin first")
	ErrTooManyAttempts = errors.New("too many failed attempts — try again later")
)

// webauthnUser adapts the single app owner to the go-webauthn User interface.
type webauthnUser struct {
	creds []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return []byte("finance-tracker-owner") }
func (u *webauthnUser) WebAuthnName() string                       { return "owner" }
func (u *webauthnUser) WebAuthnDisplayName() string                { return "Finance Tracker" }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

type AuthStatus struct {
	Enabled             bool `json:"enabled"`
	Unlocked            bool `json:"unlocked"`
	WebauthnRegistered  bool `json:"webauthn_registered"`
	WebauthnCredentials int  `json:"webauthn_credentials"`
	HasAPIToken         bool `json:"has_api_token"`
	HasAPITokenRW       bool `json:"has_api_token_rw"`
}

type AuthService struct {
	repo repository.AuthRepository

	mu       sync.Mutex
	sessions map[string]time.Time

	// Single-user app: one pending ceremony of each kind is enough.
	pendingRegistration *webauthn.SessionData
	pendingLogin        *webauthn.SessionData

	pinFailures    int
	pinLockedUntil time.Time
}

func NewAuthService(repo repository.AuthRepository) *AuthService {
	return &AuthService{
		repo:     repo,
		sessions: make(map[string]time.Time),
	}
}

// rp builds a WebAuthn relying party bound to the host the request came in
// on, so the same backend works via LAN hostname, IP, or a remote domain.
// Credentials are origin-bound: each access hostname enrolls separately.
func (s *AuthService) rp(host, origin string) (*webauthn.WebAuthn, error) {
	rpID := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		rpID = h
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "Finance Tracker",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

// Enabled fails CLOSED: if the lock state cannot be read (locked WAL,
// corrupt row), the app is treated as locked rather than exposing every
// route on a DB hiccup. Valid in-memory sessions keep working either way.
func (s *AuthService) Enabled() bool {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return true
	}
	return settings.Enabled
}

func (s *AuthService) Status(sessionToken string) AuthStatus {
	settings, _ := s.repo.GetSettings()
	creds, _ := s.repo.ListCredentials()
	enabled := settings != nil && settings.Enabled
	return AuthStatus{
		Enabled:             enabled,
		Unlocked:            !enabled || s.ValidSession(sessionToken),
		WebauthnRegistered:  len(creds) > 0,
		WebauthnCredentials: len(creds),
		HasAPIToken:         settings != nil && settings.APITokenHash != "",
		HasAPITokenRW:       settings != nil && settings.APITokenRWHash != "",
	}
}

// ── Sessions ────────────────────────────────────────────────────────

func (s *AuthService) newSession() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	// Opportunistic sweep — the map is tiny (single user), but without this
	// expired tokens only ever left the map when re-presented.
	now := time.Now()
	for t, expiry := range s.sessions {
		if now.After(expiry) {
			delete(s.sessions, t)
		}
	}
	s.sessions[token] = now.Add(sessionTTL)
	s.mu.Unlock()
	return token, nil
}

// ── PIN throttle ────────────────────────────────────────────────────

func (s *AuthService) pinThrottled() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.pinLockedUntil) {
		return ErrTooManyAttempts
	}
	return nil
}

func (s *AuthService) recordPinFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pinFailures++
	if s.pinFailures >= pinAttemptLimit {
		lockout := pinLockoutBase << (s.pinFailures - pinAttemptLimit)
		if lockout <= 0 || lockout > pinLockoutMax {
			lockout = pinLockoutMax
		}
		s.pinLockedUntil = time.Now().Add(lockout)
	}
}

func (s *AuthService) resetPinFailures() {
	s.mu.Lock()
	s.pinFailures = 0
	s.pinLockedUntil = time.Time{}
	s.mu.Unlock()
}

func (s *AuthService) ValidSession(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func (s *AuthService) DestroySession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *AuthService) destroyAllSessions() {
	s.mu.Lock()
	s.sessions = make(map[string]time.Time)
	s.mu.Unlock()
}

// ── PIN ─────────────────────────────────────────────────────────────

// SetupPin enables the lock (or changes the PIN). When a PIN is already set,
// the current one must be provided.
func (s *AuthService) SetupPin(currentPin, newPin string) error {
	if !pinPattern.MatchString(newPin) {
		return ErrInvalidPin
	}
	settings, err := s.repo.GetSettings()
	if err != nil {
		return err
	}
	if settings.Enabled {
		if err := s.pinThrottled(); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(currentPin)) != nil {
			s.recordPinFailure()
			return ErrWrongPin
		}
		s.resetPinFailures()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	settings.PinHash = string(hash)
	settings.Enabled = true
	if err := s.repo.SaveSettings(settings); err != nil {
		return err
	}
	s.destroyAllSessions()
	return nil
}

// VerifyPin checks the PIN and returns a fresh session token.
func (s *AuthService) VerifyPin(pin string) (string, error) {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return "", err
	}
	if !settings.Enabled {
		return "", ErrLockDisabled
	}
	if err := s.pinThrottled(); err != nil {
		return "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(pin)) != nil {
		s.recordPinFailure()
		return "", ErrWrongPin
	}
	s.resetPinFailures()
	return s.newSession()
}

// DisableLock turns the lock off after verifying the PIN.
func (s *AuthService) DisableLock(pin string) error {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return ErrLockDisabled
	}
	if err := s.pinThrottled(); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(pin)) != nil {
		s.recordPinFailure()
		return ErrWrongPin
	}
	s.resetPinFailures()
	settings.Enabled = false
	settings.PinHash = ""
	if err := s.repo.SaveSettings(settings); err != nil {
		return err
	}
	s.destroyAllSessions()
	return nil
}

// ── WebAuthn ────────────────────────────────────────────────────────

func (s *AuthService) loadUser() (*webauthnUser, []domain.WebauthnCredential, error) {
	rows, err := s.repo.ListCredentials()
	if err != nil {
		return nil, nil, err
	}
	user := &webauthnUser{}
	for _, row := range rows {
		var cred webauthn.Credential
		if json.Unmarshal(row.Credential, &cred) == nil {
			user.creds = append(user.creds, cred)
		}
	}
	return user, rows, nil
}

func (s *AuthService) BeginRegistration(host, origin string) (*protocol.CredentialCreation, error) {
	rp, err := s.rp(host, origin)
	if err != nil {
		return nil, err
	}
	user, _, err := s.loadUser()
	if err != nil {
		return nil, err
	}
	options, session, err := rp.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,
			UserVerification:        protocol.VerificationRequired,
			ResidentKey:             protocol.ResidentKeyRequirementPreferred,
		}),
	)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pendingRegistration = session
	s.mu.Unlock()
	return options, nil
}

func (s *AuthService) FinishRegistration(host, origin, name string, r *http.Request) error {
	s.mu.Lock()
	session := s.pendingRegistration
	s.pendingRegistration = nil
	s.mu.Unlock()
	if session == nil {
		return ErrNoCeremony
	}
	rp, err := s.rp(host, origin)
	if err != nil {
		return err
	}
	user, _, err := s.loadUser()
	if err != nil {
		return err
	}
	cred, err := rp.FinishRegistration(user, *session, r)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if name == "" {
		name = "Device"
	}
	return s.repo.SaveCredential(&domain.WebauthnCredential{Name: name, Credential: blob})
}

func (s *AuthService) BeginLogin(host, origin string) (*protocol.CredentialAssertion, error) {
	rp, err := s.rp(host, origin)
	if err != nil {
		return nil, err
	}
	user, _, err := s.loadUser()
	if err != nil {
		return nil, err
	}
	if len(user.creds) == 0 {
		return nil, errors.New("no fingerprint enrolled")
	}
	options, session, err := rp.BeginLogin(user)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pendingLogin = session
	s.mu.Unlock()
	return options, nil
}

// FinishLogin verifies the assertion and returns a fresh session token.
func (s *AuthService) FinishLogin(host, origin string, r *http.Request) (string, error) {
	s.mu.Lock()
	session := s.pendingLogin
	s.pendingLogin = nil
	s.mu.Unlock()
	if session == nil {
		return "", ErrNoCeremony
	}
	rp, err := s.rp(host, origin)
	if err != nil {
		return "", err
	}
	user, rows, err := s.loadUser()
	if err != nil {
		return "", err
	}
	cred, err := rp.FinishLogin(user, *session, r)
	if err != nil {
		return "", err
	}
	// Persist the updated sign counter for clone detection.
	for _, row := range rows {
		var stored webauthn.Credential
		if json.Unmarshal(row.Credential, &stored) == nil && string(stored.ID) == string(cred.ID) {
			if blob, err := json.Marshal(cred); err == nil {
				row.Credential = blob
				_ = s.repo.SaveCredential(&row)
			}
			break
		}
	}
	return s.newSession()
}

func (s *AuthService) ListCredentials() ([]domain.WebauthnCredential, error) {
	return s.repo.ListCredentials()
}

func (s *AuthService) DeleteCredential(id uint) error {
	return s.repo.DeleteCredential(id)
}

// ── API token (read-only machine access, e.g. the MCP server) ───────

// GenerateAPIToken mints a fresh 256-bit bearer token, stores only its
// SHA-256 hash, and returns the plaintext exactly once. Any previous token
// is invalidated.
func (s *AuthService) GenerateAPIToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "ftk_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))

	settings, err := s.repo.GetSettings()
	if err != nil {
		return "", err
	}
	settings.APITokenHash = hex.EncodeToString(sum[:])
	if err := s.repo.SaveSettings(settings); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeAPIToken deletes the stored hash — the old token stops working
// immediately.
func (s *AuthService) RevokeAPIToken() error {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return err
	}
	settings.APITokenHash = ""
	return s.repo.SaveSettings(settings)
}

// ValidAPIToken reports whether the presented bearer token matches the
// stored hash (constant-time). No hash stored = no token access at all.
// The ftkw_ (read-write) prefix is intentionally NOT accepted here — a
// read-write token must never be treated as the read-only one, or vice versa.
func (s *AuthService) ValidAPIToken(token string) bool {
	if token == "" || !strings.HasPrefix(token, "ftk_") {
		return false
	}
	settings, err := s.repo.GetSettings()
	if err != nil || settings.APITokenHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(settings.APITokenHash)) == 1
}

// ── API token (read-WRITE machine access — opt-in) ──────────────────

// GenerateAPITokenRW mints a fresh read-write bearer token (ftkw_ prefix),
// stores only its hash, and returns the plaintext once. Separate from the
// read-only token so the two are independently revocable.
func (s *AuthService) GenerateAPITokenRW() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "ftkw_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	settings, err := s.repo.GetSettings()
	if err != nil {
		return "", err
	}
	settings.APITokenRWHash = hex.EncodeToString(sum[:])
	if err := s.repo.SaveSettings(settings); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeAPITokenRW deletes the stored read-write hash.
func (s *AuthService) RevokeAPITokenRW() error {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return err
	}
	settings.APITokenRWHash = ""
	return s.repo.SaveSettings(settings)
}

// ValidAPITokenRW reports whether the presented bearer token is the stored
// read-write token (constant-time; ftkw_ prefix required).
func (s *AuthService) ValidAPITokenRW(token string) bool {
	if token == "" || !strings.HasPrefix(token, "ftkw_") {
		return false
	}
	settings, err := s.repo.GetSettings()
	if err != nil || settings.APITokenRWHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(settings.APITokenRWHash)) == 1
}
