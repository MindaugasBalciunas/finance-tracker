package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
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
	rg.GET("/transactions/suggest-labels", h.SuggestLabels)
	l := rg.Group("/labels")
	{
		l.GET("", h.Labels)
		l.POST("/apply", h.ApplyLabel)
		l.POST("/reapply", h.ReapplyRules)
		l.GET("/rules", h.Rules)
		l.DELETE("/rules/:id", h.DeleteRule)
		l.GET("/preview", h.PreviewLabel)
		l.GET("/stats", h.LabelStats)
		l.GET("/suggestions", h.LabelSuggestions)
		l.POST("/rename", h.RenameLabel)
		l.POST("/delete", h.DeleteLabel)
	}
}

type budgetInput struct {
	Name     string  `json:"name" binding:"required"`
	Kind     string  `json:"kind" binding:"required"`
	Label    string  `json:"label"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount" binding:"required,gt=0"`
	// Period "monthly" (default) or "yearly"; Fund carries unspent money
	// over (spending lines only), accruing from StartMonth (YYYY-MM).
	Period     string `json:"period"`
	Fund       bool   `json:"fund"`
	StartMonth string `json:"start_month"`
	// AmountFrom (update only) is the month a changed amount applies from:
	// "YYYY-MM", default the current month — earlier months keep the amount
	// they had. "all" rewrites the amount for every month (a correction).
	AmountFrom string `json:"amount_from"`
}

// applyBudgetInput validates the input and copies it onto b.
func applyBudgetInput(b *domain.Budget, input budgetInput) string {
	if !domain.IsValidBudgetKind(input.Kind) {
		return "kind must be fixed, investment, spending or trip"
	}
	label := domain.NormalizeLabels(input.Label)
	if input.Kind == "trip" {
		if label == "" {
			label = input.Name
		}
		l, err := service.NormalizeTripLabel(label)
		if err != nil {
			return err.Error()
		}
		label = l
	}
	if label == "" && input.Category == "" {
		return "either label or category is required"
	}
	period := input.Period
	if period == "" {
		period = domain.PeriodMonthly
	}
	if period != domain.PeriodMonthly && period != domain.PeriodYearly {
		return "period must be monthly or yearly"
	}
	if input.Fund && input.Kind != "spending" {
		return "only spending lines can be funds"
	}
	if input.StartMonth != "" {
		if _, err := time.Parse("2006-01", input.StartMonth); err != nil {
			return "start_month must be YYYY-MM"
		}
	}
	b.Name = input.Name
	b.Kind = input.Kind
	// NormalizeLabels handles multi-label budgets ("restaurant,fast food"):
	// lowercase, trimmed, deduplicated.
	b.Label = label
	b.Category = input.Category
	b.Period = period
	b.Fund = input.Fund
	b.StartMonth = input.StartMonth
	if b.Fund && b.StartMonth == "" {
		b.StartMonth = time.Now().Format("2006-01")
	}
	return ""
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
	b := &domain.Budget{Amount: input.Amount}
	if msg := applyBudgetInput(b, input); msg != "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
		return
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
	if msg := applyBudgetInput(b, input); msg != "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
		return
	}
	// A changed amount becomes a dated step so past months keep the limit
	// they were judged against; "all" is a correction across history.
	if input.Amount != b.Amount {
		switch from := input.AmountFrom; from {
		case "all":
			if err := h.repo.ResetAmounts(b.ID); err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
				return
			}
		default:
			if from == "" {
				from = time.Now().Format("2006-01")
			} else if _, err := time.Parse("2006-01", from); err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{Error: "amount_from must be YYYY-MM or all"})
				return
			}
			if err := h.repo.SetAmount(b, from, input.Amount); err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
				return
			}
		}
		b.Amount = input.Amount
	}
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

// SuggestLabels answers "what did I label this merchant last time". The form
// offers it rather than applying it — the same contract as the category
// suggestion next to it.
func (h *BudgetHandler) SuggestLabels(c *gin.Context) {
	labels, matches, err := h.repo.SuggestLabels(c.Query("type"), c.Query("comment"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if labels == nil {
		labels = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"labels": labels, "matches": matches})
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
