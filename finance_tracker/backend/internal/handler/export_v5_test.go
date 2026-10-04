package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func v5Export(t *testing.T, db *gorm.DB, path string) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	exportRouterFor(t, db).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	return rec.Body.Bytes()
}

func v5Import(t *testing.T, r http.Handler, body []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "finances.json")
	require.NoError(t, err)
	_, _ = fw.Write(body)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())
}

// v5: the bank-row id and the AI provider/master switch survive a
// wipe-and-restore. Losing external_id would let the bank queue re-offer
// rows already in the ledger.
func TestBackupRoundtrip_V5Fields(t *testing.T) {
	_, src := importRouterFor(t)
	require.NoError(t, src.AutoMigrate(&domain.ExportLog{}))
	require.NoError(t, src.Create(&domain.Transaction{Date: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Type: "expense", Amount: 65.2, Category: "Food", Comment: "Maxima", ExternalID: "eb:1:abc"}).Error)
	require.NoError(t, src.Create(&domain.AISettings{ID: 1, Model: "m", APIKey: "k",
		Provider: domain.ProviderAnthropic, Disabled: true}).Error)

	body := v5Export(t, src, "/api/v1/export/finances.json")
	var payload financeExport
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, exportSchemaVersion, payload.SchemaVersion)

	dstRouter, dst := importRouterFor(t)
	v5Import(t, dstRouter, body)

	var tx domain.Transaction
	require.NoError(t, dst.First(&tx).Error)
	assert.Equal(t, "eb:1:abc", tx.ExternalID)
	var s domain.AISettings
	require.NoError(t, dst.First(&s, 1).Error)
	assert.Equal(t, domain.ProviderAnthropic, s.Provider)
	assert.True(t, s.Disabled)

	// The same bank row already committed locally under another ID is
	// skipped, not a unique-index failure that aborts the restore.
	payload.Transactions[0].ID = 999
	again, _ := json.Marshal(payload)
	v5Import(t, dstRouter, again)
	var n int64
	dst.Model(&domain.Transaction{}).Count(&n)
	assert.EqualValues(t, 1, n)
}

// A pre-v5 backup (no enabled field) must not switch AI off.
func TestImport_PreV5AISettingsKeepsSwitch(t *testing.T) {
	r, db := importRouterFor(t)
	v5Import(t, r, []byte(`{"schema_version":4,"transactions":[],"balances":[],"stock_trades":[],"assets":[],"ai_settings":{"model":"m"}}`))
	var s domain.AISettings
	require.NoError(t, db.First(&s, 1).Error)
	assert.False(t, s.Disabled)
}

// A snapshot entered after the last full export but dated before it must
// still land in the next partial export (created_at, not date).
func TestPartialExport_IncludesBackdatedBalance(t *testing.T) {
	_, db := importRouterFor(t)
	require.NoError(t, db.AutoMigrate(&domain.ExportLog{}))
	require.NoError(t, db.Create(&domain.Balance{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Swed: 1, Total: 1, CreatedAt: time.Now().Add(-time.Hour)}).Error)
	v5Export(t, db, "/api/v1/export/finances.json")
	require.NoError(t, db.Create(&domain.Balance{Date: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Swed: 2, Total: 2, CreatedAt: time.Now().Add(time.Second)}).Error)

	var payload financeExport
	require.NoError(t, json.Unmarshal(v5Export(t, db, "/api/v1/export/finances-partial.json"), &payload))
	require.Len(t, payload.Balances, 1)
	assert.Equal(t, "2026-08-01", payload.Balances[0].Date)
}
