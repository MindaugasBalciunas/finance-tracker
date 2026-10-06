package ibkr_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"ft/internal/ibkr"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

// fakeIBKR is IBKR's OAuth server and MCP endpoint, just enough of both.
type fakeIBKR struct {
	mu        sync.Mutex
	challenge string
	scope     string
	access    string
	revoked   []string
	tools     []string
	refreshes int
}

func (f *fakeIBKR) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/register", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["token_endpoint_auth_method"] != "none" || in["scope"] != "mcp.read" {
			t.Errorf("registration %v", in)
		}
		json.NewEncoder(w).Encode(map[string]string{"client_id": "cid-1"})
	})
	mux.HandleFunc("/oauth2/api/v1/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("client_id") != "cid-1" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
			f.access = "at-1"
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 3600, "scope": "mcp.read"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "rt-1" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
			f.refreshes++
			f.access = fmt.Sprintf("at-r%d", f.refreshes)
			json.NewEncoder(w).Encode(map[string]any{"access_token": f.access, "expires_in": 3600})
		}
	})
	mux.HandleFunc("/oauth2/api/v1/token/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := r.Header.Get("Authorization") == "Bearer "+f.access
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(401)
			return
		}
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		switch msg.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "s-1")
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
			return
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		}
		if r.Header.Get("Mcp-Session-Id") != "s-1" {
			t.Error("session id not sent back")
		}
		f.mu.Lock()
		f.tools = append(f.tools, msg.Params.Name)
		f.mu.Unlock()
		var text string
		switch msg.Params.Name {
		case "get_account_summary":
			text = `{"currency":"EUR","net_liquidation":18390.05,"total_cash_value":1012.83}`
		case "get_account_positions":
			text = `{"positions":[{"contract_description":"VWCE @IBIS2","position":93,"market_price":173.2,"market_value":16107.6,"currency":"EUR","average_price":162.32,"unrealized_pnl":1011.8,"asset_class":"STK"},
				{"contract_description":"VALL @BVME.ETF","position":238,"market_price":4.46,"market_value":1062,"currency":"EUR","average_price":4.31,"asset_class":"STK"}]}`
		case "get_account_trades":
			if msg.Params.Arguments["period"] != "YEAR_TO_DATE" {
				t.Errorf("trades period %v", msg.Params.Arguments)
			}
			text = `{"trades":[
				{"trade_id":"fx1","symbol":"EUR.USD","sec_type":"CASH","currency":"USD","side":"BUY","size":26.11,"price":1.1598,"trade_time":"2026-09-01T09:03:29Z"},
				{"trade_id":"v1","symbol":"VALL","company_name":"VANGUARD FTSE GLB AL-CAP ETF","sec_type":"STK","currency":"EUR","side":"BUY","size":238,"price":4.3,"commission":2.715,"trade_time":"2026-09-01T09:03:29Z"},
				{"trade_id":"w1","symbol":"VWCE","sec_type":"STK","currency":"EUR","side":"BUY","size":93,"price":162.32,"commission":1.25,"trade_time":"2026-06-05T20:12:51Z"},
				{"trade_id":"x1","symbol":"SPCX","company_name":"SPACE EXPLORATION","sec_type":"STK","currency":"USD","side":"BUY","size":1,"price":164.43,"commission":0.35,"trade_time":"2026-06-12T15:58:59Z"},
				{"trade_id":"x2","symbol":"SPCX","sec_type":"STK","currency":"USD","side":"SELL","size":1,"price":195,"commission":0.35,"trade_time":"2026-06-15T20:35:26Z"}]}`
		case "get_account_orders":
			text = `{"orders":[{"order_id":"o1","symbol":"VWCE","side":"BUY","order_type":"LIMIT","status":"Submitted","quantity":5,"limit_price":170.5,"filled_quantity":0}]}`
		case "get_order_instructions":
			text = `{"instructions":[{"id":"i1","description":"Buy 10 VALL","direction":"BUY","quantity":10,"order_type":"LIMIT","limit_price":4.2,"time_in_force":"GTC"}]}`
		default:
			t.Errorf("unexpected tool %s", msg.Params.Name)
		}
		// Answer as a server-sent event stream, as IBKR may.
		res, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}})
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message\ndata: "+string(res)+"\n\n")
	})
	return mux
}

func TestIBKRReadOnlyConnectionAndSync(t *testing.T) {
	d := DB(t)
	f := &fakeIBKR{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	s := ibkr.New(d)
	s.AuthBase, s.MCPURL, s.Now = srv.URL, srv.URL+"/mcp", func() time.Time { return now }
	ctx := context.Background()

	if _, err := s.Sync(ctx, false); err == nil {
		t.Fatal("sync without a connection")
	}
	if _, err := s.Connect(ctx, "http://evil.example/"); err == nil {
		t.Fatal("plain-http redirect accepted")
	}
	authURL, err := s.Connect(ctx, "https://finance.example:8443/")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	q := u.Query()
	if q.Get("scope") != "mcp.read" || q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "cid-1" || !ibkr.IsCallbackState(q.Get("state")) || q.Get("resource") != srv.URL+"/mcp" {
		t.Fatalf("authorize url %s", authURL)
	}
	f.challenge = q.Get("code_challenge")
	if err := s.Callback(ctx, "good", "ibkr.forged"); err == nil {
		t.Fatal("foreign state accepted")
	}
	authURL, _ = s.Connect(ctx, "https://finance.example:8443/")
	u, _ = url.Parse(authURL)
	f.challenge = u.Query().Get("code_challenge")
	if err := s.Callback(ctx, "good", u.Query().Get("state")); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); !st.Connected || st.Scope != "mcp.read" {
		t.Fatalf("status %+v", st)
	}

	// The app's own records: an old balance, VWCE entered by hand a day off
	// IBKR's date, VALL entered a different size (a mismatch).
	Bal(t, d, "ibkr", "2026-10-04", 18209)
	wealth.SaveTrade(d, &wealth.Trade{Date: "2026-06-06", AccountID: "ibkr", Action: "buy", Ticker: "VWCE", Shares: 93, Price: 162.33, Currency: "EUR"})
	wealth.SaveTrade(d, &wealth.Trade{Date: "2026-09-01", AccountID: "ibkr", Action: "buy", Ticker: "VALL", Shares: 200, Price: 4.31, Currency: "EUR"})

	r, err := s.Sync(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.BalanceWritten || r.NetLiquidation != 18390.05 || r.AppBalance == nil || *r.AppBalance != 18209 {
		t.Fatalf("balance %+v", r)
	}
	var today, old int64
	d.QueryRow(`SELECT value FROM balances WHERE account_id='ibkr' AND date='2026-10-06' AND source='broker'`).Scan(&today)
	d.QueryRow(`SELECT value FROM balances WHERE account_id='ibkr' AND date='2026-10-04'`).Scan(&old)
	if today != 1839005 || old != 1820900 {
		t.Fatalf("balances today %d, earlier %d (must stay)", today, old)
	}
	if r.Mismatches != 1 {
		t.Errorf("mismatches %d: %+v", r.Mismatches, r.Positions)
	}
	got := map[string]ibkr.Proposal{}
	for _, p := range r.Trades {
		got[p.ExternalID] = p
	}
	if _, fx := got["fx1"]; fx || !got["w1"].Recorded || got["v1"].Recorded || got["x1"].Recorded || r.New != 3 {
		t.Fatalf("trades %+v new %d", r.Trades, r.New)
	}
	if got["x2"].Date != "2026-06-15" && got["x2"].Date != "2026-06-16" {
		t.Errorf("local date %s", got["x2"].Date)
	}

	// Orders and instructions are read, whatever IBKR calls the fields.
	if len(r.Orders) != 1 || r.Orders[0].Symbol != "VWCE" || r.Orders[0].Price != 170.5 || r.Orders[0].Status != "Submitted" ||
		len(r.Instructions) != 1 || r.Instructions[0].Side != "buy" || r.Instructions[0].Quantity != 10 || r.OrdersError != "" {
		t.Errorf("orders %+v instructions %+v err %q", r.Orders, r.Instructions, r.OrdersError)
	}
	n, err := s.Import([]string{"x1", "x2", "w1"})
	if err != nil || n != 2 {
		t.Fatalf("imported %d, %v", n, err)
	}
	if n, _ := s.Import([]string{"x1", "x2"}); n != 0 {
		t.Fatalf("imported twice: %d", n)
	}
	trades, _ := wealth.ListTrades(d)
	var buy, sell wealth.Trade
	for _, tr := range trades {
		switch tr.ExternalID {
		case "x1":
			buy = tr
		case "x2":
			sell = tr
		}
	}
	if buy.AccountID != "ibkr" || buy.Price != 164.78 || sell.Action != "sell" || sell.Price != 194.65 || !strings.Contains(buy.Notes, "commission") {
		t.Errorf("imported %+v %+v", buy, sell)
	}
	// A sync after import sees them as recorded.
	r, _ = s.Sync(ctx, false)
	if r.New != 1 {
		t.Errorf("new after import %d", r.New)
	}

	// Read tools only; an expired token is renewed with the refresh token.
	if _, err := s.Live(ctx, "create_order_instruction", nil); err != ibkr.ErrToolRefused {
		t.Fatalf("order tool: %v", err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := s.Live(ctx, "get_account_summary", nil); err != nil || f.refreshes != 1 {
		t.Fatalf("refresh %d: %v", f.refreshes, err)
	}
	for _, tool := range f.tools {
		if !ibkr.ReadTools[tool] {
			t.Errorf("called %s", tool)
		}
	}

	// Balance-only refresh keeps the last comparison.
	if r, err := s.Sync(ctx, true); err != nil || len(r.Positions) != 2 {
		t.Fatalf("balance-only %v %+v", err, r)
	}

	if err := s.Disconnect(ctx); err != nil || s.Status().Connected || len(f.revoked) == 0 {
		t.Fatalf("disconnect: %v revoked %v", err, f.revoked)
	}
}

// A token with more than read access is revoked and never kept.
func TestIBKRRefusesBroaderScope(t *testing.T) {
	d := DB(t)
	var revoked []string
	mux := http.NewServeMux()
	var challenge string
	mux.HandleFunc("/oauth2/register", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"client_id": "cid"})
	})
	mux.HandleFunc("/oauth2/api/v1/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "refresh_token": "rt", "expires_in": 3600, "scope": "mcp.read mcp.orders.submit"})
	})
	mux.HandleFunc("/oauth2/api/v1/token/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		revoked = append(revoked, r.Form.Get("token"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s := ibkr.New(d)
	s.AuthBase, s.MCPURL = srv.URL, srv.URL+"/mcp"
	u, err := s.Connect(context.Background(), "https://finance.example/")
	if err != nil {
		t.Fatal(err)
	}
	pu, _ := url.Parse(u)
	challenge = pu.Query().Get("state")
	if err := s.Callback(context.Background(), "code", challenge); err == nil || !strings.Contains(err.Error(), "more than read-only") {
		t.Fatalf("broader scope accepted: %v", err)
	}
	if s.Status().Connected || len(revoked) != 2 {
		t.Fatalf("kept or not revoked: %+v %v", s.Status(), revoked)
	}
}
