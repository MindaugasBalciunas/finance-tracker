package handler

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// budgetV2Router serves budgets CRUD and the engine-backed status/trips.
func budgetV2Router(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	r, db := aiTestRouter(t)
	NewBudgetHandler(repository.NewBudgetRepository(db)).RegisterRoutes(r.Group("/api/v1"))
	return r, db
}

// A changed amount applies from its month; "all" corrects history.
func TestBudgetUpdate_AmountHistory(t *testing.T) {
	r, db := budgetV2Router(t)
	w := budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "Kids", "kind": "spending", "category": "Kids", "amount": 200})
	require.Equal(t, 201, w.Code, w.Body.String())
	var b domain.Budget
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	assert.Equal(t, domain.PeriodMonthly, b.Period)

	w = budgetDoJSON(r, "PUT", "/api/v1/budgets/"+strconv.Itoa(int(b.ID)), map[string]any{"name": "Kids", "kind": "spending", "category": "Kids", "amount": 600, "amount_from": "2026-10"})
	require.Equal(t, 200, w.Code, w.Body.String())
	var steps []domain.BudgetAmount
	require.NoError(t, db.Order("from_month").Find(&steps).Error)
	require.Len(t, steps, 2)
	assert.Equal(t, "", steps[0].FromMonth)
	assert.EqualValues(t, 200, steps[0].Amount)
	assert.Equal(t, "2026-10", steps[1].FromMonth)

	w = budgetDoJSON(r, "GET", "/api/v1/budgets/status?month=2026-09", nil)
	require.Equal(t, 200, w.Code)
	var rep struct {
		Lines []struct {
			Name     string  `json:"name"`
			Budgeted float64 `json:"budgeted"`
		} `json:"lines"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rep))
	require.Len(t, rep.Lines, 1)
	assert.EqualValues(t, 200, rep.Lines[0].Budgeted, "September keeps the old limit")

	w = budgetDoJSON(r, "PUT", "/api/v1/budgets/"+strconv.Itoa(int(b.ID)), map[string]any{"name": "Kids", "kind": "spending", "category": "Kids", "amount": 500, "amount_from": "all"})
	require.Equal(t, 200, w.Code)
	var n int64
	db.Model(&domain.BudgetAmount{}).Count(&n)
	assert.EqualValues(t, 0, n)

	// Validation.
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "X", "kind": "fixed", "label": "loan", "amount": 1, "fund": true})
	assert.Equal(t, 400, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "X", "kind": "spending", "category": "Food", "amount": 1, "period": "weekly"})
	assert.Equal(t, 400, w.Code)
}

// A fund line reports its carried balance through /budgets/status.
func TestBudgetStatus_FundLine(t *testing.T) {
	r, db := budgetV2Router(t)
	start := time.Now().AddDate(0, -2, 0).Format("2006-01")
	w := budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{
		"name": "Vacation", "kind": "spending", "category": "Vacation", "amount": 400, "fund": true, "start_month": start})
	require.Equal(t, 201, w.Code, w.Body.String())
	require.NoError(t, db.Create(&domain.Transaction{Date: time.Now().AddDate(0, -1, 0), Type: "expense", Amount: 1000, Category: "Vacation"}).Error)

	w = budgetDoJSON(r, "GET", "/api/v1/budgets/status", nil)
	require.Equal(t, 200, w.Code)
	var rep struct {
		Lines []struct {
			FundState *struct {
				Opening   float64 `json:"opening"`
				Available float64 `json:"available"`
			} `json:"fund_state"`
		} `json:"lines"`
		FundContributions float64 `json:"fund_contributions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rep))
	require.Len(t, rep.Lines, 1)
	require.NotNil(t, rep.Lines[0].FundState)
	assert.InDelta(t, -200, rep.Lines[0].FundState.Opening, 0.01) // 2×400 − 1000
	assert.InDelta(t, 200, rep.Lines[0].FundState.Available, 0.01)
	assert.InDelta(t, 400, rep.FundContributions, 0.01)
}

// Suggested trips can be tagged, then show up costed with their budget.
func TestTrips_SuggestAssignSummarise(t *testing.T) {
	r, db := budgetV2Router(t)
	d := func(m time.Month, day int) time.Time { return time.Date(2025, m, day, 0, 0, 0, 0, time.UTC) }
	rows := []domain.Transaction{
		{Date: d(7, 20), Type: "expense", Amount: 300, Category: "Vacation", Comment: "Hotel Zakopane", Labels: "vacation,hotel"},
		{Date: d(7, 22), Type: "expense", Amount: 45.5, Category: "Vacation", Comment: "Restauracja", Labels: "vacation,restaurant"},
		{Date: d(7, 24), Type: "income", Amount: 40, Category: "Reimbursement", Comment: "Friend's share"},
		{Date: d(11, 2), Type: "expense", Amount: 120, Category: "Vacation", Comment: "Riga hostel"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	w := budgetDoJSON(r, "GET", "/api/v1/budgets/trips", nil)
	require.Equal(t, 200, w.Code)
	var res struct {
		Trips       []map[string]any `json:"trips"`
		Suggestions []struct {
			From           string `json:"from"`
			TxIDs          []uint `json:"tx_ids"`
			SuggestedLabel string `json:"suggested_label"`
		} `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Empty(t, res.Trips)
	require.Len(t, res.Suggestions, 2)
	assert.Equal(t, "2025-11-02", res.Suggestions[0].From, "newest first")
	zak := res.Suggestions[1]
	assert.Equal(t, "trip:2025-07", zak.SuggestedLabel)
	require.Len(t, zak.TxIDs, 2)

	// Tag the Zakopane rows + the refund; budget the trip.
	ids := append(zak.TxIDs, rows[2].ID)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets/trips/assign", map[string]any{"name": "Zakopane 2025", "tx_ids": ids})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"label":"trip:zakopane-2025"`)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{"name": "Zakopane 2025", "kind": "trip", "amount": 500})
	require.Equal(t, 201, w.Code, w.Body.String())

	w = budgetDoJSON(r, "GET", "/api/v1/budgets/trips", nil)
	require.Equal(t, 200, w.Code)
	var res2 struct {
		Trips []struct {
			Label     string   `json:"label"`
			Days      int      `json:"days"`
			Total     float64  `json:"total"`
			PerDay    float64  `json:"per_day"`
			Remaining *float64 `json:"remaining"`
			ByLabel   []struct {
				Name   string  `json:"name"`
				Amount float64 `json:"amount"`
			} `json:"by_label"`
		} `json:"trips"`
		Suggestions []any `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res2))
	require.Len(t, res2.Trips, 1)
	tr := res2.Trips[0]
	assert.Equal(t, "trip:zakopane-2025", tr.Label)
	assert.Equal(t, 5, tr.Days)
	assert.InDelta(t, 305.5, tr.Total, 0.01) // 300 + 45.5 − 40 refund
	assert.InDelta(t, 61.1, tr.PerDay, 0.01)
	require.NotNil(t, tr.Remaining)
	assert.InDelta(t, 194.5, *tr.Remaining, 0.01)
	assert.Equal(t, "hotel", tr.ByLabel[0].Name)
	assert.Len(t, res2.Suggestions, 1, "only Riga is left to tag")

	// The trip budget stays out of the monthly plan.
	w = budgetDoJSON(r, "GET", "/api/v1/budgets/status", nil)
	assert.NotContains(t, w.Body.String(), "Zakopane")
}

// v6 backups carry period/fund/start and the amount history.
func TestBackupRoundtrip_BudgetV6(t *testing.T) {
	_, src := importRouterFor(t)
	require.NoError(t, src.AutoMigrate(&domain.ExportLog{}, &domain.BudgetAmount{}))
	b := domain.Budget{Name: "Vacation", Kind: "spending", Category: "Vacation", Amount: 450, Period: domain.PeriodYearly, Fund: true, StartMonth: "2026-01"}
	require.NoError(t, src.Create(&b).Error)
	require.NoError(t, src.Create(&domain.BudgetAmount{BudgetID: b.ID, FromMonth: "", Amount: 400}).Error)
	require.NoError(t, src.Create(&domain.BudgetAmount{BudgetID: b.ID, FromMonth: "2026-10", Amount: 450}).Error)

	body := v5Export(t, src, "/api/v1/export/finances.json")
	dstRouter, dst := importRouterFor(t)
	require.NoError(t, dst.AutoMigrate(&domain.BudgetAmount{}))
	v5Import(t, dstRouter, body)

	var got domain.Budget
	require.NoError(t, dst.First(&got).Error)
	assert.Equal(t, domain.PeriodYearly, got.Period)
	assert.True(t, got.Fund)
	assert.Equal(t, "2026-01", got.StartMonth)
	var steps []domain.BudgetAmount
	require.NoError(t, dst.Order("from_month").Find(&steps).Error)
	require.Len(t, steps, 2)
	assert.Equal(t, got.ID, steps[1].BudgetID)
	assert.EqualValues(t, 450, steps[1].Amount)
}
