package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 7 * 24 * time.Hour

var pinPattern = regexp.MustCompile(`^\d{4,8}$`)

var (
	ErrInvalidPin   = errors.New("invalid PIN")
	ErrWrongPin     = errors.New("wrong PIN")
	ErrLockDisabled = errors.New("app lock is not enabled")
	ErrNoCeremony   = errors.New("no pending WebAuthn ceremony — call begin first")
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
}

type AuthService struct {
	repo repository.AuthRepository

	mu       sync.Mutex
	sessions map[string]time.Time

	// Single-user app: one pending ceremony of each kind is enough.
	pendingRegistration *webauthn.SessionData
	pendingLogin        *webauthn.SessionData
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

func (s *AuthService) Enabled() bool {
	settings, err := s.repo.GetSettings()
	return err == nil && settings.Enabled
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
	}
}

// ── Sessions ────────────────────────────────────────────────────────

func (s *AuthService) newSession() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return token
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
		if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(currentPin)) != nil {
			return ErrWrongPin
		}
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
	if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(pin)) != nil {
		return "", ErrWrongPin
	}
	return s.newSession(), nil
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
	if bcrypt.CompareHashAndPassword([]byte(settings.PinHash), []byte(pin)) != nil {
		return ErrWrongPin
	}
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
	return s.newSession(), nil
}

func (s *AuthService) ListCredentials() ([]domain.WebauthnCredential, error) {
	return s.repo.ListCredentials()
}

func (s *AuthService) DeleteCredential(id uint) error {
	return s.repo.DeleteCredential(id)
}
