package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ft/internal/ai"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// fakeClaude answers the Messages API from a script and records requests.
type fakeClaude struct {
	mu      sync.Mutex
	replies []string
	reqs    []map[string]any
	headers []http.Header
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/models" {
		w.Write([]byte(`{"data":[{"id":"claude-opus-5-5"},{"id":"claude-sonnet-5-5"}]}`))
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	f.reqs = append(f.reqs, req)
	f.headers = append(f.headers, r.Header.Clone())
	if len(f.replies) == 0 {
		w.WriteHeader(500)
		return
	}
	w.Write([]byte(f.replies[0]))
	f.replies = f.replies[1:]
}

func setup(t *testing.T, provider string, replies ...string) (*ai.Assistant, *fakeClaude) {
	d := DB(t)
	f := &fakeClaude{replies: replies}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	key := "sk-ant-test"
	if provider == "gateway" {
		key = "nexos-key"
	}
	ai.SaveSettings(d, ai.Settings{GatewayURL: srv.URL, APIKey: key, Model: "claude-opus-5-5", Provider: provider, Enabled: true})
	return &ai.Assistant{DB: d, Client: &ai.Client{DB: d}}, f
}

const textReply = `{"content":[{"type":"text","text":"You spent **€23** on groceries."}],"stop_reason":"end_turn","usage":{"input_tokens":1000,"output_tokens":50}}`

func TestChatToolLoopAndHistory(t *testing.T) {
	toolCall := `{"content":[{"type":"text","text":"Let me look."},{"type":"tool_use","id":"tu1","name":"search_transactions","input":{"categories":["food"]}}],"stop_reason":"tool_use","usage":{"input_tokens":900,"output_tokens":20}}`
	a, f := setup(t, "anthropic", toolCall, textReply)
	Tx(t, a.DB, ledger.Tx{Date: "2026-09-01", Amount: E(23), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
	a.SaveContext("I am the owner.")
	res, err := a.Chat(context.Background(), "How much on food?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Reply, "€23") || len(res.Tools) != 1 || res.Tools[0] != "search_transactions" || res.Changed {
		t.Fatalf("%+v", res)
	}
	if res.Usage.InputTokens != 1900 || !res.Usage.Estimated || res.Usage.CostUSD <= 0 {
		t.Fatalf("usage summed across rounds and priced: %+v", res.Usage)
	}
	// Second request carried the tool result back.
	msgs := f.reqs[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if last["type"] != "tool_result" || !strings.Contains(last["content"].(string), "Lidl") {
		t.Fatalf("tool result: %v", last)
	}
	// System prompt: cached, includes the brief; tools include web search.
	sys := f.reqs[0]["system"].([]any)
	if !strings.Contains(sys[len(sys)-1].(map[string]any)["text"].(string), "I am the owner.") || sys[len(sys)-1].(map[string]any)["cache_control"] == nil {
		t.Error("brief + cache breakpoint")
	}
	tools, _ := json.Marshal(f.reqs[0]["tools"])
	if !strings.Contains(string(tools), "web_search_20260209") {
		t.Error("web search tool")
	}
	if f.headers[0].Get("x-api-key") != "sk-ant-test" || f.headers[0].Get("anthropic-version") == "" || f.headers[0].Get("Authorization") != "" {
		t.Error("anthropic auth headers")
	}
	// History persisted (question + answer), spend recorded per round.
	hist, _ := a.History(10)
	if len(hist) != 2 || hist[0].Content != "How much on food?" {
		t.Fatal(hist)
	}
	if sp := a.Client.Spend(); sp.Calls != 2 || sp.ByKind["chat"] <= 0 {
		t.Fatal(sp)
	}
	// Next turn replays history.
	f.replies = []string{textReply}
	a.Chat(context.Background(), "And last month?", nil)
	if n := len(f.reqs[2]["messages"].([]any)); n != 3 {
		t.Errorf("replayed %d messages", n)
	}
	a.ClearHistory()
	if h, _ := a.History(10); len(h) != 0 {
		t.Error("clear")
	}
}

func TestWriteToolsAndMemory(t *testing.T) {
	create := `{"content":[{"type":"tool_use","id":"a","name":"create_transaction","input":{"date":"2026-10-03","amount":4.5,"category":"food.coffee","merchant":"Caffeine","account_id":"cash"}},
	  {"type":"tool_use","id":"b","name":"remember","input":{"note":"Prefers VWCE as core"}}],"stop_reason":"tool_use","usage":{}}`
	a, _ := setup(t, "gateway", create, textReply)
	Bal(t, a.DB, "cash", "2026-10-01", 100)
	res, err := a.Chat(context.Background(), "Add my coffee, yes confirmed", nil)
	if err != nil || !res.Changed {
		t.Fatal(res, err)
	}
	all, _ := ledger.All(a.DB, ledger.Filter{})
	if len(all) != 1 || all[0].Merchant != "Caffeine" {
		t.Fatal(all)
	}
	var cash int64
	a.DB.QueryRow(`SELECT value FROM balances WHERE account_id='cash' ORDER BY date DESC LIMIT 1`).Scan(&cash)
	if cash != int64(E(95.5)) {
		t.Errorf("cash balance follows an assistant-created expense: %d", cash)
	}
	if !strings.Contains(a.Notes(), "Prefers VWCE as core") {
		t.Error("memory", a.Notes())
	}
}

func TestGatewayAuthAndErrors(t *testing.T) {
	a, f := setup(t, "gateway", `{"error":{"type":"invalid_request_error","message":"model: claude-x"}}`)
	_, err := a.Chat(context.Background(), "hi", nil)
	if err == nil || !strings.Contains(err.Error(), "does not offer this model") {
		t.Fatal(err)
	}
	if f.headers[0].Get("Authorization") != "Bearer nexos-key" || f.headers[0].Get("x-api-key") != "" {
		t.Error("gateway uses a bearer token")
	}
	models, err := a.Client.Models(context.Background(), ai.LoadSettings(a.DB))
	if err != nil || len(models) != 2 {
		t.Fatal(models, err)
	}
}

func TestDisabledAndUnconfigured(t *testing.T) {
	a, _ := setup(t, "anthropic")
	s := ai.LoadSettings(a.DB)
	s.Enabled = false
	ai.SaveSettings(a.DB, s)
	if _, err := a.Chat(context.Background(), "hi", nil); err == nil || !strings.Contains(err.Error(), "turned off") {
		t.Fatal(err)
	}
	if _, err := a.ScanReceipt(context.Background(), ai.Image{MediaType: "image/png", Data: []byte{1}}); err == nil {
		t.Fatal("scan while off")
	}
}

func TestScanReceipt(t *testing.T) {
	reply := `{"content":[{"type":"text","text":"Here: {\"kind\":\"expense\",\"date\":\"2026-10-02\",\"amount\":-65.2,\"currency\":\"EUR\",\"category\":\"food.groceries\",\"merchant\":\"Maxima\",\"note\":\"weekly\",\"account_id\":\"house\",\"tags\":[\"kids\",\"invented\"],\"remark\":\"clear\"}"}],"stop_reason":"end_turn","usage":{}}`
	a, f := setup(t, "anthropic", reply)
	for i := 0; i < 5; i++ {
		Tx(t, a.DB, ledger.Tx{Date: "2026-09-0" + string(rune('1'+i)), Amount: E(1), Category: "food", AccountID: "swed", Tags: []string{"kids"}})
	}
	s, err := a.ScanReceipt(context.Background(), ai.Image{MediaType: "image/jpeg", Data: []byte("jpeg")})
	if err != nil {
		t.Fatal(err)
	}
	if s.Amount != E(65.2) || s.Category != "food.groceries" || s.Merchant != "Maxima" || s.Date != "2026-10-02" {
		t.Fatalf("%+v", s)
	}
	if s.AccountID != "" {
		t.Error("the house can never be the paying account")
	}
	if len(s.Tags) != 1 || s.Tags[0] != "kids" {
		t.Error("only known tags", s.Tags)
	}
	content := f.reqs[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "image" {
		t.Error("image block first")
	}
	if _, err := a.ScanReceipt(context.Background(), ai.Image{MediaType: "application/pdf"}); err == nil {
		t.Error("unsupported type accepted")
	}
}

func TestProviderInference(t *testing.T) {
	cases := []struct {
		s    ai.Settings
		want string
		url  string
	}{
		{ai.Settings{APIKey: "sk-ant-1"}, "anthropic", ai.DefaultAnthropicURL},
		{ai.Settings{APIKey: "x"}, "gateway", ai.DefaultGatewayURL},
		{ai.Settings{GatewayURL: "https://api.anthropic.com/v1/", APIKey: "x"}, "anthropic", "https://api.anthropic.com/v1"},
		{ai.Settings{Provider: "gateway", APIKey: "sk-ant-1"}, "gateway", ai.DefaultGatewayURL},
	}
	for _, c := range cases {
		if c.s.ResolvedProvider() != c.want || c.s.BaseURL() != c.url {
			t.Errorf("%+v → %s %s", c.s, c.s.ResolvedProvider(), c.s.BaseURL())
		}
	}
}

// Every read tool answers on a realistic fixture (network tools excluded).
func TestReadToolsAnswer(t *testing.T) {
	calls := `{"content":[` +
		`{"type":"tool_use","id":"1","name":"get_overview","input":{}},` +
		`{"type":"tool_use","id":"2","name":"cash_flow","input":{"granularity":"year"}},` +
		`{"type":"tool_use","id":"3","name":"spending_breakdown","input":{}},` +
		`{"type":"tool_use","id":"4","name":"get_net_worth","input":{"history_from":"2026-01-01"}},` +
		`{"type":"tool_use","id":"5","name":"get_account_history","input":{"account_id":"swed"}},` +
		`{"type":"tool_use","id":"6","name":"get_budget_status","input":{}},` +
		`{"type":"tool_use","id":"7","name":"get_recurring","input":{}},` +
		`{"type":"tool_use","id":"8","name":"get_fi","input":{}},` +
		`{"type":"tool_use","id":"9","name":"get_loans","input":{}},` +
		`{"type":"tool_use","id":"10","name":"get_trips","input":{}},` +
		`{"type":"tool_use","id":"11","name":"get_reference","input":{"what":"categories"}},` +
		`{"type":"tool_use","id":"12","name":"get_reference","input":{"what":"merchants"}},` +
		`{"type":"tool_use","id":"13","name":"get_portfolio","input":{"include_trades":true}},` +
		`{"type":"tool_use","id":"14","name":"update_transaction","input":{"id":1,"category":"food.restaurants","tags":["kristina"]}},` +
		`{"type":"tool_use","id":"15","name":"add_rule","input":{"pattern":"lidl","set_category":"food.groceries","apply_to_history":true}},` +
		`{"type":"tool_use","id":"16","name":"rename_tag","input":{"from":"kristina","to":"partner"}},` +
		`{"type":"tool_use","id":"17","name":"get_reference","input":{"what":"nonsense"}}` +
		`],"stop_reason":"tool_use","usage":{}}`
	a, f := setup(t, "anthropic", calls, textReply)
	Tx(t, a.DB, ledger.Tx{Date: "2026-09-01", Amount: E(23), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
	Bal(t, a.DB, "swed", "2026-09-30", 100)
	if _, err := a.Chat(context.Background(), "everything", nil); err != nil {
		t.Fatal(err)
	}
	results := f.reqs[1]["messages"].([]any)
	last := results[len(results)-1].(map[string]any)["content"].([]any)
	if len(last) != 17 {
		t.Fatalf("all results in ONE user message: %d", len(last))
	}
	for i, r := range last {
		m := r.(map[string]any)
		isErr, _ := m["is_error"].(bool)
		if (i == 16) != isErr {
			t.Errorf("tool %d error=%v: %v", i+1, isErr, m["content"])
		}
	}
	got, _ := ledger.Get(a.DB, 1)
	if got.Category != "food.groceries" || len(got.Tags) != 1 || got.Tags[0] != "partner" {
		t.Errorf("writes applied in order: %+v", got)
	}
}

func TestAssistAndTidy(t *testing.T) {
	assist := `{"content":[{"type":"text","text":"{\"kind\":\"expense\",\"date\":\"2026-10-03\",\"amount\":4.5,\"category\":\"food.coffee\",\"merchant\":\"Caffeine\",\"account_id\":\"cash\",\"tags\":[],\"remark\":\"\"}"}],"stop_reason":"end_turn","usage":{}}`
	tidy := `{"content":[{"type":"text","text":"[{\"id\":1,\"category\":\"food.restaurants\",\"merchant\":\"Jammi\",\"reason\":\"restaurant\"},{\"id\":2,\"category\":\"salary\"},{\"id\":99,\"category\":\"food\"}]"}],"stop_reason":"end_turn","usage":{}}`
	a, f := setup(t, "anthropic", assist, tidy)
	s, err := a.Assist(context.Background(), "coffee 4.50 cash yesterday")
	if err != nil || s.Category != "food.coffee" || s.AccountID != "cash" || s.Amount != E(4.5) {
		t.Fatal(s, err)
	}
	if !strings.Contains(f.reqs[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), "coffee 4.50 cash yesterday") {
		t.Error("prompt carries the description")
	}
	t1 := Tx(t, a.DB, ledger.Tx{Date: "2026-09-01", Amount: E(30), Category: "food", Merchant: "JAMMI", AccountID: "swed"})
	t2 := Tx(t, a.DB, ledger.Tx{Date: "2026-09-02", Amount: E(9), Category: "leisure", AccountID: "swed"})
	props, err := a.Tidy(context.Background(), []ledger.Tx{t1, t2})
	if err != nil {
		t.Fatal(err)
	}
	// Only the valid same-kind proposal survives (salary is income; 99 is unknown).
	if len(props) != 1 || props[0].ID != t1.ID || props[0].Category != "food.restaurants" {
		t.Fatalf("%+v", props)
	}
	if _, err := a.Assist(context.Background(), "  "); err == nil {
		t.Error("empty description accepted")
	}
}
