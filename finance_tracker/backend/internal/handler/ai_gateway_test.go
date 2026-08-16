package handler

import (
	"encoding/json"
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
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{}, &domain.AISettings{}, &domain.Budget{}, &domain.LabelRule{}, &domain.BudgetSettings{}, &domain.StockTrade{}))

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
	insightSvc := service.NewInsightService(insightRepo, txSvc, balSvc, budgetRepo, service.NewStockService(stockRepo))

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

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "how much did I spend?"},
		}})
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