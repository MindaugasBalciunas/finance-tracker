package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ft/internal/auth"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// The sync token downloads the plain backup at /sync/backup.json — a path
// nginx lets past the web password — and nothing else. Neither a session nor
// an API token opens that path, the sync token opens no /api route, a
// revoked or forged one is refused, and the download never carries secrets.
func TestSyncTokenDownloadsThePlainBackupAndNothingElse(t *testing.T) {
	s, c := newServer(t)
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	ro, _ := a.MintToken(false)
	s.DB.Exec(`INSERT INTO settings(key,value) VALUES('ai','{"api_key":"sk-ant-supersecretkey123","model":"m","enabled":true}')`)
	Tx(t, s.DB, ledger.Tx{Date: "2026-11-01", Amount: E(12), Category: "food", AccountID: "swed"})
	root := strings.TrimSuffix(c.base, "/api")
	get := func(path, bearer string, cookies []*http.Cookie) (int, string, http.Header) {
		t.Helper()
		req, _ := http.NewRequest("GET", root+path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}

	// Minting needs the app unlocked.
	if code, _ := c.do("POST", "/auth/sync-token", nil); code != 401 {
		t.Fatalf("minting while locked: %d", code)
	}
	c.ok("POST", "/auth/pin/login", map[string]string{"pin": "1234"}, nil)
	var minted struct{ Token, Base string }
	c.ok("POST", "/auth/sync-token", nil, &minted)
	tok := minted.Token
	if !strings.HasPrefix(tok, "ftsy_") || !strings.HasPrefix(minted.Base, "http") {
		t.Fatalf("token %q, address %q", tok, minted.Base)
	}
	var st struct {
		HasTokenSync bool `json:"has_token_sync"`
	}
	c.ok("GET", "/auth/status", nil, &st)
	if !st.HasTokenSync {
		t.Error("status should say a sync token exists")
	}

	code, body, hdr := get("/sync/backup.json?secrets=1", tok, nil)
	if code != 200 {
		t.Fatalf("sync download: %d %s", code, body)
	}
	var f struct {
		Format         string
		IncludeSecrets bool `json:"include_secrets"`
		Counts         map[string]int
	}
	json.Unmarshal([]byte(body), &f)
	if f.Format != "finance-tracker-backup" || f.IncludeSecrets || f.Counts["transactions"] != 1 {
		t.Fatalf("backup %+v", f)
	}
	if strings.Contains(body, "supersecretkey") || strings.Contains(body, "pin_hash") {
		t.Fatal("the sync download leaked a secret")
	}
	if hdr.Get("Cache-Control") != "no-store" {
		t.Error("the download must not be cached")
	}
	var marker int
	s.DB.QueryRow(`SELECT COUNT(*) FROM settings WHERE key='last_backup_download'`).Scan(&marker)
	if marker != 0 {
		t.Error("a sync must not count as the owner's off-device backup")
	}

	// Everything else is refused at that path …
	u, _ := url.Parse(c.base)
	session := c.http.Jar.Cookies(u)
	for name, cred := range map[string]struct {
		bearer  string
		cookies []*http.Cookie
	}{
		"no credential": {}, "a session": {cookies: session}, "a read-only API token": {bearer: ro},
		"a forged sync token": {bearer: "ftsy_" + strings.Repeat("0", 64)},
	} {
		code, _, hdr := get("/sync/backup.json", cred.bearer, cred.cookies)
		if code != 401 || !strings.HasPrefix(hdr.Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("%s at /sync/backup.json: %d", name, code)
		}
	}
	// … and the sync token opens nothing under /api, nor a neighbouring path.
	for _, p := range []string{"/api/overview", "/api/export/backup.json", "/api/transactions"} {
		if code, _, _ := get(p, tok, nil); code != 401 {
			t.Errorf("sync token at %s: %d", p, code)
		}
	}
	if code, _, _ := get("/sync/other", tok, nil); code != 404 {
		t.Errorf("sync token at a neighbouring path: %d", code)
	}
	// With the lock off the path still demands the token.
	if err := a.DisableLock("1234"); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := get("/sync/backup.json", "", nil); code != 401 {
		t.Errorf("lock off, no token: %d", code)
	}

	// Revoked: refused.
	c.ok("DELETE", "/auth/sync-token", nil, nil)
	if code, _, _ := get("/sync/backup.json", tok, nil); code != 401 {
		t.Errorf("revoked token: %d", code)
	}
}
