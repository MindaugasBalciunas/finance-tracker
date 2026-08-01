package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Mirrors the real export: newest first, client+employer pairs on one date,
// a partial withdrawal, EUR signs in every numeric column.
const invlSample = `Data,Įmoka/išmoka,Mokestis,Suma,Vnt. kaina,Vnt.,Operacija,Mokėtojas/Gavėjas
2026-07-10,200.00 €,0.00 €,200.00 €,1.2630 €,158.3531 €,Kliento įmoka,Mindaugas Balčiūnas
2025-12-02,1107.40 €,0.00 €,1107.40 €,1.1074 €,1000.0000 €,Dalies lėšų išmoka,Mindaugas Balčiūnas
2021-01-29,148.47 €,0.00 €,148.47 €,0.7182 €,206.7251 €,Darbdavio įmoka, Vipps MobilePay AS Lietuvos filialas
2021-01-29,148.47 €,0.00 €,148.47 €,0.7182 €,206.7251 €,Kliento įmoka,Mindaugas Balčiūnas
2020-02-28,122.70 €,0.00 €,122.70 €,0.6300 €,194.7619 €,Kliento įmoka,Mindaugas Balčiūnas
2020-02-28,122.70 €,0.00 €,122.70 €,0.6300 €,194.7619 €,Darbdavio įmoka, Danske Bank A/S Lietuvos filialas
`

func TestParseINVLCSV(t *testing.T) {
	points, err := parseINVLCSV(strings.NewReader(invlSample))
	require.NoError(t, err)
	require.Len(t, points, 4, "one point per distinct date")

	// Oldest first; client + employer both add units.
	assert.Equal(t, "2020-02-28", points[0].Date.Format("2006-01-02"))
	assert.InDelta(t, 2*194.7619*0.63, points[0].Value, 0.01)

	units2021 := 2*194.7619 + 2*206.7251
	assert.Equal(t, "2021-01-29", points[1].Date.Format("2006-01-02"))
	assert.InDelta(t, units2021*0.7182, points[1].Value, 0.01)

	// The withdrawal subtracts units at that day's price.
	afterWithdrawal := units2021 - 1000
	assert.Equal(t, "2025-12-02", points[2].Date.Format("2006-01-02"))
	assert.InDelta(t, afterWithdrawal*1.1074, points[2].Value, 0.01)

	assert.Equal(t, "2026-07-10", points[3].Date.Format("2006-01-02"))
	assert.InDelta(t, (afterWithdrawal+158.3531)*1.2630, points[3].Value, 0.01)
}

func invlRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}))
	h := NewImportHandler(repository.NewTransactionRepository(db), repository.NewBalanceRepository(db),
		repository.NewStockRepository(db), repository.NewAssetRepository(db))
	r := gin.New()
	h.RegisterRoutes(r.Group("/api/v1"))
	return r, db
}

func invlUpload(t *testing.T, r *gin.Engine, csvBody string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "invl.csv")
	require.NoError(t, err)
	_, err = fw.Write([]byte(csvBody))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/invl", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestImportINVL_EnrichCreateAndGuard(t *testing.T) {
	r, db := invlRouter(t)

	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return d
	}
	// Swed-only snapshot in the same month as the 2020-02-28 point → enriched.
	require.NoError(t, db.Create(&domain.Balance{Date: day("2020-02-29"), Swed: 1000, Total: 1000}).Error)
	// Snapshot already tracking Artea in Dec 2025 → untouched.
	require.NoError(t, db.Create(&domain.Balance{Date: day("2025-12-31"), Swed: 500, Art: 4000, Total: 4500}).Error)
	// Fully-tracked era starts 2024-01-31 (holds more than swed+art).
	require.NoError(t, db.Create(&domain.Balance{Date: day("2024-01-31"), Swed: 500, Seb: 2000, Art: 900, Total: 3400}).Error)

	rec := invlUpload(t, r, invlSample)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, `"points":4`)
	assert.Contains(t, body, `"enriched":1`, body)
	// 2021-01 has no snapshot and predates the full era → created;
	// 2026-07 has no snapshot but is inside the full era → skipped.
	assert.Contains(t, body, `"created":1`, body)
	assert.Contains(t, body, `"skipped":2`, body)

	var enriched domain.Balance
	require.NoError(t, db.First(&enriched, "swed = 1000").Error)
	assert.InDelta(t, 245.40, enriched.Art, 0.01, "2020-02 valuation lands on the swed snapshot")
	assert.InDelta(t, 1245.40, enriched.Total, 0.01, "total bumped by the delta")

	var untouched domain.Balance
	require.NoError(t, db.First(&untouched, "art = 4000").Error)
	assert.InDelta(t, 4500, untouched.Total, 0.001)

	var created domain.Balance
	require.NoError(t, db.First(&created, "swed = 0 AND art > 0").Error)
	assert.Equal(t, "2021-01-29", created.Date.Format("2006-01-02"))
	assert.InDelta(t, created.Art, created.Total, 0.001)

	// Idempotent: every month now tracks Artea → nothing changes.
	rec = invlUpload(t, r, invlSample)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), `"enriched":0`)
	assert.Contains(t, rec.Body.String(), `"created":0`)
	var count int64
	db.Model(&domain.Balance{}).Count(&count)
	assert.EqualValues(t, 4, count)
}
