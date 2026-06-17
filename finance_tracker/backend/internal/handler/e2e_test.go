package handler_test

// E2E integration tests: real SQLite in-memory DB → real repositories → real services → real HTTP handlers.
//
// Each test scenario exercises the full chain:
//   POST /balances    (create manual snapshot)
//   GET  /balances/projected (verify latest snapshot is returned)
//
// Projected balance = latest snapshot. Transactions do NOT automatically adjust it.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// e2eRouter creates a fully-wired Gin router backed by an in-memory SQLite database.
// Each call returns an isolated router with its own DB — tests never share state.
func e2eRouter(t *testing.T) *gin.Engine {
	t.Helper()

	db, err := gorm.Open(gormsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}))

	balRepo := repository.NewBalanceRepository(db)
	txRepo := repository.NewTransactionRepository(db)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	txSvc := service.NewTransactionService(txRepo, balSvc)

	r := gin.New()
	api := r.Group("/api/v1")
	handler.NewBalanceHandler(balSvc).RegisterRoutes(api)
	handler.NewTransactionHandler(txSvc).RegisterRoutes(api)
	return r
}

// --- HTTP helpers ---

func e2ePostJSON(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func e2eGetJSON(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func e2ePutJSON(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func e2eDelete(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, path, nil))
	return w
}

func parseBalance(t *testing.T, body []byte) domain.Balance {
	t.Helper()
	var b domain.Balance
	require.NoError(t, json.Unmarshal(body, &b), "response body: %s", body)
	return b
}

func parseBalanceList(t *testing.T, body []byte) []domain.Balance {
	t.Helper()
	var bs []domain.Balance
	require.NoError(t, json.Unmarshal(body, &bs), "response body: %s", body)
	return bs
}

func createSnapshot(t *testing.T, r *gin.Engine, body map[string]any) domain.Balance {
	t.Helper()
	w := e2ePostJSON(t, r, "/api/v1/balances", body)
	require.Equal(t, http.StatusCreated, w.Code, "snapshot create failed: %s", w.Body.String())
	return parseBalance(t, w.Body.Bytes())
}

func projected(t *testing.T, r *gin.Engine) domain.Balance {
	t.Helper()
	w := e2eGetJSON(t, r, "/api/v1/balances/projected")
	require.Equal(t, http.StatusOK, w.Code, "projected failed: %s", w.Body.String())
	return parseBalance(t, w.Body.Bytes())
}

// --- Tests ---

// TestE2E_NoSnapshotReturnsZeroBalance ensures that when no snapshot exists,
// the projection endpoint returns a zero balance rather than an error.
func TestE2E_NoSnapshotReturnsZeroBalance(t *testing.T) {
	r := e2eRouter(t)

	w := e2eGetJSON(t, r, "/api/v1/balances/projected")
	require.Equal(t, http.StatusOK, w.Code)

	b := parseBalance(t, w.Body.Bytes())
	assert.Equal(t, 0.0, b.Total)
	assert.Equal(t, 0.0, b.Swed)
}

// TestE2E_ProjectedReturnsLatestSnapshot verifies that GetProjected returns the
// most recently created snapshot unchanged.
func TestE2E_ProjectedReturnsLatestSnapshot(t *testing.T) {
	r := e2eRouter(t)

	createSnapshot(t, r, map[string]any{
		"date": "2026-01-01",
		"seb":  5000.0,
		"swed": 3000.0,
	})

	b := projected(t, r)
	assert.InDelta(t, 5000.0, b.Seb, 0.001)
	assert.InDelta(t, 3000.0, b.Swed, 0.001)
	assert.InDelta(t, 8000.0, b.Total, 0.001)
}

// TestE2E_NewSnapshotUpdatesProjected confirms that creating a newer snapshot
// replaces the projected value.
func TestE2E_NewSnapshotUpdatesProjected(t *testing.T) {
	r := e2eRouter(t)

	createSnapshot(t, r, map[string]any{"date": "2026-01-01", "swed": 3000.0})
	createSnapshot(t, r, map[string]any{"date": "2026-02-01", "swed": 3507.14})

	b := projected(t, r)
	assert.InDelta(t, 3507.14, b.Swed, 0.001, "projected must reflect the newer snapshot")
	assert.InDelta(t, 3507.14, b.Total, 0.001)
}

// TestE2E_TransactionsDoNotAffectProjected verifies that adding transactions
// does not change the projected balance — it stays as the latest snapshot.
func TestE2E_TransactionsDoNotAffectProjected(t *testing.T) {
	r := e2eRouter(t)

	createSnapshot(t, r, map[string]any{"date": "2026-06-13T17:09", "swed": 3507.14})

	// Add an expense — should NOT change projected balance
	w := e2ePostJSON(t, r, "/api/v1/transactions", map[string]any{
		"date": "2026-06-14", "type": "expense", "amount": 27.44,
		"category": "Housing", "debit_account": "swed",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	b := projected(t, r)
	assert.InDelta(t, 3507.14, b.Swed, 0.001, "transactions must not change projected balance")
}

// TestE2E_MultipleSameDaySnapshotsAllStored confirms that two snapshots on the
// same day with different times are both persisted and listed.
func TestE2E_MultipleSameDaySnapshotsAllStored(t *testing.T) {
	r := e2eRouter(t)

	s1 := createSnapshot(t, r, map[string]any{"date": "2026-06-13T17:09", "swed": 3507.14})
	s2 := createSnapshot(t, r, map[string]any{"date": "2026-06-13T18:00", "swed": 3480.00})

	// Both must have been assigned different IDs
	assert.NotEqual(t, s1.ID, s2.ID)

	// List must return both
	w := e2eGetJSON(t, r, "/api/v1/balances")
	require.Equal(t, http.StatusOK, w.Code)
	list := parseBalanceList(t, w.Body.Bytes())
	assert.Len(t, list, 2)

	// Projected must be the later one (18:00 → 3480.00)
	b := projected(t, r)
	assert.InDelta(t, 3480.00, b.Swed, 0.001, "projected must be the later same-day snapshot")
}

// TestE2E_ProjectedIDIsZero confirms GetProjected always returns ID=0 (not a stored record).
func TestE2E_ProjectedIDIsZero(t *testing.T) {
	r := e2eRouter(t)

	createSnapshot(t, r, map[string]any{"date": "2026-01-01T10:30", "cash": 1000.0})

	b := projected(t, r)
	assert.Equal(t, uint(0), b.ID, "projected must not expose a stored row ID")
	assert.InDelta(t, 1000.0, b.Cash, 0.001)
}

// TestE2E_SnapshotWithDatetimeStoresTime verifies that the hour:minute is preserved.
func TestE2E_SnapshotWithDatetimeStoresTime(t *testing.T) {
	r := e2eRouter(t)

	s := createSnapshot(t, r, map[string]any{"date": "2026-06-13T17:09", "swed": 3507.14})
	assert.Equal(t, 17, s.Date.Hour())
	assert.Equal(t, 9, s.Date.Minute())
}

// TestE2E_UpdateSnapshotDate confirms that a snapshot's datetime can be updated.
func TestE2E_UpdateSnapshotDate(t *testing.T) {
	r := e2eRouter(t)

	s := createSnapshot(t, r, map[string]any{"date": "2026-06-13T17:09", "swed": 3507.14})

	w := e2ePutJSON(t, r, "/api/v1/balances/"+itoa(s.ID), map[string]any{
		"date": "2026-06-14T09:00", "swed": 3479.70,
	})
	require.Equal(t, http.StatusOK, w.Code)

	updated := parseBalance(t, w.Body.Bytes())
	assert.Equal(t, 9, updated.Date.Hour())
	assert.InDelta(t, 3479.70, updated.Swed, 0.001)
}

// TestE2E_DeleteSnapshot removes a snapshot and projected falls back to the previous one.
func TestE2E_DeleteSnapshot(t *testing.T) {
	r := e2eRouter(t)

	createSnapshot(t, r, map[string]any{"date": "2026-01-01", "swed": 3000.0})
	s2 := createSnapshot(t, r, map[string]any{"date": "2026-02-01", "swed": 3507.14})

	// Projected is the newer one
	b := projected(t, r)
	assert.InDelta(t, 3507.14, b.Swed, 0.001)

	// Delete the newer snapshot
	w := e2eDelete(t, r, "/api/v1/balances/"+itoa(s2.ID))
	require.Equal(t, http.StatusNoContent, w.Code)

	// Projected now falls back to the older one
	b = projected(t, r)
	assert.InDelta(t, 3000.0, b.Swed, 0.001, "after deleting latest, projected reverts to previous")
}

func itoa(n uint) string {
	return string(rune('0'+n%10)) // only works for single-digit IDs in tests
}
