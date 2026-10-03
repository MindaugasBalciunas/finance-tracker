package handler

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
)

// chatBudget bounds a single chat request end-to-end (all tool rounds). It
// sits below nginx's proxy_read_timeout (300s) and the axios client timeout
// so the backend, not the proxy, owns the failure message.
const chatBudget = 240 * time.Second

type InsightHandler struct {
	svc service.InsightService
}

func NewInsightHandler(svc service.InsightService) *InsightHandler {
	return &InsightHandler{svc: svc}
}

func (h *InsightHandler) RegisterRoutes(rg *gin.RouterGroup) {
	// /ai/settings and /ai/models stay reachable while AI is switched off —
	// they are how the user switches it back on. Everything else that reads
	// or produces AI output sits behind requireEnabled.
	settings := rg.Group("/ai")
	settings.GET("/settings", h.GetAISettings)
	settings.PUT("/settings", h.SaveAISettings)
	settings.GET("/models", h.Models)

	g := rg.Group("/insights", h.requireEnabled)
	g.GET("/latest", h.GetLatest)
	g.POST("/generate", h.Generate)
	g.GET("", h.List)

	ai := rg.Group("/ai", h.requireEnabled)
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
	ai.POST("/scan-transaction", h.ScanTransaction)
	ai.POST("/label-reindex", h.LabelReindex)
	ai.POST("/label-reindex/apply", h.ApplyLabelSuggestions)
	// AI audit of the auto-labeling rule set, and user-approved apply.
	ai.POST("/rule-review", h.RuleReview)
	ai.POST("/rule-review/apply", h.ApplyRuleSuggestions)

	// The user's CFO-context document: GET is token-reachable (MCP exposes
	// it), PUT is session-only.
	// What the AI has cost, and what is left of the top-ups the user
	// recorded. The provider publishes no balance, so this ledger is the
	// only answer the app can give.
	ai.GET("/spend", h.SpendReport)
	ai.POST("/spend/topups", h.AddTopUp)
	ai.DELETE("/spend/topups/:id", h.DeleteTopUp)
	ai.GET("/context", h.GetAIContext)
	ai.PUT("/context", h.SaveAIContext)

	// Saved AI investment forecast: GET is free (returns the persisted
	// document), POST regenerates via the gateway — session-only.
	ai.GET("/forecast", h.GetForecast)
	ai.POST("/forecast", h.GenerateForecast)

	// Structured month-to-date budget progress. Lives under /budgets so the
	// read-only API token (and thus MCP) can reach it. Not an AI call — it is
	// plain arithmetic over the user's own budgets — so the switch doesn't
	// gate it.
	rg.GET("/budgets/status", h.BudgetStatus)
	rg.GET("/budgets/trips", h.Trips)
	rg.POST("/budgets/trips/assign", h.AssignTrip)
	rg.POST("/budgets/trips/rename", h.RenameTrip)
}

type renameTripInput struct {
	From string `json:"from" binding:"required"` // trip:old-name
	To   string `json:"to" binding:"required"`   // new name (trip: prefix optional)
}

// RenameTrip renames a trip everywhere; onto an existing trip it merges.
func (h *InsightHandler) RenameTrip(c *gin.Context) {
	var in renameTripInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	label, res, err := h.svc.RenameTrip(in.From, in.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"label": label, "result": res})
}

// Trips lists every tagged trip with its cost breakdown, plus untagged
// Vacation runs proposed as trips.
func (h *InsightHandler) Trips(c *gin.Context) {
	trips, suggestions, err := h.svc.Trips()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suggestions == nil {
		suggestions = []service.TripSuggestion{}
	}
	c.JSON(http.StatusOK, gin.H{"trips": trips, "suggestions": suggestions})
}

type assignTripInput struct {
	Name   string `json:"name" binding:"required"`
	TxIDs  []uint `json:"tx_ids" binding:"required,min=1"`
	Remove bool   `json:"remove"`
}

// AssignTrip tags exactly the given transactions with trip:<name> (or
// removes it with remove=true).
func (h *InsightHandler) AssignTrip(c *gin.Context) {
	var in assignTripInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	label, n, err := h.svc.AssignTrip(in.Name, in.TxIDs, in.Remove)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"label": label, "changed": n})
}

// requireEnabled blocks every AI surface while the master switch is off, so
// a disabled install can't be driven through the API (or MCP) behind the
// hidden UI. 404 rather than 403: with AI off the feature does not exist.
func (h *InsightHandler) requireEnabled(c *gin.Context) {
	s, err := h.svc.AISettings()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !s.Enabled() {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "AI features are turned off — enable them in AI settings"})
		return
	}
	c.Next()
}

func (h *InsightHandler) GetAIContext(c *gin.Context) {
	ctx, err := h.svc.AIContext()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ctx)
}

func (h *InsightHandler) SaveAIContext(c *gin.Context) {
	var input struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, err := h.svc.SaveAIContext(input.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ctx)
}

// BudgetStatus returns per-budget month-to-date progress; ?month=YYYY-MM
// selects a month (default: current).
func (h *InsightHandler) BudgetStatus(c *gin.Context) {
	year, month := 0, 0
	if v := c.Query("month"); v != "" {
		t, err := time.Parse("2006-01", v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "month must be YYYY-MM"})
			return
		}
		year, month = t.Year(), int(t.Month())
	}
	out, err := h.svc.BudgetStatus(year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *InsightHandler) AssistTransaction(c *gin.Context) {
	var input service.TransactionAssistInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	streamJSONResult(c, func() (any, error) { return h.svc.AssistTransaction(input) })
}

// maxScanImageBytes caps the uploaded image; a phone photo is a few MB. The
// global import-body limit is larger, but the scan route has its own tighter
// ceiling since it isn't a bulk import.
const maxScanImageBytes = 10 << 20 // 10 MiB

// ScanTransaction accepts an uploaded image (multipart 'file') and returns
// the transaction fields the vision model extracted, for the form to prefill.
func (h *InsightHandler) ScanTransaction(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attach an image in the 'file' field"})
		return
	}
	if fileHeader.Size > maxScanImageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image too large — keep it under 10 MB"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the uploaded image"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxScanImageBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the uploaded image"})
		return
	}
	mediaType := http.DetectContentType(data)

	// Vision calls are slower than text; give it a bounded budget below nginx,
	// and stream heartbeats so a proxy never 504s it.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	streamJSONResult(c, func() (any, error) { return h.svc.ScanTransaction(ctx, data, mediaType) })
}

func (h *InsightHandler) LabelReindex(c *gin.Context) {
	var input struct {
		Mode   string `json:"mode"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	_ = c.ShouldBindJSON(&input)
	if input.Mode != "" && input.Mode != "unlabeled" && input.Mode != "review" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown mode"})
		return
	}
	streamJSONResult(c, func() (any, error) { return h.svc.ReindexSuggest(input.Mode, input.Limit, input.Offset) })
}

// RuleReview asks the AI to audit the auto-labeling rule set and returns
// validated suggestions with live match counts. Suggestion-only.
func (h *InsightHandler) RuleReview(c *gin.Context) {
	streamJSONResult(c, func() (any, error) { return h.svc.RuleReview() })
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
	streamJSONResult(c, func() (any, error) { return h.svc.ApplyRuleSuggestions(input.Items) })
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
	streamJSONResult(c, func() (any, error) {
		applied, err := h.svc.ApplyLabelSuggestions(input.Items)
		if err != nil {
			return nil, err
		}
		return gin.H{"applied": applied}, nil
	})
}

func (h *InsightHandler) ViewSummary(c *gin.Context) {
	view := c.Query("view")
	from := parseInsightDate(c.Query("date_from"))
	to := parseInsightDate(c.Query("date_to"))
	refresh := c.Query("refresh") == "1"
	streamJSONResult(c, func() (any, error) {
		r, err := h.svc.ViewSummary(view, from, to, refresh)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"summary":        r.Text,
			"cost_usd":       r.CostUSD,
			"cost_estimated": r.CostEstimated,
			"input_tokens":   r.InputTokens,
			"output_tokens":  r.OutputTokens,
		}, nil
	})
}

// forecastPayload is the wire shape shared by GET (saved) and POST (fresh).
func forecastPayload(r service.ForecastResult) gin.H {
	if !r.Exists {
		return gin.H{"exists": false}
	}
	return gin.H{
		"exists":         true,
		"forecast":       r.Doc,
		"model":          r.Model,
		"cost_usd":       r.CostUSD,
		"cost_estimated": r.CostEstimated,
		"input_tokens":   r.InputTokens,
		"output_tokens":  r.OutputTokens,
		"created_at":     r.CreatedAt.Format(time.RFC3339),
	}
}

// GetForecast returns the saved forecast without touching the gateway.
func (h *InsightHandler) GetForecast(c *gin.Context) {
	r, err := h.svc.Forecast(c.Request.Context(), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, forecastPayload(r))
}

// GenerateForecast regenerates and persists the forecast — a paid AI call,
// streamed so a reverse proxy never 504s it.
func (h *InsightHandler) GenerateForecast(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	streamJSONResult(c, func() (any, error) {
		r, err := h.svc.Forecast(ctx, true)
		if err != nil {
			return nil, err
		}
		return forecastPayload(r), nil
	})
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
	// Provider is the resolved dialect ("gateway" | "anthropic"), never the
	// raw empty value — the UI shows what will actually be used.
	Provider  string `json:"provider"`
	Enabled   bool   `json:"enabled"`
	HasKey    bool   `json:"has_key"`
	UpdatedAt string `json:"updated_at"`
}

func toAISettingsResponse(s *domain.AISettings) aiSettingsResponse {
	return aiSettingsResponse{
		GatewayURL: s.GatewayURL, Model: s.Model,
		Provider: s.ResolvedProvider(), Enabled: s.Enabled(), HasKey: s.APIKey != "",
		UpdatedAt: s.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *InsightHandler) GetAISettings(c *gin.Context) {
	s, err := h.svc.AISettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAISettingsResponse(s))
}

// aiSettingsInput mirrors service.AISettingsInput: pointers so an omitted
// field keeps its stored value (the Enabled toggle PUTs only {"enabled":…}).
type aiSettingsInput struct {
	GatewayURL *string `json:"gateway_url"`
	Model      *string `json:"model"`
	Provider   *string `json:"provider"`
	Enabled    *bool   `json:"enabled"`
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
	s, err := h.svc.SaveAISettings(service.AISettingsInput{
		GatewayURL: input.GatewayURL, Model: input.Model, Provider: input.Provider,
		Enabled: input.Enabled, APIKey: input.APIKey, ClearKey: input.ClearKey,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toAISettingsResponse(s))
}

// Models returns the model ids the configured gateway offers, for the
// settings dropdown. An empty list (with ok:false) means the gateway has no
// usable models endpoint and the UI should fall back to a free-text field.
func (h *InsightHandler) Models(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	models, err := h.svc.ListModels(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"models": []string{}, "ok": false, "note": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models, "ok": true})
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
	var message string
	var image *service.ChatImage

	// Two content types: multipart (a receipt/screenshot attached, field
	// 'file', plus a 'message' form value) or the plain JSON turn.
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		message = strings.TrimSpace(c.PostForm("message"))
		if fh, err := c.FormFile("file"); err == nil && fh != nil {
			if fh.Size > maxScanImageBytes {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image too large — keep it under 10 MB"})
				return
			}
			f, oerr := fh.Open()
			if oerr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the attached image"})
				return
			}
			defer f.Close()
			data, rerr := io.ReadAll(io.LimitReader(f, maxScanImageBytes+1))
			if rerr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the attached image"})
				return
			}
			image = &service.ChatImage{Data: data, MediaType: http.DetectContentType(data)}
		}
	} else {
		var input chatInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		message = strings.TrimSpace(input.Message)
		if message == "" && len(input.Messages) > 0 {
			if last := input.Messages[len(input.Messages)-1]; last.Role == "user" {
				message = strings.TrimSpace(last.Content)
			}
		}
	}
	if message == "" && image == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must not be blank"})
		return
	}
	// Bound the whole agentic chat (tool rounds included) to a budget, and
	// stream heartbeats so a reverse proxy never 504s the long call.
	ctx, cancel := context.WithTimeout(c.Request.Context(), chatBudget)
	defer cancel()
	streamJSONResult(c, func() (any, error) {
		res, err := h.svc.Chat(ctx, message, image)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"reply":          res.Reply,
			"cost_usd":       res.CostUSD,
			"cost_estimated": res.CostEstimated,
			"input_tokens":   res.InputTokens,
			"output_tokens":  res.OutputTokens,
		}, nil
	})
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

	// Streamed: a slow gateway analysis must not 504 behind a proxy. Errors
	// arrive in the body's "error" field (200 committed up front).
	streamJSONResult(c, func() (any, error) { return h.svc.Generate(dateFrom, dateTo) })
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

// SpendReport answers "what has the AI cost me, and how much is left".
func (h *InsightHandler) SpendReport(c *gin.Context) {
	out, err := h.svc.SpendReport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// AddTopUp records money the user added at the provider. Nothing in any
// provider API reports this, so it is typed in — which is also why the
// remaining figure is only ever as current as the last entry.
func (h *InsightHandler) AddTopUp(c *gin.Context) {
	var in struct {
		AmountUSD  float64 `json:"amount_usd"`
		Note       string  `json:"note"`
		OccurredOn string  `json:"occurred_on"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	var on time.Time
	if s := strings.TrimSpace(in.OccurredOn); s != "" {
		parsed, err := time.Parse("2006-01-02", s)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "date must be YYYY-MM-DD"})
			return
		}
		on = parsed
	}
	t, err := h.svc.AddTopUp(in.AmountUSD, strings.TrimSpace(in.Note), on)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *InsightHandler) DeleteTopUp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.svc.DeleteTopUp(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
