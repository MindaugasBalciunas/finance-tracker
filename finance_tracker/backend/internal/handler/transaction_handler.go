package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type TransactionHandler struct {
	svc service.TransactionService
}

func NewTransactionHandler(svc service.TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/transactions")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/summary", h.GetSummary)
	g.GET("/comments", h.GetComments)
	g.DELETE("/batch", h.DeleteBatch)
	g.DELETE("", h.DeleteAll)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)

	rg.GET("/categories", h.ListCategories)
}

func (h *TransactionHandler) ListCategories(c *gin.Context) {
	c.JSON(http.StatusOK, domain.ValidCategories)
}

func (h *TransactionHandler) GetComments(c *gin.Context) {
	comments, err := h.svc.GetDistinctComments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, comments)
}

// Create godoc
// @Summary      Create a transaction
// @Description  Add a new expense, income, or investment record
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        input  body      service.CreateTransactionInput  true  "Transaction input"
// @Success      201    {object}  domain.Transaction
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /transactions [post]
func (h *TransactionHandler) Create(c *gin.Context) {
	var input service.CreateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if !domain.IsValidCategory(input.Category) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid category"})
		return
	}
	tx, err := h.svc.Create(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, tx)
}

// GetByID godoc
// @Summary      Get a transaction by ID
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  domain.Transaction
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /transactions/{id} [get]
func (h *TransactionHandler) GetByID(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	tx, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "transaction not found"})
		return
	}
	c.JSON(http.StatusOK, tx)
}

// Update godoc
// @Summary      Update a transaction
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        id     path      int                             true  "Transaction ID"
// @Param        input  body      service.UpdateTransactionInput  true  "Update input"
// @Success      200    {object}  domain.Transaction
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /transactions/{id} [put]
func (h *TransactionHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	var input service.UpdateTransactionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if input.Category != "" && !domain.IsValidCategory(input.Category) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid category"})
		return
	}
	tx, err := h.svc.Update(id, input)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, tx)
}

// Delete godoc
// @Summary      Delete a transaction
// @Tags         transactions
// @Param        id  path  int  true  "Transaction ID"
// @Success      204
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /transactions/{id} [delete]
func (h *TransactionHandler) Delete(c *gin.Context) {
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

func (h *TransactionHandler) DeleteBatch(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ids array required"})
		return
	}
	if err := h.svc.DeleteBatch(body.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TransactionHandler) DeleteAll(c *gin.Context) {
	if err := h.svc.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// List godoc
// @Summary      List transactions
// @Description  Returns paginated, filtered transactions
// @Tags         transactions
// @Produce      json
// @Param        date_from  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        date_to    query     string  false  "End date (YYYY-MM-DD)"
// @Param        type       query     string  false  "Type: expense | income | investment"
// @Param        category   query     string  false  "Category"
// @Param        page       query     int     false  "Page number (default 1)"
// @Param        page_size  query     int     false  "Page size (default 20)"
// @Success      200        {object}  domain.PaginatedTransactions
// @Failure      500        {object}  ErrorResponse
// @Router       /transactions [get]
func (h *TransactionHandler) List(c *gin.Context) {
	filter := buildTransactionFilter(c)
	result, err := h.svc.List(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSummary godoc
// @Summary      Get transaction summary
// @Description  Returns aggregated totals by type, category, and month
// @Tags         transactions
// @Produce      json
// @Param        date_from  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        date_to    query     string  false  "End date (YYYY-MM-DD)"
// @Success      200        {object}  domain.TransactionSummary
// @Failure      500        {object}  ErrorResponse
// @Router       /transactions/summary [get]
func (h *TransactionHandler) GetSummary(c *gin.Context) {
	filter := buildTransactionFilter(c)
	summary, err := h.svc.GetSummary(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func buildTransactionFilter(c *gin.Context) domain.TransactionFilter {
	filter := domain.TransactionFilter{
		Page:     1,
		PageSize: 20,
	}
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
	if v := c.Query("type"); v != "" {
		t := domain.TransactionType(v)
		filter.Type = &t
	}
	if v := c.Query("category"); v != "" {
		cat := domain.Category(v)
		filter.Category = &cat
	}
	if v := c.Query("label"); v != "" {
		filter.Label = strings.ToLower(strings.TrimSpace(v))
	}
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			filter.Page = p
		}
	}
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			filter.PageSize = ps
		}
	}
	return filter
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id), err
}
