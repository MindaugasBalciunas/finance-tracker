package ai_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ft/internal/ai"
	"ft/internal/api"
	"ft/internal/ledger"
	"ft/internal/market"
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
	return askCFO(t, d), f
}

// askCFO is the app's own assistant: its tools reach the real routes.
func askCFO(t *testing.T, d *sql.DB) *ai.Assistant {
	return api.New(d, filepath.Join(t.TempDir(), "finance-v2.db"), "test").AI
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

// An answer's unverified amounts are kept with it, so the warning still shows
// when the answer finished in the background and is read from the history.
func TestUncheckedKeptInHistory(t *testing.T) {
	toolCall := `{"content":[{"type":"tool_use","id":"tu1","name":"search_transactions","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`
	reply := `{"content":[{"type":"text","text":"You spent **€23** — that is **€9,999** a year."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	a, _ := setup(t, "anthropic", toolCall, reply)
	Tx(t, a.DB, ledger.Tx{Date: "2026-09-01", Amount: E(23), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
	res, err := a.Chat(context.Background(), "Food?", nil)
	if err != nil || len(res.Unchecked) != 1 || res.Unchecked[0] != "€9,999" {
		t.Fatalf("%v %+v", err, res)
	}
	h, _ := a.History(10)
	if len(h) != 2 || len(h[1].Unchecked) != 1 || h[1].Unchecked[0] != "€9,999" || h[0].Unchecked != nil {
		t.Fatalf("history: %+v", h)
	}
}

// add_recurring puts a standing order into what the cash plan reads.
func TestAddRecurringTool(t *testing.T) {
	call := `{"content":[{"type":"tool_use","id":"tu1","name":"add_recurring","input":{"name":"IBKR top-up","kind":"transfer","amount":1000,"day":1,"from_account":"swed","to_account":"ibkr"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`
	list := `{"content":[{"type":"tool_use","id":"tu2","name":"get_recurring","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`
	done := `{"content":[{"type":"text","text":"Added."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	a, f := setup(t, "anthropic", call, list, done)
	res, err := a.Chat(context.Background(), "Yes, add it", nil)
	if err != nil || !res.Changed {
		t.Fatalf("%v %+v", err, res)
	}
	for i, want := range []string{`"merchant":"IBKR top-up"`, `"IBKR top-up"`} {
		msgs := f.reqs[i+1]["messages"].([]any)
		out := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
		if out["is_error"] == true || !strings.Contains(out["content"].(string), want) {
			t.Fatalf("round %d: %v", i, out)
		}
	}
}

// The assistant changes a budget line and the plan settings through its tools.
func TestBudgetTools(t *testing.T) {
	call := `{"content":[{"type":"tool_use","id":"t1","name":"save_budget","input":{"name":"Food","kind":"spending","categories":["food"],"amount":400}},
	  {"type":"tool_use","id":"t2","name":"update_plan_settings","input":{"gross_salary":10000}}],"stop_reason":"tool_use","usage":{}}`
	done := `{"content":[{"type":"text","text":"Done."}],"stop_reason":"end_turn","usage":{}}`
	a, f := setup(t, "anthropic", call, done)
	res, err := a.Chat(context.Background(), "Yes, apply both", nil)
	if err != nil || !res.Changed {
		t.Fatalf("%v %+v", err, res)
	}
	msgs := f.reqs[1]["messages"].([]any)
	for i, b := range msgs[len(msgs)-1].(map[string]any)["content"].([]any) {
		want := []string{`"name":"Food"`, `"gross_salary":10000`}[i]
		if r := b.(map[string]any); r["is_error"] == true || !strings.Contains(r["content"].(string), want) {
			t.Fatalf("tool result: %v", r)
		}
	}
}

// The planning tools answer in Ask CFO, and writing down a decision is a
// write (confirmation required) that lands in the journal as "thinking".
func TestPlanningTools(t *testing.T) {
	market.HICPFetch = func() ([]byte, error) { return nil, errors.New("offline in tests") }
	calls := `{"content":[` +
		`{"type":"tool_use","id":"1","name":"get_payday_runbook","input":{}},` +
		`{"type":"tool_use","id":"2","name":"get_next_euro","input":{"amount":800}},` +
		`{"type":"tool_use","id":"3","name":"get_lifestyle_creep","input":{}},` +
		`{"type":"tool_use","id":"4","name":"get_price_creep","input":{}},` +
		`{"type":"tool_use","id":"5","name":"get_decisions","input":{}},` +
		`{"type":"tool_use","id":"6","name":"get_decision_impact","input":{"amount":450}},` +
		`{"type":"tool_use","id":"7","name":"log_decision","input":{"title":"Standing desk","amount":450,"category":"housing.furnishing"}},` +
		`{"type":"tool_use","id":"8","name":"get_ownership_costs","input":{}},` +
		`{"type":"tool_use","id":"10","name":"get_fi_plan","input":{}},` +
		`{"type":"tool_use","id":"11","name":"get_milestones","input":{}},` +
		`{"type":"tool_use","id":"12","name":"get_fees","input":{}},` +
		`{"type":"tool_use","id":"13","name":"get_subscriptions","input":{}},` +
		`{"type":"tool_use","id":"14","name":"get_cash_calendar","input":{"days":30}},` +
		`{"type":"tool_use","id":"9","name":"get_decision_impact","input":{"amount":0}}` +
		`],"stop_reason":"tool_use","usage":{}}`
	a, f := setup(t, "anthropic", calls, textReply)
	Tx(t, a.DB, ledger.Tx{Date: "2026-09-01", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Bal(t, a.DB, "swed", "2026-09-30", 4000)
	res, err := a.Chat(context.Background(), "plan", nil)
	if err != nil {
		t.Fatal(err)
	}
	last := f.reqs[1]["messages"].([]any)
	out := last[len(last)-1].(map[string]any)["content"].([]any)
	if len(out) != 14 {
		t.Fatalf("results %d", len(out))
	}
	for i, r := range out {
		isErr, _ := r.(map[string]any)["is_error"].(bool)
		if (i == 13) != isErr { // a free purchase has no impact: an error fed back, not a crash
			t.Errorf("tool %d error=%v: %v", i+1, isErr, r.(map[string]any)["content"])
		}
	}
	if !res.Changed { // only a write tool marks the reply changed
		t.Error("log_decision is a write")
	}
	var n int
	a.DB.QueryRow(`SELECT COUNT(*) FROM decisions WHERE title='Standing desk' AND amount=45000 AND status='thinking'`).Scan(&n)
	if n != 1 {
		t.Errorf("decision rows %d", n)
	}
}

// The views added for parity answer in Ask CFO too, and a trip's dates are
// found however the assistant spells its tag.
func TestParityToolsInAskCFO(t *testing.T) {
	a := askCFO(t, DB(t))
	run := func(name, args string) string {
		t.Helper()
		out, err := a.RunTool(context.Background(), name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	if out := run("set_trip_dates", `{"tag":"trip:Rome","start":"2026-06-23","end":"2026-06-27"}`); !strings.Contains(out, `"trip:rome"`) {
		t.Errorf("set: %s", out)
	}
	if out := run("get_trip_untagged", `{"tag":"trip:Rome"}`); !strings.Contains(out, `"in_dates"`) {
		t.Errorf("untagged: %s", out)
	}
	if out := run("get_trips", `{}`); !strings.Contains(out, `"dates":[{"tag":"trip:rome"`) {
		t.Errorf("trips lack the dates: %.300s", out)
	}
	run("set_trip_dates", `{"tag":"Trip:Rome ","clear":true}`)
	if out := run("get_trips", `{}`); strings.Contains(out, "trip:rome") {
		t.Errorf("dates not cleared: %.300s", out)
	}
	for _, c := range [][2]string{{"get_month_review", `{"month":"2026-09"}`}, {"get_checks", `{}`}, {"get_owed", `{}`}, {"get_net_worth_movement", `{}`}} {
		if out := run(c[0], c[1]); out == "" || out == "null" {
			t.Errorf("%s: %q", c[0], out)
		}
	}
	if out := run("get_forecast", `{"skip_goals":[1]}`); !strings.Contains(out, `"scenario"`) {
		t.Errorf("skip_goals alone is a what-if: %.200s", out)
	}
}

// Without an IBKR connection the tool says so plainly instead of guessing.
func TestIBKRLiveToolNotConnected(t *testing.T) {
	a := askCFO(t, DB(t))
	if _, err := a.RunTool(context.Background(), "ibkr_live", json.RawMessage(`{"what":"positions"}`)); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("not connected: %v", err)
	}
}

// Ask CFO's figures are the app's: a salary paid on the 2nd counts for the
// month before even when it falls after the window asked about.
func TestCashFlowCountsLateSalary(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-10-02", Amount: E(3000), Category: "salary", AccountID: "swed"})
	out, err := askCFO(t, d).RunTool(context.Background(), "cash_flow", json.RawMessage(`{"from":"2026-09-01","to":"2026-09-30"}`))
	if err != nil {
		t.Fatal(err)
	}
	var flows []struct {
		Period string  `json:"period"`
		Income float64 `json:"income"`
	}
	if err := json.Unmarshal([]byte(out), &flows); err != nil {
		t.Fatal(err, out)
	}
	if len(flows) != 1 || flows[0].Period != "2026-09" || flows[0].Income != 3000 {
		t.Fatalf("September should hold the salary paid on 2 October: %s", out)
	}
}

// Changing one field of a goal or a transaction leaves the others alone, and
// an empty string clears a field.
func TestEditsChangeOnlyTheFieldsGiven(t *testing.T) {
	d := DB(t)
	a := askCFO(t, d)
	run := func(name, args string) string {
		t.Helper()
		out, err := a.RunTool(context.Background(), name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	run("save_goal", `{"name":"Bike","target":900,"note":"gravel"}`)
	if out := run("save_goal", `{"id":1,"status":"bought"}`); !strings.Contains(out, `"name":"Bike"`) || !strings.Contains(out, `"target":900`) || !strings.Contains(out, `"note":"gravel"`) || !strings.Contains(out, `"status":"bought"`) {
		t.Fatalf("goal lost fields: %s", out)
	}
	tx := Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(12), Category: "food", Merchant: "Lidl", Note: "milk", AccountID: "swed"})
	run("update_transaction", fmt.Sprintf(`{"id":%d,"merchant":""}`, tx.ID))
	if got, _ := ledger.Get(d, tx.ID); got.Merchant != "" || got.Note != "milk" || got.Category != "food" {
		t.Fatalf("%+v", got)
	}
}

// Models send nulls, extra fields and 20.5 where a whole number belongs; the
// tools take what fits instead of refusing the call.
func TestToolsTolerateLooseArguments(t *testing.T) {
	d := DB(t)
	a := askCFO(t, d)
	for _, c := range [][2]string{
		{"get_overview", `{"month":"2026-09"}`},
		{"get_reference", `{"what":"categories","limit":10}`},
		{"search_transactions", `{"to":null,"limit":20.5}`},
		{"create_transaction", `{"date":"2026-09-01","amount":4.5,"category":"food.coffee","account_id":"swed","to_account_id":null}`},
	} {
		if _, err := a.RunTool(context.Background(), c[0], json.RawMessage(c[1])); err != nil {
			t.Errorf("%s %s: %v", c[0], c[1], err)
		}
	}
}

// Every Ask CFO tool reaches its route: none falls outside the read-write
// scope it is held to.
func TestEveryToolIsInScope(t *testing.T) {
	d := DB(t)
	a := askCFO(t, d)
	for _, td := range ai.ToolNames() {
		_, err := a.RunTool(context.Background(), td, json.RawMessage(`{}`))
		if err != nil && (strings.Contains(err.Error(), "scope") || strings.Contains(err.Error(), "read-only")) {
			t.Errorf("%s: %v", td, err)
		}
	}
}

// An empty salary_rules list clears the rules (back to the default).
func TestPlanSettingsClearSalaryRules(t *testing.T) {
	d := DB(t)
	a := askCFO(t, d)
	run := func(name, args string) string {
		t.Helper()
		out, err := a.RunTool(context.Background(), name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	run("update_plan_settings", `{"salary_rules":[{"from":"","paid_by_day":10}]}`)
	if out := run("get_plan_settings", `{}`); !strings.Contains(out, `"paid_by_day":10`) {
		t.Fatalf("rule not saved: %.300s", out)
	}
	run("update_plan_settings", `{"salary_rules":[]}`)
	if out := run("get_plan_settings", `{}`); strings.Contains(out, `"paid_by_day":10`) {
		t.Fatalf("rules not cleared: %.300s", out)
	}
}
