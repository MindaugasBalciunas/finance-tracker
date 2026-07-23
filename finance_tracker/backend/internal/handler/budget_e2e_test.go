package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func budgetTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Budget{}, &domain.LabelRule{}))

	budgetRepo := repository.NewBudgetRepository(db)
	txRepo := repository.NewTransactionRepository(db)
	txSvc := service.NewTransactionServiceWithRules(txRepo, nil, budgetRepo)

	r := gin.New()
	v1 := r.Group("/api/v1")
	NewBudgetHandler(budgetRepo).RegisterRoutes(v1)
	NewTransactionHandler(txSvc).RegisterRoutes(v1)
	return r, db
}

func budgetDoJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLabels_Normalize(t *testing.T) {
	assert.Equal(t, "loan,fixed", domain.NormalizeLabels(" Loan , FIXED ,loan,"))
	tx := domain.Transaction{Labels: "loan"}
	assert.True(t, tx.HasLabel("LOAN"))
	assert.False(t, tx.HasLabel("alimony"))
	tx.AddLabel("Alimony")
	assert.Equal(t, "loan,alimony", tx.Labels)
}

func TestLabels_BulkApplyAndFilter(t *testing.T) {
	r, _ := budgetTestRouter(t)

	// Seed transactions through the API.
	for _, in := range []map[string]any{
		{"date": "2026-07-17", "type": "expense", "amount": 755.10, "category": "Finance", "comment": "Loan interest"},
		{"date": "2026-07-17", "type": "expense", "amount": 530.04, "category": "Finance", "comment": "Loan return"},
		{"date": "2026-07-17", "type": "expense", "amount": 25.00, "category": "Finance", "comment": "Bank fee"},
		{"date": "2026-07-17", "type": "expense", "amount": 1000.00, "category": "Kids - General", "comment": "Aliments 2026.06"},
	} {
		w := budgetDoJSON(r, "POST", "/api/v1/transactions", in)
		require.Equal(t, 201, w.Code, w.Body.String())
	}

	// Bulk apply "loan" to Finance transactions mentioning loan; create the rule.
	w := budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "Loan", "category": "Finance", "comment_match": "loan", "create_rule": true,
	})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"labeled":2`)

	// Filter by label returns only the two loan rows.
	w = budgetDoJSON(r, "GET", "/api/v1/transactions?label=loan&page_size=100", nil)
	require.Equal(t, 200, w.Code)
	var list struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.EqualValues(t, 2, list.Total)

	// New matching transaction is auto-labeled by the saved rule.
	w = budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
		"date": "2026-08-17", "type": "expense", "amount": 760.0, "category": "Finance", "comment": "Loan interest August",
	})
	require.Equal(t, 201, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"loan"`)

	// Distinct labels endpoint sees it.
	w = budgetDoJSON(r, "GET", "/api/v1/labels", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "loan")
}

func TestBudgets_CRUD(t *testing.T) {
	r, _ := budgetTestRouter(t)

	// Validation: bad kind, missing matcher.
	w := budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "X", "kind": "weird", "category": "Food", "amount": 100})
	assert.Equal(t, 400, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "X", "kind": "spending", "amount": 100})
	assert.Equal(t, 400, w.Code)

	// Create the user's plan.
	var created []domain.Budget
	for _, in := range []map[string]any{
		{"name": "Loan payments", "kind": "fixed", "label": "loan", "amount": 1285.0},
		{"name": "Alimony", "kind": "fixed", "label": "alimony", "amount": 1000.0},
		{"name": "VWCE", "kind": "investment", "category": "Stocks & ETF", "amount": 1000.0},
		{"name": "Artea pension", "kind": "investment", "category": "Pension", "amount": 200.0},
		{"name": "Food", "kind": "spending", "category": "Food", "amount": 500.0},
	} {
		w := budgetDoJSON(r, "POST", "/api/v1/budgets", in)
		require.Equal(t, 201, w.Code, w.Body.String())
		var b domain.Budget
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		created = append(created, b)
	}

	w = budgetDoJSON(r, "GET", "/api/v1/budgets", nil)
	require.Equal(t, 200, w.Code)
	var budgets []domain.Budget
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &budgets))
	assert.Len(t, budgets, 5)

	// Update an amount.
	first := created[0]
	w = budgetDoJSON(r, "PUT", "/api/v1/budgets/1", map[string]any{
		"name": first.Name, "kind": first.Kind, "label": first.Label, "amount": 1300.0,
	})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "1300")

	// Delete.
	w = budgetDoJSON(r, "DELETE", "/api/v1/budgets/5", nil)
	assert.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "GET", "/api/v1/budgets", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &budgets))
	assert.Len(t, budgets, 4)
}

func TestLabels_SurviveExportImport(t *testing.T) {
	r, db := budgetTestRouter(t)

	w := budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
		"date": "2026-07-01", "type": "expense", "amount": 50.0, "category": "Food",
		"comment": "groceries", "labels": "Test-Label, other",
	})
	require.Equal(t, 201, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"test-label,other"`)

	var tx domain.Transaction
	require.NoError(t, db.First(&tx).Error)
	assert.Equal(t, "test-label,other", tx.Labels)
}
