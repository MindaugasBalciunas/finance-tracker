package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Label management: stats, typo/merge suggestions, rename (merge) and
// delete across the whole database — transactions, rules and budgets.

// LabelStats responds with every label's footprint, busiest first.
func (h *BudgetHandler) LabelStats(c *gin.Context) {
	stats, err := h.repo.LabelStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

type labelRenameInput struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

// RenameLabel rewrites a label everywhere. Renaming onto an existing label
// merges the two.
func (h *BudgetHandler) RenameLabel(c *gin.Context) {
	var input labelRenameInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	from := strings.ToLower(strings.TrimSpace(input.From))
	to := domain.NormalizeLabels(input.To)
	switch {
	case from == "":
		// An empty 'from' would token-match every UNLABELED transaction
		// (",," contains the empty token) — mass corruption, never valid.
		c.JSON(http.StatusBadRequest, gin.H{"error": "'from' must be a label"})
		return
	case to == "" || strings.Contains(to, ","):
		c.JSON(http.StatusBadRequest, gin.H{"error": "'to' must be a single label"})
		return
	case from == to:
		c.JSON(http.StatusBadRequest, gin.H{"error": "'from' and 'to' are the same label"})
		return
	case domain.IsFixedObligationLabel(from):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "'" + from + "' is a fixed-obligation label used by insights and budgets — it cannot be renamed"})
		return
	}
	res, err := h.repo.RenameLabel(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if res.Transactions+res.Rules+res.Budgets == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "label '" + from + "' is not used anywhere"})
		return
	}
	c.JSON(http.StatusOK, res)
}

type labelDeleteInput struct {
	Label string `json:"label" binding:"required"`
}

// DeleteLabel removes a label from every transaction and budget and deletes
// its rules. POST (not DELETE /labels/:label) keeps the route tree clear of
// the /labels/rules subtree.
func (h *BudgetHandler) DeleteLabel(c *gin.Context) {
	var input labelDeleteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	label := strings.ToLower(strings.TrimSpace(input.Label))
	if label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'label' must be a label"})
		return
	}
	if domain.IsFixedObligationLabel(label) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "'" + label + "' is a fixed-obligation label used by insights and budgets — it cannot be deleted"})
		return
	}
	res, err := h.repo.DeleteLabel(label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if res.Transactions+res.Rules+res.Budgets == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "label '" + label + "' is not used anywhere"})
		return
	}
	c.JSON(http.StatusOK, res)
}

type labelSuggestion struct {
	From      string `json:"from"`
	To        string `json:"to"`
	FromCount int    `json:"from_count"`
	ToCount   int    `json:"to_count"`
	Reason    string `json:"reason"`
}

// LabelSuggestions proposes likely merges: probable typos (small edit
// distance), singular/plural pairs, and multi-word labels that share a word
// with a single-word label. Suggestions only — the user decides.
func (h *BudgetHandler) LabelSuggestions(c *gin.Context) {
	stats, err := h.repo.LabelStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, suggestLabelMerges(stats))
}

// suggestLabelMerges pairs up similar labels. stats must be sorted by
// transaction count descending (LabelStats guarantees it), so suggestions
// default to merging the rarer label into the more common one.
func suggestLabelMerges(stats []domain.LabelStat) []labelSuggestion {
	out := []labelSuggestion{}
	seen := map[string]bool{} // unordered pair key → already suggested
	add := func(to, from domain.LabelStat, reason string) {
		key := from.Label + "\x00" + to.Label
		if from.Label > to.Label {
			key = to.Label + "\x00" + from.Label
		}
		if seen[key] || domain.IsFixedObligationLabel(from.Label) {
			return
		}
		seen[key] = true
		out = append(out, labelSuggestion{From: from.Label, To: to.Label,
			FromCount: from.Transactions, ToCount: to.Transactions, Reason: reason})
	}
	for i := 0; i < len(stats); i++ {
		for j := i + 1; j < len(stats); j++ {
			a, b := stats[i], stats[j] // a is the more common label
			minLen := min(len(a.Label), len(b.Label))
			switch {
			case b.Label == a.Label+"s" || a.Label == b.Label+"s":
				add(a, b, "singular/plural")
			case minLen >= 4 && editDistance(a.Label, b.Label) == 1,
				// Two edits only counts for longer words that share the first
				// letter — typos rarely touch it ("aparment"), while distinct
				// words at distance 2 usually don't ("heating"/"leasing").
				minLen >= 7 && a.Label[0] == b.Label[0] && editDistance(a.Label, b.Label) == 2:
				add(a, b, "possible typo")
			case sharesWholeWord(a.Label, b.Label):
				add(a, b, "related")
			}
		}
	}
	return out
}

// sharesWholeWord reports whether one label is a single word that appears
// as a whole word of the other, multi-word label ("lunch" / "work lunch",
// "bank fee" / "fees"). Guards against substring noise ("car" / "car wash"
// still matches — that IS a word match — but "bar" / "barclays" does not).
func sharesWholeWord(a, b string) bool {
	single, multi := a, b
	if strings.Contains(a, " ") == strings.Contains(b, " ") {
		return false // both single- or both multi-word: other rules cover those
	}
	if strings.Contains(a, " ") {
		single, multi = b, a
	}
	if len(single) < 4 {
		return false // "car"/"car wash" is usually deliberate nesting
	}
	for _, w := range strings.Fields(multi) {
		if w == single || w+"s" == single || w == single+"s" {
			return true
		}
	}
	return false
}

// editDistance is Damerau-Levenshtein (optimal string alignment), so a
// transposition ("lona"/"loan") counts as one edit like a real typo does.
// Label vocabularies are tiny, so the O(len²) DP is fine.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}
