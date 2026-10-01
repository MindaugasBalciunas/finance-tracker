package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type TransactionHandler struct {
	svc service.TransactionService
	// bankRepo is optional. When set, deleting a transaction that came from a
	// bank sync puts its staged row back on the review list — that is what
	// makes "Undo" after a PSD2 import return you to where you were instead of
	// dropping the row on the floor.
	bankRepo repository.BankRepository
}

func NewTransactionHandler(svc service.TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) WithBanking(repo repository.BankRepository) *TransactionHandler {
	h.bankRepo = repo
	return h
}

// restoreStagedFor is best-effort: the transactions are already gone, and
// failing the delete response because the review list could not be updated
// would be a lie about what happened.
//
// Deliberately not wired into DeleteAll: wiping the ledger is normally the
// prelude to restoring a backup, and repopulating the review list with every
// row ever imported would bury the user.
func (h *TransactionHandler) restoreStagedFor(ids []uint) {
	if h.bankRepo == nil || len(ids) == 0 {
		return
	}
	_ = h.bankRepo.RestoreByImportedTxIDs(ids)
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
	// Old clients/backups may still send retired categories — map, don't reject.
	var legacyLabels []string
	input.Category, legacyLabels = domain.CanonicalCategory(input.Type, input.Category)
	for _, l := range legacyLabels {
		input.Labels = domain.NormalizeLabels(input.Labels + "," + l)
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
	if input.Category != "" {
		input.Category, _ = domain.CanonicalCategory(input.Type, input.Category)
		if !domain.IsValidCategory(input.Category) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid category"})
			return
		}
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
	h.restoreStagedFor([]uint{id})
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
	h.restoreStagedFor(body.IDs)
	c.Status(http.StatusNoContent)
}

func (h *TransactionHandler) DeleteAll(c *gin.Context) {
	if !confirmWipe(c, "transaction") {
		return
	}
	if err := h.svc.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// confirmWipe guards the collection-level DELETE routes: wiping a whole
// table is unrecoverable (hard deletes), so one stray request must not be
// enough — the caller has to say ?confirm=all explicitly.
func confirmWipe(c *gin.Context, what string) bool {
	if c.Query("confirm") == "all" {
		return true
	}
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error: "add ?confirm=all to delete every " + what + " — this cannot be undone"})
	return false
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
	filter, ferr := buildTransactionFilter(c)
	if ferr != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ferr.Error()})
		return
	}
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
	filter, ferr := buildTransactionFilter(c)
	if ferr != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ferr.Error()})
		return
	}
	summary, err := h.svc.GetSummary(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// maxPageSize caps list responses — an unbounded page_size streamed the
// entire table in one response.
const maxPageSize = 500

func buildTransactionFilter(c *gin.Context) (domain.TransactionFilter, error) {
	filter := domain.TransactionFilter{
		Page:     1,
		PageSize: 20,
	}
	// Malformed filters are rejected, not ignored: silently dropping a
	// mistyped date_from would return all-time totals that look plausible.
	if v := c.Query("date_from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return filter, fmt.Errorf("invalid date_from %q — use YYYY-MM-DD", v)
		}
		filter.DateFrom = &t
	}
	if v := c.Query("date_to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return filter, fmt.Errorf("invalid date_to %q — use YYYY-MM-DD", v)
		}
		// Inclusive end of day, so same-day records carrying a time-of-day
		// aren't excluded (matches the export path's behavior).
		end := t.Add(24*time.Hour - time.Nanosecond)
		filter.DateTo = &end
	}
	if v := c.Query("amount_min"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return filter, fmt.Errorf("invalid amount_min %q", v)
		}
		filter.AmountMin = &f
	}
	if v := c.Query("amount_max"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return filter, fmt.Errorf("invalid amount_max %q", v)
		}
		filter.AmountMax = &f
	}
	if filter.AmountMin != nil && filter.AmountMax != nil && *filter.AmountMin > *filter.AmountMax {
		return filter, fmt.Errorf("amount_min is greater than amount_max")
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
	// label_mode=all intersects a comma list of labels (must carry every
	// one); the default matches any of them.
	if c.Query("label_mode") == "all" {
		filter.LabelMode = "all"
	}
	if v := c.Query("search"); v != "" {
		filter.Search = strings.TrimSpace(v)
	}
	// Sortable list: date (default), amount or comment, each direction.
	if v := c.Query("sort"); v == "date" || v == "amount" || v == "comment" {
		filter.Sort = v
	}
	if v := c.Query("dir"); v == "asc" || v == "desc" {
		filter.Dir = v
	}
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			filter.Page = p
		}
	}
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			filter.PageSize = min(ps, maxPageSize)
		}
	}
	return filter, nil
}

func parseID(c *gin.Context) (uint, error) {
	// 32-bit so the uint conversion can't truncate, and 0 is rejected —
	// no row ever has id 0, but GORM treats it as "no condition".
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err == nil && id == 0 {
		return 0, fmt.Errorf("invalid id 0")
	}
	return uint(id), err
}
