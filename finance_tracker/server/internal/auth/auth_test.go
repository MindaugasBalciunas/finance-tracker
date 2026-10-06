package auth_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/auth"
	. "ft/internal/testutil"
)

func TestPinLifecycle(t *testing.T) {
	s := &auth.Service{DB: DB(t)}
	if s.Enabled() {
		t.Fatal("lock off by default")
	}
	if _, err := s.VerifyPin("1234"); err != auth.ErrLockDisabled {
		t.Fatal(err)
	}
	if err := s.SetupPin("", "12"); err != auth.ErrInvalidPin {
		t.Fatal("short PIN accepted")
	}
	if err := s.SetupPin("", "1234"); err != nil {
		t.Fatal(err)
	}
	tok, err := s.VerifyPin("1234")
	if err != nil || !s.ValidSession(tok) || !s.Status(tok).Unlocked {
		t.Fatal(err)
	}
	// Changing the PIN needs the current one and ends every session.
	if err := s.SetupPin("0000", "5678"); err != auth.ErrWrongPin {
		t.Fatal(err)
	}
	if err := s.SetupPin("1234", "5678"); err != nil {
		t.Fatal(err)
	}
	if s.ValidSession(tok) {
		t.Fatal("old session survived a PIN change")
	}
	tok, _ = s.VerifyPin("5678")
	s.DestroySession(tok)
	if s.ValidSession(tok) || s.ValidSession("") {
		t.Fatal("logout")
	}
	if err := s.DisableLock("5678"); err != nil || s.Enabled() {
		t.Fatal(err)
	}
}

// Sessions live in the database, so an add-on restart keeps you unlocked.
func TestSessionsSurviveRestart(t *testing.T) {
	d := DB(t)
	s := &auth.Service{DB: d}
	s.SetupPin("", "1234")
	tok, _ := s.VerifyPin("1234")
	again := &auth.Service{DB: d}
	if !again.ValidSession(tok) {
		t.Fatal("session lost on restart")
	}
	var stored string
	d.QueryRow(`SELECT token_hash FROM auth_sessions`).Scan(&stored)
	if stored == tok || strings.Contains(stored, tok) {
		t.Fatal("session tokens must be stored hashed")
	}
}

func TestPinBruteForceThrottle(t *testing.T) {
	s := &auth.Service{DB: DB(t)}
	s.SetupPin("", "1234")
	for i := 0; i < 5; i++ {
		s.VerifyPin("0000")
	}
	if _, err := s.VerifyPin("1234"); err != auth.ErrTooManyAttempts {
		t.Fatal("even the right PIN is refused during a lockout:", err)
	}
}

func TestTokens(t *testing.T) {
	s := &auth.Service{DB: DB(t)}
	ro, _ := s.MintToken(false)
	rw, _ := s.MintToken(true)
	if !strings.HasPrefix(ro, "ftk_") || !strings.HasPrefix(rw, "ftkw_") {
		t.Fatal(ro, rw)
	}
	if s.TokenScope(ro) != "ro" || s.TokenScope(rw) != "rw" || s.TokenScope("ftk_nope") != "" || s.TokenScope("") != "" {
		t.Fatal("scopes")
	}
	// A read-write token string presented as read-only (prefix swap) is rejected.
	if s.TokenScope("ftk_"+strings.TrimPrefix(rw, "ftkw_")) != "" {
		t.Fatal("prefix confusion")
	}
	ro2, _ := s.MintToken(false)
	if s.TokenScope(ro) != "" || s.TokenScope(ro2) != "ro" {
		t.Fatal("minting replaces the old token")
	}
	s.RevokeToken(true)
	if s.TokenScope(rw) != "" {
		t.Fatal("revoke")
	}
	st := s.Status("")
	if !st.HasToken || st.HasTokenRW {
		t.Fatal(st)
	}
}

func TestPasskeysRequireEnrollment(t *testing.T) {
	s := &auth.Service{DB: DB(t)}
	if _, err := s.BeginLogin("localhost", "http://localhost"); err == nil {
		t.Fatal("login without a passkey")
	}
	if _, err := s.FinishLogin("localhost", "http://localhost", nil); err != auth.ErrNoCeremony {
		t.Fatal(err)
	}
	opts, err := s.BeginRegistration("finance.example:8443", "https://finance.example:8443")
	if err != nil || opts.Response.RelyingParty.ID != "finance.example" {
		t.Fatal("RP id is the host without the port", err)
	}
	list, _ := s.Passkeys()
	if len(list) != 0 {
		t.Fatal(list)
	}
}

// With a passkey standing in for the web password, a tap is not enough: the
// login must ask the device for the fingerprint (or its own PIN).
func TestPasskeyLoginRequiresVerification(t *testing.T) {
	d := DB(t)
	d.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('phone',?, 'x')`, `{"id":"AQID","publicKey":"AQID","authenticator":{"AAGUID":"AAAAAAAAAAAAAAAAAAAAAA==","signCount":0}}`)
	s := &auth.Service{DB: d}
	for i := 0; i < 20; i++ { // strangers may start logins; none of this may fail
		opts, err := s.BeginLogin("finance.example:8443", "https://finance.example:8443")
		if err != nil {
			t.Fatal(err)
		}
		if opts.Response.UserVerification != "required" {
			t.Fatalf("user verification %q", opts.Response.UserVerification)
		}
	}
}

// A session ends after 15 minutes without use; using it keeps it alive.
func TestSessionsEndWhenIdle(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s := &auth.Service{DB: d, Clock: func() time.Time { return now }}
	s.SetupPin("", "1234")
	tok, err := s.VerifyPin("1234")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ { // an hour of use, a request every 10 minutes
		now = now.Add(10 * time.Minute)
		if !s.ValidSession(tok) {
			t.Fatalf("active session ended after %d min", (i+1)*10)
		}
	}
	now = now.Add(auth.IdleTimeout)
	if s.ValidSession(tok) {
		t.Fatal("idle session still valid")
	}
	if st := s.Status(tok); st.Unlocked {
		t.Fatal("status should be locked after idle")
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM auth_sessions`).Scan(&n)
	if n != 0 {
		t.Fatal("expired session should be removed")
	}
}
