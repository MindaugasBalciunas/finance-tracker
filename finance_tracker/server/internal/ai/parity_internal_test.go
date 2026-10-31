package ai

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/mcptools"
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

// Ask CFO runs the MCP tools, so every argument its definitions offer must
// exist on the MCP tool — or the model's input would be silently dropped.
func TestToolArgumentsReachMCP(t *testing.T) {
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
	props := map[string]map[string]any{}
	for _, x := range res.Tools {
		s, _ := x.InputSchema.(map[string]any)
		p, _ := s["properties"].(map[string]any)
		props[x.Name] = p
	}
	handled := map[string]bool{"get_portfolio.include_trades": true} // RunTool adds the trades itself
	for _, d := range toolDefs() {
		if d.Type != "" {
			continue // server-side tools (web search)
		}
		n := d.Name
		if a, ok := mcpName[n]; ok {
			n = a
		}
		want, _ := d.InputSchema["properties"].(map[string]any)
		for k := range want {
			if _, ok := props[n][k]; !ok && !handled[d.Name+"."+k] {
				t.Errorf("Ask CFO's %s takes %q, the MCP tool %s doesn't", d.Name, k, n)
			}
		}
	}
}
