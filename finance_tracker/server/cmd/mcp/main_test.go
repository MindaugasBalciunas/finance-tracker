package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/advice"
)

// The server hands clients the shared rules, lists the ready-made analyses
// and fills them in, and the reconciliation tool reads the app's report.
func TestInstructionsPromptsAndReconciliation(t *testing.T) {
	var hits []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer ftk_test" {
			http.Error(w, "no", 401)
			return
		}
		w.Write([]byte(`[{"account_id":"swed","unexplained":0}]`))
	}))
	defer api.Close()
	apiURL, apiToken = api.URL+"/api", "ftk_test"
	now = func() time.Time { return time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC) }
	defer func() { now = time.Now }()

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := newServer().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	ins := cs.InitializeResult().Instructions
	for _, want := range []string{advice.DataModel, advice.Strategy, advice.Trust, "get_user_context first", "reconcile_recent"} {
		if !strings.Contains(ins, want) {
			t.Errorf("instructions lack %.50q", want)
		}
	}
	ps, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps.Prompts {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "budget_review,month_close,reconcile_recent,wishlist_review" {
		t.Fatalf("prompts %v", names)
	}
	get := func(name string, args map[string]string) string {
		t.Helper()
		r, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return r.Messages[0].Content.(*mcp.TextContent).Text
	}
	// On 31 March the last finished month is February, not "3 March".
	if p := get("month_close", nil); !strings.Contains(p, "Close February 2026") || !strings.Contains(p, "from 2026-02-01 to 2026-02-28") || !strings.Contains(p, "cash_flow from 2025-08-01") {
		t.Errorf("month_close default:\n%s", p)
	}
	if p := get("month_close", map[string]string{"month": "2026-09"}); !strings.Contains(p, "2026-09-30") || !strings.Contains(p, "get_goals month=2026-09") {
		t.Errorf("month_close september:\n%s", p)
	}
	if p := get("budget_review", map[string]string{"month": "2026-10"}); !strings.Contains(p, "2026-09, 2026-08 and 2026-07") {
		t.Errorf("budget_review:\n%s", p)
	}
	if p := get("reconcile_recent", nil); !strings.Contains(p, "get_reconciliation from=2026-03-01") {
		t.Errorf("reconcile_recent default:\n%s", p)
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "month_close", Arguments: map[string]string{"month": "09/2026"}}); err == nil {
		t.Error("bad month accepted")
	}
	if get("wishlist_review", nil) == "" {
		t.Error("empty wishlist_review")
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_reconciliation", Arguments: map[string]any{"from": "2026-10-04"}})
	if err != nil || res.IsError {
		t.Fatalf("tool: %v %+v", err, res)
	}
	if len(hits) != 1 || hits[0] != "/api/insights/reconcile?from=2026-10-04" {
		t.Errorf("api calls %v", hits)
	}
}
