package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Amount-range filtering works on both the list and the summary endpoint
// (they share buildTransactionFilter), and the summary carries the per-label
// breakdown — the query surface the AI tools and MCP rely on.
func TestTransactionAmountRangeAndByLabel(t *testing.T) {
	r, db := budgetTestRouter(t)
	rows := []domain.Transaction{
		{Date: time.Now().AddDate(0, 0, -3), Type: "expense", Amount: 12, Category: "Food", Comment: "Lidl", Labels: "groceries"},
		{Date: time.Now().AddDate(0, 0, -2), Type: "expense", Amount: 150, Category: "Food", Comment: "Rimi big shop", Labels: "groceries"},
		{Date: time.Now().AddDate(0, 0, -1), Type: "expense", Amount: 900, Category: "Housing", Comment: "Rent", Labels: "apartment"},
		{Date: time.Now(), Type: "expense", Amount: 500, Category: "Transfers", Comment: "Own move", Labels: "apartment"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	// amount_min alone
	w := budgetDoJSON(r, "GET", "/api/v1/transactions?amount_min=100", nil)
	require.Equal(t, 200, w.Code)
	var page struct {
		Total int64                `json:"total"`
		Data  []domain.Transaction `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.EqualValues(t, 3, page.Total, "150, 900 and the 500 transfer")

	// amount range
	w = budgetDoJSON(r, "GET", "/api/v1/transactions?amount_min=100&amount_max=200", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.EqualValues(t, 1, page.Total)
	assert.Equal(t, "Rimi big shop", page.Data[0].Comment)

	// summary honors the same params and exposes by_label (Transfers excluded)
	w = budgetDoJSON(r, "GET", "/api/v1/transactions/summary?amount_min=100", nil)
	require.Equal(t, 200, w.Code)
	var summary domain.TransactionSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
	assert.EqualValues(t, 1050, summary.TotalExpenses, "transfer excluded from totals")
	require.NotEmpty(t, summary.ByLabel)
	byLabel := map[string]domain.LabelSummary{}
	for _, ls := range summary.ByLabel {
		byLabel[ls.Label] = ls
	}
	assert.EqualValues(t, 900, byLabel["apartment"].Total, "transfer row's label excluded")
	assert.Equal(t, 1, byLabel["apartment"].Count)
	assert.EqualValues(t, 150, byLabel["groceries"].Total)
}

// /budgets/status returns structured month-to-date progress with the same
// semantics as the AI report's text section.
func TestBudgetStatusEndpoint(t *testing.T) {
	r, db := aiTestRouter(t)
	now := time.Now()
	require.NoError(t, db.Create(&domain.Budget{Name: "Loan", Kind: "fixed", Label: "loan", Amount: 1200}).Error)
	require.NoError(t, db.Create(&domain.Budget{Name: "Eating out", Kind: "spending", Label: "restaurant", Amount: 200}).Error)
	// This month: the loan payment and a restaurant bill.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: now, Type: "expense", Amount: 1200, Category: "Finance", Comment: "Busto paskola", Labels: "loan"}).Error)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: now, Type: "expense", Amount: 60, Category: "Food", Comment: "Pizza", Labels: "restaurant"}).Error)

	w := budgetDoJSON(r, "GET", "/api/v1/budgets/status", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var res struct {
		Month              string  `json:"month"`
		FixedPlanned       float64 `json:"fixed_planned"`
		SpendingPlanned    float64 `json:"spending_planned"`
		DiscretionarySpent float64 `json:"discretionary_spent"`
		Lines              []struct {
			Name      string  `json:"name"`
			Kind      string  `json:"kind"`
			Budgeted  float64 `json:"budgeted"`
			Spent     float64 `json:"spent"`
			Remaining float64 `json:"remaining"`
		} `json:"lines"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, now.Format("2006-01"), res.Month)
	assert.EqualValues(t, 1200, res.FixedPlanned)
	assert.EqualValues(t, 200, res.SpendingPlanned)
	require.Len(t, res.Lines, 2)
	byName := map[string]float64{}
	remaining := map[string]float64{}
	for _, l := range res.Lines {
		byName[l.Name] = l.Spent
		remaining[l.Name] = l.Remaining
	}
	assert.EqualValues(t, 1200, byName["Loan"], "loan payment counted against the fixed line")
	assert.EqualValues(t, 60, byName["Eating out"])
	assert.EqualValues(t, 140, remaining["Eating out"])
	// Discretionary excludes the fixed-claimed loan row; the seeded groceries
	// row from aiTestRouter is last month, so only the pizza counts.
	assert.EqualValues(t, 60, res.DiscretionarySpent, "pizza is discretionary, loan is not")

	// Bad month format is a 400, and an explicit past month returns zeros.
	w = budgetDoJSON(r, "GET", "/api/v1/budgets/status?month=2026-13", nil)
	assert.Equal(t, 400, w.Code)
	w = budgetDoJSON(r, "GET", "/api/v1/budgets/status?month=2019-01", nil)
	require.Equal(t, 200, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.EqualValues(t, 0, byNameSpent(res.Lines, "Loan"))
	assert.Equal(t, "2019-01", res.Month)
}

func byNameSpent(lines []struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Budgeted  float64 `json:"budgeted"`
	Spent     float64 `json:"spent"`
	Remaining float64 `json:"remaining"`
}, name string) float64 {
	for _, l := range lines {
		if l.Name == name {
			return l.Spent
		}
	}
	return -1
}

// The chat tool loop can reach the new tools: the model calls
// search_transactions with an amount floor, then get_budget_status, and the
// backend feeds real filtered results back.
func TestAIChatExpandedTools(t *testing.T) {
	r, db := aiTestRouter(t)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now(), Type: "expense", Amount: 240, Category: "Housing", Comment: "Big repair bill", Labels: "house"}).Error)
	require.NoError(t, db.Create(&domain.Budget{Name: "Loan", Kind: "fixed", Label: "loan", Amount: 1200}).Error)
	require.NoError(t, db.Create(&domain.LabelRule{Label: "house", CommentMatch: "repair"}).Error)

	searchCall := `{"content":[{"type":"tool_use","id":"c1","name":"search_transactions","input":{"amount_min":100}}],"stop_reason":"tool_use"}`
	statusCall := `{"content":[{"type":"tool_use","id":"c2","name":"get_budget_status","input":{}},{"type":"tool_use","id":"c3","name":"get_label_rules","input":{}}],"stop_reason":"tool_use"}`
	final := anthropicText("done")
	srv, requests := scriptedGateway(t, []string{searchCall, statusCall, final})

	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/chat", map[string]any{"message": "what big bills hit me and am I on budget?"})
	require.Equal(t, 200, w.Code, w.Body.String())

	// Request 2 carries the search result: only the 240 EUR row (the seeded
	// 50 EUR groceries row is under the floor).
	require.GreaterOrEqual(t, len(*requests), 3)
	toolMsg := extractToolContent(t, (*requests)[1], "c1")
	assert.Contains(t, toolMsg, "Big repair bill")
	assert.NotContains(t, toolMsg, "Maxima", "row under amount_min filtered out")

	// Request 3 carries budget status + rules.
	statusMsg := extractToolContent(t, (*requests)[2], "c2")
	assert.Contains(t, statusMsg, `"fixed_planned":1200`)
	rulesMsg := extractToolContent(t, (*requests)[2], "c3")
	assert.Contains(t, rulesMsg, `"comment_match":"repair"`)
}

// extractToolContent finds the tool_result block with the given tool_use_id
// in a captured gateway request.
func extractToolContent(t *testing.T, req map[string]any, callID string) string {
	t.Helper()
	msgs, ok := req["messages"].([]any)
	require.True(t, ok)
	for _, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		blocks, ok := mm["content"].([]any)
		if !ok {
			continue
		}
		for _, b := range blocks {
			bb, ok := b.(map[string]any)
			if ok && bb["type"] == "tool_result" && bb["tool_use_id"] == callID {
				s, _ := bb["content"].(string)
				return s
			}
		}
	}
	t.Fatalf("no tool_result for call %s", callID)
	return ""
}

// The MCP-facing routes stay inside the read-only token allowlist.
func TestNewRoutesTokenReachable(t *testing.T) {
	for _, path := range []string{
		"/api/v1/budgets/status",
		"/api/v1/labels/rules",
		"/api/v1/stocks",
		"/api/v1/transactions/summary",
	} {
		assert.True(t, apiTokenAllowed(path), path)
	}
	assert.False(t, apiTokenAllowed("/api/v1/ai/chat"))
}

var _ = httptest.NewRecorder // keep import if assertions change
var _ = http.MethodGet
