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
	Messages []domain.ChatMessage `json:"messages" binding:"required"`
}

func (h *InsightHandler) Chat(c *gin.Context) {
	var input chatInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages must not be empty"})
		return
	}
	for _, m := range input.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "roles must be user or assistant"})
			return
		}
		if strings.TrimSpace(m.Content) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "messages must not be blank"})
			return
		}
	}
	if input.Messages[len(input.Messages)-1].Role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "last message must be from the user"})
		return
	}
	reply, err := h.svc.Chat(input.Messages)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reply": reply})
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
	insight, err := h.svc.Generate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, insight)
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
