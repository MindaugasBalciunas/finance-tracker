package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
)

// ErrorResponse is the standard error envelope
type ErrorResponse struct {
	Error string `json:"error"`
}

type BalanceHandler struct {
	svc service.BalanceService
}

func NewBalanceHandler(svc service.BalanceService) *BalanceHandler {
	return &BalanceHandler{svc: svc}
}

func (h *BalanceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/balances")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/latest", h.GetLatest)
	g.GET("/projected", h.GetProjected)
	g.GET("/trend", h.GetTrend)
	g.GET("/allocation", h.GetAllocation)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("", h.DeleteAll)
	g.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary      Create a balance snapshot
// @Description  Record a new point-in-time balance across all accounts
// @Tags         balances
// @Accept       json
// @Produce      json
// @Param        input  body      service.CreateBalanceInput  true  "Balance input"
// @Success      201    {object}  domain.Balance
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /balances [post]
func (h *BalanceHandler) Create(c *gin.Context) {
	var input service.CreateBalanceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	b, err := h.svc.Create(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, b)
}

// GetByID godoc
// @Summary      Get balance by ID
// @Tags         balances
// @Produce      json
// @Param        id   path      int  true  "Balance ID"
// @Success      200  {object}  domain.Balance
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /balances/{id} [get]
func (h *BalanceHandler) GetByID(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	b, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "balance not found"})
		return
	}
	c.JSON(http.StatusOK, b)
}

// Update godoc
// @Summary      Update a balance snapshot
// @Tags         balances
// @Accept       json
// @Produce      json
// @Param        id     path      int                         true  "Balance ID"
// @Param        input  body      service.UpdateBalanceInput  true  "Update input"
// @Success      200    {object}  domain.Balance
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /balances/{id} [put]
func (h *BalanceHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	var input service.UpdateBalanceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	b, err := h.svc.Update(id, input)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, b)
}

// Delete godoc
// @Summary      Delete a balance snapshot
// @Tags         balances
// @Param        id  path  int  true  "Balance ID"
// @Success      204
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /balances/{id} [delete]
func (h *BalanceHandler) DeleteAll(c *gin.Context) {
	if err := h.svc.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *BalanceHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// List godoc
// @Summary      List balance snapshots
// @Tags         balances
// @Produce      json
// @Param        date_from  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        date_to    query     string  false  "End date (YYYY-MM-DD)"
// @Success      200        {array}   domain.Balance
// @Failure      500        {object}  ErrorResponse
// @Router       /balances [get]
func (h *BalanceHandler) List(c *gin.Context) {
	filter := buildBalanceFilter(c)
	balances, err := h.svc.List(filter, parseBtcPrice(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, balances)
}

// GetLatest godoc
// @Summary      Get the latest balance snapshot
// @Tags         balances
// @Produce      json
// @Success      200  {object}  domain.Balance
// @Failure      404  {object}  ErrorResponse
// @Router       /balances/latest [get]
func (h *BalanceHandler) GetLatest(c *gin.Context) {
	b, err := h.svc.GetLatest(parseBtcPrice(c))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no balance records found"})
		return
	}
	c.JSON(http.StatusOK, b)
}

func (h *BalanceHandler) GetProjected(c *gin.Context) {
	b, err := h.svc.GetProjected(parseBtcPrice(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, b)
}

func parseBtcPrice(c *gin.Context) float64 {
	if v := c.Query("btc_price"); v != "" {
		if p, err := strconv.ParseFloat(v, 64); err == nil {
			return p
		}
	}
	return 0
}

// GetTrend godoc
// @Summary      Get balance trend over time
// @Description  Returns time-series data suitable for charting
// @Tags         balances
// @Produce      json
// @Param        date_from  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        date_to    query     string  false  "End date (YYYY-MM-DD)"
// @Success      200        {object}  domain.BalanceTrend
// @Failure      500        {object}  ErrorResponse
// @Router       /balances/trend [get]
func (h *BalanceHandler) GetTrend(c *gin.Context) {
	filter := buildBalanceFilter(c)
	trend, err := h.svc.GetTrend(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, trend)
}

// GetAllocation godoc
// @Summary      Get current account allocation
// @Description  Returns percentage breakdown of the latest balance across accounts
// @Tags         balances
// @Produce      json
// @Success      200  {array}   domain.AccountAllocation
// @Failure      500  {object}  ErrorResponse
// @Router       /balances/allocation [get]
func (h *BalanceHandler) GetAllocation(c *gin.Context) {
	allocations, err := h.svc.GetAllocation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, allocations)
}

func buildBalanceFilter(c *gin.Context) domain.BalanceFilter {
	filter := domain.BalanceFilter{}
	if v := c.Query("date_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.DateFrom = &t
		}
	}
	if v := c.Query("date_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.DateTo = &t
		}
	}
	return filter
}
