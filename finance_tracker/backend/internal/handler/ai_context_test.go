package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CFO-context document round-trips, is size-capped, and gets injected
// into the AI calls as a (cached) system block.
func TestAIContextRoundtripAndInjection(t *testing.T) {
	r, _ := aiTestRouter(t)

	// Empty by default.
	w := budgetDoJSON(r, "GET", "/api/v1/ai/context", nil)
	require.Equal(t, 200, w.Code)
	var ctx struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ctx))
	assert.Equal(t, "", ctx.Content)

	// Save and read back.
	doc := "# Who I am\nEngineering manager in Vilnius. VWCE-only core; satellites capped at €1,500.\nRULE: direct, no sugar-coating."
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/context", map[string]any{"content": doc})
	require.Equal(t, 200, w.Code, w.Body.String())
	w = budgetDoJSON(r, "GET", "/api/v1/ai/context", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ctx))
	assert.Equal(t, doc, ctx.Content)

	// Oversized documents are rejected with a clear error.
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/context", map[string]any{"content": strings.Repeat("x", 33_000)})
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "32000")

	// Chat: the system prompt carries the context block.
	srv, cap := fakeGateway(t, "Understood — advising per your framework.")
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "status?"})
	require.Equal(t, 200, w.Code, w.Body.String())
	var sys string
	for _, b := range cap.Req.System {
		sys += b.Text
	}
	assert.Contains(t, sys, "USER CFO CONTEXT")
	assert.Contains(t, sys, "VWCE-only core")

	// View summary: context travels as its own system block there too.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=budget&refresh=1", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	sys = ""
	for _, b := range cap.Req.System {
		sys += b.Text
	}
	assert.Contains(t, sys, "USER CFO CONTEXT", "view reviews advise with the same context")
}

// The context route is reachable by the read-only API token (MCP), while
// writing stays session-only by the GET-only token rule.
func TestAIContextTokenReachable(t *testing.T) {
	assert.True(t, apiTokenAllowed("/api/v1/ai/context"))
	assert.False(t, apiTokenAllowed("/api/v1/ai/contextual-anything"))
}
