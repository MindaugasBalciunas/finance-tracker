package ai

import (
	"testing"
)

// The assistant's IBKR tool exists and is read-only.
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
}
