package ai

import (
	"strings"
	"testing"
	"time"

	"ft/internal/advice"
	. "ft/internal/testutil"
)

// Ask CFO's instructions carry the rules shared with the MCP server, the
// in-app ones, the date — and no formatting leftovers.
func TestSystemPromptSharesRules(t *testing.T) {
	a := &Assistant{DB: DB(t)}
	p := a.system(time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))[0].Text
	for _, want := range []string{advice.DataModel, advice.Strategy, advice.Trust, "Tuesday 13 October 2026", "#/ledger?merchant=360%20Arena", "Answer style:", "Memory:", "Changing data:", "get_reconciliation"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %.60q", want)
		}
	}
	for _, bad := range []string{"%!", "%%", "%s"} {
		if strings.Contains(p, bad) {
			t.Errorf("prompt has %q", bad)
		}
	}
}
