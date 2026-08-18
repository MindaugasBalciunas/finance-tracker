package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// AI-assisted labeling: single-transaction suggestions for the edit form
// (labels + a cleaner description, learned from the user's own history) and
// a bulk "reindex" that proposes labels for unlabeled transactions. Nothing
// is ever written without explicit user approval — suggest and apply are
// separate calls, and apply only ADDS labels.

// TransactionAssistInput carries the form's current values.
type TransactionAssistInput struct {
	Date     string  `json:"date"`
	Type     string  `json:"type"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Comment  string  `json:"comment"`
	Labels   string  `json:"labels"`
}

// TransactionAssist is the AI's proposal for one transaction.
type TransactionAssist struct {
	Labels  []string `json:"labels"`
	Comment string   `json:"comment"`
	Note    string   `json:"note,omitempty"`
}

// ReindexSuggestion proposes a labeling change for one transaction: adds
// for unlabeled rows, add+remove remaps for the review of labeled rows.
type ReindexSuggestion struct {
	ID       uint     `json:"id"`
	Date     string   `json:"date"`
	Type     string   `json:"type"`
	Amount   float64  `json:"amount"`
	Category string   `json:"category"`
	Comment  string   `json:"comment"`
	Current  []string `json:"current,omitempty"`
	Add      []string `json:"add"`
	Remove   []string `json:"remove,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

type ReindexResult struct {
	Suggestions []ReindexSuggestion `json:"suggestions"`
	Scanned     int                 `json:"scanned"`
	Remaining   int                 `json:"remaining_unlabeled"`
	// Warning is set when a scan stopped early (gateway error or unparseable
	// model output after the first chunk). Scanned then counts only the rows
	// actually processed, so advancing the offset by it re-offers the rest.
	Warning string `json:"warning,omitempty"`
}

// LabelApplyItem is one approved suggestion. Labels is the pre-v1.19 wire
// name for Add, kept as a fallback.
type LabelApplyItem struct {
	ID     uint     `json:"id"`
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
	Labels []string `json:"labels"`
}

// labelVocabulary renders the existing labels (busiest first, capped) so the
// model tags with the user's own vocabulary instead of inventing synonyms.
func (s *insightService) labelVocabulary(cap int) ([]string, map[string]bool, error) {
	if s.budgetRepo == nil {
		return nil, nil, errors.New("labels unavailable")
	}
	stats, err := s.budgetRepo.LabelStats()
	if err != nil {
		return nil, nil, err
	}
	if len(stats) > cap {
		stats = stats[:cap]
	}
	lines := make([]string, len(stats))
	allowed := make(map[string]bool, len(stats))
	for i, st := range stats {
		lines[i] = fmt.Sprintf("%s (%d uses)", st.Label, st.Transactions)
		allowed[st.Label] = true
	}
	return lines, allowed, nil
}

// AssistTransaction suggests labels and a cleaner description for one
// transaction, grounded in similar historical rows.
func (s *insightService) AssistTransaction(input TransactionAssistInput) (*TransactionAssist, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if !settings.Configured() {
		return nil, errors.New("AI gateway not configured")
	}
	if strings.TrimSpace(input.Comment) == "" {
		return nil, errors.New("add a comment first — suggestions are based on the description")
	}

	vocab, _, err := s.labelVocabulary(80)
	if err != nil {
		return nil, err
	}

	all, err := s.txSvc.ListAll()
	if err != nil {
		return nil, err
	}
	examples := similarExamples(all, input.Comment, input.Category, 12)

	prompt := fmt.Sprintf(`You help label personal-finance transactions consistently with the user's own history.

EXISTING LABELS (prefer these; invent a new lowercase label only when clearly nothing fits):
%s

SIMILAR HISTORICAL TRANSACTIONS (how this user labels and describes things):
%s

TRANSACTION TO TAG:
date %s | %s €%.2f | category %s | current labels: %q
description: %q

Reply as ONE JSON object, nothing else:
{"labels": ["1-3 labels, lowercase"], "comment": "a cleaner, human-readable description in the user's style — keep merchant names and essential facts, drop card numbers/bank noise; return the original if it is already clear", "note": "one short sentence explaining the suggestion"}`,
		strings.Join(vocab, ", "),
		strings.Join(examples, "\n"),
		input.Date, input.Type, input.Amount, input.Category, input.Labels, input.Comment)

	reply, err := callGateway(settings, []domain.ChatMessage{{Role: "user", Content: prompt}}, 2048)
	if err != nil {
		return nil, err
	}
	var out TransactionAssist
	if err := json.Unmarshal([]byte(extractJSON(reply, '{', '}')), &out); err != nil {
		return nil, fmt.Errorf("the model returned an unparseable suggestion — try again")
	}
	out.Labels = normalizeSuggested(out.Labels, nil, 3)
	out.Comment = strings.TrimSpace(out.Comment)
	return &out, nil
}

// reindexChunk bounds one gateway call; reindexLimit bounds one API call.
const reindexChunk = 25
const reindexMaxPerCall = 75

// fixedProtected blocks AI-driven removal of the fixed-obligation labels —
// they feed budgets and insights, and stripping one silently reclassifies
// committed money as discretionary.
func fixedProtected(label string) bool {
	return domain.IsFixedObligationLabel(label)
}

// splitLabels parses the comma multiset into a slice.
func splitLabels(labels string) []string {
	var out []string
	for _, l := range strings.Split(labels, ",") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func containsLabel(labels []string, l string) bool {
	for _, x := range labels {
		if x == l {
			return true
		}
	}
	return false
}

// ReindexSuggest proposes labeling changes, newest first. Two modes:
//   - "unlabeled" (default): assign labels to rows that have none, strictly
//     from the existing vocabulary.
//   - "review": audit rows that ARE labeled and propose remaps (add/remove
//     with a reason) where the labels don't fit the description, category or
//     the user's own patterns. offset pages through the labeled history.
func (s *insightService) ReindexSuggest(mode string, limit, offset int) (*ReindexResult, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if !settings.Configured() {
		return nil, errors.New("AI gateway not configured")
	}
	if mode == "" {
		mode = "unlabeled"
	}
	if mode != "unlabeled" && mode != "review" {
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
	if limit <= 0 || limit > reindexMaxPerCall {
		limit = reindexMaxPerCall
	}
	if offset < 0 {
		offset = 0
	}

	vocab, allowed, err := s.labelVocabulary(80)
	if err != nil {
		return nil, err
	}

	all, err := s.txSvc.ListAll()
	if err != nil {
		return nil, err
	}
	var unlabeled []domain.Transaction
	var labeled []domain.Transaction
	for _, tx := range all {
		if strings.TrimSpace(tx.Labels) == "" && strings.TrimSpace(tx.Comment) != "" {
			unlabeled = append(unlabeled, tx)
		} else if tx.Labels != "" {
			labeled = append(labeled, tx)
		}
	}
	sort.Slice(unlabeled, func(i, j int) bool { return unlabeled[i].Date.After(unlabeled[j].Date) })
	sort.Slice(labeled, func(i, j int) bool { return labeled[i].Date.After(labeled[j].Date) })

	// Pattern examples: the most recent labeled rows show the house style.
	examplePool := labeled
	if len(examplePool) > 40 {
		examplePool = examplePool[:40]
	}
	var exampleLines []string
	for _, tx := range examplePool {
		exampleLines = append(exampleLines, fmt.Sprintf("  %q (%s) -> %s", tx.Comment, tx.Category, tx.Labels))
	}

	// Pick this pass's batch.
	pool := unlabeled
	if mode == "review" {
		pool = labeled
	}
	total := len(pool)
	if offset > total {
		offset = total
	}
	batch := pool[offset:]
	if len(batch) > limit {
		batch = batch[:limit]
	}
	if len(batch) == 0 {
		return &ReindexResult{Suggestions: []ReindexSuggestion{}, Scanned: 0, Remaining: 0}, nil
	}

	byID := make(map[uint]domain.Transaction, len(batch))
	var suggestions []ReindexSuggestion
	for start := 0; start < len(batch); start += reindexChunk {
		end := start + reindexChunk
		if end > len(batch) {
			end = len(batch)
		}
		var txLines []string
		for _, tx := range batch[start:end] {
			byID[tx.ID] = tx
			if mode == "unlabeled" {
				txLines = append(txLines, fmt.Sprintf("  id=%d | %s | %s €%.2f | %s | %q",
					tx.ID, tx.Date.Format("2006-01-02"), tx.Type, tx.Amount, tx.Category, tx.Comment))
			} else {
				txLines = append(txLines, fmt.Sprintf("  id=%d | %s | %s €%.2f | %s | %q | labels: %s",
					tx.ID, tx.Date.Format("2006-01-02"), tx.Type, tx.Amount, tx.Category, tx.Comment, tx.Labels))
			}
		}

		var prompt string
		if mode == "unlabeled" {
			prompt = fmt.Sprintf(`You are labeling personal-finance transactions to match how this user labels similar rows.

ALLOWED LABELS (use ONLY these, never invent new ones):
%s

HOW THE USER LABELS (recent examples, description (category) -> labels):
%s

TRANSACTIONS TO LABEL:
%s

For each transaction assign 1-3 allowed labels; when nothing fits confidently, use an empty list.
Reply as ONE JSON array, nothing else: [{"id":123,"labels":["a","b"]}, ...]`,
				strings.Join(vocab, ", "), strings.Join(exampleLines, "\n"), strings.Join(txLines, "\n"))
		} else {
			prompt = fmt.Sprintf(`You are auditing label quality on personal-finance transactions. Most rows are labeled correctly — flag ONLY rows whose labels clearly do not fit the description, category or the user's own labeling patterns, and propose the fix.

ALLOWED LABELS for additions (use ONLY these):
%s

HOW THE USER LABELS (recent examples, description (category) -> labels):
%s

TRANSACTIONS TO AUDIT (with their current labels):
%s

Rules:
- Output only rows that need a change; skip correct rows entirely.
- "remove" may only contain labels the row currently has; NEVER remove: loan, alimony, leasing, evelina.
- "add" only from the allowed list; a change should leave the row with 1-3 sensible labels.
- Give a short reason. When unsure, skip the row.
Reply as ONE JSON array, nothing else: [{"id":123,"remove":["x"],"add":["y"],"reason":"..."}, ...]`,
				strings.Join(vocab, ", "), strings.Join(exampleLines, "\n"), strings.Join(txLines, "\n"))
		}

		reply, err := callGateway(settings, []domain.ChatMessage{{Role: "user", Content: prompt}}, 4096)
		if err != nil {
			// Chunks already processed are paid-for work — return them with
			// Scanned = rows actually covered, so the client's offset advance
			// re-offers the rest next pass instead of discarding everything.
			if start > 0 {
				return &ReindexResult{
					Suggestions: suggestions,
					Scanned:     start,
					Remaining:   total - offset - start,
					Warning:     fmt.Sprintf("scan stopped early (%v) — %d rows deferred to the next pass", err, len(batch)-start),
				}, nil
			}
			return nil, err
		}
		var parsed []struct {
			ID     uint     `json:"id"`
			Labels []string `json:"labels"`
			Add    []string `json:"add"`
			Remove []string `json:"remove"`
			Reason string   `json:"reason"`
		}
		if err := json.Unmarshal([]byte(extractJSON(reply, '[', ']')), &parsed); err != nil {
			// Stop at the failed chunk: counting its rows as "scanned" let
			// the client advance the offset past rows that were never
			// processed — up to 25 rows silently skipped forever per bad
			// chunk. They stay in the pool for the next pass instead.
			return &ReindexResult{
				Suggestions: suggestions,
				Scanned:     start,
				Remaining:   total - offset - start,
				Warning:     "the model returned unparseable output — the remaining rows will be retried on the next pass",
			}, nil
		}
		for _, p := range parsed {
			tx, ok := byID[p.ID]
			if !ok {
				continue // never trust IDs the model made up
			}
			current := splitLabels(tx.Labels)
			adds := normalizeSuggested(append(p.Add, p.Labels...), allowed, 3)
			// Removals: only labels actually on the row, never fixed ones.
			var removes []string
			for _, l := range normalizeSuggested(p.Remove, nil, 5) {
				if containsLabel(current, l) && !fixedProtected(l) {
					removes = append(removes, l)
				}
			}
			// Drop no-ops: adds already present count for nothing.
			var newAdds []string
			for _, l := range adds {
				if !containsLabel(current, l) {
					newAdds = append(newAdds, l)
				}
			}
			if len(newAdds) == 0 && len(removes) == 0 {
				continue
			}
			// Removal-only suggestions must serialize add as [] — a null
			// crashes clients that map over it.
			if newAdds == nil {
				newAdds = []string{}
			}
			reason := strings.TrimSpace(p.Reason)
			if len(reason) > 200 {
				reason = reason[:200]
			}
			suggestions = append(suggestions, ReindexSuggestion{
				ID: tx.ID, Date: tx.Date.Format("2006-01-02"), Type: string(tx.Type),
				Amount: tx.Amount, Category: string(tx.Category), Comment: tx.Comment,
				Current: current, Add: newAdds, Remove: removes, Reason: reason,
			})
		}
	}

	if len(suggestions) > 0 {
		s.logAIActivity("tagging", mode, fmt.Sprintf("proposed %d label changes from %d scanned rows (pending user approval)", len(suggestions), len(batch)))
	}
	return &ReindexResult{
		Suggestions: suggestions,
		Scanned:     len(batch),
		Remaining:   total - offset - len(batch),
	}, nil
}

// ApplyLabelSuggestions applies the approved changes: removals first (never
// a fixed-obligation label), then additions. Accounts are preserved.
func (s *insightService) ApplyLabelSuggestions(items []LabelApplyItem) (int, error) {
	// Additions are validated against the SAME vocabulary the scan offers —
	// without this the apply endpoint is a general bulk label writer that
	// accepts arbitrary (id, labels) pairs with no proof they came from a
	// scan.
	_, allowed, err := s.labelVocabulary(80)
	if err != nil {
		return 0, err
	}

	applied, failed := 0, 0
	var firstErr error
	fail := func(err error) {
		failed++
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, item := range items {
		adds := normalizeSuggested(append(item.Add, item.Labels...), allowed, 5)
		removes := normalizeSuggested(item.Remove, nil, 5)
		if len(adds) == 0 && len(removes) == 0 {
			continue
		}
		tx, err := s.txSvc.GetByID(item.ID)
		if err != nil {
			fail(fmt.Errorf("transaction %d: %w", item.ID, err))
			continue
		}
		var kept []string
		for _, l := range splitLabels(tx.Labels) {
			if containsLabel(removes, l) && !fixedProtected(l) {
				continue
			}
			kept = append(kept, l)
		}
		for _, l := range adds {
			if !containsLabel(kept, l) {
				kept = append(kept, l)
			}
		}
		merged := domain.NormalizeLabels(strings.Join(kept, ","))
		if merged == tx.Labels {
			continue
		}
		// Labels-only update — accounts stay untouched (nil = keep).
		if _, err := s.txSvc.Update(item.ID, UpdateTransactionInput{
			Labels: &merged,
		}); err != nil {
			fail(fmt.Errorf("transaction %d: %w", item.ID, err))
			continue
		}
		applied++
	}
	if applied > 0 {
		s.logAIActivity("tagging", "applied", fmt.Sprintf("user approved AI label changes on %d transactions", applied))
	}
	if failed > 0 {
		// Partial failures were silently folded into a smaller "applied"
		// count; surface them — a retry is safe, applied items no-op.
		return applied, fmt.Errorf("applied %d, but %d failed (first error: %v) — retrying is safe", applied, failed, firstErr)
	}
	return applied, nil
}

// similarExamples finds labeled rows resembling the comment (shared word of
// length ≥4) or sharing the category — the model learns from precedent.
func similarExamples(all []domain.Transaction, comment, category string, cap int) []string {
	words := strings.Fields(strings.ToLower(comment))
	var keys []string
	for _, w := range words {
		w = strings.Trim(w, "'\".,()*0123456789")
		if len(w) >= 4 {
			keys = append(keys, w)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Date.After(all[j].Date) })
	var byWord, byCategory []string
	for _, tx := range all {
		if tx.Labels == "" || strings.TrimSpace(tx.Comment) == "" {
			continue
		}
		low := strings.ToLower(tx.Comment)
		matched := false
		for _, k := range keys {
			if strings.Contains(low, k) {
				matched = true
				break
			}
		}
		if matched && len(byWord) < cap {
			byWord = append(byWord, fmt.Sprintf("  %q (%s) -> %s", tx.Comment, tx.Category, tx.Labels))
		} else if string(tx.Category) == category && len(byCategory) < 5 {
			byCategory = append(byCategory, fmt.Sprintf("  %q (%s) -> %s", tx.Comment, tx.Category, tx.Labels))
		}
		if len(byWord) >= cap {
			break
		}
	}
	out := append(byWord, byCategory...)
	if len(out) == 0 {
		out = []string{"  (no similar labeled transactions found)"}
	}
	return out
}

// normalizeSuggested lowercases/trims model-proposed labels, optionally
// restricts them to an allowlist, dedupes and caps the count.
func normalizeSuggested(labels []string, allowed map[string]bool, cap int) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range labels {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || strings.Contains(l, ",") || len(l) > 40 || seen[l] {
			continue
		}
		if allowed != nil && !allowed[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// extractJSON pulls the outermost JSON value from a model reply that may be
// wrapped in prose or code fences.
func extractJSON(s string, open, close byte) string {
	start := strings.IndexByte(s, open)
	end := strings.LastIndexByte(s, close)
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}
