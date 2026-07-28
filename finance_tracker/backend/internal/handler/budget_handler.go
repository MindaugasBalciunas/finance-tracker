package handler

import (
	"net/http"
	"strconv"
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
		b.GET("/settings", h.GetSettings)
		b.PUT("/settings", h.SaveSettings)
	}
	// Registered here (not in the transaction handler) because the suggestion
	// query lives on the budget repository alongside the other cross-cutting
	// transaction lookups (labels, bulk apply).
	rg.GET("/transactions/suggest-category", h.SuggestCategory)
	l := rg.Group("/labels")
	{
		l.GET("", h.Labels)
		l.POST("/apply", h.ApplyLabel)
		l.POST("/reapply", h.ReapplyRules)
		l.GET("/rules", h.Rules)
		l.DELETE("/rules/:id", h.DeleteRule)
		l.GET("/preview", h.PreviewLabel)
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
		Name: input.Name,
		Kind: input.Kind,
		// NormalizeLabels handles multi-label budgets ("restaurant,fast food"):
		// lowercase, trimmed, deduplicated.
		Label:    domain.NormalizeLabels(input.Label),
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
	b.Label = domain.NormalizeLabels(input.Label)
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

// SuggestCategory proposes a category for a new transaction based on similar
// historical ones. Returns {category:"", matches:0} when nothing similar exists.
func (h *BudgetHandler) SuggestCategory(c *gin.Context) {
	comment := c.Query("comment")
	txType := c.Query("type")
	amount := 0.0
	if v := c.Query("amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			amount = f
		}
	}
	category, matches, basis, err := h.repo.SuggestCategory(txType, comment, amount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"category": category, "matches": matches, "basis": basis})
}

func (h *BudgetHandler) GetSettings(c *gin.Context) {
	s, err := h.repo.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

type budgetSettingsInput struct {
	IncomeMode        string  `json:"income_mode" binding:"required"`
	ManualIncome      float64 `json:"manual_income"`
	GrossSalary       float64 `json:"gross_salary"`
	MonthlyDeductions float64 `json:"monthly_deductions"`
}

func (h *BudgetHandler) SaveSettings(c *gin.Context) {
	var input budgetSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if !domain.IsValidIncomeMode(input.IncomeMode) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "income_mode must be median, manual or gross"})
		return
	}
	if input.IncomeMode == "manual" && input.ManualIncome <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "manual_income must be positive"})
		return
	}
	if input.IncomeMode == "gross" && input.GrossSalary <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "gross_salary must be positive"})
		return
	}
	s, err := h.repo.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	s.IncomeMode = input.IncomeMode
	s.ManualIncome = input.ManualIncome
	s.GrossSalary = input.GrossSalary
	s.MonthlyDeductions = input.MonthlyDeductions
	if err := h.repo.SaveSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *BudgetHandler) Labels(c *gin.Context) {
	labels, err := h.repo.DistinctLabels(c.Query("category"))
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

// PreviewLabel reports what a candidate rule would do — total matching
// transactions and how many still lack the label — without writing anything.
// Backs the "make this a rule" suggestion in the transaction form.
func (h *BudgetHandler) PreviewLabel(c *gin.Context) {
	label := strings.ToLower(strings.TrimSpace(c.Query("label")))
	commentMatch := strings.TrimSpace(c.Query("comment_match"))
	category := strings.TrimSpace(c.Query("category"))
	if label == "" || (commentMatch == "" && category == "") {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "label plus comment_match or category is required"})
		return
	}
	matches, unlabeled, err := h.repo.PreviewLabel(domain.LabelRule{
		Label: label, Category: category, CommentMatch: commentMatch,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"matches": matches, "unlabeled": unlabeled})
}

// ReapplyRules runs every saved label rule over the whole transaction table —
// a deterministic migration for historical records.
func (h *BudgetHandler) ReapplyRules(c *gin.Context) {
	rules, err := h.repo.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	total := 0
	for _, rule := range rules {
		n, err := h.repo.ApplyLabel(rule)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		total += n
	}
	c.JSON(http.StatusOK, gin.H{"relabeled": total, "rules": len(rules)})
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
