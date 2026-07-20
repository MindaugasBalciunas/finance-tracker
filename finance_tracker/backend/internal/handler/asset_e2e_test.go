package handler_test

// E2E asset tests: real SQLite in-memory DB → real repository → real service → real HTTP handler.
// Covers what mock-based tests cannot: GORM persistence of nullable dates and
// the full create → list → summary → update → delete flow.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func assetE2ERouter(t *testing.T) *gin.Engine {
	t.Helper()

	db, err := gorm.Open(gormsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Asset{}))

	svc := service.NewAssetService(repository.NewAssetRepository(db))
	r := gin.New()
	handler.NewAssetHandler(svc).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func parseAsset(t *testing.T, body []byte) domain.Asset {
	t.Helper()
	var a domain.Asset
	require.NoError(t, json.Unmarshal(body, &a), "response body: %s", body)
	return a
}

func TestE2E_AssetLifecycle(t *testing.T) {
	r := assetE2ERouter(t)

	// Create the house with full mortgage details
	w := e2ePostJSON(t, r, "/api/v1/assets", map[string]any{
		"name":                "House Platiniškių 21A, Platiniškių k.",
		"type":                "real_estate",
		"purchase_date":       "2022-08-17",
		"purchase_price":      355000,
		"current_value":       410000,
		"valuation_date":      "2025-12-01",
		"loan_remaining":      263568.67,
		"loan_remaining_date": "2026-07-20",
		"loan_rate":           "6M EURIBOR + 1.3%",
		"loan_account":        "seb",
		"notes":               "Energy class B, built 2017.",
	})
	require.Equal(t, http.StatusCreated, w.Code, "create failed: %s", w.Body.String())
	house := parseAsset(t, w.Body.Bytes())
	assert.InDelta(t, 146431.33, house.Equity, 0.001, "equity computed on create response")

	// Create the car (no loan, paid off) and solar (no purchase date at all)
	w = e2ePostJSON(t, r, "/api/v1/assets", map[string]any{
		"name": "Toyota RAV4 Style Hybrid", "type": "vehicle",
		"purchase_date": "2020-11-17", "purchase_price": 34000,
		"loan_paid_off_date": "2025-11-17",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	w = e2ePostJSON(t, r, "/api/v1/assets", map[string]any{
		"name": "Solar panels 10.35 kWp (SOLAX X3 Hybrid G4)", "type": "solar",
		"purchase_price": 4551.50,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	solar := parseAsset(t, w.Body.Bytes())
	assert.Nil(t, solar.PurchaseDate, "solar has no purchase date")
	assert.Equal(t, 4551.50, solar.CurrentValue, "current value defaulted to purchase price")

	// List — all three persisted, nullable dates intact
	w = e2eGetJSON(t, r, "/api/v1/assets")
	require.Equal(t, http.StatusOK, w.Code)
	var assets []domain.Asset
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &assets))
	require.Len(t, assets, 3)

	// Summary aggregates
	w = e2eGetJSON(t, r, "/api/v1/assets/summary")
	require.Equal(t, http.StatusOK, w.Code)
	var s domain.AssetSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	assert.Equal(t, 3, s.Count)
	assert.InDelta(t, 448551.50, s.TotalValue, 0.001)
	assert.InDelta(t, 263568.67, s.TotalLoans, 0.001)
	assert.InDelta(t, 184982.83, s.NetEquity, 0.001)

	// Update the house — pay the mortgage down
	w = e2ePutJSON(t, r, fmt.Sprintf("/api/v1/assets/%d", house.ID), map[string]any{
		"name": house.Name, "type": "real_estate",
		"purchase_date": "2022-08-17", "purchase_price": 355000,
		"current_value": 410000, "valuation_date": "2025-12-01",
		"loan_remaining": 250000.00, "loan_remaining_date": "2026-12-20",
		"loan_rate": "6M EURIBOR + 1.3%", "loan_account": "seb",
	})
	require.Equal(t, http.StatusOK, w.Code, "update failed: %s", w.Body.String())
	updated := parseAsset(t, w.Body.Bytes())
	assert.InDelta(t, 160000.0, updated.Equity, 0.001, "equity reflects reduced loan")

	// Delete the car
	w = e2eDelete(t, r, "/api/v1/assets/2")
	require.Equal(t, http.StatusNoContent, w.Code)

	w = e2eGetJSON(t, r, "/api/v1/assets")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &assets))
	assert.Len(t, assets, 2)
}
