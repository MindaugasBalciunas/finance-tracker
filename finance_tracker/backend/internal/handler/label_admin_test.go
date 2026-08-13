package handler

import (
	"encoding/json"
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabelRenameMergeDelete(t *testing.T) {
	r, db := budgetTestRouter(t)
	post := func(path string, body map[string]any) map[string]any {
		w := budgetDoJSON(r, "POST", path, body)
		require.Equal(t, 200, w.Code, w.Body.String())
		var res map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
		return res
	}
	seed := func(comment, labels string) {
		w := budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
			"date": "2026-01-10", "type": "expense", "category": "Housing",
			"amount": 10.0, "comment": comment, "labels": labels,
		})
		require.Equal(t, 201, w.Code, w.Body.String())
	}
	seed("paint", "aparment")
	seed("nails", "aparment,diy")
	seed("both spellings", "aparment,apartment")
	seed("already right", "apartment")

	// A rule and a budget that reference the typo.
	w := budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "aparment", "comment_match": "paint", "create_rule": true})
	require.Equal(t, 200, w.Code, w.Body.String())
	// An identical rule already exists under the canonical spelling — the
	// renamed rule must collapse into it, not duplicate it.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "apartment", "comment_match": "paint", "create_rule": true})
	require.Equal(t, 200, w.Code, w.Body.String())
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{
		"name": "Home", "kind": "spending", "label": "aparment,house", "amount": 300.0})
	require.Equal(t, 201, w.Code, w.Body.String())

	res := post("/api/v1/labels/rename", map[string]any{"from": "aparment", "to": "apartment"})
	assert.EqualValues(t, 3, res["transactions"], "three transactions carried the typo")
	assert.EqualValues(t, 1, res["rules"])
	assert.EqualValues(t, 1, res["budgets"])

	var txs []domain.Transaction
	require.NoError(t, db.Where("(',' || labels || ',') LIKE '%,aparment,%'").Find(&txs).Error)
	assert.Empty(t, txs, "typo gone from every transaction")
	var both domain.Transaction
	require.NoError(t, db.Where("comment = ?", "both spellings").First(&both).Error)
	assert.Equal(t, "apartment", both.Labels, "merge deduplicates")

	var rules []domain.LabelRule
	require.NoError(t, db.Where("comment_match = ?", "paint").Find(&rules).Error)
	require.Len(t, rules, 1, "identical rules collapsed")
	assert.Equal(t, "apartment", rules[0].Label)

	var budget domain.Budget
	require.NoError(t, db.Where("name = ?", "Home").First(&budget).Error)
	assert.Equal(t, "apartment,house", budget.Label)

	// Delete removes the label everywhere and drops its rules.
	seed("beer", "bar,entertainment")
	w = budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "bar", "comment_match": "beer", "create_rule": true})
	require.Equal(t, 200, w.Code)
	res = post("/api/v1/labels/delete", map[string]any{"label": "bar"})
	assert.EqualValues(t, 1, res["transactions"])
	assert.EqualValues(t, 1, res["rules"])
	var beer domain.Transaction
	require.NoError(t, db.Where("comment = ?", "beer").First(&beer).Error)
	assert.Equal(t, "entertainment", beer.Labels)
	var barRules int64
	require.NoError(t, db.Model(&domain.LabelRule{}).Where("label = ?", "bar").Count(&barRules).Error)
	assert.Zero(t, barRules)

	// Fixed-obligation labels are protected.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/rename", map[string]any{"from": "loan", "to": "mortgage"})
	assert.Equal(t, 422, w.Code, w.Body.String())
	w = budgetDoJSON(r, "POST", "/api/v1/labels/delete", map[string]any{"label": "evelina"})
	assert.Equal(t, 422, w.Code, w.Body.String())
	// Merging INTO a fixed label is allowed.
	seed("instalment", "mortgage")
	res = post("/api/v1/labels/rename", map[string]any{"from": "mortgage", "to": "loan"})
	assert.EqualValues(t, 1, res["transactions"])

	// Validation.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/rename", map[string]any{"from": "x", "to": "x"})
	assert.Equal(t, 400, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/labels/rename", map[string]any{"from": "x", "to": "a,b"})
	assert.Equal(t, 400, w.Code)
	// A whitespace-only 'from' must never reach the repo: the empty token
	// matches every unlabeled transaction.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/rename", map[string]any{"from": "  ", "to": "junk"})
	assert.Equal(t, 400, w.Code, w.Body.String())
	var unlabeled int64
	require.NoError(t, db.Model(&domain.Transaction{}).
		Where("(',' || labels || ',') LIKE '%,junk,%'").Count(&unlabeled).Error)
	assert.Zero(t, unlabeled, "no transaction may gain the junk label")
	w = budgetDoJSON(r, "POST", "/api/v1/labels/delete", map[string]any{"label": "  "})
	assert.Equal(t, 400, w.Code, w.Body.String())
	// Unknown labels are a 404, not a phantom success.
	w = budgetDoJSON(r, "POST", "/api/v1/labels/rename", map[string]any{"from": "no-such-label", "to": "whatever"})
	assert.Equal(t, 404, w.Code, w.Body.String())
	w = budgetDoJSON(r, "POST", "/api/v1/labels/delete", map[string]any{"label": "no-such-label"})
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// Deleting a label that was a budget's only matcher removes the budget —
// a matcher-less budget would silently occupy the monthly plan forever.
func TestDeleteLabelRemovesDeadBudget(t *testing.T) {
	r, db := budgetTestRouter(t)
	w := budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
		"date": "2026-03-01", "type": "expense", "category": "Housing",
		"amount": 20.0, "comment": "x", "labels": "gadgets",
	})
	require.Equal(t, 201, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{
		"name": "Gadgets", "kind": "spending", "label": "gadgets", "amount": 50.0})
	require.Equal(t, 201, w.Code)
	w = budgetDoJSON(r, "POST", "/api/v1/budgets", map[string]any{
		"name": "Housing stuff", "kind": "spending", "label": "gadgets", "category": "Housing", "amount": 80.0})
	require.Equal(t, 201, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/labels/delete", map[string]any{"label": "gadgets"})
	require.Equal(t, 200, w.Code, w.Body.String())

	var dead int64
	require.NoError(t, db.Model(&domain.Budget{}).Where("name = ?", "Gadgets").Count(&dead).Error)
	assert.Zero(t, dead, "label-only budget removed with its label")
	var kept domain.Budget
	require.NoError(t, db.Where("name = ?", "Housing stuff").First(&kept).Error)
	assert.Equal(t, "", kept.Label, "category-backed budget survives, minus the label")
	assert.Equal(t, "Housing", kept.Category)
}

func TestLabelStatsAndSuggestions(t *testing.T) {
	r, _ := budgetTestRouter(t)
	seed := func(comment, labels string, amount float64) {
		w := budgetDoJSON(r, "POST", "/api/v1/transactions", map[string]any{
			"date": "2026-02-01", "type": "expense", "category": "Housing",
			"amount": amount, "comment": comment, "labels": labels,
		})
		require.Equal(t, 201, w.Code, w.Body.String())
	}
	seed("a", "apartment", 100)
	seed("b", "apartment,loan", 50)
	seed("c", "aparment", 25)
	seed("d", "work lunch", 10)
	seed("d2", "work lunch", 10)
	seed("e", "lunch", 10)

	w := budgetDoJSON(r, "GET", "/api/v1/labels/stats", nil)
	require.Equal(t, 200, w.Code)
	var stats []domain.LabelStat
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	require.NotEmpty(t, stats)
	assert.Equal(t, "apartment", stats[0].Label, "busiest label first")
	assert.Equal(t, 2, stats[0].Transactions)
	assert.InDelta(t, 150, stats[0].Amount, 0.001)
	assert.Equal(t, "2026-02-01", stats[0].LastUsed)
	for _, s := range stats {
		if s.Label == "loan" {
			assert.True(t, s.Fixed)
		} else {
			assert.False(t, s.Fixed, s.Label)
		}
	}

	w = budgetDoJSON(r, "GET", "/api/v1/labels/suggestions", nil)
	require.Equal(t, 200, w.Code)
	var sugg []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sugg))
	find := func(from, to string) map[string]any {
		for _, s := range sugg {
			if s["from"] == from && s["to"] == to {
				return s
			}
		}
		return nil
	}
	typo := find("aparment", "apartment")
	require.NotNil(t, typo, "typo pair suggested: %v", sugg)
	assert.Equal(t, "possible typo", typo["reason"])
	related := find("lunch", "work lunch")
	require.NotNil(t, related, "shared-word pair suggested: %v", sugg)
	assert.Equal(t, "related", related["reason"])
}

func TestSuggestLabelMerges(t *testing.T) {
	st := func(label string, n int) domain.LabelStat {
		return domain.LabelStat{Label: label, Transactions: n}
	}
	sugg := suggestLabelMerges([]domain.LabelStat{
		st("bar", 129), st("bars", 98), st("cash", 155), st("car", 32),
		st("tax", 13), st("taxi", 27), st("cat", 5), st("bank fee", 445), st("fees", 358),
		st("loan", 245), st("lona", 1), st("leasing", 70), st("heating", 1),
		st("groceries", 555), st("grocerys", 2),
	})
	byPair := map[string]string{}
	for _, s := range sugg {
		byPair[s.From+"->"+s.To] = s.Reason
	}
	assert.Equal(t, "singular/plural", byPair["bars->bar"])
	assert.Equal(t, "related", byPair["fees->bank fee"])
	assert.Contains(t, byPair, "lona->loan", "merging INTO a fixed label is suggestible")
	assert.Equal(t, "possible typo", byPair["grocerys->groceries"],
		"distance-2 typo with matching first letter is caught")
	// Short near-misses with different meanings stay quiet.
	assert.NotContains(t, byPair, "car->cash")
	assert.NotContains(t, byPair, "tax->taxi")
	assert.NotContains(t, byPair, "taxi->tax")
	assert.NotContains(t, byPair, "cat->car")
	assert.NotContains(t, byPair, "heating->leasing",
		"distance-2 words with different first letters are distinct words, not typos")
	// Fixed labels are never suggested as the merge source.
	for _, s := range sugg {
		assert.NotEqual(t, "loan", s.From)
	}
}