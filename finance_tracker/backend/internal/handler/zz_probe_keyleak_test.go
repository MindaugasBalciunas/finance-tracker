package handler

// TEMP PROBE — verifies whether the plaintext AI API key ships in
// AI-analysis-oriented exports (partial + purpose=export), not only the
// full backup. Delete after verification.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProbe_APIKeyInAIExports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.AIInsight{},
		&domain.AISettings{}, &domain.AIChatMessage{}, &domain.Budget{}, &domain.LabelRule{},
		&domain.BudgetSettings{}, &domain.StockTrade{}, &domain.Asset{}, &domain.ExportLog{}))

	// Wire exactly like cmd/api/main.go
	txRepo := repository.NewTransactionRepository(db)
	balRepo := repository.NewBalanceRepository(db)
	stockRepo := repository.NewStockRepository(db)
	assetRepo := repository.NewAssetRepository(db)
	budgetRepo := repository.NewBudgetRepository(db)
	insightRepo := repository.NewInsightRepository(db)
	exportLogRepo := repository.NewExportLogRepository(db)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	txSvc := service.NewTransactionServiceWithRules(txRepo, balSvc, budgetRepo)
	stockSvc := service.NewStockService(stockRepo)
	assetSvc := service.NewAssetService(assetRepo)

	r := gin.New()
	NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, exportLogRepo).
		WithBudgets(budgetRepo).WithAI(insightRepo).RegisterRoutes(r.Group("/api/v1"))

	// User has configured their AI gateway key.
	const secret = "nxs-PROBE-SECRET-KEY-12345"
	require.NoError(t, insightRepo.SaveAISettings(&domain.AISettings{ID: 1, GatewayURL: "https://gateway.nexos.ai/v1", Model: "gpt-4o", APIKey: secret}))

	// A full backup was made earlier (enables the partial export).
	require.NoError(t, exportLogRepo.Save("full"))
	require.NoError(t, db.Model(&domain.ExportLog{}).Where("1=1").Update("created_at", time.Now().Add(-24*time.Hour)).Error)

	get := func(path string) (int, string, string) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code, w.Header().Get("Content-Disposition"), w.Body.String()
	}

	type result struct {
		name, path string
	}
	for _, tc := range []result{
		{"partial (UI: 'Export for AI' → 'New since last backup')", "/api/v1/export/finances-partial.json"},
		{"purpose=export ('handing data to an AI')", "/api/v1/export/finances.json?purpose=export"},
		{"date-scoped extract", "/api/v1/export/finances.json?from=2026-01-01&to=2026-12-31"},
		{"default full backup", "/api/v1/export/finances.json"},
	} {
		code, disp, body := get(tc.path)
		require.Equal(t, http.StatusOK, code, tc.name)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &parsed))
		leaked := strings.Contains(body, secret)
		ai, _ := parsed["ai_settings"].(map[string]any)
		t.Logf("%-55s file=%s key_in_body=%v ai_settings=%v suggested_prompt=%.60q",
			tc.name, disp, leaked, ai, parsed["suggested_prompt"])
	}
}
