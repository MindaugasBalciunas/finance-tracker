package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func aiTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.AISettings{}, &domain.AIChatMessage{}, &domain.Budget{}, &domain.LabelRule{}, &domain.BudgetSettings{}, &domain.StockTrade{}, &domain.Asset{}, &domain.ExportLog{}, &domain.AIActivity{}, &domain.AIContext{}))

	// Chat context needs at least one transaction and one balance snapshot.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, -1, 0), Type: "expense", Amount: 50,
		Category: "Food", Comment: "Maxima", Labels: "groceries"}).Error)
	require.NoError(t, db.Create(&domain.Balance{Date: time.Now(), Total: 1000, Swed: 1000}).Error)

	txRepo := repository.NewTransactionRepository(db)
	balRepo := repository.NewBalanceRepository(db)
	insightRepo := repository.NewInsightRepository(db)
	budgetRepo := repository.NewBudgetRepository(db)
	stockRepo := repository.NewStockRepository(db)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	txSvc := service.NewTransactionService(txRepo, balSvc)
	insightSvc := service.NewInsightService(insightRepo, txSvc, balSvc, budgetRepo, service.NewStockService(stockRepo), service.NewAssetService(repository.NewAssetRepository(db)))

	r := gin.New()
	NewInsightHandler(insightSvc).RegisterRoutes(r.Group("/api/v1"))
	return r, db
}

// fakeGateway is an Anthropic-Messages-API stub (the nexos.ai native
// passthrough wire format) that records the last request.
func fakeGateway(t *testing.T, reply string) (*httptest.Server, *gatewayCapture) {
	t.Helper()
	cap := &gatewayCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Path = r.URL.Path
		cap.Auth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&cap.Req))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicText(reply)))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// anthropicText renders a plain text reply in the Messages API wire format.
func anthropicText(reply string) string {
	return `{"content":[{"type":"text","text":` + jsonString(reply) + `}],"stop_reason":"end_turn"}`
}

// promptText extracts the first message's text from a decoded request body —
// plain prompts travel as a single text block.
func promptText(body map[string]any) string {
	blocks := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	return blocks[0].(map[string]any)["text"].(string)
}

// systemText concatenates the top-level system blocks of a decoded request.
func systemText(body map[string]any) string {
	sys, _ := body["system"].([]any)
	var out string
	for _, b := range sys {
		out += b.(map[string]any)["text"].(string)
	}
	return out
}

type gatewayCapture struct {
	Path string
	Auth string
	Req  struct {
		Model  string `json:"model"`
		System []struct {
			Text         string         `json:"text"`
			CacheControl map[string]any `json:"cache_control"`
		} `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
}

func TestAISettingsRoundtrip(t *testing.T) {
	r, db := aiTestRouter(t)

	w := budgetDoJSON(r, "GET", "/api/v1/ai/settings", nil)
	require.Equal(t, 200, w.Code)
	var res map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, domain.DefaultGatewayURL, res["gateway_url"], "nexos.ai is the default gateway")
	assert.Equal(t, false, res["has_key"])

	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": "https://api.nexos.ai/v1/", "model": "gpt-4o", "api_key": "sk-secret-123"})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "sk-secret-123", "key never serialized")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, true, res["has_key"])
	assert.Equal(t, "https://api.nexos.ai/v1", res["gateway_url"], "trailing slash trimmed")

	// Saving with an empty key keeps the stored one.
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": "https://api.nexos.ai/v1", "model": "gpt-4o-mini", "api_key": ""})
	require.Equal(t, 200, w.Code)
	var s domain.AISettings
	require.NoError(t, db.First(&s, 1).Error)
	assert.Equal(t, "sk-secret-123", s.APIKey)
	assert.Equal(t, "gpt-4o-mini", s.Model)

	// clear_key removes it.
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": "https://api.nexos.ai/v1", "model": "gpt-4o-mini", "clear_key": true})
	require.Equal(t, 200, w.Code)
	require.NoError(t, db.First(&s, 1).Error)
	assert.Equal(t, "", s.APIKey)
}

func TestAIChatThroughGateway(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv, cap := fakeGateway(t, "You spent €50 on groceries.")

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "test-model", "api_key": "sk-test"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "how much did I spend?"})
	require.Equal(t, 200, w.Code, w.Body.String())
	var res map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, "You spent €50 on groceries.", res["reply"])

	assert.Equal(t, "/messages", cap.Path, "Anthropic-native passthrough endpoint")
	assert.Equal(t, "Bearer sk-test", cap.Auth)
	assert.Equal(t, "test-model", cap.Req.Model)
	require.NotEmpty(t, cap.Req.System, "system prompt travels as top-level blocks")
	assert.Contains(t, cap.Req.System[0].Text, "CURRENT BALANCE SNAPSHOT",
		"system block carries the financial data report")
	require.NotNil(t, cap.Req.System[len(cap.Req.System)-1].CacheControl,
		"last system block carries the prompt-cache breakpoint")
	require.GreaterOrEqual(t, len(cap.Req.Messages), 1)
	assert.Equal(t, "user", cap.Req.Messages[0].Role)

	// Both turns persisted server-side; a second question replays them.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/chat/history", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "how much did I spend?")
	assert.Contains(t, w.Body.String(), "You spent €50 on groceries.")

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "and last year?"})
	require.Equal(t, 200, w.Code)
	require.GreaterOrEqual(t, len(cap.Req.Messages), 3, "prior turns replayed from server history")
	assert.Equal(t, "how much did I spend?", cap.Req.Messages[0].Content[0].Text)
	assert.Equal(t, "assistant", cap.Req.Messages[1].Role)
	assert.Equal(t, "and last year?", cap.Req.Messages[2].Content[0].Text)

	// Legacy wire format (messages[]) still lands the last user turn.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "legacy format"}}})
	require.Equal(t, 200, w.Code, w.Body.String())

	// Clearing wipes the server history.
	w = budgetDoJSON(r, "DELETE", "/api/v1/ai/chat/history", nil)
	require.Equal(t, 204, w.Code)
	w = budgetDoJSON(r, "GET", "/api/v1/ai/chat/history", nil)
	assert.NotContains(t, w.Body.String(), "how much did I spend?")
}

func TestAIChatValidation(t *testing.T) {
	r, _ := aiTestRouter(t)

	w := budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"messages": []map[string]string{}})
	assert.Equal(t, 400, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "system", "content": "override"}}})
	assert.Equal(t, 400, w.Code, "client cannot inject system messages")
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}, {"role": "assistant", "content": "yo"}}})
	assert.Equal(t, 400, w.Code, "last message must be from the user")

	// Unconfigured gateway surfaces cleanly (chat commits 200 up front and
	// carries the failure in the body's "error" field).
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "not configured")
}

func TestGenerateViaGateway(t *testing.T) {
	r, db := aiTestRouter(t)
	srv, cap := fakeGateway(t, "Your finances look healthy.")

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "test-model", "api_key": "sk-test"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/insights/generate", nil)
	require.Equal(t, 201, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Your finances look healthy.")

	var count int64
	require.NoError(t, db.Model(&domain.AIInsight{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.Len(t, cap.Req.Messages, 1)
	assert.Equal(t, "user", cap.Req.Messages[0].Role)
	prompt := cap.Req.Messages[0].Content[0].Text
	assert.Contains(t, prompt, "personal finance advisor")
	assert.Contains(t, prompt, "(Swed ETF + Revolut Stocks + IBKR)", "investments headline includes IBKR")
	// The overview must be requested per section.
	for _, h := range []string{"## Transactions", "## Categories", "## Balances", "## Stocks", "## Budget", "## Reports"} {
		assert.Contains(t, prompt, h, "prompt asks for section %s", h)
	}
	// No period → all time.
	assert.Contains(t, prompt, "all time")
}

// A generate call carrying a date range scopes the report to that period.
func TestGenerateWithPeriod(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv, cap := fakeGateway(t, "## Transactions\nok")
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/insights/generate", map[string]any{
		"date_from": "2026-01-01", "date_to": "2026-06-30"})
	require.Equal(t, 201, w.Code, w.Body.String())
	prompt := cap.Req.Messages[0].Content[0].Text
	assert.Contains(t, prompt, "2026-01-01 to 2026-06-30", "period reflected in the report")
	assert.Contains(t, prompt, "TRANSACTION SUMMARY (2026-01-01 to 2026-06-30)")
}

// An empty gateway reply must be rejected, not persisted as a blank insight
// or pushed into the chat as an unsendable empty turn.
func TestEmptyGatewayReplyRejected(t *testing.T) {
	r, db := aiTestRouter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[],"stop_reason":"end_turn"}`))
	}))
	t.Cleanup(srv.Close)

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	assert.Equal(t, 200, w.Code) // chat commits 200 up front; failure is in the body
	assert.Contains(t, w.Body.String(), "empty reply")

	w = budgetDoJSON(r, "POST", "/api/v1/insights/generate", nil)
	assert.Equal(t, 500, w.Code)
	var count int64
	require.NoError(t, db.Model(&domain.AIInsight{}).Count(&count).Error)
	assert.Zero(t, count, "no blank insight persisted")
}

// A non-2xx gateway response must not echo the raw upstream body — with a
// user-controlled URL that would be an internal-endpoint read primitive.
func TestGatewayErrorDoesNotReflectBody(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("SECRET-INTERNAL-BODY db=postgres://u:p@localhost/prod"))
	}))
	t.Cleanup(srv.Close)

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	// Chat commits 200 up front (heartbeat streaming); the gateway failure
	// rides in the body's "error" field. The raw upstream body must still
	// never be reflected.
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "error")
	assert.NotContains(t, w.Body.String(), "SECRET-INTERNAL-BODY")
	assert.NotContains(t, w.Body.String(), "postgres://")
}

func TestGatewayTestEndpoint(t *testing.T) {
	r, _ := aiTestRouter(t)
	w := budgetDoJSON(r, "POST", "/api/v1/ai/test", nil)
	assert.Equal(t, 502, w.Code, "unconfigured gateway fails the test")

	srv, _ := fakeGateway(t, "ok")
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/test", nil)
	assert.Equal(t, 200, w.Code, w.Body.String())
}

// AI gateway settings must round-trip through the JSON backup so a
// wipe-and-restore doesn't lose the key (the "disappearing key" bug).
func TestAISettingsBackupRoundtrip(t *testing.T) {
	// Source instance with configured gateway.
	srcRouter, srcDB := aiTestRouter(t)
	w := budgetDoJSON(srcRouter, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": "https://api.nexos.ai/v1", "model": "gpt-5", "api_key": "nxs-roundtrip"})
	require.Equal(t, 200, w.Code)

	// Export via an export handler over the same DB.
	exportRouter := exportRouterFor(t, srcDB)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil)
	rec := httptest.NewRecorder()
	exportRouter.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "nxs-roundtrip", "full backup carries the key")

	// Import into a fresh instance.
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

	var s domain.AISettings
	require.NoError(t, dstDB.First(&s, 1).Error)
	assert.Equal(t, "nxs-roundtrip", s.APIKey)
	assert.Equal(t, "gpt-5", s.Model)
	assert.Equal(t, "https://api.nexos.ai/v1", s.GatewayURL)
}

// scriptedGateway returns canned responses in order and records every request.
func scriptedGateway(t *testing.T, responses []string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var requests []map[string]any
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		resp := responses[len(responses)-1] // repeat last when script runs out
		if i < len(responses) {
			resp = responses[i]
			i++
		}
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

// The chat runs an agentic loop: the model calls a read-only tool, the
// backend executes it against its own services and feeds the result back,
// and only the final answer is persisted.
func TestAIChatToolLoop(t *testing.T) {
	r, db := aiTestRouter(t)
	toolCallResp := `{"content":[{"type":"tool_use","id":"call_1","name":"search_transactions","input":{"label":"groceries","limit":5}}],"stop_reason":"tool_use"}`
	finalResp := anthropicText("You spent €50 at Maxima.")
	srv, requests := scriptedGateway(t, []string{toolCallResp, finalResp})

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "groceries this month?"})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "You spent €50 at Maxima.")

	require.Len(t, *requests, 2)
	// Request 1 declared the tool surface.
	tools, _ := (*requests)[0]["tools"].([]any)
	require.NotEmpty(t, tools, "tools must be declared to the gateway")
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"get_overview", "search_transactions", "get_summary", "get_budgets", "get_stock_quote", "get_assets"} {
		assert.True(t, names[want], "tool %s declared", want)
	}
	// Request 2 carried the executed tool result (the seeded Maxima row) as
	// a tool_result block in a user message.
	msgs := (*requests)[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	assert.Equal(t, "user", last["role"])
	tr := last["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_result", tr["type"])
	assert.Equal(t, "call_1", tr["tool_use_id"])
	assert.Contains(t, tr["content"].(string), "Maxima", "tool result contains the matching transaction")
	assert.Contains(t, tr["content"].(string), `"total_matches":1`)

	// Only the user question and final answer are persisted — no tool chatter.
	var count int64
	require.NoError(t, db.Model(&domain.AIChatMessage{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)

	// An unknown tool comes back as a recoverable error message.
	badCall := `{"content":[{"type":"tool_use","id":"call_2","name":"drop_tables","input":{}}],"stop_reason":"tool_use"}`
	srv2, requests2 := scriptedGateway(t, []string{badCall, finalResp})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv2.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "hi"})
	require.Equal(t, 200, w.Code, w.Body.String())
	msgs2 := (*requests2)[1]["messages"].([]any)
	last2 := msgs2[len(msgs2)-1].(map[string]any)
	tr2 := last2["content"].([]any)[0].(map[string]any)
	assert.Contains(t, tr2["content"].(string), "unknown tool", "unknown tools error back to the model")

	// A model that never stops calling tools is cut off at the round cap.
	srv3, requests3 := scriptedGateway(t, []string{toolCallResp})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv3.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "loop forever"})
	// The chat streams heartbeats and commits 200 up front, so a failure like
	// exceeding the tool-round cap rides in the body's "error" field.
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "tool rounds")
	assert.LessOrEqual(t, len(*requests3), 13, "loop is bounded (maxToolRounds + 1)")
}

// The view summary reviews the CURRENT tab: view-specific context goes to
// the gateway once, then the cached blurb serves repeat visits for free.
func TestViewSummary(t *testing.T) {
	r, _ := aiTestRouter(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits++
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		prompt := promptText(body)
		assert.Contains(t, prompt, "transactions", "prompt names the view")
		assert.Contains(t, prompt, "Maxima", "context carries the view's data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicText("Food dominates at €50.")))
	}))
	t.Cleanup(srv.Close)

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=transactions", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Food dominates")
	assert.Equal(t, 1, hits)

	// Second call: served from cache, no gateway spend.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=transactions", nil)
	require.Equal(t, 200, w.Code)
	assert.Equal(t, 1, hits, "cached — no second gateway call")

	// A different period is a different cache key.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=transactions&date_from=2020-01-01", nil)
	require.Equal(t, 200, w.Code)
	assert.Equal(t, 2, hits)

	// refresh=1 busts the cache.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=transactions&refresh=1", nil)
	require.Equal(t, 200, w.Code)
	assert.Equal(t, 3, hits)

	// Unknown views are rejected before any spend.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/view-summary?view=admin", nil)
	assert.Equal(t, 400, w.Code)
	assert.Equal(t, 3, hits)
}

// AI labeling: assist suggests from history, reindex proposes for unlabeled
// rows (vocabulary-only), apply is add-only and user-approved.
func TestAILabeling(t *testing.T) {
	r, db := aiTestRouter(t)
	// A labeled example (from the fixture) plus two unlabeled rows.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -3), Type: "expense", Amount: 12.5,
		Category: "Food", Comment: "50146 LIDL SNIPISKES", Labels: ""}).Error)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -2), Type: "expense", Amount: 30,
		Category: "Transport", Comment: "CIRCLE K VILNIUS", Labels: ""}).Error)

	// Scripted gateway: reindex chunk reply maps both rows; assist reply
	// suggests labels + a cleaned comment.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		prompt := promptText(body)
		w.Header().Set("Content-Type", "application/json")
		var reply string
		if strings.Contains(prompt, "TRANSACTIONS TO LABEL") {
			// find the ids the prompt actually carries
			assert.Contains(t, prompt, "LIDL")
			assert.Contains(t, prompt, "groceries", "vocabulary offered to the model")
			reply = `[{"id":2,"labels":["groceries","junk-label"]},{"id":3,"labels":["fuel"]},{"id":999,"labels":["groceries"]}]`
		} else {
			assert.Contains(t, prompt, "TRANSACTION TO TAG")
			reply = `{"labels":["groceries"],"comment":"Lidl Šnipiškės groceries","note":"Matches your Lidl pattern."}`
		}
		_, _ = w.Write([]byte(anthropicText(reply)))
	}))
	t.Cleanup(srv.Close)
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	// Single-transaction assist.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/assist-transaction", map[string]any{
		"date": "2026-08-17", "type": "expense", "category": "Food",
		"amount": 12.5, "comment": "50146 LIDL SNIPISKES"})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "groceries")
	assert.Contains(t, w.Body.String(), "Lidl Šnipiškės groceries")

	// Reindex: only known transactions and vocabulary labels survive.
	// 'fuel' is not in this tiny DB's vocabulary (only 'groceries' is), and
	// id 999 does not exist — both must be dropped.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var res struct {
		Suggestions []struct {
			ID  uint     `json:"id"`
			Add []string `json:"add"`
		} `json:"suggestions"`
		Scanned   int `json:"scanned"`
		Remaining int `json:"remaining_unlabeled"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Suggestions, 1, w.Body.String())
	assert.EqualValues(t, 2, res.Suggestions[0].ID)
	assert.Equal(t, []string{"groceries"}, res.Suggestions[0].Add, "junk-label filtered by vocabulary")
	assert.Equal(t, 2, res.Scanned)

	// Apply is add-only and preserves accounts.
	var before domain.Transaction
	require.NoError(t, db.First(&before, 2).Error)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex/apply", map[string]any{
		"items": []map[string]any{{"id": 2, "labels": []string{"groceries"}}}})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"applied":1`)
	var after domain.Transaction
	require.NoError(t, db.First(&after, 2).Error)
	assert.Equal(t, "groceries", after.Labels)
	assert.Equal(t, before.DebitAccount, after.DebitAccount, "accounts survive apply")

	// Re-applying the same labels is a no-op, not a duplicate.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex/apply", map[string]any{
		"items": []map[string]any{{"id": 2, "labels": []string{"groceries"}}}})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"applied":0`)
}

// Review mode audits LABELED rows and proposes remaps; fixed-obligation
// labels can never be removed — not by suggestion, not at apply time.
func TestAIReviewRemap(t *testing.T) {
	r, db := aiTestRouter(t)
	// Row 2: mislabeled (a bus ticket carrying 'bar'); row 3 carries a fixed label.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -2), Type: "expense", Amount: 2,
		Category: "Transport", Comment: "Public transport ticket", Labels: "bar"}).Error)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -1), Type: "expense", Amount: 1285,
		Category: "Finance", Comment: "Busto paskola", Labels: "loan"}).Error)
	// Vocabulary needs 'transport' to exist somewhere.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -5), Type: "expense", Amount: 30,
		Category: "Transport", Comment: "CIRCLE K", Labels: "transport"}).Error)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		prompt := promptText(body)
		require.Contains(t, prompt, "TRANSACTIONS TO AUDIT")
		assert.Contains(t, prompt, `labels: bar`, "audit prompt carries current labels")
		// Model proposes: fix row 2; also (maliciously) strip 'loan' from row 3
		// and remove a label row 2 doesn't have.
		reply := `[{"id":2,"remove":["bar","ghost"],"add":["transport"],"reason":"Bus ticket, not a bar."},{"id":3,"remove":["loan"],"add":["transport"],"reason":"nope"}]`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicText(reply)))
	}))
	t.Cleanup(srv.Close)
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex", map[string]any{"mode": "review"})
	require.Equal(t, 200, w.Code, w.Body.String())
	var res struct {
		Suggestions []struct {
			ID      uint     `json:"id"`
			Current []string `json:"current"`
			Add     []string `json:"add"`
			Remove  []string `json:"remove"`
			Reason  string   `json:"reason"`
		} `json:"suggestions"`
		Scanned int `json:"scanned"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	// Row 3's only proposed change was removing 'loan' (blocked) + adding
	// transport — the add survives, so it may appear; row 2 must be the fix.
	var fix *struct {
		ID      uint     `json:"id"`
		Current []string `json:"current"`
		Add     []string `json:"add"`
		Remove  []string `json:"remove"`
		Reason  string   `json:"reason"`
	}
	for i := range res.Suggestions {
		if res.Suggestions[i].ID == 2 {
			fix = &res.Suggestions[i]
		}
		if res.Suggestions[i].ID == 3 {
			assert.NotContains(t, res.Suggestions[i].Remove, "loan", "fixed labels never suggested for removal")
		}
	}
	require.NotNil(t, fix, w.Body.String())
	assert.Equal(t, []string{"transport"}, fix.Add)
	assert.Equal(t, []string{"bar"}, fix.Remove, "'ghost' dropped — not on the row")
	assert.Equal(t, []string{"bar"}, fix.Current)
	assert.Contains(t, fix.Reason, "Bus ticket")

	// Apply the remap: bar out, transport in.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex/apply", map[string]any{
		"items": []map[string]any{{"id": 2, "add": []string{"transport"}, "remove": []string{"bar"}}}})
	require.Equal(t, 200, w.Code, w.Body.String())
	var afterFix domain.Transaction
	require.NoError(t, db.First(&afterFix, 2).Error)
	assert.Equal(t, "transport", afterFix.Labels)

	// A direct attempt to remove a fixed label via apply is ignored.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex/apply", map[string]any{
		"items": []map[string]any{{"id": 3, "remove": []string{"loan"}}}})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"applied":0`)
	var afterHostile domain.Transaction
	require.NoError(t, db.First(&afterHostile, 3).Error)
	assert.Equal(t, "loan", afterHostile.Labels, "fixed label survives a hostile apply")

	// Unknown mode is rejected.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/label-reindex", map[string]any{"mode": "chaos"})
	assert.Equal(t, 400, w.Code)
}
