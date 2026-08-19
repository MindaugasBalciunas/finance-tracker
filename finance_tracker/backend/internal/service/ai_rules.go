package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// AI rule review: audit the auto-labeling rule set against the actual
// transaction history and propose fixes — new rules for recurring patterns no
// rule covers, updates for too-broad or misfiring patterns, deletions for
// dead or redundant rules. Suggestion-only: nothing changes until the user
// applies the checked items, and every proposal ships with its live match
// counts so the user sees the blast radius before approving.

// RuleSuggestion is one proposed change to the rule set.
type RuleSuggestion struct {
	Action          string `json:"action"` // add | update | delete
	RuleID          uint   `json:"rule_id,omitempty"`
	Label           string `json:"label"`
	CommentMatch    string `json:"comment_match,omitempty"`
	Category        string `json:"category,omitempty"`
	OldCommentMatch string `json:"old_comment_match,omitempty"`
	OldCategory     string `json:"old_category,omitempty"`
	Reason          string `json:"reason,omitempty"`
	// Matches counts transactions the (new) pattern hits; WouldLabel counts
	// how many of those don't carry the label yet.
	Matches    int `json:"matches"`
	WouldLabel int `json:"would_label"`
}

type RuleReviewResult struct {
	Suggestions  []RuleSuggestion `json:"suggestions"`
	RulesScanned int              `json:"rules_scanned"`
}

// RuleApplyItem is one user-approved rule change.
type RuleApplyItem struct {
	Action       string `json:"action"`
	RuleID       uint   `json:"rule_id"`
	Label        string `json:"label"`
	CommentMatch string `json:"comment_match"`
	Category     string `json:"category"`
}

type RuleApplyResult struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Deleted   int `json:"deleted"`
	Relabeled int `json:"relabeled"`
}

const ruleSuggestionCap = 20

// ruleFootprint counts, in memory (same semantics as the SQL applier), how
// many transactions a rule matches and how many of those lack its label.
func ruleFootprint(rule domain.LabelRule, all []domain.Transaction) (matches, wouldLabel int) {
	for i := range all {
		if rule.Matches(&all[i]) {
			matches++
			if !all[i].HasLabel(rule.Label) {
				wouldLabel++
			}
		}
	}
	return
}

// commentKey normalizes a description to its first two digit-free tokens —
// good enough to group recurring merchants ("wolt vilnius", "bolt food").
func commentKey(comment string) string {
	var kept []string
	for _, f := range strings.Fields(strings.ToLower(comment)) {
		f = strings.Trim(f, "'\".,()*:;-/")
		if f == "" || strings.ContainsAny(f, "0123456789") {
			continue // dates, card refs, amounts
		}
		kept = append(kept, f)
		if len(kept) == 2 {
			break
		}
	}
	return strings.Join(kept, " ")
}

// normalizeRulePattern lowercases and bounds a proposed comment pattern.
// Returns "" when the pattern is unusable.
func normalizeRulePattern(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if len(p) > 60 {
		return ""
	}
	if strings.TrimPrefix(p, "^") == "" || len(strings.TrimPrefix(p, "^")) < 2 {
		return "" // 1-char substrings match half the database
	}
	return p
}

// normalizeRuleCategory returns the canonical category or "" (any); the
// boolean is false when the value isn't a real category.
func normalizeRuleCategory(c string) (string, bool) {
	c = strings.TrimSpace(c)
	if c == "" {
		return "", true
	}
	for _, v := range domain.ValidCategories {
		if strings.EqualFold(string(v), c) {
			return string(v), true
		}
	}
	return "", false
}

// RuleReview asks the gateway to audit the rule set. The model sees every
// rule with its live footprint plus the recurring transaction patterns no
// rule covers, and proposes adds/updates/deletes; everything it returns is
// re-validated and re-counted server-side before the user sees it.
func (s *insightService) RuleReview() (*RuleReviewResult, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if !settings.Configured() {
		return nil, errors.New("AI gateway not configured")
	}
	if s.budgetRepo == nil {
		return nil, errors.New("labels unavailable")
	}

	rules, err := s.budgetRepo.ListRules()
	if err != nil {
		return nil, err
	}
	all, err := s.txSvc.ListAll()
	if err != nil {
		return nil, err
	}
	stats, err := s.budgetRepo.LabelStats()
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(stats))
	var vocab []string
	for i, st := range stats {
		allowed[st.Label] = true
		if i < 80 {
			vocab = append(vocab, st.Label)
		}
	}

	// Every rule with its live footprint — dead and pending rules stand out.
	byID := make(map[uint]domain.LabelRule, len(rules))
	rulesByLabel := map[string][]domain.LabelRule{}
	var ruleLines []string
	for _, r := range rules {
		byID[r.ID] = r
		rulesByLabel[r.Label] = append(rulesByLabel[r.Label], r)
		m, w := ruleFootprint(r, all)
		cat := r.Category
		if cat == "" {
			cat = "any"
		}
		ruleLines = append(ruleLines, fmt.Sprintf("  id=%d | label %q | pattern %q | category %s | matches %d rows (%d without the label)",
			r.ID, r.Label, r.CommentMatch, cat, m, w))
	}

	// Recurring patterns the rule set misses: labeled rows whose label no rule
	// supplies, and unlabeled rows that repeat.
	type gap struct {
		key, label string
		count      int
	}
	uncovered := map[string]*gap{}
	unlabeled := map[string]int{}
	for i := range all {
		tx := &all[i]
		key := commentKey(tx.Comment)
		if key == "" {
			continue
		}
		labels := splitLabels(tx.Labels)
		if len(labels) == 0 {
			if strings.TrimSpace(tx.Comment) != "" {
				unlabeled[key]++
			}
			continue
		}
		for _, l := range labels {
			covered := false
			for _, r := range rulesByLabel[l] {
				if r.Matches(tx) {
					covered = true
					break
				}
			}
			if !covered {
				k := key + "\x00" + l
				if uncovered[k] == nil {
					uncovered[k] = &gap{key: key, label: l}
				}
				uncovered[k].count++
			}
		}
	}
	var gaps []*gap
	for _, g := range uncovered {
		if g.count >= 3 {
			gaps = append(gaps, g)
		}
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].count > gaps[j].count })
	if len(gaps) > 30 {
		gaps = gaps[:30]
	}
	var gapLines []string
	for _, g := range gaps {
		gapLines = append(gapLines, fmt.Sprintf("  %q ×%d -> labeled %q by hand, no rule covers them", g.key, g.count, g.label))
	}
	type ug struct {
		key   string
		count int
	}
	var ugs []ug
	for k, c := range unlabeled {
		if c >= 3 {
			ugs = append(ugs, ug{k, c})
		}
	}
	sort.Slice(ugs, func(i, j int) bool { return ugs[i].count > ugs[j].count })
	if len(ugs) > 15 {
		ugs = ugs[:15]
	}
	for _, g := range ugs {
		gapLines = append(gapLines, fmt.Sprintf("  %q ×%d -> unlabeled, no rule fits", g.key, g.count))
	}
	if len(gapLines) == 0 {
		gapLines = []string{"  (none — every recurring pattern is covered)"}
	}

	prompt := fmt.Sprintf(`You are auditing the auto-labeling rules of a personal finance tracker. A rule applies its label to every transaction whose comment CONTAINS the pattern (case-insensitive substring; a leading ^ anchors to the start) and, when a category is set, whose category matches.

CURRENT RULES (with live footprint against the whole history):
%s

RECURRING PATTERNS THE RULE SET MISSES:
%s

LABELS IN USE (rules may only use these):
%s

Propose improvements to the RULE SET (not to individual transactions):
- "add": a new rule for a recurring pattern that clearly belongs to one label. Prefer the shortest distinctive merchant token ("wolt", not "wolt vilnius payment"); use ^ for short tokens that would substring-match everyday words.
- "update": fix an existing rule — too-broad patterns that mislabel, typos, a missing category restriction, or a pattern that should be anchored. Keep the rule's label.
- "delete": dead rules (0 matches) that reference merchants unlikely to return, and redundant rules fully shadowed by another rule of the same label.
Rules that work stay untouched. When unsure, skip. At most %d suggestions, highest impact first.

Reply as ONE JSON array, nothing else:
[{"action":"add","label":"delivery","comment_match":"wolt","category":"","reason":"..."},
 {"action":"update","rule_id":12,"comment_match":"^iki","category":"Food","reason":"..."},
 {"action":"delete","rule_id":7,"reason":"..."}]`,
		strings.Join(ruleLines, "\n"), strings.Join(gapLines, "\n"), strings.Join(vocab, ", "), ruleSuggestionCap)

	reply, err := callGateway(context.Background(), settings, []domain.ChatMessage{{Role: "user", Content: prompt}}, 8192)
	if err != nil {
		return nil, err
	}
	var parsed []struct {
		Action       string `json:"action"`
		RuleID       uint   `json:"rule_id"`
		Label        string `json:"label"`
		CommentMatch string `json:"comment_match"`
		Category     string `json:"category"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(extractJSON(reply, '[', ']')), &parsed); err != nil {
		return nil, errors.New("the model returned an unparseable review — try again")
	}

	seen := map[string]bool{}
	for _, r := range rules {
		seen["add|"+r.Label+"|"+strings.ToLower(r.CommentMatch)+"|"+r.Category] = true
	}
	var suggestions []RuleSuggestion
	for _, p := range parsed {
		if len(suggestions) >= ruleSuggestionCap {
			break
		}
		reason := strings.TrimSpace(p.Reason)
		if len(reason) > 200 {
			reason = reason[:200]
		}
		switch p.Action {
		case "add":
			label := strings.ToLower(strings.TrimSpace(p.Label))
			pattern := normalizeRulePattern(p.CommentMatch)
			cat, ok := normalizeRuleCategory(p.Category)
			if !ok || !allowed[label] || (pattern == "" && cat == "") {
				continue // vocabulary-only, and the rule must actually select something
			}
			key := "add|" + label + "|" + pattern + "|" + cat
			if seen[key] {
				continue
			}
			seen[key] = true
			cand := domain.LabelRule{Label: label, CommentMatch: pattern, Category: cat}
			m, w := ruleFootprint(cand, all)
			if m == 0 {
				continue // a rule that hits nothing is noise
			}
			suggestions = append(suggestions, RuleSuggestion{
				Action: "add", Label: label, CommentMatch: pattern, Category: cat,
				Reason: reason, Matches: m, WouldLabel: w,
			})
		case "update":
			old, ok := byID[p.RuleID]
			if !ok {
				continue // never trust IDs the model made up
			}
			pattern := normalizeRulePattern(p.CommentMatch)
			cat, catOK := normalizeRuleCategory(p.Category)
			if !catOK || (pattern == "" && cat == "") {
				continue
			}
			if pattern == strings.ToLower(old.CommentMatch) && cat == old.Category {
				continue // no-op
			}
			cand := domain.LabelRule{Label: old.Label, CommentMatch: pattern, Category: cat}
			m, w := ruleFootprint(cand, all)
			suggestions = append(suggestions, RuleSuggestion{
				Action: "update", RuleID: old.ID, Label: old.Label,
				CommentMatch: pattern, Category: cat,
				OldCommentMatch: old.CommentMatch, OldCategory: old.Category,
				Reason: reason, Matches: m, WouldLabel: w,
			})
		case "delete":
			old, ok := byID[p.RuleID]
			if !ok {
				continue
			}
			m, w := ruleFootprint(old, all)
			suggestions = append(suggestions, RuleSuggestion{
				Action: "delete", RuleID: old.ID, Label: old.Label,
				CommentMatch: old.CommentMatch, Category: old.Category,
				Reason: reason, Matches: m, WouldLabel: w,
			})
		}
	}
	if suggestions == nil {
		suggestions = []RuleSuggestion{}
	}
	if len(suggestions) > 0 {
		adds, updates, deletes := 0, 0, 0
		for _, sg := range suggestions {
			switch sg.Action {
			case "add":
				adds++
			case "update":
				updates++
			case "delete":
				deletes++
			}
		}
		s.logAIActivity("rule_review", "", fmt.Sprintf("proposed %d rule changes across %d rules (%d adds, %d updates, %d deletes; pending user approval)", len(suggestions), len(rules), adds, updates, deletes))
	}
	return &RuleReviewResult{Suggestions: suggestions, RulesScanned: len(rules)}, nil
}

// ApplyRuleSuggestions writes the user-approved rule changes. Adds and
// updates also label matching history (add-only — labels are never removed
// from transactions here); deletes only remove the rule, applied labels stay.
func (s *insightService) ApplyRuleSuggestions(items []RuleApplyItem) (*RuleApplyResult, error) {
	if s.budgetRepo == nil {
		return nil, errors.New("labels unavailable")
	}
	rules, err := s.budgetRepo.ListRules()
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]domain.LabelRule, len(rules))
	for _, r := range rules {
		byID[r.ID] = r
	}

	res := &RuleApplyResult{}
	for _, item := range items {
		switch item.Action {
		case "add":
			label := strings.ToLower(strings.TrimSpace(item.Label))
			pattern := normalizeRulePattern(item.CommentMatch)
			cat, ok := normalizeRuleCategory(item.Category)
			if !ok || label == "" || strings.Contains(label, ",") || len(label) > 40 || (pattern == "" && cat == "") {
				continue
			}
			rule := domain.LabelRule{Label: label, CommentMatch: pattern, Category: cat}
			// Persist the rule BEFORE relabeling history: the old order could
			// rewrite history and then fail the save, leaving relabeled rows
			// with no rule to explain them — and report nothing happened.
			if err := s.budgetRepo.SaveRule(&rule); err != nil {
				continue
			}
			res.Added++
			if n, err := s.budgetRepo.ApplyLabel(rule); err == nil {
				res.Relabeled += n
			}
		case "update":
			rule, ok := byID[item.RuleID]
			if !ok {
				continue
			}
			pattern := normalizeRulePattern(item.CommentMatch)
			cat, catOK := normalizeRuleCategory(item.Category)
			if !catOK || (pattern == "" && cat == "") {
				continue
			}
			rule.CommentMatch = pattern
			// Label stays — relabeling is delete+add. And a model reply that
			// simply omits the category must not silently widen the rule from
			// category-scoped to match-everything.
			if cat != "" {
				rule.Category = cat
			}
			if err := s.budgetRepo.SaveRule(&rule); err != nil {
				continue
			}
			res.Updated++
			if n, err := s.budgetRepo.ApplyLabel(rule); err == nil {
				res.Relabeled += n
			}
		case "delete":
			if _, ok := byID[item.RuleID]; !ok {
				continue
			}
			if err := s.budgetRepo.DeleteRule(item.RuleID); err != nil {
				continue
			}
			res.Deleted++
		}
	}
	if res.Added+res.Updated+res.Deleted > 0 {
		s.logAIActivity("rule_review", "applied", fmt.Sprintf("user approved rule changes: %d added, %d updated, %d deleted, %d transactions relabeled", res.Added, res.Updated, res.Deleted, res.Relabeled))
	}
	return res, nil
}
