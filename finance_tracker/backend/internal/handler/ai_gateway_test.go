package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.AISettings{}, &domain.AIChatMessage{}, &domain.Budget{}, &domain.LabelRule{}, &domain.BudgetSettings{}, &domain.StockTrade{}, &domain.Asset{}, &domain.ExportLog{}))

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

// fakeGateway is an OpenAI-compatible stub that records the last request.
func fakeGateway(t *testing.T, reply string) (*httptest.Server, *gatewayCapture) {
	t.Helper()
	cap := &gatewayCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Path = r.URL.Path
		cap.Auth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&cap.Req))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + jsonString(reply) + `}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type gatewayCapture struct {
	Path string
	Auth string
	Req  struct {
		Model    string               `json:"model"`
		Messages []domain.ChatMessage `json:"messages"`
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

	assert.Equal(t, "/chat/completions", cap.Path)
	assert.Equal(t, "Bearer sk-test", cap.Auth)
	assert.Equal(t, "test-model", cap.Req.Model)
	require.GreaterOrEqual(t, len(cap.Req.Messages), 2)
	assert.Equal(t, "system", cap.Req.Messages[0].Role)
	assert.Contains(t, cap.Req.Messages[0].Content, "CURRENT BALANCE SNAPSHOT",
		"system message carries the financial data report")
	assert.Equal(t, "user", cap.Req.Messages[1].Role)

	// Both turns persisted server-side; a second question replays them.
	w = budgetDoJSON(r, "GET", "/api/v1/ai/chat/history", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "how much did I spend?")
	assert.Contains(t, w.Body.String(), "You spent €50 on groceries.")

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "and last year?"})
	require.Equal(t, 200, w.Code)
	require.GreaterOrEqual(t, len(cap.Req.Messages), 4, "prior turns replayed from server history")
	assert.Equal(t, "how much did I spend?", cap.Req.Messages[1].Content)
	assert.Equal(t, "assistant", cap.Req.Messages[2].Role)
	assert.Equal(t, "and last year?", cap.Req.Messages[3].Content)

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

	// Unconfigured gateway is a clear 502, not a crash.
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	assert.Equal(t, 502, w.Code)
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
	prompt := cap.Req.Messages[0].Content
	assert.Contains(t, prompt, "personal finance advisor")
	// The overview must be requested per section.
	for _, h := range []string{"## Transactions", "## Balances", "## Stocks", "## Budget", "## Reports"} {
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
	prompt := cap.Req.Messages[0].Content
	assert.Contains(t, prompt, "2026-01-01 to 2026-06-30", "period reflected in the report")
	assert.Contains(t, prompt, "TRANSACTION SUMMARY (2026-01-01 to 2026-06-30)")
}

// An empty gateway reply must be rejected, not persisted as a blank insight
// or pushed into the chat as an unsendable empty turn.
func TestEmptyGatewayReplyRejected(t *testing.T) {
	r, db := aiTestRouter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	t.Cleanup(srv.Close)

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	assert.Equal(t, 502, w.Code)
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
	assert.Equal(t, 502, w.Code)
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
	toolCallResp := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"search_transactions","arguments":"{\"label\":\"groceries\",\"limit\":5}"}}]},"finish_reason":"tool_calls"}]}`
	finalResp := `{"choices":[{"message":{"role":"assistant","content":"You spent €50 at Maxima."},"finish_reason":"stop"}]}`
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
		fn := tl.(map[string]any)["function"].(map[string]any)
		names[fn["name"].(string)] = true
	}
	for _, want := range []string{"get_overview", "search_transactions", "get_summary", "get_budgets", "get_stock_quote", "get_assets"} {
		assert.True(t, names[want], "tool %s declared", want)
	}
	// Request 2 carried the executed tool result (the seeded Maxima row).
	msgs := (*requests)[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	assert.Equal(t, "tool", last["role"])
	assert.Equal(t, "call_1", last["tool_call_id"])
	assert.Contains(t, last["content"].(string), "Maxima", "tool result contains the matching transaction")
	assert.Contains(t, last["content"].(string), `"total_matches":1`)

	// Only the user question and final answer are persisted — no tool chatter.
	var count int64
	require.NoError(t, db.Model(&domain.AIChatMessage{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)

	// An unknown tool comes back as a recoverable error message.
	badCall := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_2","type":"function","function":{"name":"drop_tables","arguments":"{}"}}]}}]}`
	srv2, requests2 := scriptedGateway(t, []string{badCall, finalResp})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv2.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "hi"})
	require.Equal(t, 200, w.Code, w.Body.String())
	msgs2 := (*requests2)[1]["messages"].([]any)
	last2 := msgs2[len(msgs2)-1].(map[string]any)
	assert.Contains(t, last2["content"].(string), "unknown tool", "unknown tools error back to the model")

	// A model that never stops calling tools is cut off at the round cap.
	srv3, requests3 := scriptedGateway(t, []string{toolCallResp})
	w = budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv3.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "loop forever"})
	assert.Equal(t, 502, w.Code)
	assert.Contains(t, w.Body.String(), "tool rounds")
	assert.LessOrEqual(t, len(*requests3), 7, "loop is bounded")
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
		msgs := body["messages"].([]any)
		prompt := msgs[0].(map[string]any)["content"].(string)
		assert.Contains(t, prompt, "transactions", "prompt names the view")
		assert.Contains(t, prompt, "Maxima", "context carries the view's data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Food dominates at €50."}}]}`))
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
