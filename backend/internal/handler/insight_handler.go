package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
