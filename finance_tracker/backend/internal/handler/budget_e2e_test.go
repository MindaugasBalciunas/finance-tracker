package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

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

func TestLabels_ReapplyMigratesHistory(t *testing.T) {
	r, db := budgetTestRouter(t)

	// Historic rows created without labels (simulates pre-label data).
	require.NoError(t, db.Create(&domain.Transaction{Date: mustDate("2026-01-17"), Type: "expense", Amount: 755, Category: "Finance", Comment: "Loan interest"}).Error)
	require.NoError(t, db.Create(&domain.Transaction{Date: mustDate("2026-02-17"), Type: "expense", Amount: 530, Category: "Finance", Comment: "Loan return"}).Error)

	// Save a rule without touching history, then reapply.
	repo := repository.NewBudgetRepository(db)
	require.NoError(t, repo.SaveRule(&domain.LabelRule{Label: "loan", Category: "Finance", CommentMatch: "loan"}))

	w := budgetDoJSON(r, "POST", "/api/v1/labels/reapply", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"relabeled":2`)

	// Second run is idempotent.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/reapply", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"relabeled":0`)
}

func TestBudgetsAndRules_ExportImportMigration(t *testing.T) {
	// Source instance: transactions + budgets + rules.
	srcRouter, srcDB := budgetTestRouter(t)
	require.NoError(t, srcDB.AutoMigrate(&domain.Balance{}, &domain.StockTrade{}, &domain.Asset{}, &domain.ExportLog{}))

	w := budgetDoJSON(srcRouter, "POST", "/api/v1/transactions", map[string]any{
		"date": "2026-07-17", "type": "expense", "amount": 755.10, "category": "Finance", "comment": "Loan interest",
	})
	require.Equal(t, 201, w.Code)
	w = budgetDoJSON(srcRouter, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "loan", "category": "Finance", "comment_match": "loan", "create_rule": true,
	})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(srcRouter, "POST", "/api/v1/budgets", map[string]any{
		"name": "Loan payments", "kind": "fixed", "label": "loan", "amount": 1285.0,
	})
	require.Equal(t, 201, w.Code)

	// Export from source.
	srcExport := exportRouterFor(t, srcDB)
	req := httptest.NewRequest("GET", "/api/v1/export/finances.json", nil)
	rec := httptest.NewRecorder()
	srcExport.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	exported := rec.Body.Bytes()
	assert.Contains(t, string(exported), `"budgets"`)
	assert.Contains(t, string(exported), `"label_rules"`)

	// Fresh destination instance; strip labels from the payload to simulate an
	// old export — the import must relabel from the rules.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(exported, &payload))
	txs := payload["transactions"].([]any)
	for _, raw := range txs {
		delete(raw.(map[string]any), "labels")
	}
	stripped, _ := json.Marshal(payload)

	dstRouter, dstDB := importRouterFor(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "finances.json")
	_, _ = fw.Write(stripped)
	mw.Close()
	req = httptest.NewRequest("POST", "/api/v1/import/json", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	dstRouter.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"budgets":1`)
	assert.Contains(t, rec.Body.String(), `"label_rules":1`)
	assert.Contains(t, rec.Body.String(), `"relabeled":1`)

	// The historical record arrived labeled.
	var tx domain.Transaction
	require.NoError(t, dstDB.First(&tx).Error)
	assert.Equal(t, "loan", tx.Labels)

	// Re-import is a no-op (idempotent).
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	fw2, _ := mw2.CreateFormFile("file", "finances.json")
	_, _ = fw2.Write(stripped)
	mw2.Close()
	req = httptest.NewRequest("POST", "/api/v1/import/json", &buf2)
	req.Header.Set("Content-Type", mw2.FormDataContentType())
	rec = httptest.NewRecorder()
	dstRouter.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), `"budgets":0`)
	assert.Contains(t, rec.Body.String(), `"label_rules":0`)
	assert.Contains(t, rec.Body.String(), `"relabeled":0`)
}

func mustDate(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

// exportRouterFor builds a router exposing exports backed by the given DB.
func exportRouterFor(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	txRepo := repository.NewTransactionRepository(db)
	balRepo := repository.NewBalanceRepository(db)
	stockRepo := repository.NewStockRepository(db)
	assetRepo := repository.NewAssetRepository(db)
	logRepo := repository.NewExportLogRepository(db)
	budgetRepo := repository.NewBudgetRepository(db)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	txSvc := service.NewTransactionService(txRepo, nil)
	stockSvc := service.NewStockService(stockRepo)
	assetSvc := service.NewAssetService(assetRepo)
	r := gin.New()
	NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).WithBudgets(budgetRepo).RegisterRoutes(r.Group("/api/v1"))
	return r
}

// importRouterFor builds a fresh instance with import + budget support.
func importRouterFor(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.StockTrade{}, &domain.Asset{}, &domain.Budget{}, &domain.LabelRule{}))
	r := gin.New()
	NewImportHandler(
		repository.NewTransactionRepository(db),
		repository.NewBalanceRepository(db),
		repository.NewStockRepository(db),
		repository.NewAssetRepository(db),
	).WithBudgets(repository.NewBudgetRepository(db)).RegisterRoutes(r.Group("/api/v1"))
	return r, db
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
