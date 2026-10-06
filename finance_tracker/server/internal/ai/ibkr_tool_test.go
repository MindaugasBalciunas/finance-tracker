package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"ft/internal/ibkr"
	"ft/internal/testutil"
)

// The assistant's IBKR tool exists, is read-only, and says plainly when IBKR
// isn't connected instead of guessing.
func TestIBKRLiveTool(t *testing.T) {
	found := false
	for _, td := range toolDefs() {
		if td.Name == "ibkr_live" {
			found = true
			if writeTools[td.Name] {
				t.Error("ibkr_live must not be a write tool")
			}
		}
	}
	if !found {
		t.Fatal("ibkr_live missing from the tools")
	}
	d := testutil.DB(t)
	a := &Assistant{DB: d, IBKR: ibkr.New(d)}
	if _, err := a.runTool("ibkr_live", json.RawMessage(`{"what":"positions"}`)); err != ibkr.ErrNotConnected {
		t.Fatalf("not connected: %v", err)
	}
	a.IBKR = nil
	if _, err := a.runTool("ibkr_live", json.RawMessage(`{"what":"summary"}`)); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("no service: %v", err)
	}
}
