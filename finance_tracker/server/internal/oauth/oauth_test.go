package oauth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"ft/internal/oauth"
	. "ft/internal/testutil"
)

func TestAllowedRedirect(t *testing.T) {
	for u, want := range map[string]bool{
		"https://claude.ai/api/mcp/auth_callback":  true,
		"https://claude.com/api/mcp/auth_callback": true,
		"http://localhost:6274/callback":           true,
		"http://127.0.0.1:33418/cb":                true,
		"http://[::1]:9/cb":                        true,
		"http://claude.ai/api/mcp/auth_callback":   false, // not https
		"https://claude.ai.evil.example/cb":        false,
		"https://evil.example/claude.ai":           false,
		"https://user@claude.ai/cb":                false,
		"https://claude.ai/cb#x":                   false,
		"http://localhost.evil.example/cb":         false,
		"https://localhost/cb":                     false,
		"javascript:alert(1)":                      false,
		"":                                         false,
	} {
		if got := oauth.AllowedRedirect(u); got != want {
			t.Errorf("%q: %v, want %v", u, got, want)
		}
	}
}

// Codes and tokens expire, are bound to their app and redirect, and an
// app that never connected is dropped after an hour.
func TestOAuthExpiryAndBinding(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	s := &oauth.Server{DB: d, Now: func() time.Time { return now }}
	const cb = "https://claude.ai/api/mcp/auth_callback"
	if _, err := oauth.Register(d, "x", nil); err == nil {
		t.Error("no redirect accepted")
	}
	if _, err := oauth.Register(d, "x", []string{cb, cb, cb, cb, cb, cb}); err == nil {
		t.Error("six redirects accepted")
	}
	c, err := oauth.Register(d, "Claude", []string{cb})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := oauth.Register(d, "Other", []string{"http://localhost:1/cb"})
	v := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(v))
	req := oauth.Request{ClientID: c.ID, RedirectURI: cb, CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]), Method: "S256", ResponseType: "code"}
	code := func() string {
		to, err := s.Approve(req, oauth.ReadOnly)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(to)
		return u.Query().Get("code")
	}
	if _, err := s.Approve(oauth.Request{ClientID: c.ID, RedirectURI: "http://localhost:1/cb", CodeChallenge: req.CodeChallenge, Method: "S256", ResponseType: "code"}, oauth.ReadOnly); err == nil {
		t.Error("approved a redirect the app didn't register")
	}
	if _, err := s.Approve(oauth.Request{ClientID: c.ID, RedirectURI: cb, CodeChallenge: req.CodeChallenge, Method: "plain", ResponseType: "code"}, oauth.ReadOnly); err == nil {
		t.Error("plain PKCE accepted")
	}
	if _, err := s.Exchange(other.ID, code(), v, cb); err == nil {
		t.Error("another app redeemed the code")
	}
	if _, err := s.Exchange(c.ID, code(), v, "http://localhost:1/cb"); err == nil {
		t.Error("redeemed with another redirect")
	}
	cd := code()
	now = now.Add(3 * time.Minute)
	if _, err := s.Exchange(c.ID, cd, v, cb); err == nil {
		t.Error("an expired code worked")
	}
	tok, err := s.Exchange(c.ID, code(), v, cb)
	if err != nil {
		t.Fatal(err)
	}
	if s.Scope(tok.AccessToken) != oauth.ReadOnly {
		t.Fatal("fresh access token refused")
	}
	if _, err := s.Refresh(other.ID, tok.RefreshToken); err == nil {
		t.Error("another app refreshed")
	}
	now = now.Add(oauth.AccessTTL + time.Second)
	if s.Scope(tok.AccessToken) != "" {
		t.Error("an expired access token worked")
	}
	tok, err = s.Refresh(c.ID, tok.RefreshToken)
	if err != nil || s.Scope(tok.AccessToken) != oauth.ReadOnly {
		t.Fatalf("refresh after the access token expired: %v", err)
	}
	now = now.Add(oauth.RefreshTTL + time.Second)
	if _, err := s.Refresh(c.ID, tok.RefreshToken); err == nil {
		t.Error("an expired refresh token worked")
	}
	if g, _ := oauth.Grants(d); len(g) != 0 {
		t.Errorf("expired connection kept: %v", g)
	}
}
