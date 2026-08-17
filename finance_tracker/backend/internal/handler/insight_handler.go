package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type InsightHandler struct {
	svc service.InsightService
}

func NewInsightHandler(svc service.InsightService) *InsightHandler {
	return &InsightHandler{svc: svc}
}

func (h *InsightHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/insights")
	g.GET("/latest", h.GetLatest)
	g.POST("/generate", h.Generate)
	g.GET("", h.List)

	ai := rg.Group("/ai")
	ai.GET("/settings", h.GetAISettings)
	ai.PUT("/settings", h.SaveAISettings)
	ai.POST("/test", h.TestGateway)
	ai.POST("/chat", h.Chat)
	ai.GET("/chat/history", h.ChatHistory)
	ai.DELETE("/chat/history", h.ClearChat)
	// Read-only data report for machine clients (MCP get_overview) — plain
	// financial aggregates, no secrets, allowlisted for API-token access.
	ai.GET("/report", h.DataReport)
	// Auto review of the currently open view. Session-only (not in the API
	// token allowlist): every uncached call spends gateway tokens.
	ai.GET("/view-summary", h.ViewSummary)
	// AI labeling: single-transaction assist, bulk reindex suggestions and
	// user-approved apply. All session-only POSTs.
	ai.POST("/assist-transaction", h.AssistTransaction)
	ai.POST("/label-reindex", h.LabelReindex)
	ai.POST("/label-reindex/apply", h.ApplyLabelSuggestions)
	// AI audit of the auto-labeling rule set, and user-approved apply.
	ai.POST("/rule-review", h.RuleReview)
	ai.POST("/rule-review/apply", h.ApplyRuleSuggestions)
}

func (h *InsightHandler) AssistTransaction(c *gin.Context) {
	var input service.TransactionAssistInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.svc.AssistTransaction(input)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *InsightHandler) LabelReindex(c *gin.Context) {
	var input struct {
		Mode   string `json:"mode"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	_ = c.ShouldBindJSON(&input)
	out, err := h.svc.ReindexSuggest(input.Mode, input.Limit, input.Offset)
	if err != nil {
		if strings.Contains(err.Error(), "unknown mode") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// RuleReview asks the AI to audit the auto-labeling rule set and returns
// validated suggestions with live match counts. Suggestion-only.
func (h *InsightHandler) RuleReview(c *gin.Context) {
	out, err := h.svc.RuleReview()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// ApplyRuleSuggestions writes the user-approved rule changes.
func (h *InsightHandler) ApplyRuleSuggestions(c *gin.Context) {
	var input struct {
		Items []service.RuleApplyItem `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items must contain 1-100 suggestions"})
		return
	}
	out, err := h.svc.ApplyRuleSuggestions(input.Items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *InsightHandler) ApplyLabelSuggestions(c *gin.Context) {
	var input struct {
		Items []service.LabelApplyItem `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.Items) == 0 || len(input.Items) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items must contain 1-500 suggestions"})
		return
	}
	applied, err := h.svc.ApplyLabelSuggestions(input.Items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"applied": applied})
}

func (h *InsightHandler) ViewSummary(c *gin.Context) {
	view := c.Query("view")
	from := parseInsightDate(c.Query("date_from"))
	to := parseInsightDate(c.Query("date_to"))
	refresh := c.Query("refresh") == "1"
	summary, err := h.svc.ViewSummary(view, from, to, refresh)
	if err != nil {
		if strings.Contains(err.Error(), "unknown view") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}

func (h *InsightHandler) DataReport(c *gin.Context) {
	report, err := h.svc.DataReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"report": report})
}

// aiSettingsResponse never carries the key itself — only whether one is set.
type aiSettingsResponse struct {
	GatewayURL string `json:"gateway_url"`
	Model      string `json:"model"`
	HasKey     bool   `json:"has_key"`
	UpdatedAt  string `json:"updated_at"`
}

func (h *InsightHandler) GetAISettings(c *gin.Context) {
	s, err := h.svc.AISettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, aiSettingsResponse{
		GatewayURL: s.GatewayURL, Model: s.Model, HasKey: s.APIKey != "",
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	})
}

type aiSettingsInput struct {
	GatewayURL string `json:"gateway_url"`
	Model      string `json:"model"`
	// APIKey empty = keep the stored key; ClearKey removes it explicitly.
	APIKey   string `json:"api_key"`
	ClearKey bool   `json:"clear_key"`
}

func (h *InsightHandler) SaveAISettings(c *gin.Context) {
	var input aiSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s, err := h.svc.SaveAISettings(input.GatewayURL, input.Model, input.APIKey, input.ClearKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, aiSettingsResponse{
		GatewayURL: s.GatewayURL, Model: s.Model, HasKey: s.APIKey != "",
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *InsightHandler) TestGateway(c *gin.Context) {
	if err := h.svc.TestGateway(); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type chatInput struct {
	// Message is the new user turn; history is stored server-side.
	Message string `json:"message"`
	// Messages is the pre-v1.14 wire format — a stale open tab may still
	// send full history; only its last user message is used.
	Messages []domain.ChatMessage `json:"messages"`
}

func (h *InsightHandler) Chat(c *gin.Context) {
	var input chatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	message := strings.TrimSpace(input.Message)
	if message == "" && len(input.Messages) > 0 {
		if last := input.Messages[len(input.Messages)-1]; last.Role == "user" {
			message = strings.TrimSpace(last.Content)
		}
	}
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must not be blank"})
		return
	}
	reply, err := h.svc.Chat(message)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reply": reply})
}

func (h *InsightHandler) ChatHistory(c *gin.Context) {
	msgs, err := h.svc.ChatHistory()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (h *InsightHandler) ClearChat(c *gin.Context) {
	if err := h.svc.ClearChat(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetLatest godoc
// @Summary      Get latest AI insight
// @Tags         insights
// @Produce      json
// @Success      200  {object}  domain.AIInsight
// @Failure      404  {object}  ErrorResponse
// @Router       /insights/latest [get]
func (h *InsightHandler) GetLatest(c *gin.Context) {
	insight, err := h.svc.GetLatest()
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no insights yet"})
		return
	}
	c.JSON(http.StatusOK, insight)
}

// Generate godoc
// @Summary      Generate a new AI financial insight
// @Tags         insights
// @Produce      json
// @Success      201  {object}  domain.AIInsight
// @Failure      500  {object}  ErrorResponse
// @Router       /insights/generate [post]
func (h *InsightHandler) Generate(c *gin.Context) {
	// Optional reporting period — the app's selected date range. Absent or
	// unparseable dates mean all-time. Body is optional (a bare POST works).
	var input struct {
		DateFrom string `json:"date_from"`
		DateTo   string `json:"date_to"`
	}
	_ = c.ShouldBindJSON(&input)
	dateFrom := parseInsightDate(input.DateFrom)
	dateTo := parseInsightDate(input.DateTo)

	insight, err := h.svc.Generate(dateFrom, dateTo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, insight)
}

// parseInsightDate accepts a YYYY-MM-DD string, returning nil when empty or
// malformed so a bad value degrades to all-time rather than erroring.
func parseInsightDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// List godoc
// @Summary      List recent AI insights
// @Tags         insights
// @Produce      json
// @Success      200  {array}   domain.AIInsight
// @Router       /insights [get]
func (h *InsightHandler) List(c *gin.Context) {
	insights, err := h.svc.List(10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, insights)
}
