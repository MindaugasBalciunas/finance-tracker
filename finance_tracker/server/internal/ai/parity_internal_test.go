package ai

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/mcptools"
	. "ft/internal/testutil"
)

// Ask CFO and the MCP tools (stdio server and Claude's connector) offer the
// same things under the same names, and agree on which ones write — so a
// feature added to one isn't missing from the other.
func TestToolParityWithMCP(t *testing.T) {
	// Same tool, older name in Ask CFO.
	alias := map[string]string{"add_rule": "save_rule", "add_recurring": "save_recurring", "get_inbox": "get_bank_inbox"}
	// Ask CFO only: the provider's server-side web search has no MCP equivalent.
	onlyCFO := map[string]bool{"web_search": true}
	// Deliberately on one side only.
	onlyMCP := map[string]string{
		"get_user_context":     "Ask CFO puts the owner's brief and memory in its system prompt",
		"get_trades":           "raw trade list for external analysis; Ask CFO uses get_portfolio",
		"delete_recurring":     "Ask CFO hides a detected item through add_recurring",
		"get_bank_connections": "bank consent status; Ask CFO sees it in get_overview and get_checks",
	}

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
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	mcpTools := map[string]bool{}
	for _, x := range res.Tools {
		mcpTools[x.Name] = true
	}
	cfoTools := map[string]bool{}
	for _, d := range toolDefs() {
		n := d.Name
		if a, ok := alias[n]; ok {
			n = a
		}
		cfoTools[n] = true
		if !mcpTools[n] && !onlyCFO[n] {
			t.Errorf("Ask CFO has %s, the MCP tools don't", d.Name)
		}
	}
	for n := range mcpTools {
		if !cfoTools[n] && onlyMCP[n] == "" {
			t.Errorf("the MCP tools have %s, Ask CFO doesn't", n)
		}
	}
	var cfoWrites []string
	for n := range writeTools {
		if a, ok := alias[n]; ok {
			n = a
		}
		cfoWrites = append(cfoWrites, n)
	}
	var mcpWrites []string
	for _, n := range mcptools.WriteTools {
		if onlyMCP[n] == "" {
			mcpWrites = append(mcpWrites, n)
		}
	}
	slices.Sort(cfoWrites)
	slices.Sort(mcpWrites)
	if !slices.Equal(cfoWrites, mcpWrites) {
		t.Errorf("write tools differ:\nAsk CFO %v\nMCP     %v", cfoWrites, mcpWrites)
	}
}

// The views added for parity answer in Ask CFO too, and a trip's dates are
// found however the assistant spells its tag.
func TestParityToolsInAskCFO(t *testing.T) {
	a := &Assistant{DB: DB(t)}
	run := func(name, args string) string {
		t.Helper()
		out, err := a.runTool(name, json.RawMessage(args))
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
