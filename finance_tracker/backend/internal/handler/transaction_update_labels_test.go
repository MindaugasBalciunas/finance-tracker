package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdate_LabelInteractions(t *testing.T) {
	r, _ := budgetTestRouter(t)

	// Rule: neste -> fuel
	w := budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "fuel", "comment_match": "neste", "create_rule": true,
	})
	require.Equal(t, 200, w.Code)

	// Create: rule applies + manual label kept
	w = budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
		"date": "2026-07-20", "type": "expense", "amount": 50.0,
		"category": "Transport", "comment": "Neste full tank", "labels": "trip",
	})
	require.Equal(t, 201, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"trip,fuel"`)

	// Extract id
	var id float64 = 1

	// 1. ADD a label via update
	w = budgetDoJSON(r, "PUT", "/api/v1/transactions/1", map[string]any{
		"labels": "trip,fuel,vacation",
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"labels":"trip,fuel,vacation"`, "ADD via update")

	// 2. REMOVE a manual label via update
	w = budgetDoJSON(r, "PUT", "/api/v1/transactions/1", map[string]any{
		"labels": "fuel,vacation",
	})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"fuel,vacation"`, "REMOVE manual label")

	// 3. REMOVE a rule-derived label via update (comment unchanged, rule still matches)
	w = budgetDoJSON(r, "PUT", "/api/v1/transactions/1", map[string]any{
		"labels": "vacation",
	})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"vacation"`, "REMOVE rule-derived label must stick")

	// 4. Change comment to newly match the rule -> rule label auto-added
	w = budgetDoJSON(r, "PUT", "/api/v1/transactions/1", map[string]any{
		"comment": "Circle K", "labels": "vacation",
	})
	require.Equal(t, 200, w.Code)
	w = budgetDoJSON(r, "PUT", "/api/v1/transactions/1", map[string]any{
		"comment": "Neste again", "labels": "vacation",
	})
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"labels":"vacation,fuel"`, "newly-matching rule adds label")

	_ = id
}
