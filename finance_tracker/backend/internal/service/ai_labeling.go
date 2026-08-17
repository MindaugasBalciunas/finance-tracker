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

// ReindexSuggestion proposes labels for one unlabeled transaction.
type ReindexSuggestion struct {
	ID       uint     `json:"id"`
	Date     string   `json:"date"`
	Type     string   `json:"type"`
	Amount   float64  `json:"amount"`
	Category string   `json:"category"`
	Comment  string   `json:"comment"`
	Labels   []string `json:"labels"`
}

type ReindexResult struct {
	Suggestions []ReindexSuggestion `json:"suggestions"`
	Scanned     int                 `json:"scanned"`
	Remaining   int                 `json:"remaining_unlabeled"`
}

// LabelApplyItem is one approved suggestion.
type LabelApplyItem struct {
	ID     uint     `json:"id"`
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

// ReindexSuggest proposes labels for up to `limit` unlabeled transactions,
// newest first. Suggestions are strictly from the existing vocabulary —
// bulk-tagging must not invent new labels.
func (s *insightService) ReindexSuggest(limit int) (*ReindexResult, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if !settings.Configured() {
		return nil, errors.New("AI gateway not configured")
	}
	if limit <= 0 || limit > reindexMaxPerCall {
		limit = reindexMaxPerCall
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
	totalUnlabeled := len(unlabeled)
	if len(unlabeled) > limit {
		unlabeled = unlabeled[:limit]
	}
	if len(unlabeled) == 0 {
		return &ReindexResult{Suggestions: []ReindexSuggestion{}, Scanned: 0, Remaining: 0}, nil
	}

	// Pattern examples: the most recent labeled rows show the house style.
	sort.Slice(labeled, func(i, j int) bool { return labeled[i].Date.After(labeled[j].Date) })
	if len(labeled) > 40 {
		labeled = labeled[:40]
	}
	var exampleLines []string
	for _, tx := range labeled {
		exampleLines = append(exampleLines, fmt.Sprintf("  %q (%s) -> %s", tx.Comment, tx.Category, tx.Labels))
	}

	byID := make(map[uint]domain.Transaction, len(unlabeled))
	var suggestions []ReindexSuggestion
	for start := 0; start < len(unlabeled); start += reindexChunk {
		end := start + reindexChunk
		if end > len(unlabeled) {
			end = len(unlabeled)
		}
		var txLines []string
		for _, tx := range unlabeled[start:end] {
			byID[tx.ID] = tx
			txLines = append(txLines, fmt.Sprintf("  id=%d | %s | %s €%.2f | %s | %q",
				tx.ID, tx.Date.Format("2006-01-02"), tx.Type, tx.Amount, tx.Category, tx.Comment))
		}
		prompt := fmt.Sprintf(`You are labeling personal-finance transactions to match how this user labels similar rows.

ALLOWED LABELS (use ONLY these, never invent new ones):
%s

HOW THE USER LABELS (recent examples, description (category) -> labels):
%s

TRANSACTIONS TO LABEL:
%s

For each transaction assign 1-3 allowed labels; when nothing fits confidently, use an empty list.
Reply as ONE JSON array, nothing else: [{"id":123,"labels":["a","b"]}, ...]`,
			strings.Join(vocab, ", "), strings.Join(exampleLines, "\n"), strings.Join(txLines, "\n"))

		reply, err := callGateway(settings, []domain.ChatMessage{{Role: "user", Content: prompt}}, 4096)
		if err != nil {
			return nil, err
		}
		var parsed []struct {
			ID     uint     `json:"id"`
			Labels []string `json:"labels"`
		}
		if err := json.Unmarshal([]byte(extractJSON(reply, '[', ']')), &parsed); err != nil {
			continue // a bad chunk shouldn't sink the whole scan
		}
		for _, p := range parsed {
			tx, ok := byID[p.ID]
			if !ok {
				continue // never trust IDs the model made up
			}
			labels := normalizeSuggested(p.Labels, allowed, 3)
			if len(labels) == 0 {
				continue
			}
			suggestions = append(suggestions, ReindexSuggestion{
				ID: tx.ID, Date: tx.Date.Format("2006-01-02"), Type: string(tx.Type),
				Amount: tx.Amount, Category: string(tx.Category), Comment: tx.Comment, Labels: labels,
			})
		}
	}

	return &ReindexResult{
		Suggestions: suggestions,
		Scanned:     len(unlabeled),
		Remaining:   totalUnlabeled - len(unlabeled),
	}, nil
}

// ApplyLabelSuggestions adds the approved labels to their transactions.
// Add-only: existing labels are never removed, accounts are preserved.
func (s *insightService) ApplyLabelSuggestions(items []LabelApplyItem) (int, error) {
	applied := 0
	for _, item := range items {
		labels := normalizeSuggested(item.Labels, nil, 5)
		if len(labels) == 0 {
			continue
		}
		tx, err := s.txSvc.GetByID(item.ID)
		if err != nil {
			continue
		}
		merged := domain.NormalizeLabels(tx.Labels + "," + strings.Join(labels, ","))
		if merged == tx.Labels {
			continue
		}
		// Pass accounts through — Update overwrites them unconditionally.
		if _, err := s.txSvc.Update(item.ID, UpdateTransactionInput{
			Labels:        &merged,
			DebitAccount:  tx.DebitAccount,
			CreditAccount: tx.CreditAccount,
		}); err == nil {
			applied++
		}
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