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

// The AI rule review proposes adds/updates/deletes for the auto-labeling
// rule set; everything the model returns is re-validated and re-counted
// server-side (vocabulary-only adds, real rule IDs, live footprints).
func TestAIRuleReview(t *testing.T) {
	r, db := aiTestRouter(t)
	// Recurring merchant labeled by hand — no rule covers it.
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&domain.Transaction{
			Date: time.Now().AddDate(0, 0, -i), Type: "expense", Amount: 15,
			Category: "Food", Comment: "Wolt Vilnius order", Labels: "delivery"}).Error)
	}
	// A working rule (the seeded Maxima row is labeled groceries) and a dead one.
	require.NoError(t, db.Create(&domain.LabelRule{Label: "groceries", CommentMatch: "maxima"}).Error)
	dead := domain.LabelRule{Label: "bar", CommentMatch: "closed pub xyz"}
	require.NoError(t, db.Create(&dead).Error)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		prompt := body["messages"].([]any)[0].(map[string]any)["content"].(string)
		require.Contains(t, prompt, "CURRENT RULES")
		assert.Contains(t, prompt, `"closed pub xyz"`, "prompt lists existing rules")
		assert.Contains(t, prompt, `"wolt vilnius"`, "prompt surfaces uncovered recurring patterns")
		// Model proposes: a valid add, an invented label, a zero-match add,
		// an update to a made-up rule id, a real update, and a delete.
		reply := `[
		  {"action":"add","label":"delivery","comment_match":"wolt","category":"","reason":"3 hand-labeled wolt orders"},
		  {"action":"add","label":"invented","comment_match":"wolt","category":"","reason":"not in vocabulary"},
		  {"action":"add","label":"groceries","comment_match":"zzz nothing","category":"","reason":"matches nothing"},
		  {"action":"update","rule_id":999,"comment_match":"whatever","category":"","reason":"made-up id"},
		  {"action":"update","rule_id":2,"comment_match":"wolt","category":"","reason":"repoint dead pattern"},
		  {"action":"delete","rule_id":2,"reason":"dead rule"}
		]`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + jsonString(reply) + `}}]}`))
	}))
	t.Cleanup(srv.Close)
	w := budgetDoJSON(r, "PUT", "/api/v1/ai/settings", map[string]any{
		"gateway_url": srv.URL, "model": "m", "api_key": "k"})
	require.Equal(t, 200, w.Code)

	w = budgetDoJSON(r, "POST", "/api/v1/ai/rule-review", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var res struct {
		Suggestions []struct {
			Action          string `json:"action"`
			RuleID          uint   `json:"rule_id"`
			Label           string `json:"label"`
			CommentMatch    string `json:"comment_match"`
			OldCommentMatch string `json:"old_comment_match"`
			Matches         int    `json:"matches"`
			WouldLabel      int    `json:"would_label"`
		} `json:"suggestions"`
		RulesScanned int `json:"rules_scanned"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 2, res.RulesScanned)
	require.Len(t, res.Suggestions, 3, "invalid suggestions filtered: %s", w.Body.String())

	add := res.Suggestions[0]
	assert.Equal(t, "add", add.Action)
	assert.Equal(t, "delivery", add.Label)
	assert.Equal(t, "wolt", add.CommentMatch)
	assert.Equal(t, 3, add.Matches)
	assert.Equal(t, 0, add.WouldLabel, "all wolt rows already carry the label")

	upd := res.Suggestions[1]
	assert.Equal(t, "update", upd.Action)
	assert.Equal(t, dead.ID, upd.RuleID)
	assert.Equal(t, "bar", upd.Label, "updates keep the rule's label")
	assert.Equal(t, "closed pub xyz", upd.OldCommentMatch)
	assert.Equal(t, 3, upd.Matches)
	assert.Equal(t, 3, upd.WouldLabel, "wolt rows lack the 'bar' label")

	del := res.Suggestions[2]
	assert.Equal(t, "delete", del.Action)
	assert.Equal(t, dead.ID, del.RuleID)
	assert.Equal(t, 0, del.Matches, "footprint shows the rule is dead")
}

// Applying rule suggestions mutates the rule set and labels matching history
// (add-only); invalid items are skipped, not fatal.
func TestAIRuleReviewApply(t *testing.T) {
	r, db := aiTestRouter(t)
	// An unlabeled wolt row the new rule should tag, plus a labeled one.
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -1), Type: "expense", Amount: 15,
		Category: "Food", Comment: "Wolt Vilnius order", Labels: ""}).Error)
	require.NoError(t, db.Create(&domain.Transaction{
		Date: time.Now().AddDate(0, 0, -2), Type: "expense", Amount: 30,
		Category: "Transport", Comment: "CIRCLE K degalai", Labels: ""}).Error)
	stale := domain.LabelRule{Label: "transport", CommentMatch: "old gas station"}
	require.NoError(t, db.Create(&stale).Error)
	doomed := domain.LabelRule{Label: "bar", CommentMatch: "closed pub"}
	require.NoError(t, db.Create(&doomed).Error)

	w := budgetDoJSON(r, "POST", "/api/v1/ai/rule-review/apply", map[string]any{
		"items": []map[string]any{
			{"action": "add", "label": "delivery", "comment_match": "wolt"},
			{"action": "update", "rule_id": stale.ID, "comment_match": "circle k"},
			{"action": "delete", "rule_id": doomed.ID},
			{"action": "add", "label": "bad,label", "comment_match": "x y"}, // skipped: comma
			{"action": "delete", "rule_id": 999},                            // skipped: unknown id
		}})
	require.Equal(t, 200, w.Code, w.Body.String())
	var res struct {
		Added, Updated, Deleted, Relabeled int
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Added)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 1, res.Deleted)
	assert.Equal(t, 2, res.Relabeled, "wolt row + circle k row labeled")

	var rules []domain.LabelRule
	require.NoError(t, db.Order("id ASC").Find(&rules).Error)
	require.Len(t, rules, 2, "doomed rule deleted, bad add skipped")
	assert.Equal(t, "circle k", rules[0].CommentMatch, "stale rule repointed")
	assert.Equal(t, "transport", rules[0].Label)
	assert.Equal(t, "delivery", rules[1].Label)
	assert.Equal(t, "wolt", rules[1].CommentMatch)

	var woltRow, gasRow domain.Transaction
	require.NoError(t, db.First(&woltRow, 2).Error)
	require.NoError(t, db.First(&gasRow, 3).Error)
	assert.Equal(t, "delivery", woltRow.Labels)
	assert.Equal(t, "transport", gasRow.Labels)
}
