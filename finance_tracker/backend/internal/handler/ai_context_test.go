package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindaugas/finance-tracker/internal/domain"
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

// The CFO context travels with the master backup and comes back on restore
// into a fresh instance — but never clobbers a briefing that already exists.
func TestAIContextBackupRoundtrip(t *testing.T) {
	srcRouter, srcDB := aiTestRouter(t)
	doc := "# CFO briefing\nVWCE-only core. Direct tone."
	w := budgetDoJSON(srcRouter, "PUT", "/api/v1/ai/context", map[string]any{"content": doc})
	require.Equal(t, 200, w.Code)

	exportRouter := exportRouterFor(t, srcDB)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil)
	rec := httptest.NewRecorder()
	exportRouter.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "VWCE-only core", "backup carries the context")
	assert.Contains(t, rec.Body.String(), `"schema_version":4`)

	importRouter, dstDB := importRouterFor(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "finances.json")
	require.NoError(t, err)
	_, err = fw.Write(rec.Body.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	ireq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", &buf)
	ireq.Header.Set("Content-Type", mw.FormDataContentType())
	irec := httptest.NewRecorder()
	importRouter.ServeHTTP(irec, ireq)
	require.Equal(t, 200, irec.Code, irec.Body.String())

	var restored domain.AIContext
	require.NoError(t, dstDB.First(&restored, 1).Error)
	assert.Equal(t, doc, restored.Content)

	// A second import with different content must NOT clobber the existing one.
	older := strings.Replace(rec.Body.String(), "VWCE-only core", "STALE OLD RULES", 1)
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	fw2, _ := mw2.CreateFormFile("file", "finances.json")
	_, _ = fw2.Write([]byte(older))
	require.NoError(t, mw2.Close())
	ireq2 := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", &buf2)
	ireq2.Header.Set("Content-Type", mw2.FormDataContentType())
	irec2 := httptest.NewRecorder()
	importRouter.ServeHTTP(irec2, ireq2)
	require.Equal(t, 200, irec2.Code)
	require.NoError(t, dstDB.First(&restored, 1).Error)
	assert.Contains(t, restored.Content, "VWCE-only core", "existing briefing preserved")
}

// The AI dataset ZIP includes the briefing as cfo-context.md (only when set).
func TestAIZipCarriesContext(t *testing.T) {
	r, db := aiTestRouter(t)
	exportRouter := exportRouterFor(t, db)

	fetchZipNames := func() map[string]string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/export/ai.zip", nil)
		rec := httptest.NewRecorder()
		exportRouter.ServeHTTP(rec, req)
		require.Equal(t, 200, rec.Code)
		zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
		require.NoError(t, err)
		files := map[string]string{}
		for _, f := range zr.File {
			rc, err := f.Open()
			require.NoError(t, err)
			b, _ := io.ReadAll(rc)
			rc.Close()
			files[f.Name] = string(b)
		}
		return files
	}

	files := fetchZipNames()
	_, has := files["cfo-context.md"]
	assert.False(t, has, "no context file while the briefing is empty")

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/context", map[string]any{"content": "# Briefing\nFollow the 50% rule."})
	require.Equal(t, 200, w.Code)
	files = fetchZipNames()
	require.Contains(t, files, "cfo-context.md")
	assert.Contains(t, files["cfo-context.md"], "50% rule")
	assert.Contains(t, files["README.md"], "cfo-context.md", "README tells the AI to read it first")
}

// The chat's record_user_decision tool appends a dated entry to the context
// document — the only write in the chat loop, and it must confine itself to
// the Decisions log.
func TestRecordUserDecision(t *testing.T) {
	r, db := aiTestRouter(t)
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/context", map[string]any{"content": "# Briefing\nRules here."})
	require.Equal(t, 200, w.Code)

	call := `{"content":[{"type":"tool_use","id":"d1","name":"record_user_decision","input":{"decision":"Buffer floor revised to €15,000."}}],"stop_reason":"tool_use"}`
	final := anthropicText("Recorded: buffer floor €15,000.")
	srv, requests := scriptedGateway(t, []string{call, final})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "the buffer floor is now 15k, record it"})
	require.Equal(t, 200, w.Code, w.Body.String())

	var ctx domain.AIContext
	require.NoError(t, db.First(&ctx, 1).Error)
	assert.Contains(t, ctx.Content, "# Briefing\nRules here.", "original content untouched")
	assert.Contains(t, ctx.Content, "## Decisions log")
	assert.Contains(t, ctx.Content, time.Now().Format("2006-01-02")+": Buffer floor revised to €15,000.")

	// The tool result confirmed the write back to the model.
	toolMsg := extractToolContent(t, (*requests)[1], "d1")
	assert.Contains(t, toolMsg, "appended to the user's context document")

	// A second decision appends to the SAME log without duplicating the header.
	call2 := `{"content":[{"type":"tool_use","id":"d2","name":"record_user_decision","input":{"decision":"Monthly VWCE target raised to €2,000."}}],"stop_reason":"tool_use"}`
	srv2, _ := scriptedGateway(t, []string{call2, final})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{"gateway_url": srv2.URL, "model": "m"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "and vwce target 2000"})
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, db.First(&ctx, 1).Error)
	assert.Equal(t, 1, strings.Count(ctx.Content, "## Decisions log"), "one log section")
	assert.Contains(t, ctx.Content, "VWCE target raised")
}

// A removal-only review suggestion must serialize add as [] — a null there
// blank-screened the AI-tagging page (s.add.map crash).
func TestReviewRemovalOnlySerializesEmptyAdd(t *testing.T) {
	r, db := aiTestRouter(t)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -1), Type: "expense", Amount: 5,
		Category: "Transport", Comment: "Bus ticket", Labels: "bar"}).Error)

	reply := `[{"id":2,"remove":["bar"],"reason":"not a bar"}]`
	srv, _ := scriptedGateway(t, []string{anthropicText(reply)})
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex", map[string]any{"mode": "review"})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"add":[]`, "add must be an empty array, never null")
	assert.NotContains(t, w.Body.String(), `"add":null`)
}
