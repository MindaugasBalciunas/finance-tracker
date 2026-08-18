package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Shared short-term memory for the AI integrations. Every integration logs a
// compact digest of what it told the user; prompts include the last 7 days of
// digests instead of full transcripts. The block is hard-capped, so this adds
// a few hundred tokens of cross-integration context while the trimming in
// Chat removes far more — net cheaper AND better informed.

const (
	aiMemoryWindow  = 7 * 24 * time.Hour
	aiMemoryEntries = 12
	aiMemoryChars   = 3000
	// activityClip bounds one stored digest.
	activityClip = 400
)

// clipText flattens whitespace and truncates to max runes-ish (byte cut is
// fine for prompt digests; a mid-rune cut is cosmetic only).
func clipText(s string, max int) string {
	t := strings.Join(strings.Fields(s), " ")
	if len(t) > max {
		t = t[:max] + "…"
	}
	return t
}

// logAIActivity records one digest, best-effort — memory must never fail the
// interaction it remembers.
func (s *insightService) logAIActivity(kind, scope, content string) {
	content = clipText(content, activityClip)
	if content == "" {
		return
	}
	_ = s.repo.LogActivity(&domain.AIActivity{Kind: kind, Scope: scope, Content: content})
}

// recentAIContext renders the last 7 days of AI activity as a compact prompt
// block ("" when there is none). excludeKinds drops kinds that would be
// redundant in the caller's prompt (chat already replays its own history).
func (s *insightService) recentAIContext(limit int, excludeKinds ...string) string {
	entries, err := s.repo.RecentActivity(time.Now().Add(-aiMemoryWindow), aiMemoryEntries*2)
	if err != nil || len(entries) == 0 {
		return ""
	}
	excluded := map[string]bool{}
	for _, k := range excludeKinds {
		excluded[k] = true
	}
	if limit <= 0 || limit > aiMemoryEntries {
		limit = aiMemoryEntries
	}
	now := time.Now()
	var lines []string
	total := 0
	for _, e := range entries { // newest first
		if excluded[e.Kind] {
			continue
		}
		age := "today"
		if d := int(now.Sub(e.CreatedAt).Hours() / 24); d == 1 {
			age = "yesterday"
		} else if d > 1 {
			age = fmt.Sprintf("%dd ago", d)
		}
		scope := e.Kind
		if e.Scope != "" {
			scope += " " + e.Scope
		}
		line := fmt.Sprintf("- [%s, %s] %s", age, scope, e.Content)
		if total+len(line) > aiMemoryChars {
			break
		}
		lines = append(lines, line)
		total += len(line)
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "=== RECENT AI ACTIVITY (what you or other AI features already told the user, last 7 days — don't repeat, build on it; discipline points listed here were ALREADY made, do not restate them) ===\n" +
		strings.Join(lines, "\n")
}
