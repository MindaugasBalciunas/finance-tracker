package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

func TestMaxTokensStopSurfacesTruncation(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "Your spending was"}},
			"stop_reason": "max_tokens",
		})
	}))
	defer srv.Close()
	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "m"}
	msg, err := callGatewayFull(context.Background(), settings, []gatewayMessage{{Role: "user", Content: "x"}}, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.Content, "truncated") {
		t.Fatalf("expected truncation marker, got: %q", msg.Content)
	}
	if gotPath != "/messages" {
		t.Fatalf("expected the Anthropic-native /messages endpoint, got %q", gotPath)
	}
}

// The chat budget is enforced by the caller's context: once it is cancelled
// (the overall chat deadline hit, or the client went away) the gateway call
// must abort instead of running out its per-call timeout — this is what stops
// a long agentic chat from hanging until nginx returns a bare 504. A
// cancelled context must abort before the request is even sent.
func TestGatewayCallHonorsContextCancellation(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()
	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "m"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call
	if _, err := callGatewayFull(ctx, settings, []gatewayMessage{{Role: "user", Content: "x"}}, 10, nil); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("request should not have been sent on a cancelled context, got %d hits", n)
	}
}

// The wire translation: system turns become cached top-level system blocks,
// tool results merge into one user message, tools use input_schema.
func TestAnthropicWireTranslation(t *testing.T) {
	var req map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()
	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "m"}

	var call gatewayToolCall
	call.ID = "t1"
	call.Type = "function"
	call.Function.Name = "get_budgets"
	call.Function.Arguments = "{}"
	msgs := []gatewayMessage{
		{Role: "system", Content: "the big report"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []gatewayToolCall{call}},
		{Role: "tool", ToolCallID: "t1", Content: `[{"name":"Loan"}]`},
	}
	if _, err := callGatewayFull(context.Background(), settings, msgs, 100, chatTools()); err != nil {
		t.Fatal(err)
	}

	system := req["system"].([]any)
	last := system[len(system)-1].(map[string]any)
	if last["text"] != "the big report" {
		t.Fatalf("system block missing: %v", last)
	}
	if cc, ok := last["cache_control"].(map[string]any); !ok || cc["type"] != "ephemeral" {
		t.Fatalf("system block must carry the cache_control breakpoint, got %v", last)
	}

	wire := req["messages"].([]any)
	if len(wire) != 3 {
		t.Fatalf("expected user + assistant + tool_result messages, got %d", len(wire))
	}
	asst := wire[1].(map[string]any)
	tu := asst["content"].([]any)[0].(map[string]any)
	if tu["type"] != "tool_use" || tu["name"] != "get_budgets" || tu["id"] != "t1" {
		t.Fatalf("bad tool_use block: %v", tu)
	}
	toolMsg := wire[2].(map[string]any)
	tr := toolMsg["content"].([]any)[0].(map[string]any)
	if toolMsg["role"] != "user" || tr["type"] != "tool_result" || tr["tool_use_id"] != "t1" {
		t.Fatalf("bad tool_result message: %v", toolMsg)
	}

	tools := req["tools"].([]any)
	first := tools[0].(map[string]any)
	if _, ok := first["input_schema"]; !ok {
		t.Fatalf("tools must use Anthropic input_schema, got %v", first)
	}
}

// A tool_use response comes back as internal ToolCalls.
func TestAnthropicToolUseResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "tool_use", "id": "call_9", "name": "get_summary", "input": map[string]any{"label": "loan"}},
			},
			"stop_reason": "tool_use",
		})
	}))
	defer srv.Close()
	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "m"}
	msg, err := callGatewayFull(context.Background(), settings, []gatewayMessage{{Role: "user", Content: "x"}}, 100, chatTools())
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "get_summary" || msg.ToolCalls[0].ID != "call_9" {
		t.Fatalf("bad tool call mapping: %+v", msg.ToolCalls)
	}
	if !strings.Contains(msg.ToolCalls[0].Function.Arguments, `"label":"loan"`) {
		t.Fatalf("arguments not preserved: %q", msg.ToolCalls[0].Function.Arguments)
	}
}

// "model: Claude Opus 5" is exactly what the Claude API answers when it is
// handed a gateway's display name, and on its own it names no remedy. The
// annotation is the whole fix for a user staring at that string.
func TestModelErrorCarriesARemedy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "not_found_error", "message": "model: Claude Opus 5"},
		})
	}))
	defer srv.Close()

	settings := &domain.AISettings{
		GatewayURL: srv.URL, APIKey: "k",
		Model: "Claude Opus 5", Provider: domain.ProviderAnthropic,
	}
	_, err := callGateway(context.Background(), settings,
		[]domain.ChatMessage{{Role: "user", Content: "hi"}}, 64)
	if err == nil {
		t.Fatal("expected an error")
	}
	got := err.Error()
	// The upstream words are kept — they are the ground truth — and a next
	// step is appended.
	if !strings.Contains(got, "model: Claude Opus 5") {
		t.Fatalf("upstream message was lost: %q", got)
	}
	if !strings.Contains(got, "display name") || !strings.Contains(got, domain.DefaultClaudeModel) {
		t.Fatalf("no remedy offered: %q", got)
	}
}

// A model the provider genuinely does not carry is a different problem with a
// different answer, and must not be mislabelled as a display name.
func TestUnknownRealModelIDGetsTheOtherRemedy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "model: claude-opus-9"},
		})
	}))
	defer srv.Close()

	settings := &domain.AISettings{
		GatewayURL: srv.URL, APIKey: "k",
		Model: "claude-opus-9", Provider: domain.ProviderAnthropic,
	}
	_, err := callGateway(context.Background(), settings,
		[]domain.ChatMessage{{Role: "user", Content: "hi"}}, 64)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); strings.Contains(got, "display name") {
		t.Fatalf("a well-formed id must not be called a display name: %q", got)
	}
}

// Every other upstream message must pass through untouched — the annotation
// is targeted, not a general rewriter.
func TestNonModelErrorsAreNotAnnotated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "invalid x-api-key"},
		})
	}))
	defer srv.Close()

	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "claude-opus-5"}
	_, err := callGateway(context.Background(), settings,
		[]domain.ChatMessage{{Role: "user", Content: "hi"}}, 64)
	if err == nil || !strings.HasSuffix(err.Error(), "invalid x-api-key") {
		t.Fatalf("message should pass through verbatim, got: %v", err)
	}
}
