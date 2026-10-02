package handler

import (
	"fmt"
	"strings"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Learned enrichment for bank rows.
//
// classifySwedbank knows a fixed list of merchants and nothing else: every
// debit it cannot place becomes "Entertainment", every credit becomes
// "Reimbursement", and no user-defined label rule is consulted at all. A row
// typed by hand into the transaction form gets far better treatment — the
// form offers the category that similar past rows actually got, and the
// service applies the user's label rules on save.
//
// This file gives the PSD2 path those same two signals, at stage time, so
// the proposal the review queue shows is the proposal the form would have
// made. Both are *learned* from the user's own ledger; nothing here is
// hardcoded.
//
// What it deliberately does NOT touch: date, type, amount and comment. Those
// four are the content-dedup key, and history carries whatever the CSV
// importer derived for them — changing one here turns a known duplicate into
// a new row. Category and labels are free to improve.

// labelingSource is the learned half of the import: the user's own label
// rules, and the category similar past transactions were filed under. Both
// come from the budget repository, which is also what the transaction form
// asks. Optional — a handler without one stages exactly as before.
type labelingSource interface {
	ListRules() ([]domain.LabelRule, error)
	SuggestCategory(txType, comment string, amount float64) (category string, matches int, basis string, err error)
	SuggestLabels(txType, comment string) (labels []string, matches int, err error)
}

// WithLabeling wires the learned signals into the bank import.
func (h *BankHandler) WithLabeling(src labelingSource) *BankHandler {
	h.labeling = src
	return h
}

// enrich upgrades one freshly adapted row in place.
//
// Order matters: the category is resolved first, because label rules match on
// category — applying them against the classifier's placeholder would attach
// the wrong set and then leave it there.
func (h *BankHandler) enrich(row *domain.BankStagedTx) {
	if h.labeling == nil {
		return
	}
	var notes []string
	if n := h.suggestCategory(row); n != "" {
		notes = append(notes, n)
	}
	if n := h.applyRules(row); n != "" {
		notes = append(notes, n)
	}
	// Rules last-but-one, history last: a rule is the user's explicit
	// instruction and gets to set the shape; history fills in the labels they
	// have been applying by hand without ever writing a rule for them.
	if n := h.suggestLabels(row); n != "" {
		notes = append(notes, n)
	}
	row.EnrichNote = strings.Join(notes, " · ")
}

// suggestLabels copies forward the labels similar past transactions agree on.
//
// Rules cover only what the user wrote a rule for. Everything else lives in
// the history: a merchant tagged "groceries, barbora" seven times should not
// come back from the bank bare. Same merchant probes as the category lookup,
// so the two agree on what "similar" means.
func (h *BankHandler) suggestLabels(row *domain.BankStagedTx) string {
	probe := &domain.Transaction{Labels: row.Labels}
	var added []string
	for _, p := range categoryProbes(row.Comment) {
		labels, matches, err := h.labeling.SuggestLabels(string(row.Type), p)
		if err != nil || len(labels) == 0 {
			continue
		}
		for _, l := range labels {
			if !probe.HasLabel(l) {
				probe.AddLabel(l)
				added = append(added, l)
			}
		}
		if len(added) == 0 {
			return ""
		}
		row.Labels = probe.Labels
		return fmt.Sprintf("labels %s — on %d similar past transactions", strings.Join(added, ", "), matches)
	}
	return ""
}

// suggestCategory replaces a placeholder category with the one similar past
// transactions actually got. Only a placeholder is replaced: a row the
// classifier genuinely recognised (Lidl → Food) already carries a better
// answer than a comment-substring count can give.
func (h *BankHandler) suggestCategory(row *domain.BankStagedTx) string {
	if !row.CategoryGuessed {
		return ""
	}
	// Every comment probe runs with amount 0 first. SuggestCategory falls
	// back to "other rows of exactly this amount" when the comment misses,
	// and that fallback would answer on the very first (longest, least
	// likely) probe — burying the merchant lookup that was the point. A
	// €9.90 haircut came back Utilities that way, off five unrelated €9.90
	// rows, while "Barbara beauty" sat four times under Health.
	for _, probe := range categoryProbes(row.Comment) {
		if cat, matches, _, err := h.labeling.SuggestCategory(string(row.Type), probe, 0); err == nil {
			if note := h.takeCategory(row, cat, matches); note != "" {
				return fmt.Sprintf("category %s — %d past transactions matching %q", cat, matches, probe)
			}
		}
	}
	// Only now: recurring identical amounts, for subscriptions and standing
	// orders whose descriptions never repeat.
	if cat, matches, _, err := h.labeling.SuggestCategory(string(row.Type), "", row.Amount); err == nil {
		if note := h.takeCategory(row, cat, matches); note != "" {
			return fmt.Sprintf("category %s — %d past transactions of exactly this amount", cat, matches)
		}
	}
	return ""
}

// takeCategory applies a suggestion if it is usable, and reports whether it
// did.
func (h *BankHandler) takeCategory(row *domain.BankStagedTx, cat string, matches int) string {
	if matches == 0 || cat == "" {
		return ""
	}
	suggested := domain.Category(cat)
	if !domain.IsValidCategory(suggested) || suggested == row.Category {
		return ""
	}
	row.Category = suggested
	return cat
}

// categoryProbes narrows a bank comment towards the merchant, most specific
// first.
//
// SuggestCategory asks "which past comments CONTAIN this text", which is
// built for the form, where the user types a few characters. A bank comment
// is the opposite shape: "Barbora (Pirkiniai internetu)" contains the
// merchant rather than being contained by history, so the full string matches
// nothing while "barbora" matches everything it should. Trying the whole
// comment first keeps a precise hit precise; the fallbacks are what make the
// lookup fire at all.
//
// The trimming mirrors utils/rulePattern.ts, which derives the same
// merchant-ish head when turning a label into a rule.
func categoryProbes(comment string) []string {
	full := strings.TrimSpace(comment)
	if full == "" {
		return nil
	}
	probes := []string{full}

	// describe() appends the bank narrative as "Payee (detail)" — the payee
	// alone is the merchant.
	if i := strings.Index(full, " ("); i > 0 {
		probes = appendProbe(probes, full[:i])
	}
	// Legal form and trailing noise off the front word(s): "UAB Pigu" → "pigu".
	probes = appendProbe(probes, merchantHead(probes[len(probes)-1]))
	return probes
}

// merchantNoisePrefixes are the legal-form words that carry no meaning for a
// merchant lookup. Same set as the frontend's NOISE_PREFIXES.
var merchantNoisePrefixes = map[string]bool{
	"uab": true, "ab": true, "mb": true, "vsi": true, "vši": true,
	"iį": true, "si": true, "sį": true, "www": true,
}

// merchantHead keeps at most the first two meaningful words, stopping at
// anything with a digit in it — merchant names rarely need more, and the
// numbers are terminal ids and invoice references.
func merchantHead(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(".,|()\\-–—", r)
	})
	if len(parts) == 0 {
		return ""
	}
	segment := strings.ToLower(parts[0])
	var kept []string
	for _, w := range strings.Fields(segment) {
		w = strings.Trim(w, "'\"")
		if len(kept) == 0 && merchantNoisePrefixes[w] {
			continue
		}
		if len(kept) >= 2 || strings.ContainsAny(w, "0123456789") || len([]rune(w)) < 2 {
			break
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, " ")
}

// appendProbe adds a candidate when it is new and long enough for
// SuggestCategory to consider it at all (it ignores anything under 3 chars).
func appendProbe(probes []string, candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if len([]rune(candidate)) < 3 {
		return probes
	}
	for _, p := range probes {
		if strings.EqualFold(p, candidate) {
			return probes
		}
	}
	return append(probes, candidate)
}

// applyRules adds the labels the user's own rules would have added had this
// row been typed into the form. Add-only, exactly like the create path: a
// rule never removes a label the classifier set.
func (h *BankHandler) applyRules(row *domain.BankStagedTx) string {
	rules, err := h.labeling.ListRules()
	if err != nil {
		return ""
	}
	probe := ruleProbe(row)
	var added []string
	for _, rule := range rules {
		if rule.Matches(probe) && !probe.HasLabel(rule.Label) {
			probe.AddLabel(rule.Label)
			added = append(added, rule.Label)
		}
	}
	if len(added) == 0 {
		return ""
	}
	row.Labels = probe.Labels
	return "labels " + strings.Join(added, ", ") + " — from your rules"
}

// ruleProbe is the row as the rule matcher should see it.
//
// Rules match on substrings, and a bank comment is "Merchant (bank
// narrative)" — far wordier than anything the user would type. Matching the
// whole string lets the bank's own wording invent labels: a grocery run
// described as "Barbora (Pirkiniai internetu)" picked up the "internet" rule
// off the narrative. So the rules see the merchant half only.
//
// Strictly narrowing: every rule that matches the merchant also matches the
// full comment, so this can only drop false positives, never real hits. A
// rule written against the narrative is the cost, and the user can still add
// that label by hand on the row in front of them.
func ruleProbe(row *domain.BankStagedTx) *domain.Transaction {
	comment := row.Comment
	if i := strings.Index(comment, " ("); i > 0 {
		comment = comment[:i]
	}
	return &domain.Transaction{Category: row.Category, Comment: comment, Labels: row.Labels}
}

// reapplyRules re-runs the rules after the user edited a staged row, and adds
// only the labels that start matching *because of* that edit.
//
// This mirrors transactionService.Update rather than Create, and for the same
// reason: a rule that matched before the edit and still matches must stay
// silent. Otherwise removing an auto-applied label and then fixing a typo in
// the comment would put the label straight back, and the removal would look
// like it never happened.
func (h *BankHandler) reapplyRules(row *domain.BankStagedTx, matchedBefore map[string]bool) {
	if h.labeling == nil {
		return
	}
	rules, err := h.labeling.ListRules()
	if err != nil {
		return
	}
	probe := ruleProbe(row)
	for _, rule := range rules {
		if rule.Matches(probe) && !matchedBefore[ruleKey(rule)] {
			probe.AddLabel(rule.Label)
		}
	}
	row.Labels = probe.Labels
}

// matchedRules snapshots which rules apply to a row as it stands now, for
// reapplyRules to diff against.
func (h *BankHandler) matchedRules(row *domain.BankStagedTx) map[string]bool {
	out := map[string]bool{}
	if h.labeling == nil {
		return out
	}
	rules, err := h.labeling.ListRules()
	if err != nil {
		return out
	}
	probe := ruleProbe(row)
	for _, rule := range rules {
		if rule.Matches(probe) {
			out[ruleKey(rule)] = true
		}
	}
	return out
}

// ruleKey identifies a rule by what it does, not by its id — the same triple
// transactionService.Update keys on.
func ruleKey(r domain.LabelRule) string {
	return r.Label + "|" + r.Category + "|" + r.CommentMatch
}
