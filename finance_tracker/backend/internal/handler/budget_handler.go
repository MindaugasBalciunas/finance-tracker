package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

type BudgetHandler struct {
	repo repository.BudgetRepository
}

func NewBudgetHandler(repo repository.BudgetRepository) *BudgetHandler {
	return &BudgetHandler{repo: repo}
}

func (h *BudgetHandler) RegisterRoutes(rg *gin.RouterGroup) {
	b := rg.Group("/budgets")
	{
		b.GET("", h.List)
		b.POST("", h.Create)
		b.PUT("/:id", h.Update)
		b.DELETE("/:id", h.Delete)
	}
	l := rg.Group("/labels")
	{
		l.GET("", h.Labels)
		l.POST("/apply", h.ApplyLabel)
		l.GET("/rules", h.Rules)
		l.DELETE("/rules/:id", h.DeleteRule)
	}
}

type budgetInput struct {
	Name     string  `json:"name" binding:"required"`
	Kind     string  `json:"kind" binding:"required"`
	Label    string  `json:"label"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount" binding:"required,gt=0"`
}

func (h *BudgetHandler) List(c *gin.Context) {
	budgets, err := h.repo.ListBudgets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, budgets)
}

func (h *BudgetHandler) Create(c *gin.Context) {
	var input budgetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if !domain.IsValidBudgetKind(input.Kind) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "kind must be fixed, investment or spending"})
		return
	}
	if input.Label == "" && input.Category == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "either label or category is required"})
		return
	}
	b := &domain.Budget{
		Name:     input.Name,
		Kind:     input.Kind,
		Label:    strings.ToLower(strings.TrimSpace(input.Label)),
		Category: input.Category,
		Amount:   input.Amount,
	}
	if err := h.repo.SaveBudget(b); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, b)
}

func (h *BudgetHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	b, err := h.repo.GetBudget(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "budget not found"})
		return
	}
	var input budgetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if !domain.IsValidBudgetKind(input.Kind) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "kind must be fixed, investment or spending"})
		return
	}
	b.Name = input.Name
	b.Kind = input.Kind
	b.Label = strings.ToLower(strings.TrimSpace(input.Label))
	b.Category = input.Category
	b.Amount = input.Amount
	if err := h.repo.SaveBudget(b); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, b)
}

func (h *BudgetHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.repo.DeleteBudget(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *BudgetHandler) Labels(c *gin.Context) {
	labels, err := h.repo.DistinctLabels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if labels == nil {
		labels = []string{}
	}
	c.JSON(http.StatusOK, labels)
}

type applyLabelInput struct {
	Label        string `json:"label" binding:"required"`
	Category     string `json:"category"`
	CommentMatch string `json:"comment_match"`
	CreateRule   bool   `json:"create_rule"`
}

// ApplyLabel bulk-tags matching historic transactions and optionally saves
// the rule so future transactions are tagged automatically.
func (h *BudgetHandler) ApplyLabel(c *gin.Context) {
	var input applyLabelInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if input.Category == "" && input.CommentMatch == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "category or comment_match is required"})
		return
	}
	rule := domain.LabelRule{
		Label:        strings.ToLower(strings.TrimSpace(input.Label)),
		Category:     input.Category,
		CommentMatch: input.CommentMatch,
	}
	count, err := h.repo.ApplyLabel(rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if input.CreateRule {
		if err := h.repo.SaveRule(&rule); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"labeled": count, "rule_created": input.CreateRule})
}

func (h *BudgetHandler) Rules(c *gin.Context) {
	rules, err := h.repo.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if rules == nil {
		rules = []domain.LabelRule{}
	}
	c.JSON(http.StatusOK, rules)
}

func (h *BudgetHandler) DeleteRule(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.repo.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
