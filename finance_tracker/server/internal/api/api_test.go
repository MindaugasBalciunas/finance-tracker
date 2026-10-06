package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ft/internal/api"
	"ft/internal/auth"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
	hdr  map[string]string
}

func newServer(t *testing.T) (*api.Server, *client) {
	d := DB(t)
	s := api.New(d, filepath.Join(t.TempDir(), "finance-v2.db"), "test")
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return s, &client{t: t, base: srv.URL + "/api", http: &http.Client{Jar: jar}, hdr: map[string]string{}}
}

func (c *client) with(k, v string) *client {
	h := map[string]string{}
	for a, b := range c.hdr {
		h[a] = b
	}
	h[k] = v
	jar, _ := cookiejar.New(nil)
	return &client{t: c.t, base: c.base, http: &http.Client{Jar: jar}, hdr: h}
}

func (c *client) do(method, path string, body any) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func (c *client) ok(method, path string, body any, into any) {
	c.t.Helper()
	code, out := c.do(method, path, body)
	if code != 200 {
		c.t.Fatalf("%s %s → %d %s", method, path, code, out)
	}
	if into != nil {
		if err := json.Unmarshal(out, into); err != nil {
			c.t.Fatalf("%s %s: %v %s", method, path, err, out)
		}
	}
}

func TestAuthBoundaries(t *testing.T) {
	s, c := newServer(t)
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	ro, _ := a.MintToken(false)
	rw, _ := a.MintToken(true)
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(10), Category: "food", AccountID: "swed"})

	if code, _ := c.do("GET", "/health", nil); code != 200 {
		t.Fatal("health is exempt")
	}
	if code, _ := c.do("GET", "/overview", nil); code != 401 {
		t.Fatal("locked without a session")
	}
	for _, p := range [][2]string{{"POST", "/usage"}, {"GET", "/usage/events"}, {"GET", "/balances/table"}, {"GET", "/portfolio/history"}} {
		if code, _ := c.do(p[0], p[1], map[string]any{"events": []any{}}); code != 401 {
			t.Errorf("%s %s without a session: %d", p[0], p[1], code)
		}
	}
	if code, _ := c.do("POST", "/auth/pin/login", map[string]string{"pin": "0000"}); code != 401 {
		t.Fatal("wrong PIN")
	}
	c.ok("POST", "/auth/pin/login", map[string]string{"pin": "1234"}, nil)
	c.ok("GET", "/overview", nil, nil)

	tokRO := c.with("Authorization", "Bearer "+ro)
	tokRW := c.with("Authorization", "Bearer "+rw)
	bad := c.with("Authorization", "Bearer ftk_forged")
	checks := []struct {
		name   string
		c      *client
		method string
		path   string
		body   any
		want   int
	}{
		{"ro reads data", tokRO, "GET", "/transactions", nil, 200},
		{"ro reads inbox", tokRO, "GET", "/bank/inbox", nil, 200},
		{"ro cannot read AI settings", tokRO, "GET", "/ai/settings", nil, 403},
		{"ro cannot read bank settings", tokRO, "GET", "/bank/settings", nil, 403},
		{"ro cannot export", tokRO, "GET", "/export/backup.json", nil, 403},
		{"ro cannot write", tokRO, "POST", "/rules", map[string]any{"pattern": "x", "add_tags": []string{"y"}}, 403},
		{"rw edits a transaction", tokRW, "PUT", "/transactions/1", map[string]any{"date": "2026-09-01", "amount": 11, "category": "food", "account_id": "swed"}, 200},
		{"rw creates a transaction", tokRW, "POST", "/transactions", map[string]any{"date": "2026-09-02", "amount": 5, "category": "food", "account_id": "swed"}, 200},
		{"rw adds a rule", tokRW, "POST", "/rules", map[string]any{"pattern": "x", "add_tags": []string{"y"}}, 200},
		{"rw cannot bulk delete", tokRW, "POST", "/transactions/bulk", map[string]any{"ids": []int{1}, "delete": true}, 403},
		{"rw cannot accept bank rows", tokRW, "POST", "/bank/inbox/commit", map[string]any{"ids": []int{1}}, 403},
		{"rw cannot change AI settings", tokRW, "PUT", "/ai/settings", map[string]any{"model": "x"}, 403},
		{"rw cannot restore", tokRW, "POST", "/import/backup?confirm=replace", map[string]any{}, 403},
		// Routes added in 2.1–2.4 stay outside every token's scope.
		{"ro reads recurring", tokRO, "GET", "/insights/recurring", nil, 200},
		{"ro cannot read prefs", tokRO, "GET", "/prefs", nil, 403},
		{"rw cannot change prefs", tokRW, "PUT", "/prefs", map[string]any{"liquid_only": true}, 403},
		{"rw cannot add recurring", tokRW, "POST", "/recurring", map[string]any{"merchant": "x"}, 403},
		{"rw cannot hide accounts", tokRW, "PUT", "/accounts/swed", map[string]any{"name": "Swedbank", "kind": "checking", "archived": true}, 403},
		{"rw cannot set balances", tokRW, "POST", "/balances", map[string]any{"date": "2026-09-01", "values": []any{}}, 403},
		{"ro cannot download the AI export", tokRO, "GET", "/export/ai.zip", nil, 403},
		{"rw cannot run AI assist", tokRW, "POST", "/ai/assist", map[string]any{"text": "x"}, 403},
		{"ro reads the balance table (same data as /balances)", tokRO, "GET", "/balances/table?page=1", nil, 200},
		{"ro reads portfolio history (same data as /portfolio)", tokRO, "GET", "/portfolio/history?range=1y", nil, 200},
		{"ro cannot read the usage summary", tokRO, "GET", "/usage/summary", nil, 403},
		{"rw cannot clear usage", tokRW, "DELETE", "/usage", nil, 403},
		{"ro cannot read usage analytics", tokRO, "GET", "/usage/events", nil, 403},
		{"rw cannot write usage analytics", tokRW, "POST", "/usage", map[string]any{"events": []any{}}, 403},
		{"forged token", bad, "GET", "/transactions", nil, 401},
		{"token in X-API-Token (behind basic auth)", c.with("X-API-Token", ro).with("Authorization", "Basic dTpw"), "GET", "/overview", nil, 200},
	}
	for _, ck := range checks {
		if code, out := ck.c.do(ck.method, ck.path, ck.body); code != ck.want {
			t.Errorf("%s: %d, want %d %s", ck.name, code, ck.want, out)
		}
	}
	// Changing the lock needs an unlocked session, not a token.
	if code, _ := tokRW.do("POST", "/auth/pin/disable", map[string]string{"pin": "1234"}); code != 401 {
		t.Error("token disabled the lock")
	}
	c.ok("POST", "/auth/logout", nil, nil)
	if code, _ := c.do("GET", "/overview", nil); code != 401 {
		t.Error("logout")
	}
}

func TestCrossSiteWritesBlocked(t *testing.T) {
	_, c := newServer(t)
	evil := c.with("Sec-Fetch-Site", "cross-site")
	if code, _ := evil.do("POST", "/transactions/bulk", map[string]any{"ids": []int{1}, "delete": true}); code != 403 {
		t.Fatal("cross-site write accepted")
	}
	if code, _ := evil.do("GET", "/health", nil); code != 200 {
		t.Fatal("cross-site reads are not CSRF")
	}
	// A beacon from another site can't plant usage events or clear them.
	if code, _ := evil.do("POST", "/usage", map[string]any{"events": []any{map[string]any{"kind": "view", "path": "/"}}}); code != 403 {
		t.Fatal("cross-site usage write accepted")
	}
	if code, _ := evil.do("DELETE", "/usage", nil); code != 403 {
		t.Fatal("cross-site usage clear accepted")
	}
	same := c.with("Sec-Fetch-Site", "same-origin")
	if code, _ := same.do("POST", "/tags/rename", map[string]string{"from": "a", "to": "b"}); code != 200 {
		t.Fatal("same-origin write refused")
	}
	if code, _ := same.do("POST", "/usage", map[string]any{"events": []any{map[string]any{"kind": "view", "path": "/"}}}); code != 200 {
		t.Fatal("same-origin usage beacon refused")
	}
	if code, _ := c.do("POST", "/rules", bytes.Repeat([]byte("x"), 3<<20)); code == 200 {
		t.Fatal("oversized body accepted")
	}
}

func TestTransactionLifecycleMovesCashBalance(t *testing.T) {
	s, c := newServer(t)
	Bal(t, s.DB, "cash", "2026-10-01", 100)
	r := ledger.Rule{Pattern: "caffeine", SetCategory: "food.coffee", AddTags: []string{"work"}, Enabled: true}
	ledger.SaveRule(s.DB, &r)

	var created struct {
		Transaction ledger.Tx `json:"transaction"`
		Moved       []string  `json:"balances_moved"`
	}
	c.ok("POST", "/transactions", map[string]any{"date": "2026-10-03", "amount": 4.5, "kind": "expense", "merchant": "Caffeine", "account_id": "cash", "auto_fill": true}, &created)
	if created.Transaction.Category != "food.coffee" || created.Transaction.Tags[0] != "work" || len(created.Moved) != 1 {
		t.Fatalf("%+v", created)
	}
	cash := func() float64 {
		var accts []struct {
			ID      string   `json:"id"`
			Balance *float64 `json:"balance"`
		}
		c.ok("GET", "/accounts", nil, &accts)
		for _, a := range accts {
			if a.ID == "cash" {
				return *a.Balance
			}
		}
		return -1
	}
	if cash() != 95.5 {
		t.Fatal(cash())
	}
	id := created.Transaction.ID
	c.ok("PUT", "/transactions/"+itoa(id), map[string]any{"date": "2026-10-03", "amount": 10, "category": "food.coffee", "account_id": "cash"}, nil)
	if cash() != 90 {
		t.Fatal("update re-applies the delta", cash())
	}
	c.ok("DELETE", "/transactions/"+itoa(id), nil, nil)
	if cash() != 100 {
		t.Fatal("delete reverses it", cash())
	}
	// The house is never a paying account.
	if code, out := c.do("POST", "/transactions", map[string]any{"date": "2026-10-03", "amount": 4, "category": "food", "account_id": "house"}); code != 400 || !strings.Contains(string(out), "valued") {
		t.Fatal(code, string(out))
	}
	// Suggest without saving.
	var sug struct {
		Suggestion ledger.Tx `json:"suggestion"`
	}
	c.ok("POST", "/transactions/suggest", map[string]any{"kind": "expense", "note": "CAFFEINE VILNIUS"}, &sug)
	if sug.Suggestion.Category != "food.coffee" {
		t.Fatal(sug)
	}
}

func TestDeletingABankRowReturnsItToTheInbox(t *testing.T) {
	s, c := newServer(t)
	tx := Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(9), Category: "food", AccountID: "swed", ExternalID: "eb:1:x", Source: "bank"})
	s.DB.Exec(`INSERT INTO bank_inbox(external_id,date,kind,amount,state,imported_tx_id,first_seen_at,last_seen_at) VALUES('eb:1:x','2026-09-01','expense',900,'imported',?,'x','x')`, tx.ID)
	c.ok("DELETE", "/transactions/"+itoa(tx.ID), nil, nil)
	var rows []map[string]any
	c.ok("GET", "/bank/inbox", nil, &rows)
	if len(rows) != 1 {
		t.Fatal("deleted bank row should be back for review", rows)
	}
}

func TestBulkSplitOwed(t *testing.T) {
	s, c := newServer(t)
	a := Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(100), Category: "food.groceries", AccountID: "swed", Tags: []string{"old"}})
	b := Tx(t, s.DB, ledger.Tx{Date: "2026-09-02", Amount: E(10), Category: "food.groceries", AccountID: "swed"})
	c.ok("POST", "/transactions/bulk", map[string]any{"ids": []int64{a.ID, b.ID}, "set_category": "food.restaurants", "add_tags": []string{"kristina"}, "remove_tags": []string{"old"}}, nil)
	got, _ := ledger.Get(s.DB, a.ID)
	if got.Category != "food.restaurants" || strings.Join(got.Tags, ",") != "kristina" {
		t.Fatal(got)
	}
	c.ok("POST", "/transactions/"+itoa(a.ID)+"/split", map[string]any{"parts": []map[string]any{{"amount": 40, "category": "food.restaurants", "owed_by": "Tomas"}}}, nil)
	var owed map[string]float64
	c.ok("GET", "/owed", nil, &owed)
	if owed["tomas"] != 40 {
		t.Fatal(owed)
	}
	c.ok("POST", "/transactions", map[string]any{"date": "2026-09-05", "amount": 40, "category": "refunds", "account_id": "swed", "tags": []string{"owed:tomas"}}, nil)
	c.ok("GET", "/owed", nil, &owed)
	if owed["tomas"] != 0 {
		t.Fatal("repayment clears what is owed", owed)
	}
	var detail struct {
		Parts []ledger.Tx `json:"parts"`
	}
	c.ok("GET", "/transactions/"+itoa(a.ID), nil, &detail)
	if len(detail.Parts) != 1 {
		t.Fatal(detail)
	}
	c.ok("POST", "/transactions/"+itoa(a.ID)+"/unsplit", nil, nil)
	c.ok("POST", "/transactions/bulk", map[string]any{"ids": []int64{b.ID}, "delete": true}, nil)
	var list ledger.ListResult
	c.ok("GET", "/transactions?period=all", nil, &list)
	if list.Total != 2 {
		t.Fatal(list.Total)
	}
}

func TestWealthPlanInsightsEndpoints(t *testing.T) {
	s, c := newServer(t)
	Bal(t, s.DB, "house", "2025-12-01", 410000)
	Bal(t, s.DB, "mortgage", "2026-09-17", -262596.03)
	for m := 1; m <= 9; m++ {
		d := "2026-0" + string(rune('0'+m)) + "-15"
		Tx(t, s.DB, ledger.Tx{Date: d, Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, s.DB, ledger.Tx{Date: d, Amount: E(400), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
		Tx(t, s.DB, ledger.Tx{Date: d, Amount: E(500), Category: "transfer.debt", AccountID: "seb", ToAccountID: "mortgage"})
	}
	c.ok("POST", "/balances", map[string]any{"date": "2026-10-01", "values": []map[string]any{
		{"account_id": "swed", "value": 2000}, {"account_id": "btc_m", "quantity": 0.01, "price": 60000}, {"account_id": "mortgage", "value": 262000},
	}}, nil)
	var nw struct {
		NetWorth  float64            `json:"net_worth"`
		ByAccount map[string]float64 `json:"by_account"`
	}
	c.ok("GET", "/networth?date=2026-10-02", nil, &nw)
	if nw.ByAccount["btc_m"] != 600 || nw.ByAccount["mortgage"] != -262000 || nw.NetWorth != 2000+600+410000-262000 {
		t.Fatalf("%+v", nw)
	}
	var hist []map[string]any
	c.ok("GET", "/networth/history?from=2026-01-01&accounts=1", nil, &hist)
	if len(hist) < 9 {
		t.Fatal(len(hist))
	}
	var loans []map[string]any
	c.ok("GET", "/loans", nil, &loans)
	if len(loans) != 1 {
		t.Fatal(loans)
	}

	var b map[string]any
	c.ok("POST", "/budgets", map[string]any{"name": "Food", "kind": "spending", "categories": []string{"food"}, "amount": 300}, &b)
	c.ok("PUT", "/budgets/"+itoa(int64(b["id"].(float64))), map[string]any{"name": "Food", "kind": "spending", "categories": []string{"food"}, "amount": 450, "from_month": "2026-09"}, nil)
	var rep struct {
		Lines []struct {
			Name   string  `json:"name"`
			Amount float64 `json:"amount"`
			Spent  float64 `json:"spent"`
		} `json:"lines"`
		IncomeBase float64 `json:"income_base"`
	}
	c.ok("GET", "/plan?month=2026-08", nil, &rep)
	if rep.Lines[0].Amount != 300 || rep.Lines[0].Spent != 400 {
		t.Fatalf("history keeps the old amount: %+v", rep.Lines)
	}
	c.ok("GET", "/plan?month=2026-09", nil, &rep)
	if rep.Lines[0].Amount != 450 {
		t.Fatal(rep.Lines)
	}
	c.ok("PUT", "/plan/settings", map[string]any{"income_mode": "manual", "manual_income": 5050}, nil)
	c.ok("GET", "/plan?month=2026-09", nil, &rep)
	if rep.IncomeBase != 5050 {
		t.Fatal(rep.IncomeBase)
	}

	for _, p := range []string{"/overview", "/insights/cashflow?granularity=year", "/insights/breakdown?preset=ytd", "/insights/trends?months=12",
		"/insights/trends?parent=food", "/insights/recurring", "/insights/fi", "/insights/review?year=2026", "/trips", "/categories", "/tags", "/merchants",
		"/rules", "/budgets", "/trades", "/portfolio?live=0", "/bank/connections", "/bank/settings", "/ai/settings", "/ai/context", "/ai/notes", "/auth/status", "/backups"} {
		if code, out := c.do("GET", p, nil); code != 200 {
			t.Errorf("GET %s → %d %s", p, code, out)
		}
	}
	var cf []struct {
		Period    string  `json:"period"`
		Principal float64 `json:"principal"`
		Invested  float64 `json:"invested"`
	}
	c.ok("GET", "/insights/cashflow?granularity=year", nil, &cf)
	if cf[0].Principal != 4500 {
		t.Fatal("mortgage principal counted as invested", cf)
	}
	if code, _ := c.do("GET", "/market/quote/DROP;TABLE", nil); code != 400 {
		t.Error("ticker validation")
	}
	// Trips: tag rows into a trip.
	tx := Tx(t, s.DB, ledger.Tx{Date: "2026-09-20", Amount: E(300), Category: "travel.lodging", AccountID: "swed"})
	var tagged map[string]string
	c.ok("POST", "/trips/tag", map[string]any{"name": "Rome Autumn", "ids": []int64{tx.ID}}, &tagged)
	if tagged["tag"] != "trip:rome-autumn" {
		t.Fatal(tagged)
	}
}

func TestSecretsNeverEchoed(t *testing.T) {
	s, c := newServer(t)
	s.DB.Exec(`INSERT INTO settings(key,value) VALUES('bank','{"application_id":"app","private_key_pem":"-----BEGIN PRIVATE KEY-----SECRET"}')`)
	s.DB.Exec(`INSERT INTO settings(key,value) VALUES('ai','{"api_key":"sk-ant-supersecretkey123","model":"m","enabled":true}')`)
	_, out := c.do("GET", "/bank/settings", nil)
	if strings.Contains(string(out), "SECRET") || !strings.Contains(string(out), `"has_key":true`) {
		t.Error(string(out))
	}
	_, out = c.do("GET", "/ai/settings", nil)
	if strings.Contains(string(out), "supersecretkey") || !strings.Contains(string(out), `"has_key":true`) {
		t.Error(string(out))
	}
	// Saving without a key keeps the stored one.
	c.ok("PUT", "/ai/settings", map[string]any{"model": "claude-sonnet-5-5"}, nil)
	var raw string
	s.DB.QueryRow(`SELECT value FROM settings WHERE key='ai'`).Scan(&raw)
	if !strings.Contains(raw, "supersecretkey") || !strings.Contains(raw, "claude-sonnet-5-5") {
		t.Error(raw)
	}
	_, out = c.do("GET", "/export/backup.json", nil)
	if strings.Contains(string(out), "supersecretkey") || strings.Contains(string(out), "SECRET") {
		t.Error("plain backup leaked a secret")
	}
	_, out = c.do("GET", "/export/backup.json?secrets=1", nil)
	if !strings.Contains(string(out), "supersecretkey") {
		t.Error("a with-secrets backup must carry them")
	}
}

func TestBackupRestoreThroughAPI(t *testing.T) {
	s, c := newServer(t)
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(12.34), Category: "food", Merchant: "Lidl", AccountID: "swed", Tags: []string{"kids"}})
	Bal(t, s.DB, "swed", "2026-09-30", 999.99)
	_, first := c.do("GET", "/export/backup.json", nil)

	_, c2 := newServer(t)
	if code, _ := c2.do("POST", "/import/backup", first); code != 400 {
		t.Fatal("restore without confirm must be refused")
	}
	var res map[string]map[string]int
	c2.ok("POST", "/import/backup?confirm=replace", first, &res)
	if res["restored"]["transactions"] != 1 {
		t.Fatal(res)
	}
	_, second := c2.do("GET", "/export/backup.json", nil)
	strip := func(b []byte) string {
		var m map[string]any
		json.Unmarshal(b, &m)
		delete(m, "exported_at")
		o, _ := json.Marshal(m)
		return string(o)
	}
	if strip(first) != strip(second) {
		t.Fatal("backup → restore → backup changed the data")
	}
}

func TestImportV1AndExports(t *testing.T) {
	_, c := newServer(t)
	v1 := []byte(`{"transactions":[{"id":7,"date":"2026-01-02","type":"expense","amount_eur":5,"category":"Food","comment":"MAXIMA","labels":"groceries"}],
	  "balances":[{"id":1,"date":"2026-01-31","total_eur":10,"swed":10}],"accounts":[],"budgets":[],"label_rules":[],"assets":[],"stock_trades":[]}`)
	if code, _ := c.do("POST", "/import/v1", v1); code != 400 {
		t.Fatal("confirm required")
	}
	var rep map[string]any
	c.ok("POST", "/import/v1?confirm=replace", v1, &rep)
	if rep["transactions"].(float64) != 1 {
		t.Fatal(rep)
	}
	code, csv := c.do("GET", "/export/transactions.csv", nil)
	if code != 200 || !strings.Contains(string(csv), "food.groceries") || !strings.Contains(string(csv), "Maxima") {
		t.Fatal(string(csv))
	}
	code, z := c.do("GET", "/export/ai.zip", nil)
	if code != 200 {
		t.Fatal(code)
	}
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"README.md", "transactions.csv", "cashflow_monthly.csv", "networth_monthly.csv", "balances.csv", "reference.json", "context.md", "loans.csv", "today.json", "PROMPT.md"} {
		if !names[want] {
			t.Errorf("ai.zip missing %s", want)
		}
	}
	req, _ := http.NewRequest("GET", c.base+"/export/ai.zip", nil)
	if res, err := c.http.Do(req); err != nil || !strings.Contains(res.Header.Get("Content-Disposition"), "finance-for-ai-") {
		t.Errorf("export name: %v %v", err, res.Header)
	}
	c.ok("POST", "/backups", nil, nil)
	var b struct {
		Snapshots []map[string]any `json:"snapshots"`
	}
	c.ok("GET", "/backups", nil, &b)
	if len(b.Snapshots) == 0 {
		t.Fatal("manual snapshot")
	}
}

func TestCategoriesAccountsRulesTags(t *testing.T) {
	s, c := newServer(t)
	var cat ledger.Category
	c.ok("POST", "/categories", map[string]any{"parent": "food", "name": "Bakery"}, &cat)
	if cat.ID != "food.bakery" {
		t.Fatal(cat)
	}
	var acct ledger.Account
	c.ok("POST", "/accounts", map[string]any{"name": "Luminor Savings", "kind": "savings", "liquid": true}, &acct)
	if acct.ID != "luminor_savings" {
		t.Fatal(acct)
	}
	if code, _ := c.do("POST", "/accounts", map[string]any{"name": "Luminor Savings", "kind": "savings"}); code != 400 {
		t.Fatal("duplicate account")
	}
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(9), Category: "leisure", Merchant: "Patreon", AccountID: "swed"})
	var prev struct {
		Matches int `json:"matches"`
	}
	c.ok("POST", "/rules/preview", map[string]any{"pattern": "patreon"}, &prev)
	if prev.Matches != 1 {
		t.Fatal(prev)
	}
	var r ledger.Rule
	c.ok("POST", "/rules", map[string]any{"pattern": "patreon", "set_category": "subscriptions.creators", "enabled": true}, &r)
	var applied map[string]int
	c.ok("POST", "/rules/"+itoa(r.ID)+"/apply", nil, &applied)
	if applied["changed"] != 1 {
		t.Fatal(applied)
	}
	if code, _ := c.do("POST", "/rules", map[string]any{}); code != 400 {
		t.Fatal("empty rule accepted")
	}
}

func itoa(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestNewInsightAndTidyEndpoints(t *testing.T) {
	s, c := newServer(t)
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-02", Amount: E(30), Category: "food.restaurants", Merchant: "Jammi", AccountID: "swed"})
	Tx(t, s.DB, ledger.Tx{Date: today6(), Amount: E(12), Category: "food", Merchant: "Jammi", AccountID: "swed", Tags: []string{"kristina"}})
	for _, p := range []string{"/insights/month", "/insights/month?month=2026-09", "/checks", "/networth/movement", "/tags/suggestions", "/ai/topups", "/insights/pace", "/balances/table?page=1&size=50", "/portfolio/history?range=1y", "/tags/stats", "/rules/stats", "/categories/usage", "/accounts/usage"} {
		if code, out := c.do("GET", p, nil); code != 200 {
			t.Errorf("GET %s → %d %s", p, code, out)
		}
	}
	var tidy []struct {
		ID        int64  `json:"id"`
		Suggested string `json:"suggested"`
	}
	c.ok("GET", "/tidy", nil, &tidy)
	if len(tidy) != 1 || tidy[0].Suggested != "food.restaurants" {
		t.Fatalf("history suggests the leaf for a vague row: %+v", tidy)
	}
	var bd struct {
		Tags []map[string]any `json:"tags"`
	}
	c.ok("GET", "/insights/breakdown?preset=12m", nil, &bd)
	if len(bd.Tags) != 1 {
		t.Fatal(bd)
	}
	// Top-ups: add, list, delete.
	c.ok("POST", "/ai/topups", map[string]any{"amount_usd": 10, "note": "card"}, nil)
	var tops []map[string]any
	c.ok("GET", "/ai/topups", nil, &tops)
	c.ok("DELETE", "/ai/topups/"+itoa(int64(tops[0]["id"].(float64))), nil, nil)
	c.ok("GET", "/ai/topups", nil, &tops)
	if len(tops) != 0 {
		t.Fatal("top-up delete")
	}
	// AI routes fail cleanly without a key; an empty portfolio still has scenarios.
	for _, r := range []struct {
		path string
		body any
	}{{"/ai/assist", map[string]any{"text": "coffee 4.50"}}, {"/ai/tidy", map[string]any{"ids": []int64{tidy[0].ID}}}} {
		if code, _ := c.do("POST", r.path, r.body); code < 400 || code >= 500 {
			t.Errorf("POST %s without AI → %d", r.path, code)
		}
	}
	if code, out := c.do("GET", "/portfolio/scenarios", nil); code != 200 {
		t.Errorf("scenarios → %d %s", code, out)
	}
	// purpose=export does not count as the owner's backup.
	c.do("GET", "/export/backup.json?purpose=export", nil)
	var b map[string]any
	c.ok("GET", "/backups", nil, &b)
	if b["last_download"] != "" {
		t.Fatal("purpose=export bumped the backup marker")
	}
	c.do("GET", "/export/backup.json", nil)
	c.ok("GET", "/backups", nil, &b)
	if b["last_download"] == "" {
		t.Fatal("a real download records the marker")
	}
}

func TestRecurringCRUD(t *testing.T) {
	_, c := newServer(t)
	var it map[string]any
	c.ok("POST", "/recurring", map[string]any{"merchant": "Landlord", "category": "housing.rent", "cadence": "monthly", "amount": 600, "next_date": "2099-01-10"}, &it)
	id := itoa(int64(it["id"].(float64)))
	var rec struct {
		Items  []map[string]any `json:"items"`
		Hidden []map[string]any `json:"hidden"`
		Total  float64          `json:"monthly_total"`
	}
	c.ok("GET", "/insights/recurring", nil, &rec)
	if len(rec.Items) != 1 || rec.Items[0]["source"] != "manual" || rec.Total != 600 {
		t.Fatalf("%+v", rec)
	}
	c.ok("PUT", "/recurring/"+id, map[string]any{"merchant": "Landlord", "cadence": "yearly", "amount": 1200, "hidden": true}, nil)
	c.ok("GET", "/insights/recurring", nil, &rec)
	if len(rec.Items) != 0 || len(rec.Hidden) != 1 {
		t.Fatalf("hide: %+v", rec)
	}
	if code, _ := c.do("POST", "/recurring", map[string]any{"merchant": ""}); code != 400 {
		t.Error("empty merchant", code)
	}
	if code, _ := c.do("PUT", "/recurring/9999", map[string]any{"merchant": "x"}); code != 404 {
		t.Error("missing id", code)
	}
	c.ok("DELETE", "/recurring/"+id, nil, nil)
	c.ok("GET", "/insights/recurring", nil, &rec)
	if len(rec.Items)+len(rec.Hidden) != 0 {
		t.Fatal("delete")
	}
}

func TestPrefsPersist(t *testing.T) {
	_, c := newServer(t)
	var p map[string]any
	c.ok("GET", "/prefs", nil, &p)
	if p["liquid_only"] != false {
		t.Fatal(p)
	}
	c.ok("PUT", "/prefs", map[string]any{"liquid_only": true}, nil)
	c.ok("PUT", "/prefs", map[string]any{}, nil) // partial update keeps it
	c.ok("PUT", "/prefs", map[string]any{"periods": map[string]string{"wealth": "6m"}}, nil)
	c.ok("PUT", "/prefs", map[string]any{"periods": map[string]string{"cashflow": "ytd"}}, nil) // merges per chart
	c.ok("GET", "/prefs", nil, &p)
	if p["liquid_only"] != true {
		t.Fatal("liquid_only not remembered", p)
	}
	c.ok("PUT", "/prefs", map[string]any{"hidden_accounts": []string{"house", "car"}}, nil)
	c.ok("PUT", "/prefs", map[string]any{"hidden_accounts": []string{"car"}}, nil) // replaced, not merged
	c.ok("GET", "/prefs", nil, &p)
	if h := p["hidden_accounts"].([]any); len(h) != 1 || h[0] != "car" {
		t.Fatal("hidden accounts", h)
	}
	if _, set := p["history_hidden"]; set {
		t.Fatal("history columns start unconfigured (client default applies)")
	}
	c.ok("PUT", "/prefs", map[string]any{"history_hidden": []string{}}, nil) // configured: show everything
	c.ok("GET", "/prefs", nil, &p)
	if h, ok := p["history_hidden"].([]any); !ok || len(h) != 0 {
		t.Fatal("an empty choice is remembered as a choice", p["history_hidden"])
	}
	if per := p["periods"].(map[string]any); per["wealth"] != "6m" || per["cashflow"] != "ytd" {
		t.Fatal("periods", per)
	}
	if code, _ := c.do("PUT", "/prefs", map[string]any{"periods": map[string]string{"x": strings.Repeat("y", 40)}}); code != 400 {
		t.Error("oversized period accepted", code)
	}
}

func today6() string { return time.Now().AddDate(0, 0, -3).Format("2006-01-02") }

func TestDemoModeIsolated(t *testing.T) {
	s, c := newServer(t)
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-01", Amount: E(12), Category: "food", AccountID: "swed", Merchant: "RealShop"})
	var st map[string]bool
	c.ok("GET", "/demo", nil, &st)
	if st["on"] {
		t.Fatal("demo starts off")
	}
	c.ok("PUT", "/demo", map[string]bool{"on": true}, nil)
	var accts []map[string]any
	c.ok("GET", "/accounts", nil, &accts)
	ids := map[string]bool{}
	for _, a := range accts {
		ids[a["id"].(string)] = true
	}
	if !ids["broker"] || ids["ibkr"] {
		t.Fatalf("demo accounts expected: %v", ids)
	}
	var list struct {
		Total int `json:"total"`
	}
	c.ok("GET", "/transactions?limit=1&from=2000-01-01", nil, &list)
	if list.Total < 600 {
		t.Fatalf("demo transactions: %d", list.Total)
	}
	// Edits in demo mode land in the demo database only.
	c.ok("POST", "/transactions", map[string]any{"date": "2026-09-02", "amount": 5, "category": "food", "account_id": "swed", "merchant": "DemoOnly"}, nil)
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM transactions WHERE merchant='DemoOnly'`).Scan(&n)
	if n != 0 {
		t.Fatal("a demo edit reached the real database")
	}
	// Real files, credentials and money flows are out of reach.
	for _, p := range [][2]string{{"GET", "/backups"}, {"POST", "/backups"}, {"GET", "/export/backup.json"}, {"PUT", "/bank/settings"}, {"POST", "/bank/sync"},
		{"PUT", "/ai/settings"}, {"POST", "/import/backup?confirm=replace"}} {
		if code, _ := c.do(p[0], p[1], map[string]any{}); code != 409 {
			t.Errorf("%s %s in demo: %d, want 409", p[0], p[1], code)
		}
	}
	// The real lock still guards demo data, and API tokens can't flip the switch.
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	if code, _ := c.with("X-Fresh", "1").do("GET", "/overview", nil); code != 401 {
		t.Errorf("demo data without a session: %d", code)
	}
	rw, _ := a.MintToken(true)
	if code, _ := c.with("Authorization", "Bearer "+rw).do("PUT", "/demo", map[string]bool{"on": false}); code != 403 {
		t.Errorf("token toggled demo: %d", code)
	}
	c.ok("POST", "/auth/pin/login", map[string]string{"pin": "1234"}, nil)
	// Reset regenerates; switching off brings the real data back.
	c.ok("POST", "/demo/reset", nil, nil)
	c.ok("GET", "/transactions?q=DemoOnly&from=2000-01-01", nil, &list)
	if list.Total != 0 {
		t.Error("reset keeps edits")
	}
	// Handing over an unlocked device in demo mode: no way back to the real
	// data, and no way to plant a passkey, token or new PIN for later.
	c.ok("GET", "/demo", nil, &st)
	if !st["protected"] {
		t.Error("demo with a PIN should report protected")
	}
	for _, body := range []map[string]any{{"on": false}, {"on": false, "pin": "0000"}} {
		if code, _ := c.do("PUT", "/demo", body); code != 403 {
			t.Errorf("left demo with %v: %d", body, code)
		}
	}
	for _, p := range [][2]string{{"POST", "/auth/passkey/register/begin"}, {"POST", "/auth/token"}, {"DELETE", "/auth/token"}, {"POST", "/auth/pin/setup"}, {"POST", "/auth/pin/disable"}, {"DELETE", "/auth/passkeys/1"}} {
		if code, _ := c.do(p[0], p[1], map[string]any{"current": "1234", "pin": "5678"}); code != 409 {
			t.Errorf("%s %s in demo: %d, want 409", p[0], p[1], code)
		}
	}
	c.ok("PUT", "/demo", map[string]any{"on": false, "pin": "1234"}, nil)
	c.ok("GET", "/transactions?q=RealShop&from=2000-01-01", nil, &list)
	if list.Total != 1 {
		t.Error("real data after leaving demo")
	}
}

// The nginx gates answer from cookies only — nginx owns the paths:
// /auth/gate = a live session; /auth/gate/open = a passkey exists or a session.
func TestAuthGate(t *testing.T) {
	s, c := newServer(t)
	gate := func(path string, withSession bool) int {
		t.Helper()
		req, _ := http.NewRequest("GET", c.base+path, nil)
		// A raw URI header must make no difference any more.
		req.Header.Set("X-Original-URI", "/")
		hc := &http.Client{}
		if withSession {
			hc = c.http // carries the session cookie from logging in
		}
		res, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	check := func(label string, gateWant, openWant int) {
		t.Helper()
		if g, o := gate("/auth/gate", false), gate("/auth/gate/open", false); g != gateWant || o != openWant {
			t.Errorf("%s: gate %d open %d, want %d %d", label, g, o, gateWant, openWant)
		}
	}
	check("no lock", 401, 401)
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	check("PIN, no passkey", 401, 401)
	s.DB.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('phone',x'00','x')`)
	check("passkey, no session", 401, 204)
	c.ok("POST", "/auth/pin/login", map[string]string{"pin": "1234"}, nil)
	if gate("/auth/gate", true) != 204 || gate("/auth/gate/open", true) != 204 {
		t.Error("a live session should pass both gates")
	}
	req, _ := http.NewRequest("GET", c.base+"/auth/gate", nil)
	req.AddCookie(&http.Cookie{Name: "ft_session", Value: "forged"})
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Error("forged session passed")
	}
}
