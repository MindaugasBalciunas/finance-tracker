package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// The AI integrations share a short-term memory: a view summary logs its
// digest, and the next chat call sees it in the system message — so features
// build on each other instead of repeating themselves.
func TestAIMemoryAcrossIntegrations(t *testing.T) {
	r, db := aiTestRouter(t)

	// 1. A view summary is generated (fake gateway) and logged as activity.
	srv1, _ := fakeGateway(t, "Groceries doubled this month vs your average.")
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv1.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=dashboard&refresh=1", nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	var acts []domain.AIActivity
	require.NoError(t, db.Find(&acts).Error)
	require.Len(t, acts, 1)
	assert.Equal(t, "view_summary", acts[0].Kind)
	assert.Contains(t, acts[0].Content, "Groceries doubled")

	// 2. Chat sees the digest in its system message and logs its own.
	final := `{"choices":[{"message":{"role":"assistant","content":"Yes — groceries are the outlier."},"finish_reason":"stop"}]}`
	srv2, requests := scriptedGateway(t, []string{final})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv2.URL, "model": "m"}) // empty api_key keeps the stored key
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "anything unusual lately?"})
	require.Equal(t, 200, w.Code, w.Body.String())

	require.NotEmpty(t, *requests)
	system := (*requests)[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	assert.Contains(t, system, "RECENT AI ACTIVITY")
	assert.Contains(t, system, "view_summary dashboard")
	assert.Contains(t, system, "Groceries doubled")

	require.NoError(t, db.Order("id ASC").Find(&acts).Error)
	require.Len(t, acts, 2)
	assert.Equal(t, "chat", acts[1].Kind)
	assert.Contains(t, acts[1].Content, "Q: anything unusual lately?")
	assert.Contains(t, acts[1].Content, "A: Yes — groceries are the outlier.")
}

// Replayed chat history is trimmed for the gateway: turns older than the
// 7-day window are dropped and oversized turns are cut — stored history
// stays complete.
func TestChatReplayTrimming(t *testing.T) {
	r, db := aiTestRouter(t)
	long := "LONGMSG " + strings.Repeat("x", 3000)
	require.NoError(t, db.Create(&domain.AIChatMessage{
		Role: "user", Content: "ANCIENT question about crypto", CreatedAt: time.Now().AddDate(0, 0, -8)}).Error)
	require.NoError(t, db.Create(&domain.AIChatMessage{
		Role: "assistant", Content: long, CreatedAt: time.Now().AddDate(0, 0, -1)}).Error)
	require.NoError(t, db.Create(&domain.AIChatMessage{
		Role: "user", Content: "short recent question", CreatedAt: time.Now().AddDate(0, 0, -1)}).Error)

	final := `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
	srv, requests := scriptedGateway(t, []string{final})
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "and now?"})
	require.Equal(t, 200, w.Code, w.Body.String())

	require.NotEmpty(t, *requests)
	blob, err := json.Marshal((*requests)[0]["messages"])
	require.NoError(t, err)
	sent := string(blob)
	assert.NotContains(t, sent, "ANCIENT", "8-day-old turn not replayed")
	assert.Contains(t, sent, "short recent question")
	assert.Contains(t, sent, "[earlier answer trimmed]", "oversized turn truncated")
	assert.NotContains(t, sent, strings.Repeat("x", 2000), "full long body never sent")

	// Stored history is untouched: the full long message is still there.
	var stored []domain.AIChatMessage
	require.NoError(t, db.Find(&stored).Error)
	found := false
	for _, m := range stored {
		if strings.HasPrefix(m.Content, "LONGMSG") && len(m.Content) > 3000 {
			found = true
		}
	}
	assert.True(t, found, "persisted history keeps the full message")
}
