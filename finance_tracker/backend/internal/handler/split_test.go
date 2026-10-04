package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func splitEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}))
	r := gin.New()
	v1 := r.Group("/api/v1")
	// Registered alongside the transaction routes, as in main.go: the static
	// /owed and the /:id/... routes must coexist with /:id.
	NewTransactionHandler(service.NewTransactionService(repository.NewTransactionRepository(db), nil)).RegisterRoutes(v1)
	NewSplitHandler(db).RegisterRoutes(v1)
	return r, db
}

func seedTx(t *testing.T, db *gorm.DB, tx domain.Transaction) domain.Transaction {
	t.Helper()
	require.NoError(t, db.Create(&tx).Error)
	return tx
}

func TestSplitIntoCategoriesKeepsOriginalAsPartOne(t *testing.T) {
	r, db := splitEnv(t)
	orig := seedTx(t, db, domain.Transaction{Date: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), Type: "expense",
		Amount: 100, Category: "Food", Comment: "MAXIMA", ExternalID: "eb:1:x", DebitAccount: "swed"})

	rec := bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/split", orig.ID), map[string]any{
		"parts": []map[string]any{
			{"amount": 60, "category": "Food"},
			{"amount": 30, "category": "Kids", "labels": "kids"},
			{"amount": 10, "owed_by": "Tomas"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var all []domain.Transaction
	require.NoError(t, db.Order("id").Find(&all).Error)
	require.Len(t, all, 3)
	assert.Equal(t, orig.ID, all[0].ID)
	assert.Equal(t, 60.0, all[0].Amount)
	assert.Equal(t, "eb:1:x", all[0].ExternalID)
	assert.Equal(t, uint(0), all[0].SplitOf)
	assert.Equal(t, domain.Category("Kids"), all[1].Category)
	assert.Equal(t, orig.ID, all[1].SplitOf)
	assert.Equal(t, "", all[1].ExternalID)
	assert.Equal(t, domain.CategoryTransfers, all[2].Category)
	assert.Equal(t, "owed,owed-tomas", all[2].Labels)
	assert.Equal(t, "swed", all[2].DebitAccount)

	var n int64
	db.Model(&domain.Balance{}).Count(&n)
	assert.Zero(t, n, "a split never moves balances")

	// The statement row comes back as one 100.00 line: still a duplicate.
	idx := newDedupIndex(all)
	_, dup := idx.has(orig.Date, "expense", 100, "MAXIMA")
	assert.True(t, dup)
	_, dup = idx.has(orig.Date, "expense", 60, "MAXIMA")
	assert.False(t, dup, "part one alone is not the bank row")
}

func TestSplitRejectsBadParts(t *testing.T) {
	r, db := splitEnv(t)
	orig := seedTx(t, db, domain.Transaction{Date: time.Now(), Type: "expense", Amount: 50, Category: "Food"})
	url := fmt.Sprintf("/api/v1/transactions/%d/split", orig.ID)
	for name, parts := range map[string][]map[string]any{
		"wrong sum":     {{"amount": 20}, {"amount": 20}},
		"single part":   {{"amount": 50}},
		"zero part":     {{"amount": 50}, {"amount": 0}},
		"bad category":  {{"amount": 25, "category": "Nope"}, {"amount": 25}},
		"owed, no name": {{"amount": 25}, {"amount": 25, "owed_by": " ! "}},
	} {
		rec := bankJSON(t, r, http.MethodPost, url, map[string]any{"parts": parts})
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
	}
	var got domain.Transaction
	require.NoError(t, db.First(&got, orig.ID).Error)
	assert.Equal(t, 50.0, got.Amount, "a rejected split changes nothing")
}

func TestUnsplitRestoresOriginal(t *testing.T) {
	r, db := splitEnv(t)
	orig := seedTx(t, db, domain.Transaction{Date: time.Now(), Type: "expense", Amount: 80.10, Category: "Food"})
	rec := bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/split", orig.ID), map[string]any{
		"parts": []map[string]any{{"amount": 40.05}, {"amount": 40.05, "category": "Health"}},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var part domain.Transaction
	require.NoError(t, db.Where("split_of = ?", orig.ID).First(&part).Error)

	// Undo from the part, not the original — either should work.
	rec = bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/unsplit", part.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var all []domain.Transaction
	require.NoError(t, db.Find(&all).Error)
	require.Len(t, all, 1)
	assert.Equal(t, 80.10, all[0].Amount)
}

func TestOwedBalanceAndRepayment(t *testing.T) {
	r, db := splitEnv(t)
	dinner := seedTx(t, db, domain.Transaction{Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Type: "expense", Amount: 90, Category: "Food", Comment: "Dinner"})
	rec := bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/split", dinner.ID), map[string]any{
		"parts": []map[string]any{{"amount": 30}, {"amount": 60, "owed_by": "Tomas"}},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	pay := seedTx(t, db, domain.Transaction{Date: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), Type: "income", Amount: 40, Category: "Reimbursement", Comment: "Tomas"})

	rec = bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/repayment", pay.ID), map[string]any{"person": "Tomas"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got domain.Transaction
	require.NoError(t, db.First(&got, pay.ID).Error)
	assert.Equal(t, domain.CategoryTransfers, got.Category, "a repayment is not income")

	rec = bankJSON(t, r, http.MethodGet, "/api/v1/transactions/owed", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var owed []owedPerson
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &owed))
	require.Len(t, owed, 1)
	assert.Equal(t, "Tomas", owed[0].Name)
	assert.Equal(t, 60.0, owed[0].Lent)
	assert.Equal(t, 40.0, owed[0].Repaid)
	assert.Equal(t, 20.0, owed[0].Outstanding)
	assert.Len(t, owed[0].Rows, 2)

	// An expense cannot be a repayment.
	rec = bankJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/repayment", dinner.ID), map[string]any{"person": "Tomas"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Deleting the original of a split must not hide its parts from dedup.
func TestDedupOrphanPartStandsAlone(t *testing.T) {
	d := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	idx := newDedupIndex([]domain.Transaction{{ID: 5, Date: d, Type: "expense", Amount: 12, Comment: "x", SplitOf: 99}})
	_, dup := idx.has(d, "expense", 12, "x")
	assert.True(t, dup)
}

// A restore puts labels back exactly as backed up. Rules only fill rows that
// arrived with no labels at all (backups from before labels existed), and
// rows already in the database are never touched.
func TestRestoreKeepsBackedUpLabels(t *testing.T) {
	r, db := importRouterFor(t)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "bar", CommentMatch: "restoranas"}).Error)
	here := seedTx(t, db, domain.Transaction{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: "expense",
		Amount: 5, Category: "Food", Comment: "Kitas restoranas", Labels: "family"})

	payload := financeExport{SchemaVersion: exportSchemaVersion, Transactions: []txExportRow{
		{ID: 900, Date: "2026-09-26", Type: "expense", Amount: 50, Category: "Transfers", Comment: "Alaus restoranas", Labels: "owed,owed-tomas", SplitOf: 901},
		{ID: 901, Date: "2026-09-26", Type: "expense", Amount: 52.3, Category: "Food", Comment: "Alaus restoranas", Labels: "restaurant"},
		{ID: 902, Date: "2020-01-01", Type: "expense", Amount: 9, Category: "Food", Comment: "Senas restoranas"},
	}}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	v5Import(t, r, body)

	get := func(id uint) domain.Transaction {
		var tx domain.Transaction
		require.NoError(t, db.First(&tx, id).Error)
		return tx
	}
	assert.Equal(t, "owed,owed-tomas", get(900).Labels)
	assert.Equal(t, uint(901), get(900).SplitOf)
	assert.Equal(t, "restaurant", get(901).Labels)
	assert.Equal(t, "bar", get(902).Labels, "a pre-label row still gets its rule labels")
	assert.Equal(t, "family", get(here.ID).Labels, "rows already here are left alone")
}
