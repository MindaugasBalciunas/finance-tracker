package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/auth"
	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/mcptools"
	. "ft/internal/testutil"
)

type bearerRT struct{ tok string }

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.tok)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects an MCP client to /mcp with a bearer token.
func mcpSession(t *testing.T, root, tok string) (*mcp.ClientSession, error) {
	t.Helper()
	tr := &mcp.StreamableClientTransport{Endpoint: root + "/mcp", HTTPClient: &http.Client{Transport: bearerRT{tok}}, DisableStandaloneSSE: true, MaxRetries: -1}
	return mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), tr, nil)
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var n []string
	for _, x := range res.Tools {
		n = append(n, x.Name)
	}
	return n
}

// Claude's custom connector end to end: discovery, registration (redirects
// only to Claude or localhost), the owner's consent behind the app lock,
// PKCE, the MCP tools over HTTP within the chosen scope, refresh rotation
// with reuse detection, and revocation.
func TestMCPConnector(t *testing.T) {
	market.HICPFetch = func() ([]byte, error) { return nil, errors.New("offline in tests") }
	s, c := newServer(t)
	s.PublicURL = "https://budget.example"
	root := strings.TrimSuffix(c.base, "/api")
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(10), Category: "food.groceries", AccountID: "swed", Merchant: "Lidl"})

	raw := func(method, path, ctype string, body io.Reader) (int, http.Header, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, root+path, body)
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header, b
	}

	// Discovery starts from a 401 that names the metadata.
	code, hdr, _ := raw("POST", "/mcp", "application/json", strings.NewReader(`{}`))
	if code != 401 || !strings.Contains(hdr.Get("WWW-Authenticate"), `resource_metadata="https://budget.example/.well-known/oauth-protected-resource"`) {
		t.Fatalf("unauthenticated /mcp: %d %q", code, hdr.Get("WWW-Authenticate"))
	}
	var prm, asm map[string]any
	_, _, b := raw("GET", "/.well-known/oauth-protected-resource", "", nil)
	json.Unmarshal(b, &prm)
	_, _, b = raw("GET", "/.well-known/oauth-authorization-server", "", nil)
	json.Unmarshal(b, &asm)
	if prm["resource"] != "https://budget.example/mcp" || asm["token_endpoint"] != "https://budget.example/oauth/token" || asm["authorization_endpoint"] != "https://budget.example/oauth/authorize" {
		t.Fatalf("metadata %v %v", prm, asm)
	}

	reg := func(redirect string) (int, map[string]any) {
		code, _, b := raw("POST", "/oauth/register", "application/json", strings.NewReader(fmt.Sprintf(`{"client_name":"Claude","redirect_uris":[%q]}`, redirect)))
		var m map[string]any
		json.Unmarshal(b, &m)
		return code, m
	}
	for _, bad := range []string{"https://evil.example/cb", "https://claude.ai.evil.example/cb", "http://claude.ai/api/mcp/auth_callback", "javascript:alert(1)"} {
		if code, m := reg(bad); code != 400 || m["error"] != "invalid_redirect_uri" {
			t.Errorf("redirect %s registered: %d %v", bad, code, m)
		}
	}
	code, cl := reg("https://claude.ai/api/mcp/auth_callback")
	if code != 201 {
		t.Fatalf("register: %d %v", code, cl)
	}
	clientID := cl["client_id"].(string)
	const redirect = "https://claude.ai/api/mcp/auth_callback"

	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "state": {"st8"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "response_type": {"code"}}
	req := map[string]any{"client_id": clientID, "redirect_uri": redirect, "state": "st8", "code_challenge": challenge, "code_challenge_method": "S256", "response_type": "code"}

	// Consent is the owner's, behind the lock: no session, no token reaches it.
	if code, _ := c.do("GET", "/oauth/request?"+q.Encode(), nil); code != 401 {
		t.Fatalf("consent without a session: %d", code)
	}
	rwTok, _ := a.MintToken(true)
	if code, _ := c.with("Authorization", "Bearer "+rwTok).do("POST", "/oauth/approve", map[string]any{"allow": true, "scope": "rw"}); code != 403 {
		t.Fatalf("an API token approved a connection: %d", code)
	}
	c.ok("POST", "/auth/pin/login", map[string]string{"pin": "1234"}, nil)
	var info map[string]any
	c.ok("GET", "/oauth/request?"+q.Encode(), nil, &info)
	if info["client_name"] != "Claude" || info["redirect_host"] != "claude.ai" {
		t.Fatalf("request info %v", info)
	}
	other := url.Values{}
	for k, v := range q {
		other[k] = v
	}
	other.Set("redirect_uri", "https://claude.ai/elsewhere")
	if code, _ := c.do("GET", "/oauth/request?"+other.Encode(), nil); code != 400 {
		t.Error("an unregistered redirect passed the check")
	}

	approve := func(scope string) string {
		t.Helper()
		body := map[string]any{"allow": true, "scope": scope}
		for k, v := range req {
			body[k] = v
		}
		var out struct{ Redirect string }
		c.ok("POST", "/oauth/approve", body, &out)
		u, _ := url.Parse(out.Redirect)
		if u.Host != "claude.ai" || u.Query().Get("state") != "st8" || u.Query().Get("code") == "" {
			t.Fatalf("redirect %s", out.Redirect)
		}
		return u.Query().Get("code")
	}
	token := func(form url.Values) (int, map[string]any) {
		code, _, b := raw("POST", "/oauth/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		var m map[string]any
		json.Unmarshal(b, &m)
		return code, m
	}
	exchange := func(code, v string) (int, map[string]any) {
		return token(url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {code}, "code_verifier": {v}, "redirect_uri": {redirect}})
	}

	// Deny goes back to Claude with access_denied.
	var denied struct{ Redirect string }
	body := map[string]any{"allow": false}
	for k, v := range req {
		body[k] = v
	}
	c.ok("POST", "/oauth/approve", body, &denied)
	if !strings.Contains(denied.Redirect, "error=access_denied") || !strings.Contains(denied.Redirect, "state=st8") {
		t.Errorf("deny %s", denied.Redirect)
	}

	// PKCE: a wrong verifier fails, and burns the code.
	cd := approve("ro")
	if code, m := exchange(cd, strings.Repeat("x", 50)); code != 400 || m["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: %d %v", code, m)
	}
	if code, _ := exchange(cd, verifier); code != 400 {
		t.Fatal("a code worked twice")
	}

	// Read-only: reads work, writes aren't even offered.
	code, ro := exchange(approve("ro"), verifier)
	if code != 200 || ro["token_type"] != "Bearer" || ro["scope"] != "read" {
		t.Fatalf("exchange: %d %v", code, ro)
	}
	cs, err := mcpSession(t, root, ro["access_token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	names := toolNames(t, cs)
	if !slices.Contains(names, "search_transactions") || !slices.Contains(names, "ibkr_live") || slices.Contains(names, "create_transaction") || slices.Contains(names, "remember") {
		t.Errorf("read-only tools %v", names)
	}
	// The planning tools: a read-only connection reads them all, but can't
	// write down a decision.
	if slices.Contains(names, "log_decision") || !slices.Contains(names, "get_next_euro") || !slices.Contains(names, "get_ownership_costs") {
		t.Errorf("read-only planning tools %v", names)
	}
	for _, tool := range []string{"get_payday_runbook", "get_lifestyle_creep", "get_price_creep", "get_decisions", "get_ownership_costs"} {
		if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool}); err != nil || res.IsError {
			t.Errorf("%s over a read-only connection: %v %+v", tool, err, res)
		}
	}
	if res, _ := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "log_decision", Arguments: map[string]any{"title": "TV", "amount": 900}}); res != nil && !res.IsError {
		t.Error("a read-only connection logged a decision")
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_transactions", Arguments: map[string]any{"merchant": "Lidl"}})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "Lidl") {
		t.Fatalf("search: %v %+v", err, res)
	}
	// IBKR isn't connected here: the tool says so instead of failing silently.
	res, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "ibkr_live", Arguments: map[string]any{"what": "positions"}})
	if res == nil || !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "not connected") {
		t.Errorf("ibkr_live unconnected: %+v", res)
	}
	cs.Close()
	roAPI := c.with("Authorization", "Bearer "+ro["access_token"].(string))
	if code, _ := roAPI.do("GET", "/transactions", nil); code != 200 {
		t.Errorf("a read-only connection can't read the API: %d", code)
	}
	if code, _ := roAPI.do("POST", "/rules", map[string]any{"pattern": "x", "add_tags": []string{"y"}}); code != 403 {
		t.Errorf("a read-only connection wrote: %d", code)
	}
	if code, _ := roAPI.do("GET", "/ai/settings", nil); code != 403 {
		t.Errorf("a connection read settings: %d", code)
	}

	// Read and write: the write tools appear and work.
	_, rw := exchange(approve("rw"), verifier)
	cs, err = mcpSession(t, root, rw["access_token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if n := toolNames(t, cs); !slices.Contains(n, "create_transaction") {
		t.Errorf("read-write tools %v", n)
	}
	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_transaction",
		Arguments: map[string]any{"date": "2026-09-03", "amount": 7.5, "category": "food.coffee", "merchant": "Caffeine", "account_id": "swed"}})
	if err != nil || res.IsError {
		t.Fatalf("create: %v %+v", err, res)
	}
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM transactions WHERE merchant='Caffeine' AND amount=750`).Scan(&n)
	defer func() {
		var d int
		s.DB.QueryRow(`SELECT COUNT(*) FROM decisions WHERE title='Road bike' AND amount=120000 AND status='thinking'`).Scan(&d)
		if d != 1 {
			t.Errorf("log_decision wrote %d rows", d)
		}
	}()
	if n != 1 {
		t.Errorf("created %d rows", n)
	}
	// The read views added for parity with Ask CFO answer through the
	// connector (in process, within the scope), with their arguments.
	for _, c := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"get_forecast", map[string]any{"everyday_pct": -20}, `"scenario"`},
		{"get_account_history", map[string]any{"account_id": "swed", "from": "2026-01-01"}, `[`},
		{"get_checks", nil, `[`},
		{"get_month_review", map[string]any{"month": "2026-09"}, `"month":"2026-09"`},
		{"get_bank_inbox", nil, `[`},
		{"get_tax", map[string]any{"year": 2025}, `"year":2025`},
		{"get_fun_facts", map[string]any{"from": "2026-01-01", "to": "2026-12-31"}, `{`},
		{"get_net_worth_movement", map[string]any{"from": "2026-01-01"}, `[`},
		{"set_trip_dates", map[string]any{"tag": "trip:rome", "start": "2026-06-23", "end": "2026-06-27"}, `"trip:rome"`},
		{"get_trip_untagged", map[string]any{"tag": "trip:rome"}, `"in_dates"`},
		{"get_next_euro", map[string]any{"amount": 750}, `"sum":750`},
		{"get_decision_impact", map[string]any{"amount": 300}, `"lines"`},
		{"log_decision", map[string]any{"title": "Road bike", "amount": 1200, "category": "leisure.hobbies"}, `"status":"thinking"`},
	} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: c.tool, Arguments: c.args})
		if err != nil || res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, c.want) {
			t.Errorf("%s: %v %+v", c.tool, err, res)
		}
	}
	cs.Close()

	// Refresh rotates; the old access token stops; replaying the old
	// refresh token ends the whole connection.
	code, rw2 := token(url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {rw["refresh_token"].(string)}})
	if code != 200 || rw2["access_token"] == rw["access_token"] {
		t.Fatalf("refresh: %d %v", code, rw2)
	}
	if _, err := mcpSession(t, root, rw["access_token"].(string)); err == nil {
		t.Error("the replaced access token still works")
	}
	if _, err := mcpSession(t, root, rw2["access_token"].(string)); err != nil {
		t.Fatalf("the new access token: %v", err)
	}
	if code, m := token(url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {rw["refresh_token"].(string)}}); code != 400 || !strings.Contains(fmt.Sprint(m["error_description"]), "already used") {
		t.Fatalf("refresh replay: %d %v", code, m)
	}
	if _, err := mcpSession(t, root, rw2["access_token"].(string)); err == nil {
		t.Error("the connection survived a refresh replay")
	}

	// Settings lists the connections and revokes them.
	var g struct {
		Grants []struct {
			ID    int64  `json:"id"`
			Scope string `json:"scope"`
		} `json:"grants"`
		MCPURL string `json:"mcp_url"`
	}
	c.ok("GET", "/oauth/grants", nil, &g)
	if len(g.Grants) != 1 || g.Grants[0].Scope != "ro" || g.MCPURL != "https://budget.example/mcp" {
		t.Fatalf("grants %+v", g)
	}
	c.ok("DELETE", fmt.Sprintf("/oauth/grants/%d", g.Grants[0].ID), nil, nil)
	if _, err := mcpSession(t, root, ro["access_token"].(string)); err == nil {
		t.Error("a revoked connection still works")
	}
}

// /mcp skips the web password, so the plain API tokens (ftk_/ftkw_) don't
// work there — only a connection's own token — and with the app lock off a
// read-only connection is still offered no write tools.
func TestMCPRefusesAPITokens(t *testing.T) {
	s, c := newServer(t)
	root := strings.TrimSuffix(c.base, "/api")
	a := &auth.Service{DB: s.DB}
	for _, rw := range []bool{false, true} {
		tok, _ := a.MintToken(rw)
		if _, err := mcpSession(t, root, tok); err == nil {
			t.Errorf("API token (rw=%v) connected to /mcp", rw)
		}
	}
	if _, err := mcpSession(t, root, "ftoa_forged"); err == nil {
		t.Error("a forged token connected")
	}
}

// Every tool that says WRITES is hidden from read-only connections.
func TestWriteToolsListed(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := mcptools.New(nil).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, _ := cs.ListTools(ctx, nil)
	var writes []string
	for _, x := range res.Tools {
		if strings.HasPrefix(x.Description, "WRITES") {
			writes = append(writes, x.Name)
		}
	}
	slices.Sort(writes)
	want := slices.Clone(mcptools.WriteTools)
	slices.Sort(want)
	if !slices.Equal(writes, want) {
		t.Errorf("WRITES tools %v, WriteTools %v", writes, want)
	}
}

// One account's balance points: from a date, and only the latest when capped.
func TestBalancesFromAndLimit(t *testing.T) {
	s, c := newServer(t)
	for _, d := range []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"} {
		Bal(t, s.DB, "swed", d, 100)
	}
	var pts []struct{ Date string }
	c.ok("GET", "/balances?account=swed&from=2026-02-01", nil, &pts)
	if len(pts) != 3 || pts[0].Date != "2026-02-28" {
		t.Errorf("from: %+v", pts)
	}
	c.ok("GET", "/balances?account=swed&limit=2", nil, &pts)
	if len(pts) != 2 || pts[0].Date != "2026-03-31" || pts[1].Date != "2026-04-30" {
		t.Errorf("limit: %+v", pts)
	}
}
